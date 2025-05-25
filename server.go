package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"thianesh/web_server/models"
	// "thianesh/web_server/server_utils"
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

func http_handler (w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
}

func auth_handler(w http.ResponseWriter, r *http.Request) {
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
	
	logger.Debug("Response from target server:",parsed_user_data)
	// Optional: Write the response back to the original client
	w.WriteHeader(resp.StatusCode)
	w.Header().Set("Content-Type", "application/json")
	// w.Write(body)
	if err := json.NewEncoder(w).Encode(parsed_user_data); err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}
}

func main() {
	
	// Initialize the logger
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	file_server := http.FileServer(http.Dir("./static"))

	mux := http.NewServeMux()
	handler := http.HandlerFunc(http_handler)
	
	mux.Handle("GET /", file_server)
	mux.Handle("/", loggingMiddleware(handler))
	mux.Handle("GET /start", http.HandlerFunc(auth_handler) )

	fmt.Println("Server started on http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}