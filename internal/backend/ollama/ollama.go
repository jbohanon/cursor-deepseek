package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	reqcontext "github.com/danilofalcao/cursor-deepseek/internal/context"
)

const (
	defaultEndpoint = "http://localhost:11434/api"
	defaultModel    = "llama2"
)

// Options configures the Ollama backend
type Options struct {
	Endpoint     string
	DefaultModel string
}

// Backend implements the backend.Backend interface for Ollama
type Backend struct {
	endpoint     string
	defaultModel string
	client       *http.Client
}

// New creates a new Ollama backend
func New(opts Options) *Backend {
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	model := opts.DefaultModel
	if model == "" {
		model = defaultModel
	}

	return &Backend{
		endpoint:     endpoint,
		defaultModel: model,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (b *Backend) Name() string {
	return "ollama"
}

func (b *Backend) ValidateAPIKey(apiKey string) bool {
	return true // Ollama doesn't use API keys
}

// OllamaRequest represents a request to the Ollama API
type OllamaRequest struct {
	Model       string            `json:"model"`
	Messages    []backend.Message `json:"messages"`
	Stream      bool              `json:"stream"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

// OllamaResponse represents a response from the Ollama API
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

func (b *Backend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, req *backend.ChatRequest) error {
	requestID := reqcontext.GetRequestID(ctx)

	// Convert request to Ollama format
	ollamaReq := OllamaRequest{
		Model:    b.defaultModel,
		Messages: req.Messages,
		Stream:   req.Stream,
	}

	if req.Temperature != nil {
		ollamaReq.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		ollamaReq.MaxTokens = *req.MaxTokens
	}

	// Create request body
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		log.Printf("[%s] Error marshaling request: %v", requestID, err)
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", b.endpoint+"/chat", bytes.NewReader(body))
	if err != nil {
		log.Printf("[%s] Error creating request: %v", requestID, err)
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	log.Printf("[%s] Sending request to Ollama API", requestID)
	resp, err := b.client.Do(httpReq)
	if err != nil {
		log.Printf("[%s] Error sending request: %v", requestID, err)
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] Ollama API error (%d): %s", requestID, resp.StatusCode, string(body))
		return fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(body))
	}

	// Handle streaming response
	if req.Stream {
		return b.handleStreamingResponse(ctx, w, resp, b.defaultModel)
	}

	// Handle regular response
	return b.handleRegularResponse(ctx, w, resp, b.defaultModel)
}

func (b *Backend) ListModels(ctx context.Context) ([]backend.Model, error) {
	return []backend.Model{
		{
			ID:      b.defaultModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "ollama",
		},
	}, nil
}
