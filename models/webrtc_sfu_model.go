package models

import (
	"github.com/pion/webrtc/v4"
)

type UserId string

type FullConnectionDetails struct {
	Webrtc *webrtc.PeerConnection
	DataChannel *webrtc.DataChannel
	RtpSender *webrtc.RTPSender
	SDP string
}
type UserConnection struct {
	UserId UserId
	Username string
	Email string
	CompanyId string
	Rooms []*Room
	Connections []*FullConnectionDetails
}

type CompanyMembers struct {
	UserConnections map[UserId][]*UserConnection
}
