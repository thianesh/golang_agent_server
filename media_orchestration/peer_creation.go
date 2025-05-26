package mediaorchestration

import (
	"thianesh/web_server/models"

	"github.com/pion/webrtc/v4"
)

func CreateOffer() (*models.FullConnectionDetails, error) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	outputTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion",
	)
	if err != nil {
		return nil, err
	}

	rtpSender, err := peerConnection.AddTrack(outputTrack)
	if err != nil {
		return nil, err
	}

	// Read incoming RTCP packets
	// Before these packets are returned they are processed by interceptors. For things
	// like NACK this needs to be called.
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	dataChannel, err := peerConnection.CreateDataChannel("data", nil)
	if err != nil {
		return nil, err
	}

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		return nil, err
	}

	// Set the LocalDescription and start ICE gathering
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		return nil, err
	}

	// Wait for ICE gathering to complete
	<-webrtc.GatheringCompletePromise(peerConnection)

	// Now the offer with ICE candidates is ready in LocalDescription
	finalOffer := peerConnection.LocalDescription()

	return &models.FullConnectionDetails{
		Webrtc:      peerConnection,
		RtpSender:   rtpSender,
		DataChannel: dataChannel,
		SDP:         finalOffer.SDP,
	}, nil
}

func CreateAnswer(remoteOfferSDP string) (*models.FullConnectionDetails, error) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	outputTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion",
	)
	if err != nil {
		return nil, err
	}

	rtpSender, err := peerConnection.AddTrack(outputTrack)
	if err != nil {
		return nil, err
	}

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	dataChannel, err := peerConnection.CreateDataChannel("data", nil)
	if err != nil {
		return nil, err
	}

	// Set remote offer
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  remoteOfferSDP,
	}
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	// Create the answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	if err := peerConnection.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	// Wait for ICE gathering to complete
	<-webrtc.GatheringCompletePromise(peerConnection)

	finalAnswer := peerConnection.LocalDescription()

	return &models.FullConnectionDetails{
		Webrtc:      peerConnection,
		RtpSender:   rtpSender,
		DataChannel: dataChannel,
		SDP:         finalAnswer.SDP,
	}, nil
}
