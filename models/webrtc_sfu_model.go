package models

import (
	"github.com/pion/webrtc/v4"
)

type UserId string

type FullConnectionDetails struct {
	Webrtc *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
	VideoSender *webrtc.RTPSender
	AudioSender *webrtc.RTPSender
	AnswerSDP string
	OfferSDP string
	Died bool
	Offline bool
	OfflineSince int64 // Unix timestamp in seconds
	UserDataChannels map[UserId]*webrtc.DataChannel
	OnDataChannelBroadcaster func(*FullConnectionDetails)
}
type UserConnection struct {
	UserId UserId
	Username string
	Email string
	CompanyId string
	Rooms []*Room
	Connections []*FullConnectionDetails
	LastActive int64 // Unix timestamp in seconds
}

type CompanyMembers struct {
	UserConnections map[UserId][]*UserConnection
}
