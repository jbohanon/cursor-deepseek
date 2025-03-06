package logutils

import (
	"context"

	"github.com/danilofalcao/cursor-deepseek/internal/constants"
	"github.com/danilofalcao/cursor-deepseek/internal/logger"
)

// FromContext retrieves the logger from the context
func FromContext(ctx context.Context) *logger.Logger {
	if lgr, ok := ctx.Value(constants.LoggerKey).(*logger.Logger); ok {
		return lgr
	}
	return nil
}

// ContextWithLogger adds a logger to the context
func ContextWithLogger(ctx context.Context, lgr *logger.Logger) context.Context {
	return context.WithValue(ctx, constants.LoggerKey, lgr)
}
