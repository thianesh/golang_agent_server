package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"log"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func Initialize_renegotiation(single_connection *models.FullConnectionDetails) {

	renegotiate := func() {
		single_connection.RenegotiateMutex.Lock()
		defer single_connection.RenegotiateMutex.Unlock()
		defer func() {
			if single_connection.CompanySFU != nil {
				models.Send_pli_to_company_sfu(single_connection.CompanySFU)
			}
		}()

		fmt.Println("Re-Negotiation initiated wihout wait group")
		offer, _ := single_connection.Webrtc.CreateOffer(nil) // plain renegotiation; ICE stays same
		_ = single_connection.Webrtc.SetLocalDescription(offer)
		<-webrtc.GatheringCompletePromise(single_connection.Webrtc) // wait for all ICE candidates

		payload := map[string]interface{}{
			"Type": "offer",
			"SDP":  single_connection.Webrtc.LocalDescription().SDP,
		}

		b, _ := json.Marshal(payload)
		if single_connection.DataChannel != nil {
			fmt.Println("Sending offer to UserId", single_connection.UserId)
			single_connection.DataChannel.Send(b)
		} else {
			fmt.Println("No data channel to re-negotiate! UserId", single_connection.UserId)
		}
	}
	single_connection.Webrtc.OnNegotiationNeeded(func() {
		log.Println("ONN fired – creating offer for user:", single_connection.UserId)
		go renegotiate()
	})

	single_connection.Webrtc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		log.Println("SIGNALLING →", state, single_connection.UserId, single_connection.Email)
	})

	fmt.Println("Negotiator added!")
}

func Renegotiate(single_connection *models.FullConnectionDetails) {

	renegotiate := func() {
		single_connection.RenegotiateMutex.Lock()
		defer single_connection.RenegotiateMutex.Unlock()
		defer func() {
			if single_connection.CompanySFU != nil {
				models.Send_pli_to_company_sfu(single_connection.CompanySFU)
			}
		}()

		fmt.Println("Re-Negotiation initiated media Orchestration > Renegotiate")
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
