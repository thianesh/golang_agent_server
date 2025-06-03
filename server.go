package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"
	mediaorchestration "thianesh/web_server/media_orchestration"
	"thianesh/web_server/models"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/rs/cors"
)

var logger *slog.Logger

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rr := &responseRecorder{ResponseWriter: w, statusCode: 200} // default 200
		next.ServeHTTP(rr, r)
		log.Printf("Request %s %s -> %d\n", r.Method, r.URL.Path, rr.statusCode)
	})
}

var UserConnections = make(map[string]*models.UserConnection)
var CompanyAndMembers = make(map[string]*models.CompanyMembers)
var CompanySFUs = make(map[string]*models.CompanySFU)

type SDPRequest struct {
	SDP string `json:"SDP"`
}

func auth_handler(w http.ResponseWriter, r *http.Request) {
	var payload SDPRequest

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.SDP == "" {
		http.Error(w, "Please provide valid SDP", http.StatusBadRequest)
		return
	}

	// Example authentication handler
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header missing", http.StatusUnauthorized)
		return
	}

	// Define the external URL to forward the request to
	targetURL := "http://localhost:8000/functions/v1/get-connection-details"

	// Create a new GET request
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		log.Println("Request creation error:", err)
		return
	}

	// Copy Authorization header
	req.Header.Set("Authorization", authHeader)

	// Perform the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to send request", http.StatusInternalServerError)
		log.Println("Request send error:", err)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		log.Println("Read body error:", err)
		return
	}

	// Print the response to stdout

	var parsed_user_data models.AuthResponse
	json_err := json.Unmarshal(body, &parsed_user_data)
	if json_err != nil {
		http.Error(w, "Failed to parse response", http.StatusInternalServerError)
		log.Println("JSON unmarshal error:", json_err)
		return
	}

	logger.Debug("Response from target server", "data", parsed_user_data)
	// Optional: Write the response back to the original client
	w.WriteHeader(resp.StatusCode)
	w.Header().Set("Content-Type", "application/json")

	// Now we have the SDP and user details, we can accept the connection
	sdp, err := DecodeFromBase64(payload.SDP)
	if err != nil {
		http.Error(w, "Failed to decode SDP", http.StatusInternalServerError)
		log.Println("SDP decode error:", err)
		return
	}

	// accepting the offered SDP
	peerConnection, err := mediaorchestration.CreateAnswer(sdp)
	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		log.Println("Create answer error:", err)
		return
	}

	peerConnection.OfferSDP = payload.SDP
	peerConnection.AnswerSDP = peerConnection.Webrtc.LocalDescription().SDP
	peerConnection.Died = false
	peerConnection.Offline = false
	peerConnection.OfflineSince = 0

	if _, ok := UserConnections[parsed_user_data.User.ID]; !ok {
		log.Println("User not found, creating new connection")
		UserConnections[parsed_user_data.User.ID] = &models.UserConnection{
			UserId:      models.UserId(parsed_user_data.User.ID),
			Username:    parsed_user_data.User.FullName,
			Email:       parsed_user_data.User.Email,
			CompanyId:   parsed_user_data.CompanyID,
			Rooms:       []*models.Room{}, // Initialize with empty rooms
			Connections: []*models.FullConnectionDetails{},
		}
	}

	if _, ok := CompanyAndMembers[parsed_user_data.CompanyID]; !ok {
		log.Println("Company not found, creating new company members")
		CompanyAndMembers[parsed_user_data.CompanyID] = &models.CompanyMembers{
			UserConnections: make(map[models.UserId][]*models.UserConnection),
		}
	}

	if len(UserConnections[parsed_user_data.User.ID].Connections) > 0 {
		logger.Debug(fmt.Sprintf("Existing connection found for %s, connection state: %t", UserConnections[parsed_user_data.User.ID].Email, UserConnections[parsed_user_data.User.ID].Connections[0].Died))
		if UserConnections[parsed_user_data.User.ID].Connections[0].Died {
			logger.Debug("Before removing exiting connections", "connection details", UserConnections[parsed_user_data.User.ID].Connections)
			UserConnections[parsed_user_data.User.ID].Connections = slices.Delete(UserConnections[parsed_user_data.User.ID].Connections, 0, 1)
			logger.Debug("After removing exiting connections", "connection details", UserConnections[parsed_user_data.User.ID].Connections)
		}
	}
	// Don't want to add more than one user connections for now
	if len(UserConnections[parsed_user_data.User.ID].Connections) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		json.NewEncoder(w).Encode(map[string]string{
			"error": "User connection already exists. Please exit that connection to connect here.",
		})

		return
	}

	// Add the peer connection to the user's connections
	UserConnections[parsed_user_data.User.ID].Connections = append(
		UserConnections[parsed_user_data.User.ID].Connections,
		peerConnection,
	)
	// Add the user connection to the company members
	CompanyAndMembers[parsed_user_data.CompanyID].UserConnections[models.UserId(parsed_user_data.User.ID)] =
		append(CompanyAndMembers[parsed_user_data.CompanyID].UserConnections[models.UserId(parsed_user_data.User.ID)],
			UserConnections[parsed_user_data.User.ID],
		)

	// start all webrtc processes
	go mediaorchestration.SingleOrchestrator(peerConnection)

	//setup renegotiation
	peerConnection.OnDataChannelBroadcaster = func(fcd *models.FullConnectionDetails) {
		logger.Debug("Data Channel added! adding negotiator.")
		mediaorchestration.Initialize_renegotiation(fcd)
	}

	// Add company SFU process to CompanuSFUs
	if _, ok := CompanySFUs[parsed_user_data.CompanyID]; !ok {
		CompanySFUs[parsed_user_data.CompanyID] = models.NewCompanySFU()
		// Start SFU processed
		// Start Boradcasting online status
		go CompanySFUs[parsed_user_data.CompanyID].StartOnlineStatusBroadcaster()
		// Start sending HeartBeat
		go CompanySFUs[parsed_user_data.CompanyID].StartHeartBeat()
	}

	if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; !ok {
		CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)] = UserConnections[parsed_user_data.User.ID].Connections[0]
	}

	res_payload := map[string]interface{}{
		"SDP":    EncodeToBase64(peerConnection.AnswerSDP),
		"status": "success",
	}

	if err := json.NewEncoder(w).Encode(res_payload); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}

	// testing re-negotiation
	go add_track(peerConnection)

	logger.Debug("All-Set nothing pending.")
}

func add_track(peerConnection *models.FullConnectionDetails) {

	waitSignallingStable(peerConnection.Webrtc)

	time.Sleep(30 * time.Second) // simulate “later”

	outputTrack, _ := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video-2", "stream-video-id",
	)

	rtpSender, _ := peerConnection.Webrtc.AddTrack(outputTrack)

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	log.Println("Added 2nd track → renegotiation should start")
	mediaorchestration.Renegotiate(peerConnection)
}

func waitSignallingStable(pc *webrtc.PeerConnection) {
	for pc.SignalingState() != webrtc.SignalingStateStable {
		time.Sleep(1 * time.Millisecond)
	}
}

func main() {

	// Initialize the logger
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	file_server := http.FileServer(http.Dir("./static"))

	mux := http.NewServeMux()

	mux.Handle("GET /", file_server)
	mux.Handle("POST /start", http.HandlerFunc(auth_handler))

	fmt.Println("Server started on http://localhost:8080")

	handler := cors.AllowAll().Handler(mux)
	http.ListenAndServe(":8080", handler)
}

// EncodeToBase64 encodes a string (like SDP) to base64
func EncodeToBase64(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// DecodeFromBase64 decodes base64 string back to plain string
func DecodeFromBase64(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}
	return string(data), nil
}
