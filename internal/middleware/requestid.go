package middleware

import (
	"net/http"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !httpx.ValidRequestID(requestID) {
			requestID = httpx.NewRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(httpx.WithRequestID(r.Context(), requestID)))
	})
}
