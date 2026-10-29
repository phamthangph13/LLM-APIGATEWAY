package middleware

import (
	"net/http"
	"strings"
)

type CORS struct {
	allowed map[string]struct{}
	enabled bool
}

func NewCORS(origins []string) *CORS {
	cors := &CORS{
		allowed: make(map[string]struct{}, len(origins)),
		enabled: len(origins) > 0,
	}
	for _, origin := range origins {
		cors.allowed[strings.ToLower(origin)] = struct{}{}
	}
	return cors
}

func (c *CORS) Middleware(next http.Handler) http.Handler {
	if !c.enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		_, allowed := c.allowed[strings.ToLower(origin)]
		w.Header().Add("Vary", "Origin")

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")
		}

		if r.Method == http.MethodOptions {
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
