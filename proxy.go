package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/deepseek"
	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
)

const (
	deepseekEndpoint     = "https://api.deepseek.com"
	deepseekBetaEndpoint = "https://api.deepseek.com/beta"
	deepseekChatModel    = "deepseek-chat"
	deepseekCoderModel   = "deepseek-coder"
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

	opts := deepseek.Options{
		Endpoint: deepseekEndpoint,
		Model:    deepseekChatModel,
	}

	// Get DeepSeek API key
	opts.ApiKey = os.Getenv("DEEPSEEK_API_KEY")
	if opts.ApiKey == "" {
		log.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	// Parse command line arguments
	modelFlag := "chat" // default value
	for i, arg := range os.Args {
		if arg == "-model" && i+1 < len(os.Args) {
			modelFlag = os.Args[i+1]
		}
	}

	// Configure the active endpoint and model based on the flag
	switch modelFlag {
	case "coder":
		opts.Endpoint = deepseekBetaEndpoint
		opts.Model = deepseekCoderModel
	default:
		log.Printf("Invalid model specified: %s. Using default chat model.", modelFlag)
	}

	log.Printf("Initialized with model: %s using endpoint: %s", opts.Model, opts.Endpoint)

	h := handler{
		b:      deepseek.NewDeepseekBackend(opts),
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

func enableCors(w http.ResponseWriter) {
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
