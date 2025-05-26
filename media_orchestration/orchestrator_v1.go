package mediaorchestration

import (
	"fmt"
	"sync"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func SingleOrchestrator(single_connection *models.FullConnectionDetails) {
	single_connection.Webrtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("Data channel received: %s\n", dc.Label())

		dc.OnOpen(func() {
			fmt.Println("Data channel open from browser")
			dc.SendText("Hello from Pion")
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("Received message: %s\n", string(msg.Data))
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
		return
	}

	single_connection.Webrtc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Println("Connection state has changed to:", state.String())

		switch state {
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			fmt.Println("Connection closed/disconnected. Exiting goroutine.")
			closeDone()
		}
	})

	<-done // block until done is closed
}
