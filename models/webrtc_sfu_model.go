package models

import (
	"github.com/pion/webrtc/v4"
)

type UserConnection struct {
	UserId string
	Username string
	Email string
	CompanyId string
	Rooms []*Room
	Connections []webrtc.PeerConnection
}