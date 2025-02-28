package backend

import (
	"context"
	"net/http"
)

// Message represents a chat message
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Function represents a callable function
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Tool represents an available tool
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// ToolCall represents a call to a tool
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatRequest represents a request to the chat API
type ChatRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	Stream      bool        `json:"stream"`
	Functions   []Function  `json:"functions,omitempty"`
	Tools       []Tool      `json:"tools,omitempty"`
	ToolChoice  interface{} `json:"tool_choice,omitempty"`
	Temperature *float64    `json:"temperature,omitempty"`
	MaxTokens   *int        `json:"max_tokens,omitempty"`
}

// Model represents an available model
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// Backend defines the interface that all LLM backends must implement
type Backend interface {
	// Name returns the name of the backend
	Name() string

	// HandleChatCompletion handles a chat completion request
	HandleChatCompletion(ctx context.Context, w http.ResponseWriter, req *ChatRequest) error

	// ListModels returns the list of available models
	ListModels(ctx context.Context) ([]Model, error)

	// ValidateAPIKey validates the provided API key
	ValidateAPIKey(apiKey string) bool
}

// NewBackendFunc is a function that creates a new backend instance
type NewBackendFunc func(opts interface{}) (Backend, error)
