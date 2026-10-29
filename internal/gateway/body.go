package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/deepsi/llm-api-gateway/internal/httpx"
)

func readJSONObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			httpx.WriteOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large",
				"The request body exceeds the configured size limit.")
			return nil, false
		}
		if errors.Is(err, io.EOF) {
			httpx.WriteOpenAIError(w, http.StatusBadRequest, "invalid_request",
				"The request body must be a JSON object.")
			return nil, false
		}
		httpx.WriteOpenAIError(w, http.StatusBadRequest, "invalid_request",
			"The request body is not valid JSON.")
		return nil, false
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		httpx.WriteOpenAIError(w, http.StatusBadRequest, "invalid_request",
			"The request body must contain exactly one JSON object.")
		return nil, false
	}

	if payload == nil {
		httpx.WriteOpenAIError(w, http.StatusBadRequest, "invalid_request",
			"The request body must be a JSON object.")
		return nil, false
	}

	return payload, true
}

func marshalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func decodeJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON data")
	}
	return value, nil
}

func writeValidationError(w http.ResponseWriter, err error) {
	var validation *requestValidationError
	if errors.As(err, &validation) {
		httpx.WriteErrorWithParam(w, http.StatusBadRequest, "invalid_request_error",
			"invalid_request", validation.message, validation.param)
		return
	}
	httpx.WriteOpenAIError(w, http.StatusBadRequest, "invalid_request", "The request is invalid.")
}
