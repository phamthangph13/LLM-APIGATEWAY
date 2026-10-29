package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

type requestIDContextKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,128}$`)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestID(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return value
	}
	return ""
}

// ValidRequestID reports whether a caller-supplied request ID is safe to echo
// and log.
func ValidRequestID(value string) bool {
	return validRequestID.MatchString(value)
}

func NewRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(buffer)
}
