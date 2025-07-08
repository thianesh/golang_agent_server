package mediaorchestration

import (
	"thianesh/web_server/models"
	// "time"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
	// "github.com/pion/webrtc/v4/pkg/media"
)

func CreateOffer(
	parsed_user_data *models.AuthResponse,
	attach_ontrack_member_track_sync func(*models.FullConnectionDetails, *models.CompanySFU),
	company_sfu *models.CompanySFU) (*models.FullConnectionDetails, error) {

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		ICEServers: []webrtc.ICEServer{
			{
				URLs:           []string{"turn:jo.vldo.in:3478"},
				Username:       "thianesh",
				Credential:     "kjroitshhinmaanni",
				CredentialType: webrtc.ICECredentialTypePassword,
			},
		},
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
		Webrtc:              pc,
		// VideoSender:         videoSender,
		// AudioSender:         audioSender,
		// VideoSenderTrack:    videoTrack,
		// AudioSenderTrack:    audioTrack,
		UserId:              models.UserId(parsed_user_data.User.ID),
		CompanySFU:          company_sfu,
		AudioRoomsMap:       make(map[string]bool),
		VideoRoomsMap:       make(map[string]bool),
		OutgoingDataChannel: make(chan []byte, 250),
	}

	full_connection.MemberLock.Lock()
	full_connection.MemberTracks = map[string]*models.MemberOutputTrack{}
	full_connection.MemberLock.Unlock()

	attach_ontrack_member_track_sync(full_connection, company_sfu)

	/* -- create offer -------------------------------------------------- */
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, err
	}

	if err = pc.SetLocalDescription(offer); err != nil {
		return nil, err
	}

	<-webrtc.GatheringCompletePromise(pc)

	full_connection.OfferSDP = pc.LocalDescription().SDP

	return full_connection, nil
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
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		ICEServers: []webrtc.ICEServer{
			{
				URLs:           []string{"turn:jo.vldo.in:3478"},
				Username:       "thianesh",
				Credential:     "kjroitshhinmaanni",
				CredentialType: webrtc.ICECredentialTypePassword,
			},
		},
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
		OfferSDP:            remoteOfferSDP,
		UserId:              models.UserId(parsed_user_data.User.ID),
		CompanySFU:          company_sfu,
		AudioRoomsMap:       make(map[string]bool),
		VideoRoomsMap:       make(map[string]bool),
		OutgoingDataChannel: make(chan []byte, 250),
	}

	full_connection.MemberLock.Lock()
	full_connection.MemberTracks = map[string]*models.MemberOutputTrack{}
	full_connection.MemberLock.Unlock()

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
