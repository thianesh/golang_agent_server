package models

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
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
	for _, user := range sfu.Users {
		if user.Died {
			continue
		}
		if user.Webrtc != nil && user.DataChannel != nil && user.DataChannel.ReadyState() == webrtc.DataChannelStateOpen {
			if err := user.DataChannel.SendText("h"); err != nil {

				if user.Offline {
					if time.Now().Unix()-user.OfflineSince > 20 {
						// If the user is already offline and the heartbeat has failed for more than 3 seconds,
						// we mark the user as dead.
						user.Died = true
					}
				}

				// If we can't send a heartbeat, we assume the connection is dead
				user.Offline = true
				user.OfflineSince = time.Now().Unix()

				continue
			}
			// sysc_user_tracks_and_renegotiate(sfu)
		} else {
			user.FullConnectionDetailsRWLock.Lock()
			user.Died = true
			user.FullConnectionDetailsRWLock.Unlock()
		}
	}

	// remove dead users
	for userId, user := range sfu.Users {
		if user.Died {
			user.Webrtc.Close()
			sfu.RemoveUser(userId)
			continue
		}
	}
}

func (sfu *CompanySFU) SendOnlineStatus() {
	userCount := len(sfu.Users)
	// UserActiveList := []UserId{}

	type media_details map[string]string

	sfu.CompanySFUsMutex.RLock()
	all_users := sfu.Users
	sfu.CompanySFUsMutex.RUnlock()

	// i := 0
	// sfu.CompanySFUsMutex.RLock()
	// defer sfu.CompanySFUsMutex.RUnlock()

	for _, user_full := range all_users {
		// UserActiveList = append(UserActiveList, UserId(user_id))
		// i++

		if user_full.Died {
			continue
		}
		for _, user := range all_users {

			members_media_ids := make(map[UserId]media_details, userCount)

			user.MemberLock.Lock()
			for member_id, member_track := range user.MemberTracks {
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
				members_media_ids[UserId(member_id)] = media_details{
					"audio":        audio_track_id,
					"video":        video_track_id,
					"audio_stream": audio_stream_id,
					"video_stream": video_stream_id,
				}
			}
			user.MemberLock.Unlock()

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
				if user.Died || (user.DataChannel != nil && user.DataChannel.ReadyState() != webrtc.DataChannelStateOpen) {
					user.DiedLock.Unlock()
					return
				}
				user.DiedLock.Unlock()

				if user.DataChannel != nil {
					// if err := user.DataChannel.Send(jsonBytes); err != nil {
					// 	user.Offline = true
					// 	user.OfflineSince = time.Now().Unix()
					// }
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
