package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/openrouter"
	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
)

type handler struct {
	b      backend.Backend
	apikey string
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}

	opts := openrouter.Options{
		Endpoint: "https://openrouter.ai/api/v1",
		Model:    "deepseek/deepseek-chat",
	}
	// Get OpenRouter API key
	opts.ApiKey = os.Getenv("OPENROUTER_API_KEY")
	if opts.ApiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	h := handler{
		b:      openrouter.NewOpenrouterBackend(opts),
		apikey: opts.ApiKey,
	}

	server := &http.Server{
		Addr:    ":9000",
		Handler: http.HandlerFunc(h.proxyHandler),
	}

	// Enable HTTP/2 support
	http2.ConfigureServer(server, &http2.Server{})

	log.Printf("Starting proxy server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func enableCors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func (h handler) proxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s", r.Method, r.URL.Path)

	enableCors(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Validate API key
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("Missing or invalid Authorization header")
		http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}

	userAPIKey := strings.TrimPrefix(authHeader, "Bearer ")
	if userAPIKey != h.apikey {
		log.Printf("Invalid API key provided")
		http.Error(w, "Invalid API key", http.StatusForbidden)
		return
	}

	switch r.URL.Path {
	case "/v1/chat/completions":
		h.b.HandleChatCompletions(w, r)
	case "/v1/models":
		h.b.HandleModelsRequest(w)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}

}
