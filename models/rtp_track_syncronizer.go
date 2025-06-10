package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

func Sync_track(peer_connection *FullConnectionDetails, company_sfu *CompanySFU) {
	peer_connection.Webrtc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		// fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)

		mime := track.Codec().MimeType

		// Track has started, of type 111: audio/opus
		// Track has started, of type 96: video/VP8

		if strings.HasPrefix(mime, "audio/") {
			peer_connection.AudioReceiver = track

			for {
				rtp, _, readErr := track.ReadRTP()

				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}

				for _, user := range company_sfu.Users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					// Peerconnection is sending the RTP tracks for the user to receive the connection to him must be stable.
					if user.Webrtc.SignalingState() != webrtc.SignalingStateStable {
						fmt.Println("Signaling state is not stable for user", user.Email, user.UserId, "state:", user.Webrtc.SignalingState())
						continue
					}

					if peer_connection.Webrtc.SignalingState() != webrtc.SignalingStateStable {
						fmt.Println("Signaling state is not stable for peer connection", peer_connection.Email, peer_connection.UserId, "state:", peer_connection.Webrtc.SignalingState())
						continue
					}

					// fmt.Println("Signaling state is stable for user", user.Email, user.UserId, "state:", user.Webrtc.SignalingState(), "and peer connection", peer_connection.Email, peer_connection.UserId, "state:", peer_connection.Webrtc.SignalingState())

					if user.MemberTracks[string(peer_connection.UserId)] == nil {
						fmt.Println("Member Track (AudioTrack + VideoTrack) is nill for ", peer_connection.UserId)
						// go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}

					if user.MemberTracks[string(peer_connection.UserId)].AudioTrack == nil {
						fmt.Println("Audio track is nill for ", string(peer_connection.UserId))
						// go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}

					if writeErr := user.MemberTracks[string(peer_connection.UserId)].AudioSenderTrack.WriteRTP(rtp); writeErr != nil {
						fmt.Println("Unable to Write RTP")
						continue
					}
				}
			}

		} else if strings.HasPrefix(mime, "video/") {
			peer_connection.VideoReceiver = track

			// Sending PLI for new connections to receive the video stream
			SendPLI(track, peer_connection.Webrtc, 10*time.Second)

			for {
				rtp, _, readErr := track.ReadRTP()

				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}

				for _, user := range company_sfu.Users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					// Peerconnection is sending the RTP tracks for the user to receive the connection to him must be stable.
					if user.Webrtc.SignalingState() != webrtc.SignalingStateStable {
						fmt.Println("Signaling state is not stable for user", user.Email, user.UserId, "state:", user.Webrtc.SignalingState())
						continue
					}

					if peer_connection.Webrtc.SignalingState() != webrtc.SignalingStateStable {
						fmt.Println("Signaling state is not stable for peer connection", peer_connection.Email, peer_connection.UserId, "state:", peer_connection.Webrtc.SignalingState())
						continue
					}

					// fmt.Println("Signaling state is stable for user", user.Email, user.UserId, "state:", user.Webrtc.SignalingState(), "and peer connection", peer_connection.Email, peer_connection.UserId, "state:", peer_connection.Webrtc.SignalingState())

					if user.MemberTracks[string(peer_connection.UserId)] == nil {
						fmt.Println("Member Track (AudioTrack + VideoTrack) is nill for ", peer_connection.UserId, "for user", user.Email, user.UserId)
						// go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}

					if user.MemberTracks[string(peer_connection.UserId)].VideoTrack == nil {
						fmt.Println("Video track is nill for ", string(peer_connection.UserId), "for user", user.Email, user.UserId)
						// go sysc_user_tracks_and_renegotiate(company_sfu)
						continue
					}

					if writeErr := user.MemberTracks[string(peer_connection.UserId)].VideoSenderTrack.WriteRTP(rtp); writeErr != nil {
						fmt.Println("Unable to Write RTP")
						continue
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

		fmt.Println("Re-Negotiation initiated with wait group")
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
			fmt.Println("No data channel to re-negotiate! user_id: ", single_connection.UserId)
		}
	}
	renegotiate()
}

func Renegotiate_no_waitgroup(single_connection *FullConnectionDetails) {

	renegotiate := func() {
		single_connection.RenegotiateMutex.Lock()
		defer single_connection.RenegotiateMutex.Unlock()

		fmt.Println("Re-Negotiation initiated without wait group line 264")
		offer, _ := single_connection.Webrtc.CreateOffer(nil) // plain renegotiation; ICE stays same
		_ = single_connection.Webrtc.SetLocalDescription(offer)
		<-webrtc.GatheringCompletePromise(single_connection.Webrtc) // wait for all ICE candidates

		payload := map[string]interface{}{
			"Type": "offer",
			"SDP":  single_connection.Webrtc.LocalDescription().SDP,
		}

		b, _ := json.Marshal(payload)
		if single_connection.DataChannel != nil {
			fmt.Println("Negotiation Strated successfully, sending New offer in data channel for user:", single_connection.UserId)
			single_connection.DataChannel.Send(b)
		} else {
			fmt.Println("No data channel to re-negotiate! user:", single_connection.UserId)
		}
	}
	renegotiate()
}

func sysc_user_tracks_and_renegotiate(company_sfu *CompanySFU) {

	fmt.Println("Syncing user tracks and renegotiating...")

	users_to_renegotiate := make([]*FullConnectionDetails, 0)

	for _, user := range company_sfu.Users {

		if user.Died {
			continue
		}

		if user.MemberTracks == nil {
			user.MemberTracks = map[string]*MemberOutputTrack{}
		}

		// Each user should have all the memebers connection except his own
		for _, users_connction_check := range company_sfu.Users {

			if user.UserId == users_connction_check.UserId {
				continue
			}

			track_exists := false

			audio_track := company_sfu.Users[users_connction_check.UserId].AudioReceiver
			video_track := company_sfu.Users[users_connction_check.UserId].VideoReceiver

			if audio_track != nil {

				// If audio track is not present for the user, we will add it
				if _, ok := user.MemberTracks[string(users_connction_check.UserId)]; ok {
					if user.MemberTracks[string(users_connction_check.UserId)].AudioSenderTrack == nil {
						if _, ok := AddAudioTrack(user, company_sfu, users_connction_check); ok {
							fmt.Println("Added audio track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
							track_exists = true
						}
					}
				} else {
					// If the member track is not present, we will add it
					if _, ok := AddAudioTrack(user, company_sfu, users_connction_check); ok {
						fmt.Println("Added audio track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
						track_exists = true
					}
				}

			}

			if video_track != nil {

				// If audio track is not present for the user, we will add it
				if _, ok := user.MemberTracks[string(users_connction_check.UserId)]; ok {
					if user.MemberTracks[string(users_connction_check.UserId)].VideoSenderTrack == nil {
						if _, ok := AddVideoTrack(user, company_sfu, users_connction_check); ok {
							fmt.Println("Added video track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
							track_exists = true
						}
					}
				} else {
					// If the member track is not present, we will add it
					if _, ok := AddVideoTrack(user, company_sfu, users_connction_check); ok {
						fmt.Println("Added video track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
						track_exists = true
					}
				}

			}
			if track_exists {
				users_to_renegotiate = append(users_to_renegotiate, user)
			}

		}
	}
}

func AddAudioTrack(user *FullConnectionDetails, company_sfu *CompanySFU, users_connction_check *FullConnectionDetails) (error, bool) {

	if _, ok := company_sfu.Users[users_connction_check.UserId]; !ok {
		return fmt.Errorf("user not found in company sfu"), false
	}

	track := company_sfu.Users[users_connction_check.UserId].AudioReceiver

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType: track.Codec().MimeType,
		},
		string(users_connction_check.UserId)+"_audio", string(user.UserId)+"_"+string(users_connction_check.UserId)+"_audio",
	)
	if err != nil {
		return err, false
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
		return err, false
	}

	if user.MemberTracks[string(users_connction_check.UserId)] == nil {
		user.MemberTracks[string(users_connction_check.UserId)] = &MemberOutputTrack{
			Accessible: true,
			Status:     "online",
		}
	}

	user.MemberTracks[string(users_connction_check.UserId)].AudioTrack = audioSender
	user.MemberTracks[string(users_connction_check.UserId)].AudioSenderTrack = audioTrack
	user.MemberTracks[string(users_connction_check.UserId)].DataTrack = users_connction_check.DataChannel
	user.MemberTracks[string(users_connction_check.UserId)].Status = "online"
	user.MemberTracks[string(users_connction_check.UserId)].Accessible = true
	return nil, true
}

func AddVideoTrack(user *FullConnectionDetails, company_sfu *CompanySFU, users_connction_check *FullConnectionDetails) (error, bool) {

	if _, ok := company_sfu.Users[users_connction_check.UserId]; !ok {
		return fmt.Errorf("user not found in company sfu"), false
	}

	track := company_sfu.Users[users_connction_check.UserId].VideoReceiver

	VideoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType: track.Codec().MimeType,
		},
		string(users_connction_check.UserId)+"_video", string(user.UserId)+"_"+string(users_connction_check.UserId)+"_video",
	)
	if err != nil {
		return err, false
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
		return err, false
	}

	if user.MemberTracks[string(users_connction_check.UserId)] == nil {
		user.MemberTracks[string(users_connction_check.UserId)] = &MemberOutputTrack{
			Accessible: true,
			Status:     "online",
		}
	}

	user.MemberTracks[string(users_connction_check.UserId)].VideoTrack = VideoSender
	user.MemberTracks[string(users_connction_check.UserId)].VideoSenderTrack = VideoTrack
	user.MemberTracks[string(users_connction_check.UserId)].DataTrack = users_connction_check.DataChannel
	user.MemberTracks[string(users_connction_check.UserId)].Status = "online"
	user.MemberTracks[string(users_connction_check.UserId)].Accessible = true
	return nil, true
}

func UniqueUsers(input []*FullConnectionDetails) []*FullConnectionDetails {
	seen := make(map[*FullConnectionDetails]bool)
	unique := make([]*FullConnectionDetails, 0)

	for _, u := range input {
		if !seen[u] {
			seen[u] = true
			unique = append(unique, u)
		}
	}
	return unique
}
