package utils

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateRequestID creates a new random request ID
func GenerateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
