package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

const maxUpstreamErrorBytes = 1 << 20

func (g *Gateway) writeUpstreamError(w http.ResponseWriter, r *http.Request, response *http.Response, model *models.Model) {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxUpstreamErrorBytes))
	requestID := httpx.RequestID(r.Context())

	g.log.Warn("upstream returned an error",
		"status", response.StatusCode,
		"model", model.ID,
		"request_id", requestID,
	)

	clientStatus, code, message, includeDetail := classifyUpstreamError(response.StatusCode)
	if includeDetail {
		if detail := g.sanitizer.Sanitize(extractUpstreamMessage(body)); detail != "" {
			message += " " + detail
		}
	}

	if response.StatusCode == http.StatusTooManyRequests {
		if retryAfter := sanitizeRetryAfter(response.Header.Get("Retry-After")); retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
	}

	httpx.WriteError(w, clientStatus, httpx.ErrorTypeForStatus(clientStatus), code, message)
}

func classifyUpstreamError(status int) (clientStatus int, code, message string, includeDetail bool) {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return http.StatusBadRequest, "upstream_request_rejected",
			"The upstream model rejected the request.", true
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusBadGateway, "upstream_authentication_error",
			"The upstream model service rejected the gateway credentials.", false
	case http.StatusNotFound:
		return http.StatusBadGateway, "upstream_model_unavailable",
			"The configured upstream model is unavailable.", false
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return http.StatusGatewayTimeout, "upstream_timeout",
			"The upstream model service timed out.", false
	case http.StatusRequestEntityTooLarge:
		return http.StatusRequestEntityTooLarge, "upstream_request_too_large",
			"The request is too large for the upstream model service.", false
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests, "upstream_rate_limited",
			"The upstream model service is rate limiting requests.", true
	default:
		return http.StatusBadGateway, "upstream_error",
			"The upstream model service failed to process the request.", false
	}
}

func extractUpstreamMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}

	if nested, ok := envelope["error"].(map[string]any); ok {
		if message, ok := nested["message"].(string); ok {
			return message
		}
	}
	for _, key := range []string{"message", "detail"} {
		if message, ok := envelope[key].(string); ok {
			return message
		}
	}
	return ""
}

func sanitizeRetryAfter(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 || seconds > 86400 {
		return ""
	}
	return strconv.Itoa(seconds)
}
