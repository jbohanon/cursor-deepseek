package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/context"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// LoggingMiddleware logs request and response details
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = context.GenerateRequestID()
		}

		// Add request ID to context and response headers
		ctx := context.WithRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)

		// Create wrapped response writer to capture status and size
		wrapped := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		// Log request
		log.Printf("[%s] Request: %s %s %s",
			requestID,
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
		)
		if backend := r.Header.Get("X-Backend"); backend != "" {
			log.Printf("[%s] Backend: %s", requestID, backend)
		}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log response
		duration := time.Since(start)
		log.Printf("[%s] Response: %d %s %d bytes %v",
			requestID,
			wrapped.status,
			http.StatusText(wrapped.status),
			wrapped.size,
			duration,
		)
	})
}
