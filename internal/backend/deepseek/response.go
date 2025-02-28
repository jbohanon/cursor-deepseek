package deepseek

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

func (b *Backend) handleStreamingResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
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

				// Parse response into common format
				var streamResp backend.StreamingResponse
				if err := json.Unmarshal(line, &streamResp); err != nil {
					log.Printf("[%s] Error parsing response: %v", requestID, err)
					errCh <- fmt.Errorf("error parsing response: %w", err)
					return
				}

				// Write response using common util
				if err := util.WriteJSON(w, http.StatusOK, streamResp); err != nil {
					log.Printf("[%s] Error writing response: %v", requestID, err)
					errCh <- fmt.Errorf("error writing response: %w", err)
					return
				}

				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}()

	return <-errCh
}

func (b *Backend) handleRegularResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
	requestID := reqcontext.GetRequestID(ctx)

	var completionResp backend.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		log.Printf("[%s] Error reading response: %v", requestID, err)
		return fmt.Errorf("error reading response: %w", err)
	}

	return util.WriteJSON(w, http.StatusOK, completionResp)
}
