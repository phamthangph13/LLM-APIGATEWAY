package gateway

import (
	"regexp"
	"strings"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

type replacement struct {
	pattern     *regexp.Regexp
	replacement string
}

type textSanitizer struct {
	literals []literalReplacement
	patterns []replacement
	maxRunes int
}

type literalReplacement struct {
	value       string
	replacement string
}

func newTextSanitizer(cfg *config.Config, registry *models.Registry) textSanitizer {
	sanitizer := textSanitizer{maxRunes: 400}

	for _, model := range registry.List() {
		if model.UpstreamID == "" {
			continue
		}
		sanitizer.literals = append(sanitizer.literals, literalReplacement{
			value:       model.UpstreamID,
			replacement: model.ID,
		})
		if model.SystemPrompt != "" {
			sanitizer.literals = append(sanitizer.literals, literalReplacement{
				value:       model.SystemPrompt,
				replacement: "[server-managed DeepSI instruction]",
			})
		}
	}

	if cfg.Upstream.BaseURL != "" {
		sanitizer.literals = append(sanitizer.literals,
			literalReplacement{value: cfg.Upstream.BaseURL, replacement: "[upstream]"},
			literalReplacement{value: cfg.Upstream.BaseURL + "/", replacement: "[upstream]/"},
		)
	}
	for _, secret := range []string{cfg.Upstream.APIKey, cfg.Auth.APIKey} {
		if len(secret) >= 4 {
			sanitizer.literals = append(sanitizer.literals, literalReplacement{
				value:       secret,
				replacement: "[redacted]",
			})
		}
	}

	sanitizer.patterns = []replacement{
		{
			pattern:     regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
			replacement: "$1[redacted]",
		},
		{
			pattern:     regexp.MustCompile(`(?i)\b(?:sk|nvapi)-[A-Za-z0-9._-]+`),
			replacement: "[redacted]",
		},
	}

	return sanitizer
}

func (s textSanitizer) Sanitize(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}

	message = strings.NewReplacer(
		"\r", " ",
		"\n", " ",
		"\t", " ",
	).Replace(message)

	for _, literal := range s.literals {
		message = strings.ReplaceAll(message, literal.value, literal.replacement)
	}
	for _, pattern := range s.patterns {
		message = pattern.pattern.ReplaceAllString(message, pattern.replacement)
	}

	message = strings.Join(strings.Fields(message), " ")
	if runes := []rune(message); len(runes) > s.maxRunes {
		message = string(runes[:s.maxRunes]) + "..."
	}
	return message
}
