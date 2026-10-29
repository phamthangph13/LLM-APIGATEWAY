package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
)

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Error("panic recovered",
						"panic", recovered,
						"request_id", httpx.RequestID(r.Context()),
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					if recorder, ok := w.(*responseRecorder); ok && recorder.wrote {
						return
					}
					httpx.WriteOpenAIError(w, http.StatusInternalServerError, "gateway_error",
						"The gateway encountered an internal error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
