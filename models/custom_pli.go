package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// Track which SSRCs already have a PLI sender running
var pliSenderActive = make(map[uint32]bool)
var pliSenderMutex sync.Mutex

func SendPLI(track *webrtc.TrackRemote, pc *webrtc.PeerConnection, interval time.Duration) {
	ssrc := uint32(track.SSRC())

	// Check if PLI sender is already active for this track
	pliSenderMutex.Lock()
	if pliSenderActive[ssrc] {
		pliSenderMutex.Unlock()
		fmt.Println("PLI sender already active for SSRC:", ssrc)
		return
	}
	pliSenderActive[ssrc] = true
	pliSenderMutex.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			pliSenderMutex.Lock()
			delete(pliSenderActive, ssrc)
			pliSenderMutex.Unlock()
			fmt.Println("PLI sender stopped for SSRC:", ssrc)
		}()

		for range ticker.C {
			// Check if connection is still active
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed ||
				pc.ConnectionState() == webrtc.PeerConnectionStateDisconnected ||
				pc.ConnectionState() == webrtc.PeerConnectionStateFailed {
				fmt.Println("Connection no longer active, stopping PLI sender")
				return
			}

			err := pc.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: ssrc,
				},
			})
			if err != nil {
				fmt.Println("failed to send PLI:", err)
				return // Optional: exit on failure
			} else {
				fmt.Println("PLI sent")
			}
		}
	}()
}

func SendInstantPLI(track *webrtc.TrackRemote, pc *webrtc.PeerConnection) {
	go func() {
		err := pc.WriteRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{
				MediaSSRC: uint32(track.SSRC()),
			},
		})
		if err != nil {
			fmt.Println("failed to send PLI:", err)
		} else {
			fmt.Println("PLI sent manually")
		}
	}()
}

func Send_pli_to_company_sfu(sfu *CompanySFU) {

	sfu.CompanySFUsMutex.RLock()
	all_users := sfu.Users
	sfu.CompanySFUsMutex.RUnlock()

	for _, user := range all_users {
		if user.Webrtc == nil {
			continue
		}
		if user.VideoReceiver != nil {
			SendInstantPLI(user.VideoReceiver, user.Webrtc)
		}
	}
}
