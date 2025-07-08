package models

import (
	"context"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type UserId string
type RoomId string

type MemberOutputTrack struct {
	AudioTrack         *webrtc.RTPSender
	VideoTrack         *webrtc.RTPSender
	VideoSenderTrack   *webrtc.TrackLocalStaticRTP
	AudioSenderTrack   *webrtc.TrackLocalStaticRTP
	DataTrack          *webrtc.DataChannel
	Accessible         bool
	Status             string
	AudioBuffer        chan *rtp.Packet
	VideoBuffer        chan *rtp.Packet
	AudioBufferClose   sync.Mutex
	VideoBufferClose   sync.Mutex
	PipeAudio          bool
	PipeVideo          bool
	AudioPipeLock      sync.Mutex
	VideoPipeLock      sync.Mutex
	AudioForwardCancel context.CancelFunc
	VideoForwardCancel context.CancelFunc
	AudioTrackCancel   context.CancelFunc
	VideoTrackCancel   context.CancelFunc
}

type FullConnectionDetails struct {
	Webrtc        *webrtc.PeerConnection
	DataChannel   *webrtc.DataChannel
	OfferAccepted bool
	// VideoSender *webrtc.RTPSender
	// AudioSender *webrtc.RTPSender
	// VideoSenderTrack *webrtc.TrackLocalStaticRTP
	// AudioSenderTrack *webrtc.TrackLocalStaticRTP
	AnswerSDP                   string
	OfferSDP                    string
	Died                        bool
	Offline                     bool
	OfflineSince                int64 // Unix timestamp in seconds
	MemberTracks                map[string]*MemberOutputTrack
	OnDataChannelBroadcaster    func(*FullConnectionDetails)
	UserId                      UserId
	Username                    string
	Email                       string
	CompanyId                   string
	Rooms                       []*Room
	LastActive                  int64
	AudioReceiver               *webrtc.TrackRemote
	VideoReceiver               *webrtc.TrackRemote
	RenegotiateMutex            sync.Mutex
	SignallingState             webrtc.SignalingState
	CompanySFU                  *CompanySFU
	VideoRoomsMap               map[string]bool
	AudioRoomsMap               map[string]bool
	AudioPipeLockRoom           sync.RWMutex
	VideoPipeLockRoom           sync.RWMutex
	OutgoingDataChannel         chan []byte
	DiedLock                    sync.Mutex
	MemberLock                  sync.Mutex
	FullConnectionDetailsRWLock sync.RWMutex
}

type RoutingCondition struct {
	UserIds []UserId
	RoomIds []RoomId
}
