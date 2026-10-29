package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorBody is the OpenAI-compatible error envelope used by every gateway
// failure path.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param,omitempty"`
	Code    string  `json:"code,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func WriteError(w http.ResponseWriter, status int, errorType, code, message string) {
	WriteErrorWithParam(w, status, errorType, code, message, "")
}

func WriteErrorWithParam(w http.ResponseWriter, status int, errorType, code, message, param string) {
	detail := ErrorDetail{
		Message: message,
		Type:    errorType,
		Code:    code,
	}
	if param != "" {
		detail.Param = &param
	}
	WriteJSON(w, status, ErrorBody{Error: detail})
}

// WriteOpenAIError writes an error with a sensible OpenAI error type derived
// from the HTTP status.
func WriteOpenAIError(w http.ResponseWriter, status int, code, message string) {
	WriteError(w, status, ErrorTypeForStatus(status), code, message)
}

func ErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "server_error"
	default:
		return "invalid_request_error"
	}
}
