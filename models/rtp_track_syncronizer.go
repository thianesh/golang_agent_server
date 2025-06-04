package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

var track_lock sync.Mutex

func Sync_track(peer_connection *FullConnectionDetails, company_sfu *CompanySFU) {
	peer_connection.Webrtc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) { //nolint: revive
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)

		mime := track.Codec().MimeType

		// Track has started, of type 111: audio/opus
		// Track has started, of type 96: video/VP8

		track_lock.Lock()

		if strings.HasPrefix(mime, "audio/") {

			// Since we received an event of rtp channel
			// no all memeber should be able to receive the strem if available
			for _, user := range company_sfu.Users {

				audioTrack, err := webrtc.NewTrackLocalStaticRTP(
					webrtc.RTPCodecCapability{
						MimeType: track.Codec().MimeType,
					},
					company_sfu.CompanyID, string(user.UserId),
				)
				if err != nil {
					continue
				}
				audioSender, err := user.Webrtc.AddTrack(audioTrack)
				if err != nil {
					continue
				}
				user.MemberTracks[string(peer_connection.UserId)] = &MemberOutputTrack{
					AudioTrack:       audioSender,
					AudioSenderTrack: audioTrack,
					DataTrack:        peer_connection.DataChannel,
					Accessible:       true,
					Status:           "online",
				}
				go Renegotiate(peer_connection)
			}

			for {
				rtp, _, readErr := track.ReadRTP()
				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}

				if writeErr := peer_connection.AudioSenderTrack.WriteRTP(rtp); writeErr != nil {
					fmt.Println("Unable to Write RTP")
					break
				}
			}

		} else if strings.HasPrefix(mime, "video/") {
			for {
				rtp, _, readErr := track.ReadRTP()
				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}

				if writeErr := peer_connection.VideoSenderTrack.WriteRTP(rtp); writeErr != nil {
					fmt.Println("Unable to Write RTP")
					break
				}
			}
		}

		track_lock.Unlock()

	})
}

func Renegotiate(single_connection *FullConnectionDetails) {
	var mu sync.Mutex

	renegotiate := func() {
		mu.Lock()
		defer mu.Unlock()

		fmt.Println("Re-Negotiation initiated")
		offer, _ := single_connection.Webrtc.CreateOffer(nil) // plain renegotiation; ICE stays same
		_ = single_connection.Webrtc.SetLocalDescription(offer)
		<-webrtc.GatheringCompletePromise(single_connection.Webrtc) // wait for all ICE candidates

		payload := map[string]interface{}{
			"Type": "offer",
			"SDP":  single_connection.Webrtc.LocalDescription().SDP,
		}

		b, _ := json.Marshal(payload)
		if single_connection.DataChannel != nil {
			single_connection.DataChannel.Send(b)
		} else {
			fmt.Println("No data channel to re-negotiate!")
		}
	}
	renegotiate()
}
