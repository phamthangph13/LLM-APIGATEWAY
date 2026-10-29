package gateway

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

func (g *Gateway) resolveModel(w http.ResponseWriter, payload map[string]any, expected models.Kind) (*models.Model, bool) {
	name, err := modelFromPayload(payload)
	if err != nil {
		writeValidationError(w, err)
		return nil, false
	}

	model, ok := g.registry.Resolve(name)
	if !ok {
		httpx.WriteErrorWithParam(w, http.StatusNotFound, "invalid_request_error", "model_not_found",
			"The model '"+sanitizeModelName(name)+"' does not exist. Available models: "+
				strings.Join(g.registry.PublicIDs(), ", ")+".", "model")
		return nil, false
	}

	if expected != "" && model.Kind != expected {
		httpx.WriteErrorWithParam(w, http.StatusBadRequest, "invalid_request_error",
			"unsupported_model_for_endpoint",
			"The model '"+model.ID+"' does not support this endpoint.", "model")
		return nil, false
	}

	return model, true
}

func sanitizeModelName(name string) string {
	var builder strings.Builder
	for _, character := range name {
		if unicode.IsControl(character) {
			continue
		}
		builder.WriteRune(character)
		if builder.Len() >= 80 {
			break
		}
	}
	return builder.String()
}
