package models

import (
	"sync"

	"github.com/pion/rtp"
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
	AudioBuffer      chan *rtp.Packet
	VideoBuffer      chan *rtp.Packet
	PipeAudio        bool
	PipeVideo        bool
	AudioPipeLock    sync.Mutex
	VideoPipeLock    sync.Mutex
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
	VideoRoomsMap            map[string]bool
	AudioRoomsMap            map[string]bool
	AudioPipeLockRoom        sync.RWMutex
	VideoPipeLockRoom        sync.RWMutex
	OutgoingDataChannel      chan []byte
	DiedLock                 sync.Mutex
}

type RoutingCondition struct {
	UserIds []UserId
	RoomIds []RoomId
}
