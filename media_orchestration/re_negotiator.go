package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func Initialize_renegotiation(single_connection *models.FullConnectionDetails) {
	var mu sync.Mutex

	renegotiate := func() {
		mu.Lock()
		defer mu.Unlock()

		fmt.Println("Re-Negotiation initiated")
		offer, _ := single_connection.Webrtc.CreateOffer(nil) // plain renegotiation; ICE stays same
		_ = single_connection.Webrtc.SetLocalDescription(offer)
		<-webrtc.GatheringCompletePromise(single_connection.Webrtc) // wait for all ICE candidates

		payload := map[string]interface{}{
			"Type": "offer",
			"SDP":  single_connection.Webrtc.LocalDescription().SDP,
		}

		b, _ := json.Marshal(payload)
		if single_connection.DataChannel != nil {
			single_connection.DataChannel.Send(b)
		} else {
			fmt.Println("No data channel to re-negotiate!")
		}
	}
	single_connection.Webrtc.OnNegotiationNeeded(func() {
		log.Println("ONN fired – creating offer")
		go renegotiate()
	})

	single_connection.Webrtc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		log.Println("SIGNALLING →", state)
	})

	fmt.Println("Negotiator added!")
}

func Renegotiate(single_connection *models.FullConnectionDetails) {
	var mu sync.Mutex

	renegotiate := func() {
		mu.Lock()
		defer mu.Unlock()

		fmt.Println("Re-Negotiation initiated")
		offer, _ := single_connection.Webrtc.CreateOffer(nil) // plain renegotiation; ICE stays same
		_ = single_connection.Webrtc.SetLocalDescription(offer)
		<-webrtc.GatheringCompletePromise(single_connection.Webrtc) // wait for all ICE candidates

		payload := map[string]interface{}{
			"Type": "offer",
			"SDP":  single_connection.Webrtc.LocalDescription().SDP,
		}

		b, _ := json.Marshal(payload)
		if single_connection.DataChannel != nil {
			single_connection.DataChannel.Send(b)
		} else {
			fmt.Println("No data channel to re-negotiate!")
		}
	}
	renegotiate()
}