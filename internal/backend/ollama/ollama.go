package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	ollama "github.com/danilofalcao/cursor-deepseek/internal/api/ollama/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/utils"
	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
	"github.com/pkg/errors"
)

var _ backend.Backend = &ollamaBackend{}

type ollamaBackend struct {
	endpoint     string
	models       map[string]string
	defaultModel string
	apikey       string
	timeout      time.Duration
}

type Options struct {
	Endpoint     string
	Models       map[string]string
	DefaultModel string
	ApiKey       string
	Timeout      time.Duration
}

func NewOllamaBackend(opts Options) backend.Backend {
	return &ollamaBackend{
		endpoint:     opts.Endpoint,
		models:       opts.Models,
		defaultModel: opts.DefaultModel,
		apikey:       opts.ApiKey,
		timeout:      opts.Timeout,
	}
}

// Name returns the name of the backend
func (b *ollamaBackend) Name() string {
	return "ollama"
}

// HandleChatCompletion handles a chat completion request. This method must capture and
// return to the client all errors on the provided writer.
func (b *ollamaBackend) HandleChatCompletion(ctx context.Context, w http.ResponseWriter, _ *http.Request, req *openai.ChatCompletionRequest) {
	lgr, ctx := logutils.FromContext(ctx).Clone(b.Name())

	// Store original model name for response
	originalModel := req.Model

	// Convert model internally
	mappedModel, ok := b.models[originalModel]
	if !ok {
		mappedModel = b.defaultModel
	}
	req.Model = mappedModel
	lgr.Debugf(ctx, "Model converted to: %s (original: %s)", mappedModel, originalModel)

	// Convert to Ollama request format
	ollamaReq := ollama.Request{
		Model:    mappedModel,
		Messages: convertMessages(req.Messages),
		Stream:   req.Stream,
	}

	if req.Temperature != nil {
		ollamaReq.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		ollamaReq.MaxTokens = *req.MaxTokens
	}

	// Create Ollama request
	ollamaReqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		err = errors.Wrap(err, "error marshalling ollama request")
		lgr.Error(ctx, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lgr.Debugf(ctx, "ollamaReqBody: %s", string(ollamaReqBody))
	// Send request to Ollama
	ollamaResp, err := http.Post(
		fmt.Sprintf("%s/chat", b.endpoint),
		"application/json",
		bytes.NewBuffer(ollamaReqBody),
	)
	if err != nil {
		err = errors.Wrap(err, "error POSTing ollama request")
		lgr.Error(ctx, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ollamaResp.Body.Close()

	if req.Stream {
		handleStreamingResponse(ctx, w, ollamaResp, originalModel)
	} else {
		handleRegularResponse(ctx, w, ollamaResp, originalModel)
	}
}

// ListModels returns the list of available models
func (b *ollamaBackend) ListModels(ctx context.Context) ([]openai.Model, error) {
	openAiModels := make([]openai.Model, 0, len(b.models))
	for servedModel := range b.models {
		openAiModels = append(openAiModels, openai.Model{
			ID:      servedModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "ollama",
		})
	}
	if len(openAiModels) == 0 {
		openAiModels = append(openAiModels, openai.Model{
			ID:      b.defaultModel,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "ollama",
		})
	}
	return openAiModels, nil
}

// ValidateAPIKey validates the provided API key
func (b *ollamaBackend) ValidateAPIKey(apiKey string) bool {
	return utils.SecureCompareString(apiKey, b.apikey)
}

func handleStreamingResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, originalModel string) {
	lgr := logutils.FromContext(ctx)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		lgr.Error(ctx, "streaming unsupported")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				err = errors.Wrap(err, "error reading stream")
				lgr.Error(ctx, err.Error())
				return // the break below gets out of the loop and returns, but it's a long loop
			}
			break
		}

		var ollamaResp ollama.Response
		if err := json.Unmarshal(line, &ollamaResp); err != nil {
			err = errors.Wrapf(err, "error unmarshaling response %s", string(line))
			lgr.Error(ctx, err.Error())
			continue
		}

		openAIResp := openai.ChatCompletionStreamResponse{
			ID:      "chatcmpl-" + time.Now().Format("20060102150405"),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   originalModel,
			Choices: []openai.StreamChoice{
				{
					Index: 0,
					Delta: openai.Delta{
						Content: openai.Content_String{Content: ollamaResp.Message.Content},
						Role:    "assistant",
					},
				},
			},
		}

		if ollamaResp.Done {
			openAIResp.Choices[0].FinishReason = "stop"
		}

		data, err := json.Marshal(openAIResp)
		if err != nil {
			err = errors.Wrap(err, "error marshaling OpenAI response")
			lgr.Error(ctx, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lgr.Tracef(ctx, "data: %+v", string(data))
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if ollamaResp.Done {
			break
		}
	}
}

func handleRegularResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, originalModel string) {
	lgr := logutils.FromContext(ctx)
	var ollamaResp ollama.Response
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrapf(err, "error reading response: %s", string(b))
		lgr.Error(ctx, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(b, &ollamaResp)
	if err != nil {
		err = errors.Wrapf(err, "error unmarshaling response: %s", string(b))
		lgr.Error(ctx, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to OpenAI format
	openAIResp := openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + time.Now().Format("20060102150405"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: openai.Content_String{Content: ollamaResp.Message.Content},
				},
				FinishReason: "stop",
			},
		},
	}

	lgr.Debugf(ctx, "openAIResp: %+v", openAIResp)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(openAIResp); err != nil {
		err = errors.Wrap(err, "error encoding JSON response on the wire")
		lgr.Error(ctx, err.Error())
	}
}
