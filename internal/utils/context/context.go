package contextutils

import (
	"context"

	"github.com/danilofalcao/cursor-deepseek/internal/constants"
)

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(constants.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, constants.RequestIDKey, requestID)
}
