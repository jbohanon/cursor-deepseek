package middleware

import (
	"context"
	"net/http"
	"time"
)

type ApiKeyValidationFunc func(string) bool
type Params struct {
	ApiKey         string
	AuthValidation ApiKeyValidationFunc
	Timeout        time.Duration
}

func Wrap(ctx context.Context, handler http.Handler, params Params) http.Handler {
	// These middlewares will be executed in the reverse order of their
	// wrapping. i.e. the last wrap operation will be the first one executed
	// on a request.
	if params.ApiKey != "" {
		handler = withApiKeyAuth(handler, params.ApiKey, params.AuthValidation)
	}
	handler = withCors(handler)
	handler = withLogging(handler)
	handler = withContext(ctx, handler, params.Timeout)
	return handler
}
