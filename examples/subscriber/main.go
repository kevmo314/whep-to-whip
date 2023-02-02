package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

func main() {
	pcs := make(map[string]*webrtc.PeerConnection)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/"):]
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)

		case http.MethodPost:
			// WHIP create
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
				ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
			})
			if err != nil {
				log.Printf("Failed to create peer connection: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
				log.Printf("Connection State has changed %s", connectionState.String())
			})

			pc.OnTrack(func(tr *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
				log.Printf("Got track: %s", tr.StreamID())

				for {
					p, _, err := tr.ReadRTP()
					if err != nil {
						log.Printf("Failed to read RTP: %s", err)
						return
					}
					log.Printf("Got packet: %d", p.SequenceNumber)
				}
			})

			gatherComplete := webrtc.GatheringCompletePromise(pc)

			if len(body) > 0 {
				if err := pc.SetRemoteDescription(webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  string(body),
				}); err != nil {
					log.Printf("Failed to set remote description: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				answer, err := pc.CreateAnswer(nil)
				if err != nil {
					log.Printf("Failed to create answer: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if err := pc.SetLocalDescription(answer); err != nil {
					log.Printf("Failed to set local description: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			} else {
				offer, err := pc.CreateOffer(nil)
				if err != nil {
					log.Printf("Failed to create offer: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if err := pc.SetLocalDescription(offer); err != nil {
					log.Printf("Failed to set local description: %s", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
			<-gatherComplete

			pcid := uuid.NewString()

			w.Header().Set("Content-Type", "application/sdp")
			w.Header().Set("Location", fmt.Sprintf("http://%s/%s", r.Host, pcid))

			if _, err := w.Write([]byte(pc.LocalDescription().SDP)); err != nil {
				log.Printf("Failed to write response: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			pcs[pcid] = pc

		case http.MethodDelete:
			pc, ok := pcs[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			if err := pc.Close(); err != nil {
				log.Printf("Failed to close peer connection: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			delete(pcs, id)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Listening on port %s", port)

	panic(http.ListenAndServe(":"+port, nil))
}
