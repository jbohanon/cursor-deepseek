package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	deepseek "github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
	openai "github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
)

const (
	deepseekEndpoint     = "https://api.deepseek.com"
	deepseekBetaEndpoint = "https://api.deepseek.com/beta"
	deepseekChatModel    = "deepseek-chat"
	deepseekCoderModel   = "deepseek-coder"
	gpt4oModel           = "gpt-4o"
)

var deepseekAPIKey string

// Configuration structure
type Config struct {
	endpoint string
	model    string
}

var activeConfig Config

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}

	// Get DeepSeek API key
	deepseekAPIKey = os.Getenv("DEEPSEEK_API_KEY")
	if deepseekAPIKey == "" {
		log.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	// Parse command line arguments
	modelFlag := "chat" // default value
	for i, arg := range os.Args {
		if arg == "-model" && i+1 < len(os.Args) {
			modelFlag = os.Args[i+1]
		}
	}

	// Configure the active endpoint and model based on the flag
	switch modelFlag {
	case "coder":
		activeConfig = Config{
			endpoint: deepseekBetaEndpoint,
			model:    deepseekCoderModel,
		}
	case "chat":
		activeConfig = Config{
			endpoint: deepseekEndpoint,
			model:    deepseekChatModel,
		}
	default:
		log.Printf("Invalid model specified: %s. Using default chat model.", modelFlag)
		activeConfig = Config{
			endpoint: deepseekEndpoint,
			model:    deepseekChatModel,
		}
	}

	log.Printf("Initialized with model: %s using endpoint: %s", activeConfig.model, activeConfig.endpoint)
}

// OpenAI compatible request structure
type ChatRequest = openai.ChatCompletionRequest

func convertToolChoice(choice interface{}) string {
	if choice == nil {
		return ""
	}

	// If string "auto" or "none"
	if str, ok := choice.(string); ok {
		switch str {
		case "auto", "none":
			return str
		}
	}

	// Try to parse as map for function call
	if choiceMap, ok := choice.(map[string]interface{}); ok {
		if choiceMap["type"] == "function" {
			return "auto" // DeepSeek doesn't support specific function selection, default to auto
		}
	}

	return ""
}

func convertMessages(messages []openai.Message) []deepseek.Message {
	converted := make([]deepseek.Message, len(messages))
	for i, msg := range messages {
		log.Printf("Converting message %d - Role: %s", i, msg.Role)
		converted[i] = deepseek.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			log.Printf("Processing assistant message with %d tool calls", len(msg.ToolCalls))
			// DeepSeek expects tool_calls in a specific format
			toolCalls := make([]deepseek.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = deepseek.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: deepseek.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
				log.Printf("Tool call %d - ID: %s, Function: %s", j, tc.ID, tc.Function.Name)
			}
			converted[i].ToolCalls = toolCalls
		}

		// Handle function response messages
		if msg.Role == "function" {
			log.Printf("Converting function response to tool response")
			// Convert to tool response format
			converted[i].Role = "tool"
		}
	}

	// Log the final converted messages
	for i, msg := range converted {
		log.Printf("Final message %d - Role: %s, Content: %s", i, msg.Role, truncateString(msg.Content, 50))
		if len(msg.ToolCalls) > 0 {
			log.Printf("Message %d has %d tool calls", i, len(msg.ToolCalls))
		}
	}

	return converted
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// DeepSeek request structure
type DeepSeekRequest = deepseek.Request

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	server := &http.Server{
		Addr:    ":9000",
		Handler: http.HandlerFunc(proxyHandler),
	}

	// Enable HTTP/2 support
	http2.ConfigureServer(server, &http2.Server{})

	log.Printf("Starting proxy server on %s", server.Addr)
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

	// Validate API key
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("Missing or invalid Authorization header")
		http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}

	userAPIKey := strings.TrimPrefix(authHeader, "Bearer ")
	if userAPIKey != deepseekAPIKey {
		log.Printf("Invalid API key provided")
		http.Error(w, "Invalid API key", http.StatusUnauthorized)
		return
	}

	// Handle /v1/models endpoint
	if r.URL.Path == "/v1/models" && r.Method == "GET" {
		log.Printf("Handling /v1/models request")
		handleModelsRequest(w)
		return
	}

	// Log headers for debugging
	log.Printf("Request headers: %+v", r.Header)

	// Read and log request body for debugging
	var chatReq ChatRequest
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

	// Handle models endpoint
	if r.URL.Path == "/v1/models" {
		handleModelsRequest(w)
		return
	}

	// Only handle API requests with /v1/ prefix
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		log.Printf("Invalid path: %s", r.URL.Path)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

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
	chatReq.Model = deepseekChatModel
	log.Printf("Model converted to: %s (original: %s)", deepseekChatModel, originalModel)

	// Convert to DeepSeek request format
	deepseekReq := deepseek.Request{
		Model:    deepseekChatModel,
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
	targetURL := activeConfig.endpoint + r.URL.Path
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
	proxyReq.Header.Set("Authorization", "Bearer "+deepseekAPIKey)
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

func convertTools(tools []openai.Tool) []deepseek.Tool {
	converted := make([]deepseek.Tool, len(tools))
	for i, tool := range tools {
		converted[i] = deepseek.Tool{
			Type: tool.Type,
			Function: deepseek.Function{
				Name:        tool.Function.Name,
				Parameters:  tool.Function.Parameters,
				Description: tool.Function.Description,
			},
		}
	}
	return converted
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

func convertResponseChoices(choices []deepseek.Choice) []openai.Choice {
	openaiChoices := make([]openai.Choice, len(choices))
	for i, choice := range choices {
		openaiChoices[i] = openai.Choice{
			Index:        choice.Index,
			Message:      convertResponseMessage(choice.Message),
			FinishReason: choice.FinishReason,
		}
	}
	return openaiChoices
}

func convertResponseMessage(message deepseek.Message) openai.Message {
	return openai.Message{
		Role:       message.Role,
		Content:    message.Content,
		ToolCalls:  convertResponseToolCalls(message.ToolCalls),
		ToolCallID: message.ToolCallID,
		Name:       message.Name,
	}
}

func convertResponseToolCalls(toolCalls []deepseek.ToolCall) []openai.ToolCall {
	openaiToolCalls := make([]openai.ToolCall, 0)
	for i, tc := range toolCalls {
		log.Printf("Tool call %d: %+v", i, tc)
		// Ensure the tool call has the required fields
		if tc.Function.Name == "" {
			log.Printf("Warning: Empty function name in tool call %d", i)
			continue
		}
		openaiToolCalls = append(openaiToolCalls, openai.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: openai.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return openaiToolCalls
}

func copyHeaders(dst, src http.Header) {
	// Headers to skip
	skipHeaders := map[string]bool{
		"Content-Length":    true,
		"Content-Encoding":  true,
		"Transfer-Encoding": true,
		"Connection":        true,
	}

	for k, vv := range src {
		if !skipHeaders[k] {
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func handleModelsRequest(w http.ResponseWriter) {
	log.Printf("Handling models request")

	// Get the requested model from the query parameters
	response := openai.ModelsResponse{
		Object: "list",
		Data: []openai.Model{
			{
				ID:      deepseekChatModel,
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

func readResponse(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error creating gzip reader: %v", err)
		}
		defer gzReader.Close()
		reader = gzReader
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = flate.NewReader(resp.Body)
	}

	return io.ReadAll(reader)
}
