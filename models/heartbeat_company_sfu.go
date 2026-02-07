package models

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	gowebrtc "github.com/pion/webrtc/v4"
)

type CompanySFU struct {
	Users                 map[UserId]*FullConnectionDetails
	Rooms                 map[RoomId]*Room
	onlineStatusTicker    chan struct{}
	HeartBeatTicker       chan struct{}
	MaxUserConnections    int
	MaxRooms              int
	MaxUsers              int
	CompanyID             string
	RtpSyncNeededMutex    sync.Mutex
	RtpSyncNeeded         bool
	Renegotiating         bool
	CompanySFUsMutex      sync.RWMutex
	CompanySFUUsersRMLock sync.RWMutex
}

func NewCompanySFU() *CompanySFU {
	return &CompanySFU{
		Users:              make(map[UserId]*FullConnectionDetails),
		Rooms:              make(map[RoomId]*Room),
		onlineStatusTicker: make(chan struct{}),
		MaxUserConnections: 100,
		MaxRooms:           100,
		MaxUsers:           100,
		RtpSyncNeeded:      false,
	}
}

func (sfu *CompanySFU) RemoveUser(userId UserId) {
	sfu.CompanySFUsMutex.Lock()
	delete(sfu.Users, userId)
	sfu.CompanySFUsMutex.Unlock()
}

func (sfu *CompanySFU) Heartbeat() {
	sfu.CompanySFUsMutex.RLock()
	usersCopy := make(map[UserId]*FullConnectionDetails)
	for k, v := range sfu.Users {
		usersCopy[k] = v
	}
	sfu.CompanySFUsMutex.RUnlock()

	deadUsers := []UserId{}

	for userId, user := range usersCopy {
		user.DiedLock.Lock()
		isDead := user.Died
		user.DiedLock.Unlock()

		if isDead {
			deadUsers = append(deadUsers, userId)
			continue
		}

		user.FullConnectionDetailsRWLock.RLock()
		webrtcConn := user.Webrtc
		dc := user.DataChannel
		user.FullConnectionDetailsRWLock.RUnlock()

		// Check if WebRTC connection is nil or closed
		if webrtcConn == nil {
			user.FullConnectionDetailsRWLock.Lock()
			user.Died = true
			user.FullConnectionDetailsRWLock.Unlock()
			deadUsers = append(deadUsers, userId)
			continue
		}

		// If DataChannel isn't ready yet, skip this user - they're still connecting
		// Don't mark them as dead, just skip the heartbeat for now
		if dc == nil || dc.ReadyState() != gowebrtc.DataChannelStateOpen {
			// User is still setting up, skip heartbeat but don't kill them
			continue
		}

		// DataChannel is open, try to send heartbeat
		if err := dc.SendText("h"); err != nil {
			user.FullConnectionDetailsRWLock.Lock()
			if user.Offline {
				if time.Now().Unix()-user.OfflineSince > 20 {
					// If the user is already offline and the heartbeat has failed for more than 20 seconds,
					// we mark the user as dead.
					user.Died = true
					deadUsers = append(deadUsers, userId)
				}
			} else {
				// If we can't send a heartbeat, we assume the connection is offline
				user.Offline = true
				user.OfflineSince = time.Now().Unix()
			}
			user.FullConnectionDetailsRWLock.Unlock()
			continue
		}

		// Heartbeat succeeded, mark user as online if they were offline
		user.FullConnectionDetailsRWLock.Lock()
		if user.Offline {
			user.Offline = false
			user.OfflineSince = 0
		}
		user.FullConnectionDetailsRWLock.Unlock()
	}

	// remove dead users (outside of iteration)
	for _, userId := range deadUsers {
		user, ok := usersCopy[userId]
		if ok && user.Webrtc != nil {
			user.Webrtc.Close()
		}
		sfu.RemoveUser(userId)
	}
}

func (sfu *CompanySFU) SendOnlineStatus() {
	userCount := len(sfu.Users)
	// UserActiveList := []UserId{}

	type media_details map[string]string

	sfu.CompanySFUsMutex.RLock()
	all_users := map[UserId]*FullConnectionDetails{}
	for key, value := range sfu.Users {
		all_users[key] = value
	}
	sfu.CompanySFUsMutex.RUnlock()
	fmt.Println("Sending online status")

	// i := 0
	// sfu.CompanySFUsMutex.RLock()
	// defer sfu.CompanySFUsMutex.RUnlock()

	for _, user_full := range all_users {
		// UserActiveList = append(UserActiveList, UserId(user_id))
		// i++
		user_full.DiedLock.Lock()
		user_full_dead := user_full.Died
		user_full.DiedLock.Unlock()

		if user_full_dead {
			continue
		}
		for _, user := range all_users {

			members_media_ids := make(map[UserId]media_details, userCount)

			user.MemberLock.Lock()
			memberTracks := user.MemberTracks
			user.MemberLock.Unlock()
			for member_id, member_track := range memberTracks {
				val, ok := all_users[UserId(member_id)]

				if !ok || val == nil {
					continue
				}

				val.DiedLock.Lock()
				memberDead := val.Died
				val.DiedLock.Unlock()
				if memberDead {
					continue
				}

				audio_track_id := ""
				video_track_id := ""
				audio_stream_id := ""
				video_stream_id := ""

				if member_track.AudioTrack != nil {
					audio_track_id = member_track.AudioSenderTrack.ID()
					audio_stream_id = member_track.AudioSenderTrack.StreamID()
				}
				if member_track.VideoTrack != nil {
					video_track_id = member_track.VideoSenderTrack.ID()
					video_stream_id = member_track.VideoSenderTrack.StreamID()
				}
				if all_users[UserId(member_id)].DataChannel != nil && all_users[UserId(member_id)].DataChannel.ReadyState() != gowebrtc.DataChannelStateOpen {
					continue
				}
				members_media_ids[UserId(member_id)] = media_details{
					"audio":        audio_track_id,
					"video":        video_track_id,
					"audio_stream": audio_stream_id,
					"video_stream": video_stream_id,
				}
			}

			members_media_ids[user.UserId] = media_details{
				"audio":        "",
				"video":        "",
				"audio_stream": "",
				"video_stream": "",
			}

			payload := map[string]interface{}{
				"event_source": "sfu",
				"event":        "online_status",
				"data": map[string]interface{}{
					"active_users": members_media_ids,
					"total_users":  userCount,
				},
			}

			jsonBytes, err := json.Marshal(payload)
			if err != nil {
				panic(err)
			}

			go func() {

				user.DiedLock.Lock()
				user_died := user.Died
				user.DiedLock.Unlock()
				if user_died || (user.DataChannel != nil && user.DataChannel.ReadyState() != gowebrtc.DataChannelStateOpen) {
					fmt.Println("Not able to send data in Datachannel either died or not ready, for ", user.Email)
					return
				}

				if user.DataChannel != nil {
					select {
					case user.OutgoingDataChannel <- jsonBytes:
						// queued
					default:
						fmt.Println("OutgoingDataChannel buffer full for", user.UserId, "- dropping online status messages")
					}
				}
			}()
		}
	}
}

func (sfu *CompanySFU) Destroy() {
	// Close all user connections
	for _, user := range sfu.Users {
		if user.Webrtc != nil {
			user.Webrtc.Close()
		}
	}

	// Signal Online status ticker
	if sfu.onlineStatusTicker != nil {
		close(sfu.onlineStatusTicker)
	}
	// Singal HeartBeat ticker
	if sfu.HeartBeatTicker != nil {
		close(sfu.HeartBeatTicker)
	}

	// Clear maps
	sfu.Users = nil
	sfu.Rooms = nil
}

func (sfu *CompanySFU) StartOnlineStatusBroadcaster() {
	sfu.onlineStatusTicker = make(chan struct{})

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sfu.onlineStatusTicker:
				return // exit goroutine cleanly
			case <-ticker.C:
				sfu.SendOnlineStatus()
			}
		}
	}()
}

func (sfu *CompanySFU) StartHeartBeat() {
	sfu.HeartBeatTicker = make(chan struct{})

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sfu.HeartBeatTicker:
				return // exit goroutine cleanly
			case <-ticker.C:
				sfu.Heartbeat()
			}
		}
	}()
}

func (sfu *CompanySFU) start_instant_renegotiator() {
	sysc_user_tracks_and_renegotiate(sfu)
	// sfu.RtpSyncNeededMutex.Lock()
	// if !sfu.Renegotiating && sfu.RtpSyncNeeded {
	// 	sysc_user_tracks_and_renegotiate(sfu)
	// 	sfu.RtpSyncNeededMutex.Unlock()
	// }
	// sfu.RtpSyncNeededMutex.Unlock()
}

func (sfu *CompanySFU) Start_instant_renegotiator_caller() {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C
			sfu.start_instant_renegotiator()
		}
	}()
}
