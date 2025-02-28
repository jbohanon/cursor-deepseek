package openrouter

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
	"github.com/danilofalcao/cursor-deepseek/internal/backend/util"
	reqcontext "github.com/danilofalcao/cursor-deepseek/internal/context"
)

const (
	defaultEndpoint = "https://openrouter.ai/api/v1"
	defaultModel    = "deepseek/deepseek-chat"
)

// Options configures the OpenRouter backend
type Options struct {
	APIKey string
}

// Backend implements the backend.Backend interface for OpenRouter
type Backend struct {
	apiKey string
	client *http.Client
}

// New creates a new OpenRouter backend
func New(opts Options) *Backend {
	return &Backend{
		apiKey: opts.APIKey,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (b *Backend) Name() string {
	return "openrouter"
}

func (b *Backend) ValidateAPIKey(apiKey string) bool {
	return util.SecureCompareString(apiKey, b.apiKey)
}

func (b *Backend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, req *backend.ChatRequest) error {
	requestID := reqcontext.GetRequestID(ctx)

	// Create request body (OpenRouter uses OpenAI-compatible format)
	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("[%s] Error marshaling request: %v", requestID, err)
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", defaultEndpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		log.Printf("[%s] Error creating request: %v", requestID, err)
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+b.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/danilofalcao/cursor-deepseek")
	httpReq.Header.Set("X-Title", "Cursor DeepSeek")

	// Send request
	log.Printf("[%s] Sending request to OpenRouter API", requestID)
	resp, err := b.client.Do(httpReq)
	if err != nil {
		log.Printf("[%s] Error sending request: %v", requestID, err)
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] OpenRouter API error (%d): %s", requestID, resp.StatusCode, string(body))
		return fmt.Errorf("OpenRouter API error (%d): %s", resp.StatusCode, string(body))
	}

	// Handle streaming response
	if req.Stream {
		return b.handleStreamingResponse(ctx, w, resp)
	}

	// Handle regular response
	return b.handleRegularResponse(ctx, w, resp)
}

func (b *Backend) ListModels(ctx context.Context) ([]backend.Model, error) {
	return []backend.Model{
		{
			ID:      defaultModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "openrouter",
		},
	}, nil
}
