package middleware

import (
	"net/http"
	"time"

	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
)

type responseWriter struct {
	http.ResponseWriter
	status        int
	size          int
	headerWritten bool
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
	rw.headerWritten = true
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// Note the contract from the underlying ResponseWriter interface that
	// "If [ResponseWriter.WriteHeader] has not yet been called, Write calls
	// WriteHeader(http.StatusOK) before writing the data."
	// and set our internal status appropriately
	if !rw.headerWritten {
		rw.status = http.StatusOK
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// LoggingMiddleware logs request and response details
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lgr := logutils.FromContext(r.Context())
		start := time.Now()

		// Create wrapped response writer to capture status and size
		wrapped := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusInternalServerError,
		}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log response
		duration := time.Since(start)
		lgr.Infof(r.Context(), "Request: %s, %s // Response: %d %s %d bytes %v",
			r.Method,
			r.Pattern,
			wrapped.status,
			http.StatusText(wrapped.status),
			wrapped.size,
			duration,
		)
	})
}
