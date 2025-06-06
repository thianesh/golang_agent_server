package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

var audio_track_lock sync.Mutex
var video_track_lock sync.Mutex

func Sync_track(peer_connection *FullConnectionDetails, company_sfu *CompanySFU) {
	peer_connection.Webrtc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) { //nolint: revive
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)

		mime := track.Codec().MimeType

		// Track has started, of type 111: audio/opus
		// Track has started, of type 96: video/VP8

		if strings.HasPrefix(mime, "audio/") {
			peer_connection.AudioReceiver = track

			// Since we received an event of rtp channel
			// no all memeber should be able to receive the strem if available
			audio_track_lock.Lock()
			var wg sync.WaitGroup

			for _, user := range company_sfu.Users {

				if user.UserId == peer_connection.UserId {
					continue
				}

				fmt.Println(company_sfu)

				audioTrack, err := webrtc.NewTrackLocalStaticRTP(
					webrtc.RTPCodecCapability{
						MimeType: track.Codec().MimeType,
					},
					string(peer_connection.UserId), string(user.UserId)+"_"+string(peer_connection.UserId),
				)
				if err != nil {
					continue
				}
				audioSender, err := user.Webrtc.AddTrack(audioTrack)

				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if _, _, rtcpErr := audioSender.Read(rtcpBuf); rtcpErr != nil {
							return
						}
					}
				}()

				if err != nil {
					continue
				}

				if _, ok := user.MemberTracks[string(peer_connection.UserId)]; !ok {
					member_output_track := &MemberOutputTrack{
						AudioTrack:       audioSender,
						AudioSenderTrack: audioTrack,
						DataTrack:        peer_connection.DataChannel,
						Accessible:       true,
						Status:           "online",
					}
					user.MemberTracks[string(peer_connection.UserId)] = member_output_track
				} else {
					user.MemberTracks[string(peer_connection.UserId)].AudioTrack = audioSender
					user.MemberTracks[string(peer_connection.UserId)].AudioSenderTrack = audioTrack
				}

				wg.Add(1)
				go Renegotiate(peer_connection, &wg)
			}

			wg.Wait()
			audio_track_lock.Unlock()

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

				for _, user := range company_sfu.Users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					if user.MemberTracks[string(peer_connection.UserId)] == nil {
						fmt.Println("AudioTrack is nill for ", peer_connection.UserId)
						go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}
					if writeErr := user.MemberTracks[string(peer_connection.UserId)].AudioSenderTrack.WriteRTP(rtp); writeErr != nil {
						fmt.Println("Unable to Write RTP")
						break
					}
				}
			}

		} else if strings.HasPrefix(mime, "video/") {
			peer_connection.VideoReceiver = track

			// Since we received an event of rtp channel
			// no all memeber should be able to receive the strem if available
			video_track_lock.Lock()
			var wg sync.WaitGroup

			for _, user := range company_sfu.Users {

				if user.UserId == peer_connection.UserId {
					continue
				}

				fmt.Println(company_sfu)

				VideoTrack, err := webrtc.NewTrackLocalStaticRTP(
					webrtc.RTPCodecCapability{
						MimeType: track.Codec().MimeType,
					},
					string(peer_connection.UserId), string(user.UserId)+"_"+string(peer_connection.UserId),
				)
				if err != nil {
					continue
				}
				VideoSender, err := user.Webrtc.AddTrack(VideoTrack)

				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if _, _, rtcpErr := VideoSender.Read(rtcpBuf); rtcpErr != nil {
							return
						}
					}
				}()

				if err != nil {
					continue
				}

				if _, ok := user.MemberTracks[string(peer_connection.UserId)]; !ok {
					member_output_track := &MemberOutputTrack{
						VideoTrack:       VideoSender,
						VideoSenderTrack: VideoTrack,
						DataTrack:        peer_connection.DataChannel,
						Accessible:       true,
						Status:           "online",
					}
					user.MemberTracks[string(peer_connection.UserId)] = member_output_track
				} else {
					user.MemberTracks[string(peer_connection.UserId)].VideoTrack = VideoSender
					user.MemberTracks[string(peer_connection.UserId)].VideoSenderTrack = VideoTrack
				}

				wg.Add(1)
				go Renegotiate(peer_connection, &wg)
			}

			wg.Wait()
			video_track_lock.Unlock()

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

				for _, user := range company_sfu.Users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					if user.MemberTracks[string(peer_connection.UserId)] == nil {
						fmt.Println("Video is nill for ", peer_connection.UserId)
						go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}
					if writeErr := user.MemberTracks[string(peer_connection.UserId)].VideoSenderTrack.WriteRTP(rtp); writeErr != nil {
						fmt.Println("Unable to Write RTP")
						break
					}
				}

			}
		}

	})
}

func Renegotiate(single_connection *FullConnectionDetails, wg *sync.WaitGroup) {
	var mu sync.Mutex

	renegotiate := func() {
		mu.Lock()
		defer mu.Unlock()
		defer wg.Done()

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

var full_sync_lock sync.Mutex

func sysc_user_tracks_and_renegotiate(company_sfu *CompanySFU) {

	full_sync_lock.Lock()
	defer full_sync_lock.Unlock()

	for _, user := range company_sfu.Users {
		if user.MemberTracks == nil {
			user.MemberTracks = map[string]*MemberOutputTrack{}
		}
		// Each user should have all the memebers connection except his own
		for _, users_connction_check := range company_sfu.Users {

			if user.UserId == users_connction_check.UserId {
				continue
			}

			// now lets check
			if _, ok := user.MemberTracks[string(users_connction_check.UserId)]; !ok {

				if _, ok := company_sfu.Users[users_connction_check.UserId]; !ok {
					continue
				}

				//creating audio track
				if company_sfu.Users[users_connction_check.UserId].AudioReceiver == nil {
					continue
				}

				track := company_sfu.Users[users_connction_check.UserId].AudioReceiver

				audioTrack, err := webrtc.NewTrackLocalStaticRTP(
					webrtc.RTPCodecCapability{
						MimeType: track.Codec().MimeType,
					},
					string(users_connction_check.UserId), string(user.UserId)+"_"+string(users_connction_check.UserId)+"_audio",
				)
				if err != nil {
					continue
				}
				audioSender, err := user.Webrtc.AddTrack(audioTrack)

				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if _, _, rtcpErr := audioSender.Read(rtcpBuf); rtcpErr != nil {
							return
						}
					}
				}()

				if err != nil {
					continue
				}
				member_output_track := &MemberOutputTrack{
					AudioTrack:       audioSender,
					AudioSenderTrack: audioTrack,
					DataTrack:        users_connction_check.DataChannel,
					Accessible:       true,
					Status:           "online",
				}

				user.MemberTracks[string(users_connction_check.UserId)] = member_output_track

				// now setting video track
				viceo_track := company_sfu.Users[users_connction_check.UserId].VideoReceiver

				VideoTrack, err := webrtc.NewTrackLocalStaticRTP(
					webrtc.RTPCodecCapability{
						MimeType: viceo_track.Codec().MimeType,
					},
					string(users_connction_check.UserId), string(user.UserId)+"_"+string(users_connction_check.UserId)+"_video",
				)
				if err != nil {
					continue
				}
				VideoSender, err := user.Webrtc.AddTrack(VideoTrack)

				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if _, _, rtcpErr := VideoSender.Read(rtcpBuf); rtcpErr != nil {
							return
						}
					}
				}()

				if err != nil {
					continue
				}

				user.MemberTracks[string(users_connction_check.UserId)].VideoTrack = VideoSender
				user.MemberTracks[string(users_connction_check.UserId)].VideoSenderTrack = VideoTrack

			}

		}
	}

}
