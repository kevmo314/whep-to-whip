package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		panic("Not enough arguments")
	}

	source := os.Args[1]
	destination := os.Args[2]

	// post an empty message to WHEP source to get an SDP
	offer, err := http.Post(source, "application/json", nil)
	if err != nil {
		panic(err)
	}

	location, err := url.Parse(offer.Header.Get("Location"))
	if err != nil {
		panic(err)
	}

	log.Printf("location: %s", location.String())

	// post the SDP to WHIP destination
	answer, err := http.Post(destination, "application/json", offer.Body)
	if err != nil {
		panic(err)
	}

	// patch the answer to WHEP source
	resp, err := http.DefaultClient.Do(&http.Request{
		Method: http.MethodPatch,
		URL:    location,
		Header: http.Header{
			"Content-Type": []string{"application/sdp"},
		},
		Body: answer.Body,
	})

	if err != nil {
		panic(err)
	}

	if resp.StatusCode != http.StatusOK {
		panic("Failed to patch answer")
	}

	log.Printf("Success!")
}
