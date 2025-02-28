package util

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WriteJSON writes a JSON response with proper headers
func WriteJSON(w http.ResponseWriter, status int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// GenerateResponseID generates a unique response ID
func GenerateResponseID() string {
	return fmt.Sprintf("chatcmpl-%s", time.Now().Format("20060102150405"))
}
