package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	ollama "github.com/danilofalcao/cursor-deepseek/internal/api/ollama/v1"
	openai "github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
)

const (
	ollamaEndpoint = "http://localhost:11434/api"
	defaultModel   = "llama2"
)

// Configuration structure
type Config struct {
	endpoint string
	model    string
}

var activeConfig Config

func init() {
	// Load .env file
	log.Printf("Variant: OLLAMA")
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}

	// Get custom Ollama endpoint if specified
	customEndpoint := os.Getenv("OLLAMA_API_ENDPOINT")
	if customEndpoint != "" {
		activeConfig.endpoint = customEndpoint
	} else {
		activeConfig.endpoint = ollamaEndpoint
	}

	// Get custom Ollama endpoint if specified
	modelenv := os.Getenv("DEFAULT_MODEL")
	if modelenv != "" {
		activeConfig.model = modelenv
	} else {
		//no environment set so check for command line argument
		modelFlag := defaultModel // default value
		for i, arg := range os.Args {
			if arg == "-model" && i+1 < len(os.Args) {
				modelFlag = os.Args[i+1]
			}
		}
		activeConfig.model = modelFlag
	}

	log.Printf("Initialized with model: %s using endpoint: %s", activeConfig.model, activeConfig.endpoint)
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	server := &http.Server{
		Addr:    ":9000",
		Handler: http.HandlerFunc(proxyHandler),
	}

	// Enable HTTP/2 support
	http2.ConfigureServer(server, &http2.Server{})

	log.Printf("Starting Ollama proxy server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func enableCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s", r.Method, r.URL.Path)

	if r.Method == "OPTIONS" {
		enableCors(w)
		return
	}

	enableCors(w)

	switch r.URL.Path {
	case "/v1/chat/completions":
		handleChatCompletions(w, r)
	case "/v1/models":
		handleModelsRequest(w)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var chatReq openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("chatReq: %+v", chatReq)

	// Store original model name for response
	originalModel := chatReq.Model
	if originalModel == "" {
		originalModel = activeConfig.model
	}

	// Always use the configured model internally
	chatReq.Model = activeConfig.model
	log.Printf("Model converted to: %s (original: %s)", activeConfig.model, originalModel)

	// Convert to Ollama request format
	ollamaReq := ollama.Request{
		Model:    activeConfig.model,
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
		fmt.Sprintf("%s/chat", activeConfig.endpoint),
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

func convertMessages(messages []openai.Message) []ollama.Message {
	ollamaMessages := make([]ollama.Message, len(messages))
	for i, message := range messages {
		var content string
		switch message.GetContent().(type) {
		case openai.Content_String:
			content = message.GetContentString()
		case openai.Content_Array:
			contentArray := message.GetContentArray()
			for i := range contentArray {
				t := contentArray.GetContentPartTextAtIndex(i).Text
				if t != "" {
					content += "; " + t
				}
			}
		}
		ollamaMessages[i] = ollama.Message{
			Role:    message.Role,
			Content: content,
		}
	}
	return ollamaMessages
}

func handleStreamingResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, originalModel string) {
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

func handleModelsRequest(w http.ResponseWriter) {
	log.Printf("Handling models request")

	response := openai.ModelsResponse{
		Object: "list",
		Data: []openai.Model{
			{
				ID:      activeConfig.model,
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
