package models

import (
	"encoding/json"
	"time"
)

type RoomId string

type CompanySFU struct {
	Users              map[UserId]*FullConnectionDetails
	Rooms              map[RoomId]*Room
	onlineStatusTicker chan struct{}
	HeartBeatTicker    chan struct{}
	MaxUserConnections int
	MaxRooms           int
	MaxUsers           int
}

func NewCompanySFU() *CompanySFU {
	return &CompanySFU{
		Users:              make(map[UserId]*FullConnectionDetails),
		Rooms:              make(map[RoomId]*Room),
		onlineStatusTicker: make(chan struct{}),
		MaxUserConnections: 100,
		MaxRooms:           100,
		MaxUsers:           100,
	}
}

func (sfu *CompanySFU) RemoveUser(userId UserId) {
	delete(sfu.Users, userId)
}

func (sfu *CompanySFU) Heartbeat() {
	for _, user := range sfu.Users {
		if user.Died {
			continue
		}
		if user.Webrtc != nil && user.DataChannel != nil {
			if err := user.DataChannel.SendText("h"); err != nil {

				if user.Offline {
					if time.Now().Unix()-user.OfflineSince > 6 {
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
		} else {
			user.Died = true
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
	UserActiveList := make([]UserId, userCount)
	i := 0
	for user_id := range sfu.Users {
		UserActiveList[i] = user_id
		i++
	}
	payload := map[string]interface{}{
		"event_source": "sfu",
		"event":        "online_status",
		"data": map[string]interface{}{
			"active_users": UserActiveList,
			"total_users":  userCount,
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	for _, user := range sfu.Users {
		if user.DataChannel != nil {
			if err := user.DataChannel.Send(jsonBytes); err != nil {
				user.Offline = true
				user.OfflineSince = time.Now().Unix()
			}
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
		ticker := time.NewTicker(6 * time.Second)
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

func (sfu *CompanySFU) CreateRrtForEachUser() {

}
