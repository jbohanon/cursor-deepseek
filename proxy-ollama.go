package main

import (
	"log"
	"net/http"
	"os"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/ollama"
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
	log.Printf("Variant: OLLAMA")
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}

	opts := ollama.Options{
		Endpoint: "http://localhost:11434/api",
		Model:    "deepseek-r1:14b",
	}
	// Get custom Ollama endpoint if specified
	customEndpoint := os.Getenv("OLLAMA_API_ENDPOINT")
	if customEndpoint != "" {
		opts.Endpoint = customEndpoint
	}

	// Get custom Ollama endpoint if specified
	modelenv := os.Getenv("DEFAULT_MODEL")
	if modelenv != "" {
		opts.Model = modelenv
	} else {
		//no environment set so check for command line argument
		for i, arg := range os.Args {
			if arg == "-model" && i+1 < len(os.Args) {
				opts.Model = os.Args[i+1]
			}
		}
	}

	h := handler{
		b:      ollama.NewOllamaBackend(opts),
		apikey: "",
	}

	server := &http.Server{
		Addr:    ":9000",
		Handler: http.HandlerFunc(h.proxyHandler),
	}

	// Enable HTTP/2 support
	http2.ConfigureServer(server, &http2.Server{})

	log.Printf("Starting Ollama proxy server on %s", server.Addr)
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

	switch r.URL.Path {
	case "/v1/chat/completions":
		h.b.HandleChatCompletions(w, r)
	case "/v1/models":
		h.b.HandleModelsRequest(w)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
