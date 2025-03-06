package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"golang.org/x/net/http2"
)

// TODO: Implement the DeepSeek backend as a backend.Backend

var _ backend.Backend = &deepseekBackend{}

type deepseekBackend struct {
	endpoint string
	model    string
	apikey   string
}

type Options struct {
	Endpoint string
	Model    string
	ApiKey   string
}

func NewDeepseekBackend(opts Options) backend.Backend {
	return &deepseekBackend{
		endpoint: opts.Endpoint,
		model:    opts.Model,
	}
}

func (b *deepseekBackend) HandleModelsRequest(w http.ResponseWriter) {
	log.Printf("Handling models request")

	// Get the requested model from the query parameters
	response := openai.ModelsResponse{
		Object: "list",
		Data: []openai.Model{
			{
				ID:      b.model,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "deepseek",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	log.Printf("Models response sent successfully")
}
func (b *deepseekBackend) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Read and log request body for debugging
	var chatReq openai.ChatCompletionRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if err := json.Unmarshal(body, &chatReq); err != nil {
		log.Printf("Error parsing request JSON: %v", err)
		log.Printf("Raw request body: %s", string(body))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Parsed request: %+v", chatReq)

	// Restore the body for further reading
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	log.Printf("Request body: %s", string(body))

	// Parse the request to check for streaming - reuse existing chatReq
	if err := json.Unmarshal(body, &chatReq); err != nil {
		log.Printf("Error parsing request JSON: %v", err)
		http.Error(w, "Error parsing request", http.StatusBadRequest)
		return
	}

	log.Printf("Requested model: %s", chatReq.Model)

	// Store original model name for response
	originalModel := chatReq.Model

	// Convert to deepseek-chat internally
	chatReq.Model = b.model
	log.Printf("Model converted to: %s (original: %s)", b.model, originalModel)

	// Convert to DeepSeek request format
	deepseekReq := deepseek.Request{
		Model:    b.model,
		Messages: convertMessages(chatReq.Messages),
		Stream:   chatReq.Stream,
	}

	// Copy optional parameters if present
	if chatReq.Temperature != nil {
		deepseekReq.Temperature = *chatReq.Temperature
	}
	if chatReq.MaxTokens != nil {
		deepseekReq.MaxTokens = *chatReq.MaxTokens
	}

	// Handle tools/functions
	if len(chatReq.Tools) > 0 {
		deepseekReq.Tools = convertTools(chatReq.Tools)
		if tc := convertToolChoice(chatReq.ToolChoice); tc != "" {
			deepseekReq.ToolChoice = tc
		}
	} else if len(chatReq.Functions) > 0 {
		// Convert functions to tools format
		tools := make([]deepseek.Tool, len(chatReq.Functions))
		for i, fn := range chatReq.Functions {
			tools[i] = deepseek.Tool{
				Type: "function",
				Function: deepseek.Function{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  fn.Parameters,
				},
			}
		}
		deepseekReq.Tools = tools

		// Convert tool_choice if present
		if tc := convertToolChoice(chatReq.ToolChoice); tc != "" {
			deepseekReq.ToolChoice = tc
		}
	}

	// Create new request body
	modifiedBody, err := json.Marshal(deepseekReq)
	if err != nil {
		log.Printf("Error creating modified request body: %v", err)
		http.Error(w, "Error creating modified request", http.StatusInternalServerError)
		return
	}

	log.Printf("Modified request body: %s", string(modifiedBody))

	// Create the proxy request to DeepSeek
	targetURL := b.endpoint + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("Forwarding to: %s", targetURL)
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		log.Printf("Error creating proxy request: %v", err)
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	copyHeaders(proxyReq.Header, r.Header)

	// Set DeepSeek API key and content type
	proxyReq.Header.Set("Authorization", "Bearer "+b.apikey)
	proxyReq.Header.Set("Content-Type", "application/json")
	if chatReq.Stream {
		proxyReq.Header.Set("Accept", "text/event-stream")
	}

	// Add Accept-Language header from request
	if acceptLanguage := r.Header.Get("Accept-Language"); acceptLanguage != "" {
		proxyReq.Header.Set("Accept-Language", acceptLanguage)
	}

	log.Printf("Proxy request headers: %v", proxyReq.Header)

	// Create a custom client with keepalive
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS:   nil,
		},
		Timeout: 5 * time.Minute,
	}

	// Send the request
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Error forwarding request: %v", err)
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("DeepSeek response status: %d", resp.StatusCode)
	log.Printf("DeepSeek response headers: %v", resp.Header)

	// Handle error responses
	if resp.StatusCode >= 400 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading error response: %v", err)
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}
		log.Printf("DeepSeek error response: %s", string(respBody))

		// Forward the error response
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	// Handle streaming response
	if chatReq.Stream {
		handleStreamingResponse(w, r, resp, originalModel)
		return
	}

	// Handle regular response
	handleRegularResponse(w, resp, originalModel)
}
func handleStreamingResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, originalModel string) {
	log.Printf("Starting streaming response handling with model: %s", originalModel)
	log.Printf("Response status: %d", resp.StatusCode)
	log.Printf("Response headers: %+v", resp.Header)

	// Set headers for streaming response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	// Create a buffered reader for the response body
	reader := bufio.NewReader(resp.Body)

	// Create a context with cancel for cleanup
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Start a goroutine to send heartbeats
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Send a heartbeat comment
				if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
					log.Printf("Error sending heartbeat: %v", err)
					cancel()
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled, ending stream")
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					continue
				}
				log.Printf("Error reading stream: %v", err)
				cancel()
				return
			}

			// Skip empty lines
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}

			// Write the line to the response
			if _, err := w.Write(line); err != nil {
				log.Printf("Error writing to response: %v", err)
				cancel()
				return
			}

			// Flush the response writer
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			} else {
				log.Printf("Warning: ResponseWriter does not support Flush")
			}
		}
	}
}

func handleRegularResponse(w http.ResponseWriter, resp *http.Response, originalModel string) {
	log.Printf("Handling regular (non-streaming) response")
	log.Printf("Response status: %d", resp.StatusCode)
	log.Printf("Response headers: %+v", resp.Header)

	// Read and log response body
	body, err := readResponse(resp)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		http.Error(w, "Error reading response from upstream", http.StatusInternalServerError)
		return
	}

	log.Printf("Original response body: %s", string(body))

	// Parse the DeepSeek response
	var deepseekResp deepseek.Response

	if err := json.Unmarshal(body, &deepseekResp); err != nil {
		log.Printf("Error parsing DeepSeek response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Convert to OpenAI format
	openAIResp := openai.ChatCompletionResponse{
		ID:      deepseekResp.ID,
		Object:  "chat.completion",
		Created: deepseekResp.Created,
		Model:   originalModel,
		Usage: openai.Usage{
			PromptTokens:     deepseekResp.Usage.PromptTokens,
			CompletionTokens: deepseekResp.Usage.CompletionTokens,
			TotalTokens:      deepseekResp.Usage.TotalTokens,
		},
		Choices: convertResponseChoices(deepseekResp.Choices),
	}

	// Convert back to JSON
	modifiedBody, err := json.Marshal(openAIResp)
	if err != nil {
		log.Printf("Error creating modified response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("Modified response body: %s", string(modifiedBody))

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(modifiedBody)
	log.Printf("Modified response sent successfully")
}
