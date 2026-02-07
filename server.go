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
	"sync"
	"thianesh/web_server/hystersisloadmanagement"
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

var UserConnections = make(map[string]*models.FullConnectionDetails)
var UserConnectionsMutex = sync.Mutex{}

var CompanySFUs = make(map[string]*models.CompanySFU)
var CompanySFUsMutex = sync.RWMutex{}

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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	// Now we have the SDP and user details, we can accept the connection
	sdp, err := DecodeFromBase64(payload.SDP)
	logger.Debug("Response from target server", "121", parsed_user_data.User.Email)

	if err != nil {
		http.Error(w, "Failed to decode SDP", http.StatusInternalServerError)
		log.Println("SDP decode error:", err)
		return
	}

	UserConnectionsMutex.Lock()
	logger.Debug("Response from target server", "130", parsed_user_data.User.Email)
	if existingConn, ok := UserConnections[parsed_user_data.User.ID]; ok {
		logger.Debug(fmt.Sprintf("Existing connection found for %s, connection state: %t", existingConn.Email, existingConn.Died))

		CompanySFUsMutex.RLock()
		companySFU, sfuExists := CompanySFUs[parsed_user_data.CompanyID]
		CompanySFUsMutex.RUnlock()

		if sfuExists {
			companySFU.CompanySFUUsersRMLock.RLock()
			_, userInSFU := companySFU.Users[models.UserId(parsed_user_data.User.ID)]
			companySFU.CompanySFUUsersRMLock.RUnlock()

			if userInSFU {
				existingConn.FullConnectionDetailsRWLock.RLock()
				isDead := existingConn.Died
				existingConn.FullConnectionDetailsRWLock.RUnlock()

				if !isDead {
					UserConnectionsMutex.Unlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{
						"error": "User connection already exists. Please exit that connection to connect here.",
					})
					return
				}
			}
		}
	}
	UserConnectionsMutex.Unlock()
	// Add company SFU process to CompanuSFUs

	logger.Debug("Response from target server", "149", parsed_user_data.User.Email)
	CompanySFUsMutex.Lock()

	companySFU, sfuExists := CompanySFUs[parsed_user_data.CompanyID]
	if !sfuExists {
		companySFU = models.NewCompanySFU()
		CompanySFUs[parsed_user_data.CompanyID] = companySFU
		// Start SFU processed
		// Start Boradcasting online status
		go companySFU.StartOnlineStatusBroadcaster()
		// Start sending HeartBeat
		go companySFU.StartHeartBeat()
		go companySFU.Start_instant_renegotiator_caller()
	}
	companySFU.CompanyID = parsed_user_data.CompanyID

	CompanySFUsMutex.Unlock()

	logger.Debug("Response from target server", "165", parsed_user_data.User.Email)

	// Get companySFU safely
	CompanySFUsMutex.RLock()
	companySFU = CompanySFUs[parsed_user_data.CompanyID]
	CompanySFUsMutex.RUnlock()

	// accepting the offered SDP
	peer_connection, err := mediaorchestration.CreateAnswer(
		sdp,
		&parsed_user_data,
		models.Sync_track,
		companySFU)

	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		log.Println("Create answer error:", err)
		return
	}

	log.Println("Answer created successfully", parsed_user_data.User.Email)

	// Set up connection details with proper locking
	peer_connection.FullConnectionDetailsRWLock.Lock()
	peer_connection.OfferSDP = payload.SDP
	peer_connection.AnswerSDP = peer_connection.Webrtc.LocalDescription().SDP
	peer_connection.Died = false
	peer_connection.Offline = false
	peer_connection.OfflineSince = 0
	peer_connection.Username = parsed_user_data.User.FullName
	peer_connection.Email = parsed_user_data.User.Email
	peer_connection.CompanyId = parsed_user_data.CompanyID
	peer_connection.Rooms = []*models.Room{}
	peer_connection.FullConnectionDetailsRWLock.Unlock()

	// Store in UserConnections map
	UserConnectionsMutex.Lock()
	UserConnections[parsed_user_data.User.ID] = peer_connection
	UserConnectionsMutex.Unlock()

	conn := peer_connection

	// Add rooms to companySFU
	companySFU.CompanySFUsMutex.Lock()
	for _, room := range parsed_user_data.Rooms {
		roomCopy := room // Create a copy to avoid pointer issues
		companySFU.Rooms[models.RoomId(room.ID)] = &roomCopy
	}
	companySFU.CompanySFUsMutex.Unlock()
	// start all webrtc processes
	go mediaorchestration.SingleOrchestrator(conn, companySFU, &UserConnections, &UserConnectionsMutex)
	log.Println("Started single orchestrator for user : ", parsed_user_data.User.Email)

	//setup renegotiation
	conn.FullConnectionDetailsRWLock.Lock()
	conn.OnDataChannelBroadcaster = func(fcd *models.FullConnectionDetails) {
		logger.Debug("Data Channel added! adding negotiator.")

		// starting data sending routine with proper cancellation
		go func(c *models.FullConnectionDetails) {
			for {
				c.FullConnectionDetailsRWLock.RLock()
				outChan := c.OutgoingDataChannel
				c.FullConnectionDetailsRWLock.RUnlock()

				if outChan == nil {
					return
				}

				msg, ok := <-outChan
				if !ok {
					return // channel closed
				}

				c.FullConnectionDetailsRWLock.RLock()
				dc := c.DataChannel
				c.FullConnectionDetailsRWLock.RUnlock()

				if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
					if err := dc.Send(msg); err != nil {
						fmt.Println("Failed to send via datachannel:", err)
					}
				}
			}
		}(fcd)

		fcd.RenegotiateMutex = sync.Mutex{}
		mediaorchestration.Initialize_renegotiation(fcd)
	}
	conn.FullConnectionDetailsRWLock.Unlock()
	log.Println("Added data channel onDatachannel : ", parsed_user_data.User.Email)

	companySFU.CompanySFUsMutex.Lock()
	if _, ok := companySFU.Users[models.UserId(parsed_user_data.User.ID)]; !ok {
		companySFU.Users[models.UserId(parsed_user_data.User.ID)] = conn
	}
	companySFU.CompanySFUsMutex.Unlock()

	conn.FullConnectionDetailsRWLock.RLock()
	answerSDP := conn.AnswerSDP
	conn.FullConnectionDetailsRWLock.RUnlock()

	res_payload := map[string]interface{}{
		"SDP":    EncodeToBase64(answerSDP),
		"status": "success",
	}
	log.Println("Sending Answer SDP to user ", parsed_user_data.User.Email)

	if err := json.NewEncoder(w).Encode(res_payload); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}

	// testing re-negotiation
	// go add_track(UserConnections[parsed_user_data.User.ID])

	fmt.Println("All-Set nothing pending.", parsed_user_data.User.Email)
}

func send_utility(w http.ResponseWriter, r *http.Request, usage *hystersisloadmanagement.SystemConsumption, mu *sync.RWMutex) {
	// Example authentication handler
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header missing", http.StatusUnauthorized)
		return
	}

	mu.RLock()
	res_payload := map[string]interface{}{
		"cpu": usage.CPUPercent,
		"ram": usage.RAMPercent,
	}
	mu.RUnlock()

	if err := json.NewEncoder(w).Encode(res_payload); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}
}

func WaitSignallingStable(pc *webrtc.PeerConnection) {
	for pc.SignalingState() != webrtc.SignalingStateStable {
		time.Sleep(1 * time.Millisecond)
	}
}

var usage hystersisloadmanagement.SystemConsumption
var mu sync.RWMutex

func main() {

	go hystersisloadmanagement.StartSystemMonitorAndSendAnalytics(&usage, &mu, &CompanySFUs, &CompanySFUsMutex)
	// Initialize the logger
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	file_server := http.FileServer(http.Dir("./static"))

	mux := http.NewServeMux()

	mux.Handle("GET /", file_server)
	mux.Handle("POST /start", http.HandlerFunc(auth_handler))
	mux.Handle("GET /health-check", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send_utility(w, r, &usage, &mu)
	}))

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
