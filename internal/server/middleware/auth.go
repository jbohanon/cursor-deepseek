package middleware

import (
	"log"
	"net/http"
	"strings"

	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
)

func withApiKeyAuth(next http.Handler, apikey string, apikeyValidation func(apikey string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Validate API key
		// TODO: add support for API key in custom header
		apiKey := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		if apiKey == "" {
			logutils.FromContext(ctx).Warn(ctx, "No API Key provided")
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		log.Println(apiKey)
		if !apikeyValidation(apiKey) {
			logutils.FromContext(ctx).Warn(ctx, "Invalid API Key provided")
			http.Error(w, "Invalid API key", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)

	})
}
