package gateway

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
)

const maxUpstreamResponseBytes = 64 << 20
const maxSSELineBytes = 8 << 20

func (g *Gateway) forwardResponse(w http.ResponseWriter, r *http.Request, response *http.Response, model *models.Model) {
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		g.writeUpstreamError(w, r, response, model)
		return
	}

	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		g.streamResponse(w, r, response, model)
		return
	}

	if response.StatusCode == http.StatusNoContent {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(response.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxUpstreamResponseBytes+1))
	if err != nil {
		g.log.Warn("read upstream response failed", "error", err, "request_id", httpx.RequestID(r.Context()))
		httpx.WriteOpenAIError(w, http.StatusBadGateway, "upstream_error",
			"The gateway could not read the upstream response.")
		return
	}
	if len(body) > maxUpstreamResponseBytes {
		g.log.Warn("upstream response exceeded size limit", "request_id", httpx.RequestID(r.Context()))
		httpx.WriteOpenAIError(w, http.StatusBadGateway, "upstream_response_too_large",
			"The upstream response exceeded the gateway size limit.")
		return
	}

	normalized, err := normalizeModel(body, model.ID)
	if err != nil {
		g.log.Warn("upstream response was not a JSON object", "error", err, "request_id", httpx.RequestID(r.Context()))
		httpx.WriteOpenAIError(w, http.StatusBadGateway, "upstream_error",
			"The upstream model service returned an invalid response.")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	if _, err := w.Write(normalized); err != nil {
		g.log.Debug("write client response failed", "error", err, "request_id", httpx.RequestID(r.Context()))
	}
}

func (g *Gateway) streamResponse(w http.ResponseWriter, r *http.Request, response *http.Response, model *models.Model) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteOpenAIError(w, http.StatusInternalServerError, "gateway_error",
			"Streaming is not supported by the server configuration.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	flusher.Flush()

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), maxSSELineBytes)
	requestID := httpx.RequestID(r.Context())

	for scanner.Scan() {
		if r.Context().Err() != nil {
			return
		}

		line := scanner.Text()
		output := line
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload != "" && payload != "[DONE]" {
				normalized, err := normalizeModel([]byte(payload), model.ID)
				if err != nil {
					g.log.Warn("malformed upstream SSE data", "error", err, "request_id", requestID)
					g.writeSSEError(w, flusher)
					return
				}
				output = "data: " + string(normalized)
			}
		}

		if _, err := io.WriteString(w, output+"\n"); err != nil {
			return
		}
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil && !isContextDone(r.Context()) {
		g.log.Warn("upstream SSE read failed", "error", err, "request_id", requestID)
	}
}

func (g *Gateway) writeSSEError(w http.ResponseWriter, flusher http.Flusher) {
	payload, err := marshalJSON(map[string]any{
		"error": map[string]any{
			"message": "The upstream model service returned a malformed stream.",
			"type":    "server_error",
			"code":    "upstream_stream_error",
		},
	})
	if err == nil {
		_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
	}
	flusher.Flush()
}

func normalizeModel(body []byte, publicID string) ([]byte, error) {
	value, err := decodeJSONValue(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("upstream response must be a JSON object")
	}
	object["model"] = publicID
	return marshalJSON(object)
}

func isContextDone(ctx context.Context) bool {
	return ctx.Err() != nil
}
