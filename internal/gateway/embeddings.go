package gateway

import (
	"context"
	"net/http"
	"strings"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

func (g *Gateway) Embeddings(w http.ResponseWriter, r *http.Request) {
	payload, ok := readJSONObject(w, r)
	if !ok {
		return
	}

	model, ok := g.resolveModel(w, payload, models.KindEmbeddings)
	if !ok {
		return
	}

	if err := validateEmbeddingInput(payload); err != nil {
		writeValidationError(w, err)
		return
	}

	inputType, err := embeddingInputType(payload, g.cfg.Models.E1DefaultInputType)
	if err != nil {
		writeValidationError(w, err)
		return
	}
	if inputType != "" {
		payload["input_type"] = inputType
	}

	modality, err := embeddingModality(payload, g.cfg.Models.E1DefaultModality)
	if err != nil {
		writeValidationError(w, err)
		return
	}
	if modality != "" {
		payload["modality"] = modality
	}

	payload["model"] = model.UpstreamID

	body, err := marshalJSON(payload)
	if err != nil {
		g.log.Error("encode embeddings payload", "error", err, "request_id", httpx.RequestID(r.Context()))
		httpx.WriteOpenAIError(w, http.StatusInternalServerError, "gateway_error",
			"The gateway could not prepare the upstream request.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.HTTP.RequestTimeout)
	defer cancel()

	response, err := g.upstream.Embeddings(ctx, body)
	if err != nil {
		g.writeTransportError(w, r, err)
		return
	}

	g.forwardResponse(w, r, response, model)
}

func validateEmbeddingInput(payload map[string]any) error {
	raw, exists := payload["input"]
	if !exists || raw == nil {
		return &requestValidationError{message: "You must provide an input parameter.", param: "input"}
	}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return &requestValidationError{message: "The input parameter must not be empty.", param: "input"}
		}
	case []any:
		if len(value) == 0 {
			return &requestValidationError{message: "The input array must not be empty.", param: "input"}
		}
	default:
		return &requestValidationError{
			message: "The input parameter must be a string or an array.",
			param:   "input",
		}
	}
	return nil
}

func embeddingInputType(payload map[string]any, fallback string) (string, error) {
	raw, exists := payload["input_type"]
	if !exists || raw == nil {
		return fallback, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", &requestValidationError{
			message: "The input_type parameter must be \"query\" or \"passage\".",
			param:   "input_type",
		}
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "query" && value != "passage" {
		return "", &requestValidationError{
			message: "The input_type parameter must be \"query\" or \"passage\".",
			param:   "input_type",
		}
	}
	return value, nil
}

func embeddingModality(payload map[string]any, fallback string) (string, error) {
	raw, exists := payload["modality"]
	if !exists || raw == nil {
		return fallback, nil
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &requestValidationError{
			message: "The modality parameter must be a non-empty string.",
			param:   "modality",
		}
	}
	return strings.TrimSpace(value), nil
}
