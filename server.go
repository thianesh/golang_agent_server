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
	if _, ok := UserConnections[parsed_user_data.User.ID]; ok {
		logger.Debug(fmt.Sprintf("Existing connection found for %s, connection state: %t", UserConnections[parsed_user_data.User.ID].Email, UserConnections[parsed_user_data.User.ID].Died))

		if _, ok := CompanySFUs[parsed_user_data.CompanyID]; ok {
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RLock()
			if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; ok {
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RLock()
				if !UserConnections[parsed_user_data.User.ID].Died {
					UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)

					json.NewEncoder(w).Encode(map[string]string{
						"error": "User connection already exists. Please exit that connection to connect here.",
					})

					UserConnectionsMutex.Unlock()
					return
				}
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
			}
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RUnlock()
		}

	}
	UserConnectionsMutex.Unlock()
	// Add company SFU process to CompanuSFUs

	logger.Debug("Response from target server", "149", parsed_user_data.User.Email)
	CompanySFUsMutex.Lock()

	if _, ok := CompanySFUs[parsed_user_data.CompanyID]; !ok {
		CompanySFUs[parsed_user_data.CompanyID] = models.NewCompanySFU()
		// Start SFU processed
		// Start Boradcasting online status
		go CompanySFUs[parsed_user_data.CompanyID].StartOnlineStatusBroadcaster()
		// Start sending HeartBeat
		go CompanySFUs[parsed_user_data.CompanyID].StartHeartBeat()
		go CompanySFUs[parsed_user_data.CompanyID].Start_instant_renegotiator_caller()
	}
	CompanySFUs[parsed_user_data.CompanyID].CompanyID = parsed_user_data.CompanyID

	CompanySFUsMutex.Unlock()

	logger.Debug("Response from target server", "165", parsed_user_data.User.Email)
	// accepting the offered SDP
	peer_connection, err := mediaorchestration.CreateAnswer(
		sdp,
		&parsed_user_data,
		models.Sync_track,
		CompanySFUs[parsed_user_data.CompanyID])

	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		log.Println("Create answer error:", err)
		return
	}

	log.Println("Answer created successfully", parsed_user_data.User.Email)

	var conn *models.FullConnectionDetails
	UserConnectionsMutex.Lock()
	UserConnections[parsed_user_data.User.ID] = peer_connection
	UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.Lock()

	UserConnections[parsed_user_data.User.ID].OfferSDP = payload.SDP
	UserConnections[parsed_user_data.User.ID].AnswerSDP = UserConnections[parsed_user_data.User.ID].Webrtc.LocalDescription().SDP
	UserConnections[parsed_user_data.User.ID].Died = false
	UserConnections[parsed_user_data.User.ID].Offline = false
	UserConnections[parsed_user_data.User.ID].OfflineSince = 0

	UserConnections[parsed_user_data.User.ID].Username = parsed_user_data.User.FullName
	UserConnections[parsed_user_data.User.ID].Email = parsed_user_data.User.Email
	UserConnections[parsed_user_data.User.ID].CompanyId = parsed_user_data.CompanyID
	UserConnections[parsed_user_data.User.ID].Rooms = []*models.Room{}
	UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.Unlock()

	conn = UserConnections[parsed_user_data.User.ID]
	UserConnectionsMutex.Unlock()

	for _, room := range parsed_user_data.Rooms {
		CompanySFUsMutex.Lock()
		CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Lock()
		CompanySFUs[parsed_user_data.CompanyID].Rooms[models.RoomId(room.ID)] = &room
		CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Unlock()
		CompanySFUsMutex.Unlock()
	}
	// start all webrtc processes
	go mediaorchestration.SingleOrchestrator(conn, CompanySFUs[parsed_user_data.CompanyID], &UserConnections, &UserConnectionsMutex)
	log.Println("Started single orchestrator for user : ", parsed_user_data.User.Email)

	//setup renegotiation
	UserConnectionsMutex.Lock()
	conn.OnDataChannelBroadcaster = func(fcd *models.FullConnectionDetails) {
		logger.Debug("Data Channel added! adding negotiator.")

		// starting data sending routine
		go func(conn *models.FullConnectionDetails) {
			conn.FullConnectionDetailsRWLock.RLock()
			defer conn.FullConnectionDetailsRWLock.RUnlock()
			for msg := range conn.OutgoingDataChannel {
				if conn.DataChannel != nil && conn.DataChannel.ReadyState() == webrtc.DataChannelStateOpen {
					err := conn.DataChannel.Send(msg)
					if err != nil {
						fmt.Println("Failed to send via datachannel:", err)
					}
				}
			}
		}(fcd)

		fcd.RenegotiateMutex = sync.Mutex{}
		mediaorchestration.Initialize_renegotiation(fcd)
	}
	UserConnectionsMutex.Unlock()
	log.Println("Added data channel onDatachannel : ", parsed_user_data.User.Email)

	CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Lock()
	if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; !ok {
		CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)] = conn
	}
	CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Unlock()

	res_payload := map[string]interface{}{
		"SDP":    EncodeToBase64(conn.AnswerSDP),
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

var offer_sync_mutex = sync.Mutex{}
var offers_created = map[string]*models.FullConnectionDetails{}

func save_offer(user_id string, full_connection *models.FullConnectionDetails) {
	offer_sync_mutex.Lock()
	offers_created[user_id] = full_connection
	offer_sync_mutex.Unlock()

	// Auto-remove after 5 seconds if not answered
	time.AfterFunc(5*time.Second, func() {
		remove_offer(user_id)
	})
}

func get_saved_offer(user_id string) (*models.FullConnectionDetails, bool) {
	offer_sync_mutex.Lock()
	val, ok := offers_created[user_id]
	offer_sync_mutex.Unlock()
	return val, ok
}

func remove_offer(user_id string) {
	offer_sync_mutex.Lock()
	defer offer_sync_mutex.Unlock()

	fullConn, ok := offers_created[user_id]
	if ok {
		state := fullConn.Webrtc.ConnectionState()

		if state != webrtc.PeerConnectionStateConnected {
			_ = fullConn.Webrtc.Close()
		}

		delete(offers_created, user_id)
	}
}

func accept_answer(full_connection *models.FullConnectionDetails, answerSDP string) error {
	full_connection.FullConnectionDetailsRWLock.Lock()
	defer full_connection.FullConnectionDetailsRWLock.Unlock()
	err := full_connection.Webrtc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	})
	full_connection.OfferAccepted = false
	if err == nil {
		full_connection.OfferAccepted = true
	}
	return err
}

func offer_creator_for_user(w http.ResponseWriter, r *http.Request) {
	var payload SDPRequest

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
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

	UserConnectionsMutex.Lock()
	logger.Debug("Response from target server", "130", parsed_user_data.User.Email)
	if _, ok := UserConnections[parsed_user_data.User.ID]; ok {
		logger.Debug(fmt.Sprintf("Existing connection found for %s, connection state: %t", UserConnections[parsed_user_data.User.ID].Email, UserConnections[parsed_user_data.User.ID].Died))

		if _, ok := CompanySFUs[parsed_user_data.CompanyID]; ok {
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RLock()
			if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; ok {
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RLock()
				if !UserConnections[parsed_user_data.User.ID].Died {
					UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)

					json.NewEncoder(w).Encode(map[string]string{
						"error": "User connection already exists. Please exit that connection to connect here.",
					})

					UserConnectionsMutex.Unlock()
					return
				}
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
			}
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RUnlock()
		}

	}
	UserConnectionsMutex.Unlock()
	// Add company SFU process to CompanuSFUs

	logger.Debug("Response from target server", "149", parsed_user_data.User.Email)
	CompanySFUsMutex.Lock()

	if _, ok := CompanySFUs[parsed_user_data.CompanyID]; !ok {
		CompanySFUs[parsed_user_data.CompanyID] = models.NewCompanySFU()
		// Start SFU processed
		// Start Boradcasting online status
		go CompanySFUs[parsed_user_data.CompanyID].StartOnlineStatusBroadcaster()
		// Start sending HeartBeat
		go CompanySFUs[parsed_user_data.CompanyID].StartHeartBeat()
		go CompanySFUs[parsed_user_data.CompanyID].Start_instant_renegotiator_caller()
	}
	CompanySFUs[parsed_user_data.CompanyID].CompanyID = parsed_user_data.CompanyID

	CompanySFUsMutex.Unlock()

	logger.Debug("Response from target server", "165", parsed_user_data.User.Email)

	// creating the offered SDP
	peer_connection, err := mediaorchestration.CreateOffer(
		&parsed_user_data,
		models.Sync_track,
		CompanySFUs[parsed_user_data.CompanyID])

	if err != nil {
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		log.Println("Create answer error:", err)
		return
	}

	save_offer(parsed_user_data.User.ID, peer_connection)

	log.Println("Offer created successfully", parsed_user_data.User.Email)

	// Optional: Write the response back to the original client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	res_payload := map[string]interface{}{
		"SDP":    EncodeToBase64(peer_connection.OfferSDP),
		"status": "success",
	}

	if err := json.NewEncoder(w).Encode(res_payload); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}

}

func answer_acceptor_for_user(w http.ResponseWriter, r *http.Request) {
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
	logger.Debug("Response from target server", "534", parsed_user_data.User.Email)

	if err != nil {
		http.Error(w, "Failed to decode SDP", http.StatusInternalServerError)
		log.Println("SDP decode error:", err)
		return
	}

	UserConnectionsMutex.Lock()
	logger.Debug("Response from target server", "130", parsed_user_data.User.Email)
	if _, ok := UserConnections[parsed_user_data.User.ID]; ok {
		logger.Debug(fmt.Sprintf("Existing connection found for %s, connection state: %t", UserConnections[parsed_user_data.User.ID].Email, UserConnections[parsed_user_data.User.ID].Died))

		if _, ok := CompanySFUs[parsed_user_data.CompanyID]; ok {
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RLock()
			if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; ok {
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RLock()
				if !UserConnections[parsed_user_data.User.ID].Died {
					UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)

					json.NewEncoder(w).Encode(map[string]string{
						"error": "User connection already exists. Please exit that connection to connect here.",
					})

					UserConnectionsMutex.Unlock()
					return
				}
				UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.RUnlock()
			}
			CompanySFUs[parsed_user_data.CompanyID].CompanySFUUsersRMLock.RUnlock()
		}

	}
	UserConnectionsMutex.Unlock()
	// Add company SFU process to CompanuSFUs

	logger.Debug("Response from target server", "149", parsed_user_data.User.Email)
	CompanySFUsMutex.Lock()

	if _, ok := CompanySFUs[parsed_user_data.CompanyID]; !ok {
		CompanySFUs[parsed_user_data.CompanyID] = models.NewCompanySFU()
		// Start SFU processed
		// Start Boradcasting online status
		go CompanySFUs[parsed_user_data.CompanyID].StartOnlineStatusBroadcaster()
		// Start sending HeartBeat
		go CompanySFUs[parsed_user_data.CompanyID].StartHeartBeat()
		go CompanySFUs[parsed_user_data.CompanyID].Start_instant_renegotiator_caller()
	}
	CompanySFUs[parsed_user_data.CompanyID].CompanyID = parsed_user_data.CompanyID

	CompanySFUsMutex.Unlock()

	logger.Debug("Response from target server", "165", parsed_user_data.User.Email)
	// accepting the offered SDP
	full_connection, ok := get_saved_offer(parsed_user_data.User.ID)

	offer_sync_mutex.Lock()
	delete(offers_created, parsed_user_data.User.ID)
	offer_sync_mutex.Unlock()

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "The connection was not accepted quick or offer is not created.",
		})
		return
	}
	// setting the answer from browser/client
	err = accept_answer(full_connection, sdp)

	if err != nil {
		http.Error(w, "Failed to accept answer", http.StatusInternalServerError)
		log.Println("Create answer error:", err)
		return
	}

	log.Println("Answer accepted successfully", parsed_user_data.User.Email)

	var conn *models.FullConnectionDetails
	UserConnectionsMutex.Lock()
	UserConnections[parsed_user_data.User.ID] = full_connection
	UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.Lock()

	UserConnections[parsed_user_data.User.ID].OfferSDP = payload.SDP
	UserConnections[parsed_user_data.User.ID].AnswerSDP = UserConnections[parsed_user_data.User.ID].Webrtc.LocalDescription().SDP
	UserConnections[parsed_user_data.User.ID].Died = false
	UserConnections[parsed_user_data.User.ID].Offline = false
	UserConnections[parsed_user_data.User.ID].OfflineSince = 0

	UserConnections[parsed_user_data.User.ID].Username = parsed_user_data.User.FullName
	UserConnections[parsed_user_data.User.ID].Email = parsed_user_data.User.Email
	UserConnections[parsed_user_data.User.ID].CompanyId = parsed_user_data.CompanyID
	UserConnections[parsed_user_data.User.ID].Rooms = []*models.Room{}
	UserConnections[parsed_user_data.User.ID].FullConnectionDetailsRWLock.Unlock()

	conn = UserConnections[parsed_user_data.User.ID]
	UserConnectionsMutex.Unlock()

	for _, room := range parsed_user_data.Rooms {
		CompanySFUsMutex.Lock()
		CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Lock()
		CompanySFUs[parsed_user_data.CompanyID].Rooms[models.RoomId(room.ID)] = &room
		CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Unlock()
		CompanySFUsMutex.Unlock()
	}
	// start all webrtc processes
	go mediaorchestration.SingleOrchestrator(conn, CompanySFUs[parsed_user_data.CompanyID], &UserConnections, &UserConnectionsMutex)
	log.Println("Started single orchestrator for user : ", parsed_user_data.User.Email)

	//setup renegotiation
	UserConnectionsMutex.Lock()
	conn.OnDataChannelBroadcaster = func(fcd *models.FullConnectionDetails) {
		logger.Debug("Data Channel added! adding negotiator.")

		// starting data sending routine
		go func(conn *models.FullConnectionDetails) {
			conn.FullConnectionDetailsRWLock.RLock()
			defer conn.FullConnectionDetailsRWLock.RUnlock()
			for msg := range conn.OutgoingDataChannel {
				if conn.DataChannel != nil && conn.DataChannel.ReadyState() == webrtc.DataChannelStateOpen {
					err := conn.DataChannel.Send(msg)
					if err != nil {
						fmt.Println("Failed to send via datachannel:", err)
					}
				}
			}
		}(fcd)

		fcd.RenegotiateMutex = sync.Mutex{}
		mediaorchestration.Initialize_renegotiation(fcd)
	}
	UserConnectionsMutex.Unlock()
	log.Println("Added data channel onDatachannel : ", parsed_user_data.User.Email)

	CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Lock()
	if _, ok := CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)]; !ok {
		CompanySFUs[parsed_user_data.CompanyID].Users[models.UserId(parsed_user_data.User.ID)] = conn
	}
	CompanySFUs[parsed_user_data.CompanyID].CompanySFUsMutex.Unlock()

	res_payload := map[string]interface{}{
		"status": "success",
	}
	log.Println("Connection Established ", parsed_user_data.User.Email)

	if err := json.NewEncoder(w).Encode(res_payload); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}

	// testing re-negotiation
	// go add_track(UserConnections[parsed_user_data.User.ID])

	fmt.Println("All-Set nothing pending. connection established 684", parsed_user_data.User.Email)
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

	mux.Handle("POST /start-offer", http.HandlerFunc(offer_creator_for_user))
	mux.Handle("POST /set-answer", http.HandlerFunc(answer_acceptor_for_user))

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
