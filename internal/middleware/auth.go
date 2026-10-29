package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/httpx"
)

type Authenticator struct {
	enabled bool
	keyHash [32]byte
}

func NewAuthenticator(cfg config.AuthConfig) *Authenticator {
	authenticator := &Authenticator{enabled: cfg.Enabled}
	if cfg.Enabled {
		authenticator.keyHash = sha256.Sum256([]byte(cfg.APIKey))
	}
	return authenticator
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	if !a.enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			a.reject(w)
			return
		}

		candidate := sha256.Sum256([]byte(parts[1]))
		if subtle.ConstantTimeCompare(candidate[:], a.keyHash[:]) != 1 {
			a.reject(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Authenticator) reject(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="deepsi-llm-api-gateway"`)
	httpx.WriteError(w, http.StatusUnauthorized, "authentication_error",
		"invalid_api_key", "Missing or invalid API key.")
}
