package main

import (
	"fmt"
	"log"
	"net/http"
)

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


func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", http_handler)

	fmt.Println("Server started on http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}