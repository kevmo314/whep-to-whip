package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/x264"
	_ "github.com/pion/mediadevices/pkg/driver/videotest"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"
)

func main() {
	// a simple WHEP publisher that publishes a demo video stream.
	pcs := make(map[string]*webrtc.PeerConnection)

	x264Params, _ := x264.NewParams()
	x264Params.Preset = x264.PresetMedium
	x264Params.BitRate = 1_000_000 // 1mbps

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
	)

	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(constraint *mediadevices.MediaTrackConstraints) {
			constraint.Width = prop.Int(640)
			constraint.Height = prop.Int(480)
		},
		Codec: codecSelector,
	})
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/"):]
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)

		case http.MethodPost:
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

			for _, t := range stream.GetTracks() {
				log.Printf("Adding track: %s", t.StreamID())

				if _, err := pc.AddTransceiverFromTrack(t, webrtc.RtpTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly}); err != nil {
					log.Printf("Failed to add track: %s", err)
					return
				}
			}

			gatherComplete := webrtc.GatheringCompletePromise(pc)

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
		case http.MethodPatch:
			if r.Header.Get("Content-Type") != "application/sdp" {
				// Trickle ICE/ICE restart not implemented
				panic("Not implemented")
			}

			pc, ok := pcs[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  string(body),
			}); err != nil {
				log.Printf("Failed to set remote description: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
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
		port = "8080"
	}

	log.Printf("Listening on port %s", port)

	panic(http.ListenAndServe(":"+port, nil))
}
