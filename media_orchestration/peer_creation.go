package mediaorchestration

import (
	"thianesh/web_server/models"
	// "time"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
	// "github.com/pion/webrtc/v4/pkg/media"
)

func CreateOffer() (*models.FullConnectionDetails, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}

	// // VP8 video track
	// videoTrack, err := webrtc.NewTrackLocalStaticRTP(
	// 	webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
	// 	"video", "pion-video",
	// )
	// if err != nil {
	// 	return nil, err
	// }
	// videoSender, err := pc.AddTrack(videoTrack)
	// if err != nil {
	// 	return nil, err
	// }
	// go drainRTCP(videoSender)

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
		Webrtc: pc,
		// VideoSender: videoSender,
		DataChannel: dc,
		OfferSDP:    pc.LocalDescription().SDP,
	}, nil
}

func DrainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func createWebRTCAPI() *webrtc.API {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		panic(err)
	}

	pliInterceptor, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		panic(err)
	}
	interceptorRegistry.Add(pliInterceptor)

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)
}

func CreateAnswer(
	remoteOfferSDP string,
	parsed_user_data *models.AuthResponse,
	attach_ontrack_member_track_sync func(*models.FullConnectionDetails, *models.CompanySFU),
	company_sfu *models.CompanySFU) (*models.FullConnectionDetails, error) {

	// api := createWebRTCAPI()

	// pc, err := api.NewPeerConnection(webrtc.Configuration{
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{

		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}

	/* -- video track --------------------------------------------------- */
	// videoTrack, err := webrtc.NewTrackLocalStaticRTP(
	// 	webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
	// 	"video", "pion-video",
	// )
	// if err != nil {
	// 	return nil, err
	// }
	// videoSender, err := pc.AddTrack(videoTrack)
	// if err != nil {
	// 	return nil, err
	// }
	// go drainRTCP(videoSender)

	/* -- audio track --------------------------------------------------- */
	// audioTrack, err := webrtc.NewTrackLocalStaticRTP(
	// 	webrtc.RTPCodecCapability{
	// 		MimeType: webrtc.MimeTypeOpus,
	// 	},
	// 	"audio", "pion-audio",
	// )
	// if err != nil {
	// 	return nil, err
	// }
	// audioSender, err := pc.AddTrack(audioTrack)
	// if err != nil {
	// 	return nil, err
	// }
	// go drainRTCP(audioSender)

	full_connection := &models.FullConnectionDetails{
		Webrtc: pc,
		// VideoSender:      videoSender,
		// AudioSender:      audioSender,
		// VideoSenderTrack: videoTrack,
		// AudioSenderTrack: audioTrack,
		OfferSDP:     remoteOfferSDP,
		UserId:       models.UserId(parsed_user_data.User.ID),
		MemberTracks: map[string]*models.MemberOutputTrack{},
		CompanySFU:   company_sfu,
	}

	attach_ontrack_member_track_sync(full_connection, company_sfu)

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

	<-webrtc.GatheringCompletePromise(pc)

	full_connection.AnswerSDP = pc.LocalDescription().SDP

	return full_connection, nil
}
