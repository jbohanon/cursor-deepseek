package backend

import (
	"context"
	"net/http"

	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
)

// Backend defines the interface that all LLM backends must implement
type Backend interface {
	// Name returns the name of the backend
	Name() string

	// HandleChatCompletion handles a chat completion request. This method must capture and
	// return to the client all errors on the provided writer.
	HandleChatCompletion(ctx context.Context, w http.ResponseWriter, r *http.Request, req *openai.ChatCompletionRequest)

	// ListModels returns the list of available models
	ListModels(ctx context.Context) ([]openai.Model, error)

	// ValidateAPIKey validates the provided API key
	ValidateAPIKey(apiKey string) bool
}
