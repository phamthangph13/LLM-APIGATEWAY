package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

func (g *Gateway) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	payload, ok := readJSONObject(w, r)
	if !ok {
		return
	}

	model, ok := g.resolveModel(w, payload, models.KindChat)
	if !ok {
		return
	}

	if _, err := boolField(payload, "stream"); err != nil {
		writeValidationError(w, err)
		return
	}

	messages, err := messagesFromPayload(payload)
	if err != nil {
		writeValidationError(w, err)
		return
	}

	merged, err := mergeIdentity(model, messages)
	if err != nil {
		writeValidationError(w, err)
		return
	}

	payload["model"] = model.UpstreamID
	payload["messages"] = merged

	body, err := marshalJSON(payload)
	if err != nil {
		g.log.Error("encode request payload", "error", err, "request_id", httpx.RequestID(r.Context()))
		httpx.WriteOpenAIError(w, http.StatusInternalServerError, "gateway_error",
			"The gateway could not prepare the upstream request.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.HTTP.RequestTimeout)
	defer cancel()

	response, err := g.upstream.ChatCompletions(ctx, body)
	if err != nil {
		g.writeTransportError(w, r, err)
		return
	}

	g.forwardResponse(w, r, response, model)
}

func (g *Gateway) writeTransportError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		g.log.Debug("client canceled upstream request", "request_id", httpx.RequestID(r.Context()))
		return
	}

	requestID := httpx.RequestID(r.Context())
	if isTimeout(err) {
		g.log.Warn("upstream request timed out", "error", err, "request_id", requestID)
		httpx.WriteOpenAIError(w, http.StatusGatewayTimeout, "upstream_timeout",
			"The upstream model service timed out.")
		return
	}

	g.log.Error("upstream request failed", "error", err, "request_id", requestID)
	httpx.WriteOpenAIError(w, http.StatusBadGateway, "upstream_unreachable",
		"The upstream model service is unreachable.")
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netError net.Error
	return errors.As(err, &netError) && netError.Timeout()
}
