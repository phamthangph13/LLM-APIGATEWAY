package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/gateway"
	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/middleware"
	"github.com/deepsi/llm-api-gateway/internal/models"
	"github.com/deepsi/llm-api-gateway/internal/providers/ninerouter"
)

const Version = "0.1.0"

type Server struct {
	cfg      *config.Config
	log      *slog.Logger
	registry *models.Registry
	upstream *ninerouter.Client
	gateway  *gateway.Gateway
	limiter  *middleware.RateLimiter

	handler    http.Handler
	httpServer *http.Server
	started    time.Time
}

func New(cfg *config.Config, log *slog.Logger) (*Server, error) {
	registry, err := models.NewRegistry(cfg.Models.M1, cfg.Models.S1, cfg.Models.V1, cfg.Models.E1)
	if err != nil {
		return nil, fmt.Errorf("initialize model registry: %w", err)
	}

	upstream, err := ninerouter.New(cfg.Upstream.BaseURL, cfg.Upstream.APIKey)
	if err != nil {
		return nil, fmt.Errorf("initialize 9Router client: %w", err)
	}

	server := &Server{
		cfg:      cfg,
		log:      log,
		registry: registry,
		upstream: upstream,
		gateway:  gateway.New(cfg, registry, upstream, log),
		limiter:  middleware.NewRateLimiter(cfg.Limits.RateLimitPerMinute),
		started:  time.Now(),
	}
	server.handler = server.buildHandler()
	server.httpServer = &http.Server{
		Handler:           server.handler,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.HTTP.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.HTTP.Addr(), err)
	}
	defer listener.Close()
	return s.Serve(ctx, listener)
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	s.log.Info("gateway listening",
		"addr", listener.Addr().String(),
		"env", s.cfg.App.Env,
		"models", strings.Join(s.registry.PublicIDs(), ","),
		"auth_enabled", s.cfg.Auth.Enabled,
		"rate_limit_per_minute", s.cfg.Limits.RateLimitPerMinute,
	)
	go s.checkUpstream(ctx)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			_ = s.httpServer.Close()
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		<-errCh
		s.log.Info("gateway stopped")
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) Close() {
	s.limiter.Close()
	s.upstream.CloseIdleConnections()
}

func (s *Server) buildHandler() http.Handler {
	api := http.Handler(http.HandlerFunc(s.dispatchAPI))
	api = middleware.NewAuthenticator(s.cfg.Auth).Middleware(api)
	api = s.limiter.Middleware(api)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
			api.ServeHTTP(w, r)
			return
		}
		s.dispatchPublic(w, r)
	})

	handler := middleware.LimitBody(s.cfg.HTTP.MaxBodyBytes)(root)
	handler = middleware.NewCORS(s.cfg.CORS.AllowedOrigins).Middleware(handler)
	handler = middleware.Recover(s.log)(handler)
	handler = middleware.Logging(s.log, s.cfg.Limits.TrustProxyHeaders)(handler)
	handler = middleware.RequestID(handler)
	return handler
}

func (s *Server) dispatchPublic(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET")
			return
		}
		s.handleHealth(w, r)
	case "/readyz":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET")
			return
		}
		s.handleReady(w, r)
	default:
		httpx.WriteOpenAIError(w, http.StatusNotFound, "not_found", "Unknown endpoint.")
	}
}

func (s *Server) dispatchAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/models":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		s.gateway.Models(w, r)
	case "/v1/chat/completions":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		s.gateway.ChatCompletions(w, r)
	case "/v1/embeddings":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		s.gateway.Embeddings(w, r)
	default:
		httpx.WriteOpenAIError(w, http.StatusNotFound, "not_found", "Unknown API endpoint.")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"service":        "deepsi-llm-api-gateway",
		"version":        Version,
		"uptime_seconds": int64(time.Since(s.started).Seconds()),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	upstreamIDs, err := s.upstream.ListModels(ctx)
	if err != nil {
		s.log.Warn("readiness check failed", "error", err, "request_id", httpx.RequestID(r.Context()))
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "unready",
			"service": "deepsi-llm-api-gateway",
			"checks": map[string]any{
				"upstream": map[string]any{"ok": false, "error": "unreachable"},
			},
		})
		return
	}

	missing := s.registry.MissingPublic(upstreamIDs)
	status := "ok"
	httpStatus := http.StatusOK
	modelsCheck := map[string]any{"ok": true}
	if len(missing) > 0 {
		status = "degraded"
		modelsCheck = map[string]any{"ok": false, "missing": missing}
		if s.cfg.Upstream.ReadinessRequireModels {
			status = "unready"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	httpx.WriteJSON(w, httpStatus, map[string]any{
		"status":  status,
		"service": "deepsi-llm-api-gateway",
		"checks": map[string]any{
			"upstream": map[string]any{"ok": true, "model_count": len(upstreamIDs)},
			"models":   modelsCheck,
		},
	})
}

func (s *Server) checkUpstream(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	upstreamIDs, err := s.upstream.ListModels(checkCtx)
	if err != nil {
		s.log.Warn("startup upstream check failed", "error", err)
		return
	}
	missing := s.registry.MissingPublic(upstreamIDs)
	if len(missing) == 0 {
		s.log.Info("all configured DeepSI upstream models are present in 9Router")
		return
	}

	missingUpstream := make([]string, 0, len(missing))
	for _, model := range s.registry.List() {
		for _, publicID := range missing {
			if model.ID == publicID {
				missingUpstream = append(missingUpstream, model.UpstreamID)
			}
		}
	}
	s.log.Warn("configured upstream models are missing from 9Router",
		"missing_public", strings.Join(missing, ","),
		"missing_upstream", strings.Join(missingUpstream, ","),
	)
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	httpx.WriteOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed",
		"The HTTP method is not allowed for this endpoint.")
}
