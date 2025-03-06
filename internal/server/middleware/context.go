package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/utils"
	contextutils "github.com/danilofalcao/cursor-deepseek/internal/utils/context"
)

// withContext takes the server's context including its logger, injects a request ID and
// timeout, and sets it as the request's context.
func withContext(ctx context.Context, next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// set timeout
		ctx, _ = context.WithTimeout(r.Context(), timeout)

		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = utils.GenerateRequestID()
		}
		// Add request ID to context and response headers
		ctx = contextutils.WithRequestID(ctx, requestID)
		w.Header().Set("X-Request-ID", requestID)

		// set our request's context
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
