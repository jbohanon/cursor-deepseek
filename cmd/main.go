package main

import (
	"log"
	"os"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/deepseek"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/ollama"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/openrouter"
	"github.com/danilofalcao/cursor-deepseek/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}

	// Initialize backend based on environment variables
	var b backend.Backend
	var err error

	switch {
	case os.Getenv("DEEPSEEK_API_KEY") != "":
		b = deepseek.New(deepseek.Options{
			APIKey:   os.Getenv("DEEPSEEK_API_KEY"),
			Endpoint: os.Getenv("DEEPSEEK_ENDPOINT"),
		})
		log.Printf("Using DeepSeek backend")

	case os.Getenv("OPENROUTER_API_KEY") != "":
		b = openrouter.New(openrouter.Options{
			APIKey: os.Getenv("OPENROUTER_API_KEY"),
		})
		log.Printf("Using OpenRouter backend")

	case os.Getenv("OLLAMA_API_ENDPOINT") != "":
		b = ollama.New(ollama.Options{
			Endpoint:     os.Getenv("OLLAMA_API_ENDPOINT"),
			DefaultModel: getEnvOrDefault("DEFAULT_MODEL", "llama2"),
			// TODO: Add support for model mapping
		})
		log.Printf("Using Ollama backend")

	default:
		log.Fatal("No backend configured. Please set one of: DEEPSEEK_API_KEY, OPENROUTER_API_KEY, or OLLAMA_API_ENDPOINT")
	}

	// Create and start server
	srv, err := server.New(server.Options{
		Port:    getEnvOrDefault("PORT", "9000"),
		Backend: b,
	})
	if err != nil {
		log.Fatalf("Error creating server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
