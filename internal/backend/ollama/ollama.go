package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	ollama "github.com/danilofalcao/cursor-deepseek/internal/api/ollama/v1"
	openai "github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/backend"
)

// TODO: Implement the Ollama backend as a backend.Backend

var _ backend.Backend = &ollamaBackend{}

type ollamaBackend struct {
	endpoint string
	model    string
}

type Options struct {
	Endpoint string
	Model    string
}

func NewOllamaBackend(opts Options) backend.Backend {
	return &ollamaBackend{
		endpoint: opts.Endpoint,
		model:    opts.Model,
	}
}

func (b *ollamaBackend) HandleModelsRequest(w http.ResponseWriter) {
	log.Printf("Handling models request")

	response := openai.ModelsResponse{
		Object: "list",
		Data: []openai.Model{
			{
				ID:      b.model,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "ollama",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	log.Printf("Models response sent successfully")
}
func (b *ollamaBackend) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var chatReq openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("chatReq: %+v", chatReq)

	// Store original model name for response
	originalModel := chatReq.Model
	if originalModel == "" {
		originalModel = b.model
	}

	// Always use the configured model internally
	chatReq.Model = b.model
	log.Printf("Model converted to: %s (original: %s)", b.model, originalModel)

	// Convert to Ollama request format
	ollamaReq := ollama.Request{
		Model:    b.model,
		Messages: convertMessages(chatReq.Messages),
		Stream:   chatReq.Stream,
	}

	if chatReq.Temperature != nil {
		ollamaReq.Temperature = *chatReq.Temperature
	}
	if chatReq.MaxTokens != nil {
		ollamaReq.MaxTokens = *chatReq.MaxTokens
	}

	// Create Ollama request
	ollamaReqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		log.Printf("ERROR: failed to marshal ollama request: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("ollamaReqBody: %s", string(ollamaReqBody))
	// Send request to Ollama
	ollamaResp, err := http.Post(
		fmt.Sprintf("%s/chat", b.endpoint),
		"application/json",
		bytes.NewBuffer(ollamaReqBody),
	)
	if err != nil {
		log.Printf("ERROR: POST failed: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ollamaResp.Body.Close()

	if chatReq.Stream {
		handleStreamingResponse(w, r, ollamaResp, originalModel)
	} else {
		handleRegularResponse(w, ollamaResp, originalModel)
	}
}

func handleStreamingResponse(w http.ResponseWriter, _ *http.Request, resp *http.Response, originalModel string) {

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading stream: %v", err)
			}
			break
		}

		var ollamaResp ollama.Response
		if err := json.Unmarshal(line, &ollamaResp); err != nil {
			log.Printf("Error unmarshaling response: %v", err)
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

		if data, err := json.Marshal(openAIResp); err == nil {
			log.Printf("data: %+v", string(data))
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		} else {
			log.Printf("ERROR: failed to marshal openAIResp: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if ollamaResp.Done {
			break
		}
	}
}

func handleRegularResponse(w http.ResponseWriter, resp *http.Response, originalModel string) {
	var ollamaResp ollama.Response
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
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

	log.Printf("openAIResp: %+v", openAIResp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAIResp)
}
