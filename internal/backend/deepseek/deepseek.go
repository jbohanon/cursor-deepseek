package deepseek

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
	defaultEndpoint     = "https://api.deepseek.com"
	defaultBetaEndpoint = "https://api.deepseek.com/beta"
	defaultChatModel    = "deepseek-chat"
	defaultCoderModel   = "deepseek-coder"
)

// Options configures the DeepSeek backend
type Options struct {
	APIKey   string
	Endpoint string
}

// Backend implements the backend.Backend interface for DeepSeek
type Backend struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

// New creates a new DeepSeek backend
func New(opts Options) *Backend {
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	return &Backend{
		apiKey:   opts.APIKey,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (b *Backend) Name() string {
	return "deepseek"
}

func (b *Backend) ValidateAPIKey(apiKey string) bool {
	return util.SecureCompareString(apiKey, b.apiKey)
}

// DeepSeekRequest represents a request to the DeepSeek API
type DeepSeekRequest struct {
	Model       string            `json:"model"`
	Messages    []backend.Message `json:"messages"`
	Stream      bool              `json:"stream"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

func (b *Backend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, req *backend.ChatRequest) error {
	requestID := reqcontext.GetRequestID(ctx)

	// Validate model
	if req.Model != defaultChatModel && req.Model != defaultCoderModel {
		log.Printf("[%s] Invalid model: %s", requestID, req.Model)
		return fmt.Errorf("invalid model: %s", req.Model)
	}

	// Convert request to DeepSeek format
	deepseekReq := DeepSeekRequest{
		Model:    req.Model,
		Messages: convertMessages(req.Messages),
		Stream:   req.Stream,
	}

	if req.Temperature != nil {
		deepseekReq.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		deepseekReq.MaxTokens = *req.MaxTokens
	}

	// Create request body
	body, err := json.Marshal(deepseekReq)
	if err != nil {
		log.Printf("[%s] Error marshaling request: %v", requestID, err)
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", b.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		log.Printf("[%s] Error creating request: %v", requestID, err)
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+b.apiKey)

	// Send request
	log.Printf("[%s] Sending request to DeepSeek API", requestID)
	resp, err := b.client.Do(httpReq)
	if err != nil {
		log.Printf("[%s] Error sending request: %v", requestID, err)
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] DeepSeek API error (%d): %s", requestID, resp.StatusCode, string(body))
		return fmt.Errorf("DeepSeek API error (%d): %s", resp.StatusCode, string(body))
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
			ID:      defaultChatModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "deepseek",
		},
		{
			ID:      defaultCoderModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "deepseek",
		},
	}, nil
}

// Helper functions

func convertMessages(messages []backend.Message) []backend.Message {
	converted := make([]backend.Message, len(messages))
	for i, msg := range messages {
		converted[i] = msg

		// Convert function role to tool role
		if msg.Role == "function" {
			converted[i].Role = "tool"
		}

		// Handle tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCalls := make([]backend.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = backend.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
			converted[i].ToolCalls = toolCalls
		}
	}
	return converted
}

func convertToolChoice(choice interface{}) string {
	if choice == nil {
		return ""
	}

	switch v := choice.(type) {
	case string:
		if v == "auto" || v == "none" {
			return v
		}
	case map[string]interface{}:
		if v["type"] == "function" {
			return "auto" // DeepSeek doesn't support specific function selection
		}
	}

	return ""
}
