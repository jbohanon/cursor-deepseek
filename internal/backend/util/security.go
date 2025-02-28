package util

import (
	"crypto/subtle"
)

// SecureCompareString compares two strings in constant time to prevent timing attacks
func SecureCompareString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
