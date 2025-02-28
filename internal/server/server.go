package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/context"
	"github.com/danilofalcao/cursor-deepseek/internal/middleware"
	"golang.org/x/net/http2"
)

// Options configures the server
type Options struct {
	Port    string
	Backend backend.Backend
}

// Server represents the API server
type Server struct {
	port    string
	backend backend.Backend
}

// New creates a new server instance
func New(opts Options) (*Server, error) {
	if opts.Port == "" {
		return nil, fmt.Errorf("port is required")
	}
	if opts.Backend == nil {
		return nil, fmt.Errorf("backend is required")
	}

	return &Server{
		port:    opts.Port,
		backend: opts.Backend,
	}, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/models", s.handleModels)

	// Create server with middleware
	handler := middleware.LoggingMiddleware(mux)

	srv := &http.Server{
		Addr:    ":" + s.port,
		Handler: handler,
	}

	// Enable HTTP/2 support
	if err := http2.ConfigureServer(srv, nil); err != nil {
		return fmt.Errorf("error configuring HTTP/2: %w", err)
	}

	log.Printf("Starting server on port %s", s.port)
	return srv.ListenAndServe()
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	requestID := context.GetRequestID(r.Context())

	// Enable CORS
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	// Validate request method
	if r.Method != "POST" {
		log.Printf("[%s] Invalid method: %s", requestID, r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate API key
	// TODO: add support for API key in custom header
	apiKey := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !s.backend.ValidateAPIKey(apiKey) {
		log.Printf("[%s] Invalid API key", requestID)
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req backend.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[%s] Error parsing request: %v", requestID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle request
	if err := s.backend.HandleChatCompletion(r.Context(), w, &req); err != nil {
		log.Printf("[%s] Error handling chat completion: %v", requestID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	requestID := context.GetRequestID(r.Context())

	// Enable CORS
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	// Validate request method
	if r.Method != "GET" {
		log.Printf("[%s] Invalid method: %s", requestID, r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get models
	models, err := s.backend.ListModels(r.Context())
	if err != nil {
		log.Printf("[%s] Error listing models: %v", requestID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   models,
	}); err != nil {
		log.Printf("[%s] Error encoding response: %v", requestID, err)
	}
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
}
