package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/server/logger"
	"github.com/danilofalcao/cursor-deepseek/internal/server/middleware"
	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
	"github.com/pkg/errors"
	"golang.org/x/net/http2"
)

// Options configures the server
type Options struct {
	Port     string
	Backend  backend.Backend
	LogLevel string
	ApiKey   string
	Timeout  string
	ExitCh   chan string
}

// Server represents the API server
type Server struct {
	ctx     context.Context
	port    string
	backend backend.Backend
	apikey  string
	timeout time.Duration
	exitCh  chan string
}

// New creates a new server instance
func New(ctx context.Context, opts Options) (*Server, error) {
	// set up the server's logger
	lgr := logger.New(
		ctx,
		"server",
		logger.LevelFromString(opts.LogLevel),
		opts.ExitCh,
	)
	ctx = logutils.ContextWithLogger(ctx, lgr)

	timeout, err := time.ParseDuration(opts.Timeout)
	if err != nil {
		timeout = time.Second * 30
	}

	if opts.Port == "" {
		return nil, fmt.Errorf("port is required")
	}
	if opts.Backend == nil {
		return nil, fmt.Errorf("backend is required")
	}

	return &Server{
		ctx:     ctx,
		port:    opts.Port,
		backend: opts.Backend,
		apikey:  opts.ApiKey,
		timeout: timeout,
		exitCh:  opts.ExitCh,
	}, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/models", s.handleModels)

	// Create server with middleware
	handler := middleware.Wrap(s.ctx, mux, middleware.Params{
		ApiKey:         s.apikey,
		AuthValidation: s.backend.ValidateAPIKey,
		Timeout:        s.timeout,
	})

	srv := &http.Server{
		Addr:        ":" + s.port,
		Handler:     handler,
		BaseContext: func(l net.Listener) context.Context { return s.ctx },
	}

	// Enable HTTP/2 support
	if err := http2.ConfigureServer(srv, nil); err != nil {
		return fmt.Errorf("error configuring HTTP/2: %w", err)
	}

	logutils.FromContext(s.ctx).Infof(s.ctx, "Starting server on port %s", s.port)
	return srv.ListenAndServe()
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lgr := logutils.FromContext(ctx)
	// Validate request method
	if r.Method != "POST" {
		lgr.Infof(ctx, "Invalid method %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		err = errors.Wrap(err, "error parsing request")
		lgr.Error(ctx, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle request
	s.backend.HandleChatCompletion(r.Context(), w, r, &req)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lgr := logutils.FromContext(ctx)
	// Validate request method
	if r.Method != "GET" {
		lgr.Infof(ctx, "Invalid method %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get models
	models, err := s.backend.ListModels(ctx)
	if err != nil {
		err = errors.Wrap(err, "error listing models")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   models,
	}); err != nil {
		err = errors.Wrap(err, "error encoding response")
		lgr.Error(ctx, err.Error())
	}
}
