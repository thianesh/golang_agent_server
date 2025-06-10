package models

import (
	"fmt"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

func SendPLI(track *webrtc.TrackRemote, pc *webrtc.PeerConnection, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			err := pc.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(track.SSRC()),
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

func Send_pli_to_company_sfu(sfu *CompanySFU){
	for _, user := range sfu.Users {
		if user.Webrtc == nil {
			continue
		}
		if user.VideoReceiver != nil {
			SendInstantPLI(user.VideoReceiver, user.Webrtc)
		}
	}
}