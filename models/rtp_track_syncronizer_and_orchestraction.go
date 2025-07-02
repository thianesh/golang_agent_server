package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtp"
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

var lastSent sync.Map // key = *webrtc.TrackLocalStaticRTP , value = time.Time

func sendEmptyAudio(sender *webrtc.TrackLocalStaticRTP) {
	// 3-byte Opus comfort-noise frame (RFC 6716 §3.1.2)
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version: 2,
			// PayloadType is overwritten by Pion when it binds, so 0 is fine.
		},
		Payload: []byte{0xF8, 0xFF, 0xFE},
	}
	_ = sender.WriteRTP(pkt)
}

func sendEmptyVideo(sender *webrtc.TrackLocalStaticRTP) {
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version: 2,
			Padding: true, // RFC 6263 keep-alive
			// PayloadType is overwritten by Pion just like above.
		},
		Payload:     []byte{0x00}, // single padding byte
		PaddingSize: 1,
	}
	_ = sender.WriteRTP(pkt)
}

func Forward(ctx context.Context, buf <-chan *rtp.Packet, sender *webrtc.TrackLocalStaticRTP, tag string) {
	ticker := time.NewTicker(1 * time.Second) // how often we may inject keep-alive
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("[" + tag + "] context cancelled")
			return

		case pkt, ok := <-buf:
			if !ok {
				fmt.Println("[" + tag + "] buffer closed")
				return
			}
			if err := sender.WriteRTP(pkt); err != nil {
				fmt.Printf("[%s] WriteRTP failed: %v\n", tag, err)
				return
			}
			lastSent.Store(sender, time.Now()) // remember last real packet time

		case <-ticker.C:
			if v, ok := lastSent.Load(sender); ok && time.Since(v.(time.Time)) < time.Second {
				// we wrote something real in the last second → skip keep-alive
				continue
			}
			if strings.Contains(tag, "audio") {
				sendEmptyAudio(sender)
			} else {
				sendEmptyVideo(sender)
			}
		}
	}
}

// func Forward(ctx context.Context, buf <-chan *rtp.Packet, sender *webrtc.TrackLocalStaticRTP, tag string) {
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			fmt.Println("[" + tag + "] context cancelled")
// 			return
// 		case pkt, ok := <-buf:
// 			if !ok {
// 				fmt.Println("[" + tag + "] buffer closed")
// 				return
// 			}
// 			if err := sender.WriteRTP(pkt); err != nil {
// 				fmt.Printf("[%s] WriteRTP failed: %v\n", tag, err)
// 				return
// 			}
// 		}
// 	}
// }

func Sync_track(peer_connection *FullConnectionDetails, company_sfu *CompanySFU) {

	company_sfu.CompanySFUsMutex.RLock()
	all_users := company_sfu.Users
	company_sfu.CompanySFUsMutex.RUnlock()

	// refresh_users := func() {
	// 	company_sfu.CompanySFUsMutex.RLock()
	// 	all_users = company_sfu.Users
	// 	company_sfu.CompanySFUsMutex.RUnlock()
	// }

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
					fmt.Println("Unable to read Audio RTP")
					break
				}

				company_sfu.CompanySFUsMutex.RLock()
				all_users = company_sfu.Users
				company_sfu.CompanySFUsMutex.RUnlock()

				for _, user := range all_users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					if _, ok := all_users[user.UserId]; !ok {
						continue
					}

					all_users[user.UserId].FullConnectionDetailsRWLock.RLock()
					if all_users[user.UserId] == nil || all_users[user.UserId].Webrtc == nil || all_users[user.UserId].Died || all_users[user.UserId].DataChannel == nil {
						all_users[user.UserId].FullConnectionDetailsRWLock.RUnlock()
						continue
					}
					all_users[user.UserId].FullConnectionDetailsRWLock.RUnlock()

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

					user.MemberLock.Lock()
					memberTrack := user.MemberTracks[string(peer_connection.UserId)]
					user.MemberLock.Unlock()
					if memberTrack == nil {
						fmt.Println("Member Track (AudioTrack + VideoTrack) is nill for ", peer_connection.UserId)
						company_sfu.RtpSyncNeededMutex.Lock()
						company_sfu.RtpSyncNeeded = true
						company_sfu.RtpSyncNeededMutex.Unlock()
						continue
					}

					if memberTrack.AudioTrack == nil {
						fmt.Println("Audio track is nill for ", string(peer_connection.UserId))
						company_sfu.RtpSyncNeededMutex.Lock()
						company_sfu.RtpSyncNeeded = true
						company_sfu.RtpSyncNeededMutex.Unlock()
						continue
					}

					peer_connection.MemberLock.Lock()
					peerTrack := peer_connection.MemberTracks[string(user.UserId)]
					peer_connection.MemberLock.Unlock()
					if peerTrack == nil {
						continue
					}
					// Route check
					peerTrack.AudioPipeLock.Lock()
					should_pipe := peerTrack.PipeAudio
					peerTrack.AudioPipeLock.Unlock()

					// Route check
					if should_pipe {
						select {
						case memberTrack.AudioBuffer <- rtp.Clone():
							continue
							// successfully passed to buff
						default:
							// buff full leaving for now
						}
					}

					// now need to chech if user is broadcasing to room and if this particular user is in that room send the media
					peer_connection.AudioPipeLockRoom.RLock()
					rooms := peer_connection.AudioRoomsMap
					peer_connection.AudioPipeLockRoom.RUnlock()

					for room_id, state := range rooms {
						if !state {
							continue
						}
						if Contains(*company_sfu.Rooms[RoomId(room_id)].AccessList, string(peer_connection.UserId)) {
							select {
							case memberTrack.AudioBuffer <- rtp.Clone():
								continue
								// successfully passed to buff
							default:
								// buff full leaving for now
							}
						}
					}

					// user.MemberTracks[string(peer_connection.UserId)].AudioBuffer <- rtp.Clone()

				}
			}

		} else if strings.HasPrefix(mime, "video/") {
			peer_connection.VideoReceiver = track

			// Sending PLI for new connections to receive the video stream
			SendPLI(track, peer_connection.Webrtc, 5*time.Second)

			for {
				rtp, _, readErr := track.ReadRTP()

				company_sfu.CompanySFUsMutex.RLock()
				all_users = company_sfu.Users
				company_sfu.CompanySFUsMutex.RUnlock()

				if readErr != nil {
					fmt.Println("Unable to read Video RTP")
					break
				}

				for _, user := range all_users {

					if user.UserId == peer_connection.UserId {
						continue
					}

					if _, ok := all_users[user.UserId]; !ok {
						continue
					}

					all_users[user.UserId].FullConnectionDetailsRWLock.RLock()
					if all_users[user.UserId] == nil || all_users[user.UserId].Webrtc == nil || all_users[user.UserId].Died || all_users[user.UserId].DataChannel == nil {
						all_users[user.UserId].FullConnectionDetailsRWLock.RUnlock()
						continue
					}
					all_users[user.UserId].FullConnectionDetailsRWLock.RUnlock()
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

					user.MemberLock.Lock()
					memberTrack := user.MemberTracks[string(peer_connection.UserId)]
					user.MemberLock.Unlock()
					if memberTrack == nil {
						fmt.Println("Member Track (AudioTrack + VideoTrack) is nill for ", peer_connection.UserId, "for user", user.Email, user.UserId)
						company_sfu.RtpSyncNeededMutex.Lock()
						company_sfu.RtpSyncNeeded = true
						company_sfu.RtpSyncNeededMutex.Unlock()
						continue
					}
					if memberTrack.VideoTrack == nil {
						fmt.Println("Video track is nill for ", string(peer_connection.UserId), "for user", user.Email, user.UserId)
						company_sfu.RtpSyncNeededMutex.Lock()
						company_sfu.RtpSyncNeeded = true
						company_sfu.RtpSyncNeededMutex.Unlock()
						continue
					}

					peer_connection.MemberLock.Lock()
					peerTrack := peer_connection.MemberTracks[string(user.UserId)]
					peer_connection.MemberLock.Unlock()
					if peerTrack == nil {
						continue
					}

					// Route check
					peerTrack.VideoPipeLock.Lock()
					should_pipe := peerTrack.PipeVideo
					peerTrack.VideoPipeLock.Unlock()

					if should_pipe {
						select {
						case memberTrack.VideoBuffer <- rtp.Clone():
							continue
							// successfully passed to buff
						default:
							// buff full leaving for now
						}
					}

					// now need to chech if user is broadcasing to room and if this particular user is in that room send the media
					peer_connection.VideoPipeLockRoom.RLock()
					rooms := peer_connection.VideoRoomsMap
					peer_connection.VideoPipeLockRoom.RUnlock()

					for room_id, state := range rooms {
						if !state {
							continue
						}
						if Contains(*company_sfu.Rooms[RoomId(room_id)].AccessList, string(peer_connection.UserId)) {
							select {
							case memberTrack.VideoBuffer <- rtp.Clone():
								continue
								// successfully passed to buff
							default:
								// buff full leaving for now
							}
						}
					}

					// user.MemberTracks[string(peer_connection.UserId)].VideoBuffer <- rtp.Clone()

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

	fmt.Println("Entering sysc_user_tracks_and_renegotiate function")

	company_sfu.RtpSyncNeededMutex.Lock()
	RtpSyncNeeded := company_sfu.RtpSyncNeeded
	Renegotiating := company_sfu.Renegotiating
	company_sfu.RtpSyncNeededMutex.Unlock()

	if !RtpSyncNeeded || Renegotiating {
		return
	}

	defer func() {
		company_sfu.RtpSyncNeededMutex.Lock()
		company_sfu.RtpSyncNeeded = false
		company_sfu.Renegotiating = false
		company_sfu.RtpSyncNeededMutex.Unlock()
	}()

	company_sfu.RtpSyncNeededMutex.Lock()
	company_sfu.Renegotiating = true
	company_sfu.RtpSyncNeededMutex.Unlock()

	fmt.Println("Syncing user tracks and renegotiating...")

	users_to_renegotiate := make([]*FullConnectionDetails, 0)

	company_sfu.CompanySFUsMutex.RLock()
	all_users := company_sfu.Users
	company_sfu.CompanySFUsMutex.RUnlock()

	// refresh_users := func() {
	// 	company_sfu.CompanySFUsMutex.RLock()
	// 	all_users = company_sfu.Users
	// 	company_sfu.CompanySFUsMutex.RUnlock()
	// }

	for _, user := range all_users {

		user.FullConnectionDetailsRWLock.RLock()
		is_user_alive := user.Died
		user.FullConnectionDetailsRWLock.RUnlock()

		if is_user_alive {
			continue
		}

		user.MemberLock.Lock()
		if user.MemberTracks == nil {
			user.MemberTracks = map[string]*MemberOutputTrack{}
		}
		user.MemberLock.Unlock()

		// Each user should have all the memebers connection except his own
		for _, users_connction_check := range all_users {

			if user.UserId == users_connction_check.UserId {
				continue
			}

			track_exists := false

			audio_track := all_users[users_connction_check.UserId].AudioReceiver
			video_track := all_users[users_connction_check.UserId].VideoReceiver

			if audio_track != nil {

				// If audio track is not present for the user, we will add it
				user.MemberLock.Lock()
				mt, ok := user.MemberTracks[string(users_connction_check.UserId)]
				user.MemberLock.Unlock()
				if ok {
					if mt.AudioSenderTrack == nil {
						if _, ok := AddAudioTrack(user, company_sfu, users_connction_check); ok {
							fmt.Println("Added audio track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
							track_exists = true
							user.MemberLock.Lock()
							user.MemberTracks[string(users_connction_check.UserId)].AudioBuffer = make(chan *rtp.Packet, 256)

							ctx, cancel := context.WithCancel(context.Background())
							user.MemberTracks[string(users_connction_check.UserId)].AudioForwardCancel = cancel

							go Forward(ctx, user.MemberTracks[string(users_connction_check.UserId)].AudioBuffer, user.MemberTracks[string(users_connction_check.UserId)].AudioSenderTrack, string(users_connction_check.UserId)+">> audio")
							user.MemberLock.Unlock()
						}
					}
				} else {
					// If the member track is not present, we will add it
					if _, ok := AddAudioTrack(user, company_sfu, users_connction_check); ok {
						fmt.Println("Added audio track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
						track_exists = true
						user.MemberLock.Lock()

						ctx, cancel := context.WithCancel(context.Background())
						user.MemberTracks[string(users_connction_check.UserId)].AudioForwardCancel = cancel

						user.MemberTracks[string(users_connction_check.UserId)].AudioBuffer = make(chan *rtp.Packet, 256)
						go Forward(ctx, user.MemberTracks[string(users_connction_check.UserId)].AudioBuffer, user.MemberTracks[string(users_connction_check.UserId)].AudioSenderTrack, string(users_connction_check.UserId)+">> audio")
						user.MemberLock.Unlock()
					}
				}

			}

			if video_track != nil {

				// If audio track is not present for the user, we will add it
				user.MemberLock.Lock()
				mt, ok := user.MemberTracks[string(users_connction_check.UserId)]
				user.MemberLock.Unlock()
				if ok {
					if mt.VideoSenderTrack == nil {
						if _, ok := AddVideoTrack(user, company_sfu, users_connction_check); ok {
							fmt.Println("Added video track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
							track_exists = true
							user.MemberLock.Lock()

							ctx, cancel := context.WithCancel(context.Background())
							user.MemberTracks[string(users_connction_check.UserId)].VideoForwardCancel = cancel

							user.MemberTracks[string(users_connction_check.UserId)].VideoBuffer = make(chan *rtp.Packet, 1024)
							go Forward(ctx, user.MemberTracks[string(users_connction_check.UserId)].VideoBuffer, user.MemberTracks[string(users_connction_check.UserId)].VideoSenderTrack, string(users_connction_check.UserId)+">> video")
							user.MemberLock.Unlock()

						}
					}
				} else {
					// If the member track is not present, we will add it
					if _, ok := AddVideoTrack(user, company_sfu, users_connction_check); ok {
						fmt.Println("Added video track for", users_connction_check.Email, " to the user", user.Email, user.UserId)
						track_exists = true
						user.MemberLock.Lock()

						ctx, cancel := context.WithCancel(context.Background())
						user.MemberTracks[string(users_connction_check.UserId)].VideoForwardCancel = cancel

						user.MemberTracks[string(users_connction_check.UserId)].VideoBuffer = make(chan *rtp.Packet, 1024)
						go Forward(ctx, user.MemberTracks[string(users_connction_check.UserId)].VideoBuffer, user.MemberTracks[string(users_connction_check.UserId)].VideoSenderTrack, string(users_connction_check.UserId)+">> video")
						user.MemberLock.Unlock()
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

	company_sfu.CompanySFUsMutex.RLock()
	all_users := company_sfu.Users
	company_sfu.CompanySFUsMutex.RUnlock()

	// refresh_users := func() {
	// 	company_sfu.CompanySFUsMutex.RLock()
	// 	all_users = company_sfu.Users
	// 	company_sfu.CompanySFUsMutex.RUnlock()
	// }

	if _, ok := all_users[users_connction_check.UserId]; !ok {
		return fmt.Errorf("user not found in company sfu"), false
	}

	track := all_users[users_connction_check.UserId].AudioReceiver

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

	user.MemberLock.Lock()
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
	user.MemberLock.Unlock()
	return nil, true
}

func AddVideoTrack(user *FullConnectionDetails, company_sfu *CompanySFU, users_connction_check *FullConnectionDetails) (error, bool) {

	company_sfu.CompanySFUsMutex.RLock()
	all_users := company_sfu.Users
	company_sfu.CompanySFUsMutex.RUnlock()

	// refresh_users := func() {
	// 	company_sfu.CompanySFUsMutex.RLock()
	// 	all_users = company_sfu.Users
	// 	company_sfu.CompanySFUsMutex.RUnlock()
	// }

	if _, ok := all_users[users_connction_check.UserId]; !ok {
		return fmt.Errorf("user not found in company sfu"), false
	}

	track := all_users[users_connction_check.UserId].VideoReceiver

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

	user.MemberLock.Lock()
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
	user.MemberLock.Unlock()
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
