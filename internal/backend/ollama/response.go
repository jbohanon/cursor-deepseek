package ollama

import (
	"bufio"
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

func (b *Backend) handleStreamingResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, model string) error {
	requestID := reqcontext.GetRequestID(ctx)

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create buffered reader
	reader := bufio.NewReader(resp.Body)

	// Create a channel for errors
	errCh := make(chan error, 1)

	// Create a context with timeout
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	go func() {
		for {
			select {
			case <-streamCtx.Done():
				log.Printf("[%s] Stream timeout or cancelled", requestID)
				errCh <- streamCtx.Err()
				return
			default:
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						log.Printf("[%s] Stream completed", requestID)
						errCh <- nil
						return
					}
					log.Printf("[%s] Error reading stream: %v", requestID, err)
					errCh <- fmt.Errorf("error reading stream: %w", err)
					return
				}

				if len(line) <= 1 {
					continue
				}

				// Parse Ollama response
				var ollamaResp OllamaResponse
				if err := json.Unmarshal(line, &ollamaResp); err != nil {
					log.Printf("[%s] Error parsing response: %v", requestID, err)
					errCh <- fmt.Errorf("error parsing response: %w", err)
					return
				}

				// Convert to OpenAI format
				openAIResp := backend.StreamingResponse{
					ID:      util.GenerateResponseID(),
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   model,
					Choices: []struct {
						Index        int           `json:"index"`
						Delta        backend.Delta `json:"delta"`
						FinishReason string        `json:"finish_reason,omitempty"`
					}{
						{
							Index: 0,
							Delta: backend.Delta{
								Role:    "assistant",
								Content: ollamaResp.Message.Content,
							},
						},
					},
				}

				if ollamaResp.Done {
					openAIResp.Choices[0].FinishReason = "stop"
					if err := util.WriteJSON(w, http.StatusOK, openAIResp); err != nil {
						log.Printf("[%s] Error writing response: %v", requestID, err)
						errCh <- fmt.Errorf("error writing response: %w", err)
						return
					}
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					errCh <- nil
					return
				}
			}
		}
	}()

	return <-errCh
}

func (b *Backend) handleRegularResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, model string) error {
	requestID := reqcontext.GetRequestID(ctx)

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		log.Printf("[%s] Error reading response: %v", requestID, err)
		return fmt.Errorf("error reading response: %w", err)
	}

	// Convert to OpenAI format
	openAIResp := backend.CompletionResponse{
		ID:      util.GenerateResponseID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []struct {
			Index        int             `json:"index"`
			Message      backend.Message `json:"message"`
			FinishReason string          `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: backend.Message{
					Role:    "assistant",
					Content: ollamaResp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
	}

	return util.WriteJSON(w, http.StatusOK, openAIResp)
}
