package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

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

				// Validate JSON before writing
				if !json.Valid(line) {
					log.Printf("[%s] Invalid JSON response from OpenRouter", requestID)
					errCh <- fmt.Errorf("invalid JSON response from OpenRouter")
					return
				}

				// Write the line (OpenRouter responses are already in OpenAI format)
				if _, err := w.Write(line); err != nil {
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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[%s] Error reading response: %v", requestID, err)
		return fmt.Errorf("error reading response: %w", err)
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Write response (OpenRouter responses are already in OpenAI format)
	if _, err := w.Write(body); err != nil {
		log.Printf("[%s] Error writing response: %v", requestID, err)
		return fmt.Errorf("error writing response: %w", err)
	}

	return nil
}
