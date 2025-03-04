package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"maps"

	"github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	deepseekconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/deepseek"
	"github.com/danilofalcao/cursor-deepseek/internal/utils"
	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
	"github.com/pkg/errors"
	"golang.org/x/net/http2"
)

var _ backend.Backend = &deepseekBackend{}

type deepseekBackend struct {
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

func NewDeepseekBackend(opts Options) backend.Backend {
	return &deepseekBackend{
		endpoint: opts.Endpoint,
		model:    opts.Model,
		apikey:   opts.ApiKey,
		timeout:  opts.Timeout,
	}
}

// Name returns the name of the backend
func (b *deepseekBackend) Name() string {
	return "deepseek"
}

// HandleChatCompletion handles a chat completion request
func (b *deepseekBackend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, r *http.Request, req *openai.ChatCompletionRequest) {
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

	// Copy optional parameters if present
	if req.Temperature != nil {
		deepseekReq.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		deepseekReq.MaxTokens = *req.MaxTokens
	}

	// Handle tools/functions
	if len(req.Tools) > 0 {
		deepseekReq.Tools = convertTools(req.Tools)
		if tc := convertToolChoice(req.ToolChoice); tc != "" {
			deepseekReq.ToolChoice = tc
		}
	} else if len(req.Functions) > 0 {
		// Convert functions to tools format
		tools := make([]deepseek.Tool, len(req.Functions))
		for i, fn := range req.Functions {
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
		if tc := convertToolChoice(req.ToolChoice); tc != "" {
			deepseekReq.ToolChoice = tc
		}
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

	// Create the proxy request to DeepSeek
	targetURL := b.endpoint + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	lgr.Infof(ctx, "Forwarding to: %s", targetURL)
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(modifiedBody))
	if err != nil {
		err = errors.Wrap(err, "error creating proxy request")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	copyHeaders(proxyReq.Header, r.Header)

	// Set DeepSeek API key and content type
	proxyReq.Header.Set("Authorization", "Bearer "+b.apikey)
	proxyReq.Header.Set("Content-Type", "application/json")
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
		Timeout: b.timeout,
	}

	// Send the request
	resp, err := client.Do(proxyReq)
	if err != nil {
		err = errors.Wrap(err, "error forwarding request")
		lgr.Error(ctx, err.Error())
		http.Error(w, "Error forwarding request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	lgr.Debugf(ctx, "DeepSeek response status: %d", resp.StatusCode)
	lgr.Debugf(ctx, "DeepSeek response headers: %v", resp.Header)

	// Handle error responses
	if resp.StatusCode >= http.StatusBadRequest {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading error response")
			lgr.Error(ctx, err.Error())
			http.Error(w, "Error reading response", http.StatusInternalServerError)
			return
		}
		lgr.Infof(ctx, "DeepSeek error response: %s", string(respBody))

		// Forward the error response
		maps.Copy(w.Header(), resp.Header)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	// Handle streaming response
	if req.Stream {
		handleStreamingResponse(ctx, w, r, resp, originalModel)
		return
	}

	// Handle regular response
	handleRegularResponse(ctx, w, resp, originalModel)
}

// ListModels returns the list of available models
func (b *deepseekBackend) ListModels(ctx context.Context) ([]openai.Model, error) {
	return []openai.Model{
		{
			ID:      deepseekconstants.DefaultChatModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "deepseek",
		},
		{
			ID:      deepseekconstants.DefaultCoderModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "deepseek",
		},
	}, nil
}

// ValidateAPIKey validates the provided API key
func (b *deepseekBackend) ValidateAPIKey(apiKey string) bool {
	return utils.SecureCompareString(apiKey, b.apikey)
}
func handleStreamingResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, resp *http.Response, originalModel string) {
	lgr := logutils.FromContext(ctx)
	lgr.Debugf(ctx, "Starting streaming response handling with model: %s", originalModel)
	lgr.Debugf(ctx, "Response status: %d", resp.StatusCode)
	lgr.Debugf(ctx, "Response headers: %+v", resp.Header)

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
					err = errors.Wrap(err, "error sending heartbeat")
					lgr.Error(ctx, err.Error())
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
			lgr.Info(ctx, "Context cancelled, ending stream")
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					continue
				}
				err = errors.Wrap(err, "error reading stream")
				lgr.Error(ctx, err.Error())
				cancel()
				return
			}

			// Skip empty lines
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}

			// Write the line to the response
			if _, err := w.Write(line); err != nil {
				err = errors.Wrap(err, "error writing response")
				lgr.Error(ctx, err.Error())
				cancel()
				return
			}

			// Flush the response writer
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			} else {
				lgr.Warn(ctx, "ResponseWriter does not support Flush")
			}
		}
	}
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
