# DeepSI LLM API Gateway — API Reference

Base URL: `http://127.0.0.1:8085`

All `/v1` endpoints require:

```http
Authorization: Bearer <LLM_GATEWAY_API_KEY>
```

Health endpoints are public. Responses use OpenAI-compatible JSON structures and errors.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness only. |
| `GET` | `/readyz` | 9Router reachability and configured-model availability. |
| `GET` | `/v1/models` | Public DeepSI model list. |
| `POST` | `/v1/chat/completions` | Chat completions, streaming, tools, multimodal payloads. |
| `POST` | `/v1/embeddings` | Embeddings through `DeepSI-E1`. |

## Health

`GET /healthz` → `200 OK`

```json
{
  "status": "ok",
  "service": "deepsi-llm-api-gateway",
  "version": "0.1.0",
  "uptime_seconds": 42
}
```

`GET /readyz` checks 9Router `/v1/models` with a 5-second timeout. It never performs a generation call.

Ready with all configured models present:

```json
{
  "status": "ok",
  "service": "deepsi-llm-api-gateway",
  "checks": {
    "upstream": {"ok": true, "model_count": 4},
    "models": {"ok": true}
  }
}
```

Reachable but missing models (default non-strict mode, HTTP 200):

```json
{
  "status": "degraded",
  "service": "deepsi-llm-api-gateway",
  "checks": {
    "upstream": {"ok": true, "model_count": 1},
    "models": {"ok": false, "missing": ["DeepSI-S1", "DeepSI-V1", "DeepSI-E1"]}
  }
}
```

Upstream unreachable, or strict mode with missing models → `503` and `"status": "unready"`. Readyz responses never include upstream model IDs or the internal 9Router URL.

## Models

`GET /v1/models` → `200 OK`

```json
{
  "object": "list",
  "data": [
    {"id": "DeepSI-M1", "object": "model", "created": 1767225600, "owned_by": "deepsi"},
    {"id": "DeepSI-S1", "object": "model", "created": 1767225600, "owned_by": "deepsi"},
    {"id": "DeepSI-V1", "object": "model", "created": 1767225600, "owned_by": "deepsi"},
    {"id": "DeepSI-E1", "object": "model", "created": 1767225600, "owned_by": "deepsi"}
  ]
}
```

Public model IDs are matched case-insensitively. Upstream provider IDs are not accepted as aliases.

## Chat completions

`POST /v1/chat/completions`

Supported public models: `DeepSI-M1`, `DeepSI-S1`, `DeepSI-V1`.

The gateway forwards the OpenAI-compatible request body after:

1. Resolving the public model to the configured upstream ID.
2. Prepending the server-managed DeepSI system instruction.
3. Converting caller-supplied `system`/`developer` messages into subordinate user-level context.
4. Preserving all other fields, including tools, tool calls, multimodal content, and sampling parameters.

Example:

```bash
curl -sS http://127.0.0.1:8085/v1/chat/completions \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "DeepSI-M1",
    "messages": [
      {"role": "user", "content": "Write a Go function that reverses a string."}
    ],
    "temperature": 0.2,
    "max_tokens": 512,
    "stream": false
  }'
```

Response model fields are normalized to the public DeepSI ID:

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "created": 1767225600,
  "model": "DeepSI-M1",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "..."},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 12, "completion_tokens": 80, "total_tokens": 92}
}
```

### Streaming

Set `"stream": true`. The gateway relays SSE incrementally and flushes each line. `data: [DONE]` is preserved. Every `data:` chunk with a `model` field is normalized to the public DeepSI ID.

```bash
curl -sS -N http://127.0.0.1:8085/v1/chat/completions \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"DeepSI-S1","stream":true,"messages":[{"role":"user","content":"2+2?"}]}'
```

Client cancellation propagates to the upstream request. The gateway does not buffer the whole response before responding.

### Identity merging policy

- Exactly one `system` message is sent upstream: the selected model's DeepSI identity prompt.
- Caller `system` and `developer` messages are changed to `user` and prefixed with an explicit subordinate-context marker.
- A caller cannot inject an additional system message or place a conflicting system instruction after the enforced prompt.
- `DeepSI-E1` receives no chat identity prompt.

## Embeddings

`POST /v1/embeddings`

Supported public model: `DeepSI-E1`.

```bash
curl -sS http://127.0.0.1:8085/v1/embeddings \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "DeepSI-E1",
    "input": ["first document", "second document"],
    "input_type": "passage",
    "dimensions": 1024
  }'
```

Rules:

- `model` must resolve to `DeepSI-E1`; chat models return `400 unsupported_model_for_endpoint`.
- `input` is required and must be a non-empty string or array.
- `input_type` is optional. When omitted, `DEEPSI_E1_DEFAULT_INPUT_TYPE` (default `query`) is applied. Only `query` and `passage` are accepted.
- `modality` is forwarded when supplied. If omitted and `DEEPSI_E1_DEFAULT_MODALITY` is configured, that default is applied.
- Other OpenAI-compatible embedding parameters (`dimensions`, `encoding_format`, etc.) are preserved.
- No chat system prompt or `messages` field is sent upstream.
- The response `model` is normalized to `DeepSI-E1`.

## Errors

OpenAI-compatible envelope:

```json
{
  "error": {
    "message": "The model 'gpt-4o' does not exist. Available models: DeepSI-M1, DeepSI-S1, DeepSI-V1, DeepSI-E1.",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_found"
  }
}
```

| Status | Typical code | Meaning |
|---|---|---|
| `400` | `invalid_request`, `unsupported_model_for_endpoint`, `upstream_request_rejected` | Invalid body/model/endpoint combination, or upstream rejected the request. |
| `401` | `invalid_api_key` | Missing or invalid gateway bearer token. |
| `404` | `model_not_found`, `not_found` | Unknown public model or unknown API path. |
| `405` | `method_not_allowed` | Wrong HTTP method; `Allow` header is set. |
| `413` | `request_too_large`, `upstream_request_too_large` | Body exceeds gateway or upstream limits. |
| `429` | `rate_limit_exceeded`, `upstream_rate_limited` | Gateway or upstream rate limit. `Retry-After` is preserved for upstream limits. |
| `500` | `gateway_error` | Internal gateway error. |
| `502` | `upstream_unreachable`, `upstream_authentication_error`, `upstream_model_unavailable`, `upstream_error` | 9Router/upstream failure. Raw provider errors are not exposed. |
| `503` | — | `/readyz` only: gateway cannot reach 9Router or strict readiness is failing. This endpoint returns a plain status object, not an error envelope. |
| `504` | `upstream_timeout` | Upstream generation exceeded `REQUEST_TIMEOUT_SECONDS`. |

`X-Request-ID` is returned on every response. Provide a valid `X-Request-ID` to have it echoed; otherwise the gateway generates one.

## Authentication and limits

- Bearer authentication is required for all `/v1` endpoints in production and whenever `LLM_GATEWAY_API_KEY` is set.
- In development only, an empty key disables authentication and logs a warning.
- `RATE_LIMIT_PER_MINUTE` applies per caller credential hash when an `Authorization` header is present, otherwise per remote IP. `0` disables rate limiting.
- `MAX_REQUEST_BODY_BYTES` limits inbound request bodies.
- CORS is disabled unless `CORS_ALLOWED_ORIGINS` is configured. Wildcards are rejected.
