package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"sync"
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func Contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

type route_struct map[string]bool
type payload_struct struct {
	Type             string       `json:"Type"`
	SDP              string       `json:"SDP,omitempty"`
	Audio_route      route_struct `json:"audio_route,omitempty"`
	Video_route      route_struct `json:"video_route,omitempty"`
	Audio_route_room route_struct `json:"audio_route_room,omitempty"`
	Video_route_room route_struct `json:"video_route_room,omitempty"`
	Data             string       `json:"data,omitempty"`
}

type VideoStatus struct {
	Video bool
}

type AudioStatus struct {
	Audio bool
}

type room_user_broadcast_video map[models.RoomId]VideoStatus
type room_user_broadcast_audio map[models.RoomId]AudioStatus

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
			payload := payload_struct{}
			err := json.Unmarshal(msg.Data, &payload)

			if err != nil || payload.Type == "" {
				fmt.Println("Failed to parse message or empty type:", err)
				return
			}

			if payload.Type == "answer" {
				if payload.SDP == "" {
					return
				}
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
			} else if payload.Type == "audio_route" {
				fmt.Println("Received audio route data:", payload.Audio_route)
				for user_id, pipe_audio := range payload.Audio_route {
					if single_connection.MemberTracks != nil &&
						single_connection.MemberTracks[user_id] != nil {

						single_connection.MemberTracks[user_id].AudioPipeLock.Lock()
						single_connection.MemberTracks[user_id].PipeAudio = pipe_audio
						single_connection.MemberTracks[user_id].AudioPipeLock.Unlock()

					}
				}
			} else if payload.Type == "video_route" {
				fmt.Println("Received video route data:", payload.Video_route)
				for user_id, pipe_video := range payload.Video_route {
					if single_connection.MemberTracks != nil &&
						single_connection.MemberTracks[user_id] != nil {

						single_connection.MemberTracks[user_id].VideoPipeLock.Lock()
						single_connection.MemberTracks[user_id].PipeVideo = pipe_video
						single_connection.MemberTracks[user_id].VideoPipeLock.Unlock()

					}
				}
			} else if payload.Type == "video_route_room" {
				fmt.Println("Received room video route data:", payload.Video_route_room)

				// Accumulate all rooms into one payload
				broadcastMap := room_user_broadcast_video{}

				for room_id, pipe_video := range payload.Video_route_room {

					roomId := models.RoomId(room_id)

					company_sfu.CompanySFUsMutex.RLock()
					room_data, roomExists := company_sfu.Rooms[roomId]
					company_sfu.CompanySFUsMutex.RUnlock()

					if !roomExists || room_data == nil {
						continue
					}

					if !Contains(*room_data.AccessList, string(single_connection.UserId)) {
						continue
					}

					single_connection.VideoPipeLockRoom.Lock()
					single_connection.VideoRoomsMap[room_id] = pipe_video
					single_connection.VideoPipeLockRoom.Unlock()

					broadcastMap[roomId] = VideoStatus{Video: pipe_video}
				}

				// Nothing to broadcast?  Exit early.
				if len(broadcastMap) == 0 {
					return
				}

				// Final payload with *all* rooms
				room_payload := map[string]interface{}{
					string(single_connection.UserId): &broadcastMap,
					"event":                           "video_room_event",
				}

				room_broadcast_bytes, err := json.Marshal(&room_payload)
				if err != nil {
					fmt.Println("Failed to marshal video_room_event payload:", err)
					return
				}

				// Send one message per user (even if they’re in multiple rooms)
				sent := map[models.UserId]bool{}
				for room_id := range broadcastMap {
					room := company_sfu.Rooms[room_id]
					for _, room_member_id := range *room.AccessList {
						uid := models.UserId(room_member_id)
						if sent[uid] {
							continue
						}
						user, ok := company_sfu.Users[uid]
						if !ok || user.DataChannel == nil {
							continue
						}
						select {
						case user.OutgoingDataChannel <- room_broadcast_bytes:
							// queued
						default:
							fmt.Println("OutgoingDataChannel buffer full for", uid, "- dropping video_room_event message")
						}
						sent[uid] = true
					}
				}
			} else if payload.Type == "audio_route_room" {
				fmt.Println("Received room audio route data:", payload.Audio_route_room)

				// Accumulate all rooms into one payload
				broadcastMap := room_user_broadcast_audio{}

				for room_id, pipe_audio := range payload.Audio_route_room {

					roomId := models.RoomId(room_id)

					company_sfu.CompanySFUsMutex.RLock()
					room_data, roomExists := company_sfu.Rooms[roomId]
					company_sfu.CompanySFUsMutex.RUnlock()

					if !roomExists || room_data == nil {
						continue
					}

					if !Contains(*room_data.AccessList, string(single_connection.UserId)) {
						continue
					}

					single_connection.AudioPipeLockRoom.Lock()
					single_connection.AudioRoomsMap[room_id] = pipe_audio
					single_connection.AudioPipeLockRoom.Unlock()

					broadcastMap[roomId] = AudioStatus{Audio: pipe_audio}
				}

				// If there’s nothing to send, skip
				if len(broadcastMap) == 0 {
					return
				}

				// Prepare final payload
				room_payload := map[string]interface{}{
					string(single_connection.UserId): &broadcastMap,
					"event":                           "audio_room_event",
				}

				room_broadcast_bytes, err := json.Marshal(&room_payload)
				if err != nil {
					fmt.Println("Failed to marshal audio_room_event payload:", err)
					return
				}

				// Send one message to all room members
				sent := map[models.UserId]bool{}
				for room_id := range broadcastMap {
					room := company_sfu.Rooms[room_id]
					for _, room_member_id := range *room.AccessList {
						uid := models.UserId(room_member_id)
						if sent[uid] {
							continue
						}
						user, ok := company_sfu.Users[uid]
						if !ok || user.DataChannel == nil {
							continue
						}
						select {
						case user.OutgoingDataChannel <- room_broadcast_bytes:
						default:
							fmt.Println("OutgoingDataChannel buffer full for", uid, "- dropping audio_room_event message")
						}
						sent[uid] = true
					}
				}
			}

			if payload.Type == "data" {
				fmt.Println("Received data message:", payload.Data)
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
			company_sfu.SendOnlineStatus()

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
							close(user.MemberTracks[member_id].AudioBuffer)
							err := user.Webrtc.RemoveTrack(user.MemberTracks[member_id].AudioTrack)
							if err != nil {
								fmt.Println("Error removing audio track:", err)
							}
						}
						if user.MemberTracks[member_id].VideoTrack != nil {
							user.MemberTracks[member_id].VideoTrack.Stop()
							close(user.MemberTracks[member_id].VideoBuffer)
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
