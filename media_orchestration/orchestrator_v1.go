package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"sync"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func SingleOrchestrator(single_connection *models.FullConnectionDetails) {
	single_connection.Webrtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("Data channel received: %s\n", dc.Label())

		single_connection.DataChannel = dc

		dc.OnOpen(func() {
			fmt.Println("Data channel open from browser")
			dc.SendText("Hello from Pion")
			single_connection.OnDataChannelBroadcaster(single_connection)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("Received message: %s\n", string(msg.Data))

			var payload struct {
				Type string `json:"Type"`
				SDP  string `json:"SDP"`
			}

			err := json.Unmarshal(msg.Data, &payload)

			if err != nil {
				fmt.Println("Failed to parse message:", err)
				return
			}

			if payload.Type == "answer" {
				fmt.Println("Received SDP answer, setting remote description")
				answer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeAnswer,
					SDP:  payload.SDP,
				}

				if err := single_connection.Webrtc.SetRemoteDescription(answer); err != nil {
					fmt.Println("Failed to set remote description:", err)
				} else {
					fmt.Println("Remote description set successfully")
				}
			}

		})
	})

	done := make(chan struct{})

	var once sync.Once

	closeDone := func() {
		once.Do(func() {
			close(done)
		})
	}

	live_state := single_connection.Webrtc.ConnectionState()

	switch live_state {
	case webrtc.PeerConnectionStateDisconnected,
		webrtc.PeerConnectionStateFailed,
		webrtc.PeerConnectionStateClosed:
		fmt.Println("Connection closed/disconnected. Exiting goroutine.")
		closeDone()
		single_connection.Died = true
	}

	single_connection.Webrtc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Println("Connection state has changed to:", state.String())

		switch state {
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			fmt.Println("Connection closed/disconnected. Exiting goroutine.")
			closeDone()
			single_connection.Died = true
		}
	})

	<-done // block until done is closed
}
