package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"maps"
	"net/http"
	"time"

	deepseek "github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/utils"
	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
	"github.com/pkg/errors"
	"golang.org/x/net/http2"
)

var _ backend.Backend = &openrouterBackend{}

type openrouterBackend struct {
	endpoint string
	model    string
	apikey   string
	timeout  time.Duration
}

type Options struct {
	Endpoint string
	Model    string
	ApiKey   string
	Timeout  time.Duration
}

func NewOpenrouterBackend(opts Options) backend.Backend {
	return &openrouterBackend{
		endpoint: opts.Endpoint,
		model:    opts.Model,
		apikey:   opts.ApiKey,
		timeout:  opts.Timeout,
	}
}

// Name returns the name of the backend
func (b *openrouterBackend) Name() string {
	return "openrouter"
}

// HandleChatCompletion handles a chat completion request. This method must capture and
// return to the client all errors on the provided writer.
func (b *openrouterBackend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, r *http.Request, req *openai.ChatCompletionRequest) {
	lgr, ctx := logutils.FromContext(ctx).Clone(b.Name())

	lgr.Debugf(ctx, "Requested model: %s", req.Model)

	// Store original model name for response
	originalModel := req.Model

	// Convert to deepseek-chat internally
	req.Model = b.model
	lgr.Debugf(ctx, "Model converted to: %s (original: %s)", b.model, originalModel)

	// Convert to DeepSeek request format
	deepseekReq := deepseek.Request{
		Model:    b.model,
		Messages: convertMessages(req.Messages),
		Stream:   req.Stream,
	}

	// Set default temperature if not provided
	if req.Temperature != nil {
		deepseekReq.Temperature = *req.Temperature
	} else {
		defaultTemp := 0.7
		deepseekReq.Temperature = defaultTemp
	}

	// Set default max tokens if not provided
	if req.MaxTokens != nil {
		deepseekReq.MaxTokens = *req.MaxTokens
	} else {
		defaultMaxTokens := 4096
		deepseekReq.MaxTokens = defaultMaxTokens
	}

	// Handle tools and tool choice
	if len(req.Tools) > 0 {
		deepseekReq.Tools = convertTools(req.Tools)
		deepseekReq.ToolChoice = convertToolChoice(req.ToolChoice)
	} else if len(req.Functions) > 0 {
		// Convert legacy functions to tools
		tools := make([]deepseek.Tool, len(req.Functions))
		for i, fn := range req.Functions {
			tools[i] = deepseek.Tool{
				Type: "function",
				Function: deepseek.Function{
					Name:        fn.Name,
					Parameters:  fn.Parameters,
					Description: fn.Description,
				},
			}
		}
		deepseekReq.Tools = tools
		deepseekReq.ToolChoice = convertToolChoice(req.ToolChoice)
	}

	// Create new request body
	modifiedBody, err := json.Marshal(deepseekReq)
	if err != nil {
		err = errors.Wrap(err, "error creating modified request body")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error creating modified request", http.StatusInternalServerError)
		return
	}

	lgr.Debugf(ctx, "Modified request body: %s", string(modifiedBody))

	// Create the proxy request to OpenRouter
	targetURL := b.endpoint + "/chat/completions"
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	log.Printf("Forwarding to: %s", targetURL)
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		err = errors.Wrap(err, "error creating proxy request")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	copyHeaders(proxyReq.Header, r.Header)

	// Set OpenRouter API key and required headers
	proxyReq.Header.Set("Authorization", "Bearer "+b.apikey)
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("HTTP-Referer", "https://github.com/danilofalcao/cursor-deepseek") // Optional, for OpenRouter rankings
	proxyReq.Header.Set("X-Title", "Cursor DeepSeek")                                      // Optional, for OpenRouter rankings
	if req.Stream {
		proxyReq.Header.Set("Accept", "text/event-stream")
	}

	lgr.Debugf(ctx, "Proxy request headers: %v", proxyReq.Header)

	// Create a custom client with keepalive
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS:   nil,
		},
		// Remove global timeout as we'll handle timeouts per request type
		Timeout: 0,
	}

	// Create context with timeout based on streaming
	if !req.Stream {
		// Use timeout only for non-streaming requests
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}

	// Create the request with context
	proxyReq = proxyReq.WithContext(ctx)

	// Send the request
	resp, err := client.Do(proxyReq)
	if err != nil {
		err = errors.Wrap(err, "error forwarding request")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	lgr.Debugf(ctx, "OpenRouter response status: %d", resp.StatusCode)
	lgr.Debugf(ctx, "OpenRouter response headers: %v", resp.Header)

	// Handle error responses
	if resp.StatusCode >= http.StatusBadRequest {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrapf(err, "error reading error response")
			lgr.Error(ctx, err.Error())
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}

		lgr.Infof(ctx, "OpenRouter error response: %s", string(respBody))

		// Forward the error response
		maps.Copy(w.Header(), resp.Header)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	// Handle streaming response
	if req.Stream {
		handleStreamingResponse(ctx, w, resp)
		return
	}

	// Handle regular response
	handleRegularResponse(ctx, w, resp, originalModel)
}

// ListModels returns the list of available models
func (b *openrouterBackend) ListModels(ctx context.Context) ([]openai.Model, error) {
	return []openai.Model{
		{
			ID:      b.model,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "deepseek",
		},
	}, nil
}

// ValidateAPIKey validates the provided API key
func (b *openrouterBackend) ValidateAPIKey(apiKey string) bool {
	return utils.SecureCompareString(apiKey, b.apikey)
}

func handleStreamingResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) {
	lgr := logutils.FromContext(ctx)
	lgr.Debug(ctx, "Starting streaming response handling")

	// Set headers for streaming response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	// Create a buffered reader for the response body
	reader := bufio.NewReaderSize(resp.Body, 1024)

	// Create a context that will be cancelled when the client disconnects
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create a channel for errors
	errChan := make(chan error, 1)

	// Start processing in a goroutine
	go func() {
		defer close(errChan)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Read until we get a complete SSE message
				var buffer bytes.Buffer
				for {
					line, err := reader.ReadBytes('\n')
					if err != nil {
						if err == io.EOF {
							lgr.Debug(ctx, "EOF reached")
							return
						}
						err = errors.Wrap(err, "error reading from upstream server stream")
						errChan <- err
						return
					}

					// Log the received line for debugging
					lgr.Debugf(ctx, "Received line: %s", string(line))

					// Write to buffer
					buffer.Write(line)

					// If we've reached the end of an event (double newline)
					if bytes.HasSuffix(buffer.Bytes(), []byte("\n\n")) {
						break
					}
				}

				// Get the complete message
				message := buffer.Bytes()

				// Skip if empty
				if len(bytes.TrimSpace(message)) == 0 {
					continue
				}

				// Write the message
				if _, err := w.Write(message); err != nil {
					err = errors.Wrap(err, "error writing to downstream client stream")
					errChan <- err
					return
				}

				// Flush after each complete message
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
					lgr.Debug(ctx, "flushed message to client")
				}
			}
		}
	}()

	// Wait for completion or error
	select {
	case err := <-errChan:
		if err != nil {
			lgr.Error(ctx, err.Error())
		}
	case <-ctx.Done():
		lgr.Info(ctx, "context cancelled")
	}

	lgr.Info(ctx, "streaming response handler completed")
}

func handleRegularResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, originalModel string) {
	lgr := logutils.FromContext(ctx)
	lgr.Infof(ctx, "Handling regular (non-streaming) response")
	lgr.Debugf(ctx, "Response status: %d", resp.StatusCode)
	lgr.Debugf(ctx, "Response headers: %+v", resp.Header)

	// Read and log response body
	body, err := readResponse(resp)
	if err != nil {
		err = errors.Wrap(err, "error reading response")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error reading response from upstream", http.StatusInternalServerError)
		return
	}

	lgr.Debugf(ctx, "Original response body: %s", string(body))

	// Parse the DeepSeek response
	var deepseekResp deepseek.Response
	if err := json.Unmarshal(body, &deepseekResp); err != nil {
		err = errors.Wrap(err, "error parsing DeepSeek response")
		lgr.Error(ctx, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Use the original model name instead of hardcoding gpt-4o
	deepseekResp.Model = originalModel

	// If we have tools calls, make sure the have type "function"
	for i, choice := range deepseekResp.Choices {
		if choice.Message.ToolCalls != nil {
			for j, tc := range choice.Message.ToolCalls {
				tc.Type = "function"
				choice.Message.ToolCalls[j] = tc
			}
			deepseekResp.Choices[i] = choice
		}
	}

	// Convert back to JSON
	modifiedBody, err := json.Marshal(deepseekResp)
	if err != nil {
		err = errors.Wrap(err, "error creating modified response")
		lgr.Error(ctx, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lgr.Debugf(ctx, "Modified response body: %s", string(modifiedBody))

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(modifiedBody)
	lgr.Info(ctx, "unary response handler completed")
}
