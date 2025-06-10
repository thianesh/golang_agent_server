package models

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

type UserId string
type RoomId string

type MemberOutputTrack struct {
	AudioTrack       *webrtc.RTPSender
	VideoTrack       *webrtc.RTPSender
	VideoSenderTrack *webrtc.TrackLocalStaticRTP
	AudioSenderTrack *webrtc.TrackLocalStaticRTP
	DataTrack        *webrtc.DataChannel
	Accessible       bool
	Status           string
}

type FullConnectionDetails struct {
	Webrtc      *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
	// VideoSender *webrtc.RTPSender
	// AudioSender *webrtc.RTPSender
	// VideoSenderTrack *webrtc.TrackLocalStaticRTP
	// AudioSenderTrack *webrtc.TrackLocalStaticRTP
	AnswerSDP                string
	OfferSDP                 string
	Died                     bool
	Offline                  bool
	OfflineSince             int64 // Unix timestamp in seconds
	MemberTracks             map[string]*MemberOutputTrack
	OnDataChannelBroadcaster func(*FullConnectionDetails)
	UserId                   UserId
	Username                 string
	Email                    string
	CompanyId                string
	Rooms                    []*Room
	LastActive               int64
	AudioReceiver            *webrtc.TrackRemote
	VideoReceiver            *webrtc.TrackRemote
	RenegotiateMutex         sync.Mutex
	SignallingState          webrtc.SignalingState
	CompanySFU               *CompanySFU
}

type RoutingCondition struct {
	UserIds []UserId
	RoomIds []RoomId
}
