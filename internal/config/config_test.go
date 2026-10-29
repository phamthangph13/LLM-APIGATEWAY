package config

import (
	"log/slog"
	"strings"
	"testing"
)

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(lookupFrom(nil))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.App.Env != "development" || cfg.App.IsProduction {
		t.Fatalf("unexpected app environment: %+v", cfg.App)
	}
	if cfg.App.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected log level: %v", cfg.App.LogLevel)
	}
	if cfg.HTTP.Port != 8085 || cfg.HTTP.Host != "127.0.0.1" {
		t.Fatalf("unexpected HTTP address: %s", cfg.HTTP.Addr())
	}
	if cfg.Upstream.BaseURL != DefaultNineRouterBaseURL {
		t.Fatalf("unexpected upstream URL: %s", cfg.Upstream.BaseURL)
	}
	if cfg.Models.M1 != "nvidia/nemotron-3.5-super-vl-120b-a12b" {
		t.Fatalf("unexpected M1 mapping: %s", cfg.Models.M1)
	}
	if cfg.Models.S1 != "nvidia/z-ai/glm4.7" {
		t.Fatalf("unexpected S1 mapping: %s", cfg.Models.S1)
	}
	if cfg.Models.V1 != "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning" {
		t.Fatalf("unexpected V1 mapping: %s", cfg.Models.V1)
	}
	if cfg.Models.E1 != "nvidia/nemotron-3.5-embed-8b" {
		t.Fatalf("unexpected E1 mapping: %s", cfg.Models.E1)
	}
	if cfg.Auth.Enabled {
		t.Fatal("development mode without a key must not require authentication")
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("expected a warning about disabled authentication in development")
	}
}

func TestParseOverrides(t *testing.T) {
	cfg, err := Parse(lookupFrom(map[string]string{
		"APP_ENV":                      "production",
		"HOST":                         "0.0.0.0",
		"PORT":                         "18087",
		"LOG_LEVEL":                    "debug",
		"NINE_ROUTER_BASE_URL":         "https://router.internal:9443/v1/",
		"NINE_ROUTER_API_KEY":          "router-secret",
		"LLM_GATEWAY_API_KEY":          "gateway-secret-123456",
		"REQUEST_TIMEOUT_SECONDS":      "30",
		"MAX_REQUEST_BODY_BYTES":       "1048576",
		"RATE_LIMIT_PER_MINUTE":        "0",
		"READINESS_REQUIRE_MODELS":     "true",
		"CORS_ALLOWED_ORIGINS":         "https://app.example.com, https://admin.example.com/",
		"DEEPSI_M1_UPSTREAM_MODEL":     "nvidia/custom-m1",
		"DEEPSI_S1_UPSTREAM_MODEL":     "nvidia/custom-s1",
		"DEEPSI_V1_UPSTREAM_MODEL":     "nvidia/custom-v1",
		"DEEPSI_E1_UPSTREAM_MODEL":     "nvidia/custom-e1",
		"DEEPSI_E1_DEFAULT_INPUT_TYPE": "passage",
		"DEEPSI_E1_DEFAULT_MODALITY":   "text",
		"TRUST_PROXY_HEADERS":          "true",
	}))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !cfg.App.IsProduction || cfg.App.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected app config: %+v", cfg.App)
	}
	if cfg.HTTP.Addr() != "0.0.0.0:18087" {
		t.Fatalf("unexpected addr: %s", cfg.HTTP.Addr())
	}
	if cfg.Upstream.BaseURL != "https://router.internal:9443/v1" {
		t.Fatalf("unexpected upstream URL: %s", cfg.Upstream.BaseURL)
	}
	if !cfg.Auth.Enabled || cfg.Upstream.APIKey != "router-secret" {
		t.Fatal("secrets were not loaded")
	}
	if !cfg.Upstream.ReadinessRequireModels {
		t.Fatal("readiness strict mode was not loaded")
	}
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Fatalf("unexpected CORS origins: %#v", cfg.CORS.AllowedOrigins)
	}
	if cfg.Models.E1DefaultInputType != "passage" || cfg.Models.E1DefaultModality != "text" {
		t.Fatalf("unexpected embedding defaults: %+v", cfg.Models)
	}
	if !cfg.Limits.TrustProxyHeaders {
		t.Fatal("trust proxy setting was not loaded")
	}
}

func TestParseValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		wantSub string
	}{
		{
			name:    "invalid app env",
			values:  map[string]string{"APP_ENV": "staging"},
			wantSub: "APP_ENV",
		},
		{
			name:    "invalid port",
			values:  map[string]string{"PORT": "abc"},
			wantSub: "PORT",
		},
		{
			name:    "port out of range",
			values:  map[string]string{"PORT": "70000"},
			wantSub: "PORT",
		},
		{
			name:    "invalid base URL",
			values:  map[string]string{"NINE_ROUTER_BASE_URL": "not-a-url"},
			wantSub: "NINE_ROUTER_BASE_URL",
		},
		{
			name:    "base URL with credentials",
			values:  map[string]string{"NINE_ROUTER_BASE_URL": "http://user:pass@localhost:20128/v1"},
			wantSub: "credentials",
		},
		{
			name:    "base URL with query",
			values:  map[string]string{"NINE_ROUTER_BASE_URL": "http://localhost:20128/v1?x=1"},
			wantSub: "query",
		},
		{
			name:    "invalid timeout",
			values:  map[string]string{"REQUEST_TIMEOUT_SECONDS": "0"},
			wantSub: "REQUEST_TIMEOUT_SECONDS",
		},
		{
			name:    "invalid log level",
			values:  map[string]string{"LOG_LEVEL": "verbose"},
			wantSub: "LOG_LEVEL",
		},
		{
			name:    "wildcard CORS",
			values:  map[string]string{"CORS_ALLOWED_ORIGINS": "*"},
			wantSub: "wildcard",
		},
		{
			name:    "invalid CORS origin",
			values:  map[string]string{"CORS_ALLOWED_ORIGINS": "not an origin"},
			wantSub: "CORS_ALLOWED_ORIGINS",
		},
		{
			name:    "invalid E1 input type",
			values:  map[string]string{"DEEPSI_E1_DEFAULT_INPUT_TYPE": "document"},
			wantSub: "DEEPSI_E1_DEFAULT_INPUT_TYPE",
		},
		{
			name:    "production without gateway key",
			values:  map[string]string{"APP_ENV": "production"},
			wantSub: "LLM_GATEWAY_API_KEY",
		},
		{
			name: "production with short gateway key",
			values: map[string]string{
				"APP_ENV":             "production",
				"LLM_GATEWAY_API_KEY": "short",
			},
			wantSub: "at least",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(lookupFrom(test.values))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), test.wantSub)
			}
		})
	}
}

func TestParseWarnings(t *testing.T) {
	cfg, err := Parse(lookupFrom(map[string]string{
		"NINE_ROUTER_BASE_URL":     "https://router.example.com/v1",
		"DEEPSI_E1_UPSTREAM_MODEL": "nvidia/nemotron-3.5-super-vl-120b-a12b",
	}))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	joined := strings.Join(cfg.Warnings, " | ")
	if !strings.Contains(joined, "not loopback") {
		t.Fatalf("expected non-loopback warning, got %q", joined)
	}
	if !strings.Contains(joined, "same upstream") {
		t.Fatalf("expected duplicate upstream warning, got %q", joined)
	}
}
