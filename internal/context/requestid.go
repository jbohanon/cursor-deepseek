package context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
)

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GenerateRequestID creates a new random request ID
func GenerateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
