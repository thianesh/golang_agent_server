package mediaorchestration

import (
	"encoding/json"
	"fmt"
	"sync"
	"thianesh/web_server/models"
	"time"

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

func wait(sec int) {
	time.Sleep(time.Duration(sec) * time.Second)
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
	Route_to         string       `json:"route_to,omitempty"`
}

type VideoStatus struct {
	Video bool
}

type AudioStatus struct {
	Audio bool
}

type room_user_broadcast_video map[models.RoomId]VideoStatus
type room_user_broadcast_audio map[models.RoomId]AudioStatus

func SingleOrchestrator(single_connection *models.FullConnectionDetails, company_sfu *models.CompanySFU, user_connections *map[string]*models.FullConnectionDetails, user_connection_mutex *sync.Mutex) {
	single_connection.Webrtc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("Data channel received: %s\n", dc.Label())

		single_connection.DataChannel = dc

		dc.OnOpen(func() {
			fmt.Println("Data channel open from browser")
			dc.SendText("Hello from Pion")
			single_connection.OnDataChannelBroadcaster(single_connection)
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Println("Received message: %s\n", single_connection.Email, string(msg.Data))
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
					single_connection.MemberLock.Lock()
					trackExists := single_connection.MemberTracks != nil &&
						single_connection.MemberTracks[user_id] != nil
					if trackExists {
						single_connection.MemberTracks[user_id].AudioPipeLock.Lock()
						single_connection.MemberTracks[user_id].PipeAudio = pipe_audio
						single_connection.MemberTracks[user_id].AudioPipeLock.Unlock()
					}
					single_connection.MemberLock.Unlock()

					if trackExists {
						company_sfu.CompanySFUsMutex.RLock()
						user, ok := company_sfu.Users[models.UserId(user_id)]
						company_sfu.CompanySFUsMutex.RUnlock()
						if ok {
							if user.OutgoingDataChannel == nil {
								continue
							}
							media_route_payload := map[string]interface{}{
								"event": "media_route_audio",
								"data": map[string]bool{
									string(single_connection.UserId): pipe_audio,
								},
							}

							route_byte, err := json.Marshal(media_route_payload)
							if err != nil {
								continue
							}

							select {
							case user.OutgoingDataChannel <- route_byte:
								// queued
							default:
								fmt.Println("OutgoingDataChannel buffer full for", user_id, "- dropping video_room_event message")
							}
						}

					}
				}
			} else if payload.Type == "video_route" {
				fmt.Println("Received video route data:", payload.Video_route)
				for user_id, pipe_video := range payload.Video_route {
					single_connection.MemberLock.Lock()
					trackExists := single_connection.MemberTracks != nil &&
						single_connection.MemberTracks[user_id] != nil
					if trackExists {
						single_connection.MemberTracks[user_id].VideoPipeLock.Lock()
						single_connection.MemberTracks[user_id].PipeVideo = pipe_video
						single_connection.MemberTracks[user_id].VideoPipeLock.Unlock()
					}
					single_connection.MemberLock.Unlock()
					if trackExists {
						company_sfu.CompanySFUsMutex.RLock()
						user, ok := company_sfu.Users[models.UserId(user_id)]
						company_sfu.CompanySFUsMutex.RUnlock()
						if ok {
							if user.OutgoingDataChannel == nil {
								continue
							}
							media_route_payload := map[string]interface{}{
								"event": "media_route_video",
								"data": map[string]bool{
									string(single_connection.UserId): pipe_video,
								},
							}

							route_byte, err := json.Marshal(media_route_payload)
							if err != nil {
								continue
							}

							select {
							case user.OutgoingDataChannel <- route_byte:
								// queued
							default:
								fmt.Println("OutgoingDataChannel buffer full for", user_id, "- dropping video_room_event message")
							}
						}

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
					"event":                          "video_room_event",
				}

				room_broadcast_bytes, err := json.Marshal(&room_payload)
				if err != nil {
					fmt.Println("Failed to marshal video_room_event payload:", err)
					return
				}

				// Send one message per user (even if they’re in multiple rooms)
				sent := map[models.UserId]bool{}
				company_sfu.CompanySFUsMutex.RLock()
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
				company_sfu.CompanySFUsMutex.RUnlock()
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
					"event":                          "audio_room_event",
				}

				room_broadcast_bytes, err := json.Marshal(&room_payload)
				if err != nil {
					fmt.Println("Failed to marshal audio_room_event payload:", err)
					return
				}

				// Send one message to all room members
				sent := map[models.UserId]bool{}
				company_sfu.CompanySFUsMutex.RLock()
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
				company_sfu.CompanySFUsMutex.RUnlock()
			} else if payload.Type == "route_to" {
				to_address := payload.Route_to
				if to_address == "" {
					return
				}
				company_sfu.CompanySFUsMutex.RLock()
				user, ok := company_sfu.Users[models.UserId(to_address)]
				company_sfu.CompanySFUsMutex.RUnlock()
				if !ok || user.Died || user.Webrtc == nil {
					return
				}

				room_payload := map[string]interface{}{
					"data":       payload.Data,
					"route_to":   payload.Route_to,
					"route_from": single_connection.UserId,
					"Type":       payload.Type,
				}

				room_broadcast_bytes, err := json.Marshal(&room_payload)
				if err != nil {
					fmt.Println("Failed to marshal audio_room_event payload:", err)
					return
				}

				select {
				case user.OutgoingDataChannel <- room_broadcast_bytes:
				default:
					fmt.Println("OutgoingDataChannel buffer full for", user.UserId, "- dropping audio_room_event message")
				}

			}

			if payload.Type == "data" {
				fmt.Println("Received data message:", payload.Data)
			}

		})

		dc.OnClose(func() {
			fmt.Println("Data channel closed for user", single_connection.Email)
		})
	})

	done := make(chan struct{})

	var once sync.Once

	closeDone := func() {
		once.Do(func() {
			close(done)
			close_connection(single_connection, company_sfu)
			user_connection_mutex.Lock()
			delete(*user_connections, string(single_connection.UserId))
			user_connection_mutex.Unlock()
		})
	}

	live_state := single_connection.Webrtc.ConnectionState()

	switch live_state {
	case webrtc.PeerConnectionStateDisconnected,
		webrtc.PeerConnectionStateFailed,
		webrtc.PeerConnectionStateClosed:
		closeDone()

	}

	single_connection.Webrtc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Println(single_connection.Email, single_connection.UserId, "Connection state has changed to:", state.String())

		switch state {
		case webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed:
			closeDone()
		}
	})

	<-done
}

func close_connection(single_connection *models.FullConnectionDetails, company_sfu *models.CompanySFU) {
	fmt.Println("Connection closed/disconnected. Exiting goroutine.")
	single_connection.DiedLock.Lock()
	single_connection.Died = true
	single_connection.DiedLock.Unlock()

	single_connection.MemberLock.Lock()
	single_connection.MemberTracks = map[string]*models.MemberOutputTrack{}
	single_connection.MemberLock.Unlock()

	company_sfu.SendOnlineStatus()

	company_sfu.CompanySFUsMutex.Lock()
	delete(company_sfu.Users, single_connection.UserId)
	company_sfu.CompanySFUsMutex.Unlock()

	single_connection.FullConnectionDetailsRWLock.Lock()
	if single_connection.OutgoingDataChannel != nil {
		close(single_connection.OutgoingDataChannel)
		single_connection.OutgoingDataChannel = nil
	}
	single_connection.FullConnectionDetailsRWLock.Unlock()

	// now I have to remove this member track from all the users.
	company_sfu.CompanySFUsMutex.RLock()
	defer company_sfu.CompanySFUsMutex.RUnlock()

	for _, user := range company_sfu.Users {

		func() {
			user.MemberLock.Lock()
			defer user.MemberLock.Unlock()

			if user.MemberTracks == nil {
				return
			}
			for member_id := range user.MemberTracks {
				if member_id == string(single_connection.UserId) {
					if user.MemberTracks[member_id].AudioTrack != nil {

						user.MemberTracks[member_id].AudioTrack.Stop()

						user.MemberTracks[member_id].AudioBufferClose.Lock()
						if user.MemberTracks[member_id].AudioBuffer != nil {
							close(user.MemberTracks[member_id].AudioBuffer)
							user.MemberTracks[member_id].AudioBuffer = nil
						}
						user.MemberTracks[member_id].AudioBufferClose.Unlock()

						err := user.Webrtc.RemoveTrack(user.MemberTracks[member_id].AudioTrack)
						if err != nil {
							fmt.Println("Error removing audio track:", err)
						}
					}
					if user.MemberTracks[member_id].VideoTrack != nil {
						user.MemberTracks[member_id].VideoTrack.Stop()

						user.MemberTracks[member_id].VideoBufferClose.Lock()
						if user.MemberTracks[member_id].VideoBuffer != nil {
							close(user.MemberTracks[member_id].VideoBuffer)
							user.MemberTracks[member_id].VideoBuffer = nil
						}
						user.MemberTracks[member_id].VideoBufferClose.Unlock()

						err := user.Webrtc.RemoveTrack(user.MemberTracks[member_id].VideoTrack)
						if err != nil {
							fmt.Println("Error removing video track:", err)
						}
					}
					
					cancel_all(user.MemberTracks[member_id])

					delete(user.MemberTracks, member_id)

					fmt.Println("Removed member track for user:", user.UserId, "member_id:", member_id)
					fmt.Println(">>>>>>>>> Intiating removed re-negotiation <<<<<<<<")

					if !user.Died && user.Webrtc != nil {
						fmt.Println("User is alive in the SFU, initiating renegotiation.")
						fmt.Println("Total tracks for user:", user.Webrtc.GetTransceivers())
						fmt.Println("Sender tracks for user:", user.Webrtc.GetSenders())
						fmt.Println("Receiver tracks for user:", user.Webrtc.GetReceivers())
						fmt.Println("Re-negotiation will be started after 5 seconds!")

						wait(5)

						go Renegotiate(user)
					} else {
						fmt.Println("User is dead or WebRTC is nil, skipping renegotiation.")
					}
					break
				}
			}
		}()
	}

}

func cancel_all(member_track *models.MemberOutputTrack) {

	// member_track.AudioForwardCancel()
	// member_track.VideoForwardCancel()
	// member_track.VideoTrackCancel()
	// member_track.AudioTrackCancel()

	if member_track.AudioForwardCancel != nil {
		member_track.AudioForwardCancel()
	}
	if member_track.VideoForwardCancel != nil {
		member_track.VideoForwardCancel()
	}
	if member_track.VideoTrackCancel != nil {
		member_track.VideoTrackCancel()
	}
	if member_track.AudioTrackCancel != nil {
		member_track.AudioTrackCancel()
	}

}
