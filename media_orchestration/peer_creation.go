package mediaorchestration

import (
	"fmt"
	"thianesh/web_server/models"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func CreateOffer() (*models.FullConnectionDetails, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}

	// VP8 video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "pion-video",
	)
	if err != nil {
		return nil, err
	}
	videoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		return nil, err
	}
	go drainRTCP(videoSender)

	// Data-channel (optional)
	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		return nil, err
	}

	// SDP offer
	offer, _ := pc.CreateOffer(nil)
	_ = pc.SetLocalDescription(offer)
	<-webrtc.GatheringCompletePromise(pc)

	return &models.FullConnectionDetails{
		Webrtc:      pc,
		VideoSender: videoSender,
		DataChannel: dc,
		OfferSDP:    pc.LocalDescription().SDP,
	}, nil
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func PumpSilence(track *webrtc.TrackLocalStaticSample) {
	cn := []byte{0xF8, 0xFF, 0xFE}
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		track.WriteSample(media.Sample{Data: cn, Duration: 20 * time.Millisecond})
	}
}

func PumpBlack(track *webrtc.TrackLocalStaticSample) {
	black := []byte{0x90, 0x90, 0x90} // trivial VP8 payload
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		track.WriteSample(media.Sample{Data: black, Duration: 500 * time.Millisecond})
	}
}


func CreateAnswer(remoteOfferSDP string) (*models.FullConnectionDetails, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}

	/* -- video track --------------------------------------------------- */
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "pion-video",
	)
	if err != nil {
		return nil, err
	}
	videoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		return nil, err
	}
	go drainRTCP(videoSender)

	/* -- audio track --------------------------------------------------- */
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
		},
		"audio", "pion-audio",
	)
	if err != nil {
		return nil, err
	}
	audioSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		return nil, err
	}
	go drainRTCP(audioSender)

	/* -- handle remote offer ------------------------------------------ */
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  remoteOfferSDP,
	}); err != nil {
		return nil, err
	}

	answer, _ := pc.CreateAnswer(nil)
	if err = pc.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) { //nolint: revive
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)
		if track.PayloadType() != 96 {
			for {
				rtp, _, readErr := track.ReadRTP()
				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}
				
				if writeErr := audioTrack.WriteRTP(rtp); writeErr != nil {
					fmt.Println("Unable to Write RTP")
					break
				}
			}
		} else {
			for {
				rtp, _, readErr := track.ReadRTP()
				if readErr != nil {
					fmt.Println("Unable to read RTP")
					break
				}
				
				if writeErr := videoTrack.WriteRTP(rtp); writeErr != nil {
					fmt.Println("Unable to Write RTP")
					break
				}
			}
		}
	})

	<-webrtc.GatheringCompletePromise(pc)

	return &models.FullConnectionDetails{
		Webrtc:      pc,
		VideoSender: videoSender,
		AudioSender: audioSender,
		AnswerSDP:   pc.LocalDescription().SDP,
		OfferSDP:    remoteOfferSDP,
	}, nil
}
