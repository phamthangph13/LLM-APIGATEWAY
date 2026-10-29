package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultPort                          = 8085
	DefaultHost                          = "127.0.0.1"
	DefaultNineRouterBaseURL             = "http://127.0.0.1:20128/v1"
	DefaultRequestTimeout                = 120 * time.Second
	DefaultMaxBodyBytes            int64 = 20 << 20
	DefaultShutdownTimeout               = 15 * time.Second
	DefaultReadHeaderTimeout             = 10 * time.Second
	DefaultRateLimitPerMinute            = 60
	MinimumProductionGatewayKeyLen       = 16
)

// Config is the fully validated gateway configuration.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Upstream UpstreamConfig
	Auth     AuthConfig
	Limits   LimitsConfig
	CORS     CORSConfig
	Models   ModelsConfig
	Warnings []string
}

type AppConfig struct {
	Env          string
	IsProduction bool
	LogLevel     slog.Level
	LogLevelName string
}

type HTTPConfig struct {
	Host              string
	Port              int
	RequestTimeout    time.Duration
	MaxBodyBytes      int64
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
}

type UpstreamConfig struct {
	BaseURL                string
	APIKey                 string
	ReadinessRequireModels bool
}

type AuthConfig struct {
	APIKey  string
	Enabled bool
}

type LimitsConfig struct {
	RateLimitPerMinute int
	TrustProxyHeaders  bool
}

type CORSConfig struct {
	AllowedOrigins []string
}

type ModelsConfig struct {
	M1                 string
	S1                 string
	V1                 string
	E1                 string
	E1DefaultInputType string
	E1DefaultModality  string
}

func (c HTTPConfig) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Load reads .env when present (without overriding existing environment
// variables) and then parses the process environment.
func Load() (Config, error) {
	_ = godotenv.Load()
	return Parse(os.LookupEnv)
}

// Parse builds a configuration from a lookup function. It is exported so the
// configuration can be tested deterministically without touching the process
// environment.
func Parse(lookup func(string) (string, bool)) (Config, error) {
	env := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
		return fallback
	}

	appEnv := strings.ToLower(env("APP_ENV", "development"))
	if appEnv != "development" && appEnv != "production" {
		return Config{}, fmt.Errorf("invalid APP_ENV: must be development or production")
	}

	logLevelName := strings.ToLower(env("LOG_LEVEL", "info"))
	logLevel, err := parseLogLevel(logLevelName)
	if err != nil {
		return Config{}, err
	}

	port, err := envInt(lookup, "PORT", DefaultPort)
	if err != nil {
		return Config{}, err
	}
	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("invalid PORT: must be between 1 and 65535")
	}

	host := env("HOST", DefaultHost)
	if err := validateHost(host); err != nil {
		return Config{}, err
	}

	timeoutSeconds, err := envInt(lookup, "REQUEST_TIMEOUT_SECONDS", int(DefaultRequestTimeout/time.Second))
	if err != nil {
		return Config{}, err
	}
	if timeoutSeconds < 1 || timeoutSeconds > 3600 {
		return Config{}, fmt.Errorf("invalid REQUEST_TIMEOUT_SECONDS: must be between 1 and 3600")
	}

	maxBody, err := envInt64(lookup, "MAX_REQUEST_BODY_BYTES", DefaultMaxBodyBytes)
	if err != nil {
		return Config{}, err
	}
	if maxBody < 1024 || maxBody > 1<<30 {
		return Config{}, fmt.Errorf("invalid MAX_REQUEST_BODY_BYTES: must be between 1024 and 1073741824")
	}

	rateLimit, err := envInt(lookup, "RATE_LIMIT_PER_MINUTE", DefaultRateLimitPerMinute)
	if err != nil {
		return Config{}, err
	}
	if rateLimit < 0 || rateLimit > 1000000 {
		return Config{}, fmt.Errorf("invalid RATE_LIMIT_PER_MINUTE: must be between 0 and 1000000")
	}

	trustProxy, err := envBool(lookup, "TRUST_PROXY_HEADERS", false)
	if err != nil {
		return Config{}, err
	}

	requireModels, err := envBool(lookup, "READINESS_REQUIRE_MODELS", false)
	if err != nil {
		return Config{}, err
	}

	baseURL, err := validateBaseURL(env("NINE_ROUTER_BASE_URL", DefaultNineRouterBaseURL))
	if err != nil {
		return Config{}, err
	}

	originalInputType := strings.ToLower(env("DEEPSI_E1_DEFAULT_INPUT_TYPE", "query"))
	if originalInputType != "query" && originalInputType != "passage" {
		return Config{}, fmt.Errorf("invalid DEEPSI_E1_DEFAULT_INPUT_TYPE: must be query or passage")
	}

	origins, err := parseOrigins(env("CORS_ALLOWED_ORIGINS", ""))
	if err != nil {
		return Config{}, err
	}

	gatewayKey := env("LLM_GATEWAY_API_KEY", "")
	upstreamKey := env("NINE_ROUTER_API_KEY", "")

	cfg := Config{
		App: AppConfig{
			Env:          appEnv,
			IsProduction: appEnv == "production",
			LogLevel:     logLevel,
			LogLevelName: logLevelName,
		},
		HTTP: HTTPConfig{
			Host:              host,
			Port:              port,
			RequestTimeout:    time.Duration(timeoutSeconds) * time.Second,
			MaxBodyBytes:      maxBody,
			ShutdownTimeout:   DefaultShutdownTimeout,
			ReadHeaderTimeout: DefaultReadHeaderTimeout,
		},
		Upstream: UpstreamConfig{
			BaseURL:                baseURL,
			APIKey:                 upstreamKey,
			ReadinessRequireModels: requireModels,
		},
		Auth: AuthConfig{
			APIKey:  gatewayKey,
			Enabled: gatewayKey != "",
		},
		Limits: LimitsConfig{
			RateLimitPerMinute: rateLimit,
			TrustProxyHeaders:  trustProxy,
		},
		CORS: CORSConfig{AllowedOrigins: origins},
		Models: ModelsConfig{
			M1:                 env("DEEPSI_M1_UPSTREAM_MODEL", "nvidia/nemotron-3.5-super-vl-120b-a12b"),
			S1:                 env("DEEPSI_S1_UPSTREAM_MODEL", "stepfun-ai/step-5-preview"),
			V1:                 env("DEEPSI_V1_UPSTREAM_MODEL", "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning"),
			E1:                 env("DEEPSI_E1_UPSTREAM_MODEL", "nvidia/nemotron-3.5-embed-8b"),
			E1DefaultInputType: originalInputType,
			E1DefaultModality:  strings.TrimSpace(env("DEEPSI_E1_DEFAULT_MODALITY", "")),
		},
	}

	for _, model := range []string{cfg.Models.M1, cfg.Models.S1, cfg.Models.V1, cfg.Models.E1} {
		if model == "" {
			return Config{}, fmt.Errorf("upstream model identifiers must not be empty")
		}
		if len(model) > 256 {
			return Config{}, fmt.Errorf("upstream model identifier is too long (max 256 characters)")
		}
	}

	if cfg.App.IsProduction {
		if !cfg.Auth.Enabled {
			return Config{}, fmt.Errorf("LLM_GATEWAY_API_KEY is required when APP_ENV=production")
		}
		if len(cfg.Auth.APIKey) < MinimumProductionGatewayKeyLen {
			return Config{}, fmt.Errorf("LLM_GATEWAY_API_KEY must be at least %d characters in production", MinimumProductionGatewayKeyLen)
		}
	} else if !cfg.Auth.Enabled {
		cfg.Warnings = append(cfg.Warnings,
			"LLM_GATEWAY_API_KEY is empty; gateway authentication is disabled because APP_ENV=development")
	}

	if upstreamKey == "" && !isLoopbackURL(baseURL) {
		cfg.Warnings = append(cfg.Warnings,
			"NINE_ROUTER_API_KEY is empty while NINE_ROUTER_BASE_URL is not loopback; configure 9Router authentication before production use")
	}
	if !isLoopbackURL(baseURL) {
		cfg.Warnings = append(cfg.Warnings,
			"NINE_ROUTER_BASE_URL is not loopback; restrict 9Router to a private network and review its security advisory status")
	}

	if duplicate := firstDuplicate([]string{cfg.Models.M1, cfg.Models.S1, cfg.Models.V1, cfg.Models.E1}); duplicate != "" {
		cfg.Warnings = append(cfg.Warnings,
			"multiple public DeepSI models use the same upstream model identifier; check the DEEPSI_*_UPSTREAM_MODEL values")
	}

	return cfg, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL: must be debug, info, warn, or error")
	}
}

func envInt(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be an integer", key)
	}
	return value, nil
}

func envInt64(lookup func(string) (string, bool), key string, fallback int64) (int64, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be an integer", key)
	}
	return value, nil
}

func envBool(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("invalid %s: must be a boolean", key)
	}
	return value, nil
}

func validateHost(host string) error {
	if host == "" {
		return fmt.Errorf("invalid HOST: must not be empty")
	}
	if strings.ContainsAny(host, " \t\r\n/?#@") {
		return fmt.Errorf("invalid HOST: must be a hostname or IP address")
	}
	return nil
}

func validateBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid NINE_ROUTER_BASE_URL: must be an absolute http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid NINE_ROUTER_BASE_URL: scheme must be http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("invalid NINE_ROUTER_BASE_URL: embedded credentials are not allowed")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid NINE_ROUTER_BASE_URL: query strings and fragments are not allowed")
	}
	if parsed.Path != "" && !strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("invalid NINE_ROUTER_BASE_URL: path must be absolute")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func parseOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	origins := make([]string, 0, 4)
	for _, candidate := range strings.Split(raw, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == "*" {
			return nil, fmt.Errorf("invalid CORS_ALLOWED_ORIGINS: wildcard origins are not allowed; list explicit origins")
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid CORS_ALLOWED_ORIGINS: each entry must be an absolute origin")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("invalid CORS_ALLOWED_ORIGINS: scheme must be http or https")
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, fmt.Errorf("invalid CORS_ALLOWED_ORIGINS: entries must be origins without paths, queries, or credentials")
		}
		origin := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
}

func isLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func firstDuplicate(values []string) string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return value
		}
		seen[value] = struct{}{}
	}
	return ""
}
