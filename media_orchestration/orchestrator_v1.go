package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"sync"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func SingleOrchestrator(single_connection *models.FullConnectionDetails, company_sfu *models.CompanySFU) {
	single_connection.Webrtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("Data channel received: %s\n", dc.Label())

		single_connection.DataChannel = dc

		dc.OnOpen(func() {
			fmt.Println("Data channel open from browser")
			dc.SendText("Hello from Pion")
			single_connection.OnDataChannelBroadcaster(single_connection)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// fmt.Printf("Received message: %s\n", string(msg.Data))

			var payload struct {
				Type string `json:"Type"`
				SDP  string `json:"SDP"`
			}

			err := json.Unmarshal(msg.Data, &payload)

			if err != nil {
				// fmt.Println("Failed to parse message:", err)
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
		fmt.Println(single_connection.Email, single_connection.UserId, "Connection state has changed to:", state.String())



		switch state {
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			fmt.Println("Connection closed/disconnected. Exiting goroutine.")
			closeDone()
			single_connection.Died = true
			single_connection.MemberTracks = map[string]*models.MemberOutputTrack{}

			delete(company_sfu.Users, single_connection.UserId)

			// now I have to remove this member track from all the users.
			for _, user := range company_sfu.Users {
				if user.MemberTracks == nil {
					continue
				}
				for member_id := range user.MemberTracks {
					if member_id == string(single_connection.UserId) {

						if user.MemberTracks[member_id].AudioTrack != nil {
							user.MemberTracks[member_id].AudioTrack.Stop()
							err := user.Webrtc.RemoveTrack(user.MemberTracks[member_id].AudioTrack)
							if err != nil {
								fmt.Println("Error removing audio track:", err)
							}
						}
						if user.MemberTracks[member_id].VideoTrack != nil {
							user.MemberTracks[member_id].VideoTrack.Stop()
							err := user.Webrtc.RemoveTrack(user.MemberTracks[member_id].VideoTrack)
							if err != nil {
								fmt.Println("Error removing video track:", err)
							}
						}

						delete(user.MemberTracks, member_id)
						fmt.Println("Removed member track for user:", user.UserId, "member_id:", member_id)
						fmt.Println(">>>>>>>>> Intiating removed re-negotiation <<<<<<<<")
						if !user.Died && user.Webrtc != nil {
							fmt.Println("User is alive in the SFU, initiating renegotiation.")
							fmt.Println("Total tracks for user:", user.Webrtc.GetTransceivers())
							fmt.Println("Sender tracks for user:", user.Webrtc.GetSenders())
							fmt.Println("Receiver tracks for user:", user.Webrtc.GetReceivers())

							go Renegotiate(user)
						} else {
							fmt.Println("User is dead or WebRTC is nil, skipping renegotiation.")
						}
						break
					}
				}
			}
		}
	})

	<-done // block until done is closed
}
