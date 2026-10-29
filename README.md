# DeepSI LLM API Gateway

An independent, OpenAI-compatible Go gateway for the DeepSI model family. It exposes four public DeepSI models, enforces a server-managed DeepSI identity on every chat-capable model, normalizes public model identifiers, and forwards all inference traffic through **9Router**. The gateway never connects directly to NVIDIA, StepFun, or any other AI provider.

## Status and verification

- Mocked-upstream unit and integration tests pass, and the gateway builds and runs under PM2.
- 9Router discovery succeeds at `http://127.0.0.1:20128/v1`.
- **Live DeepSI inference is not available yet.** The installed 9Router instance (0.4.80) currently exposes 35 upstream models, but none of the four required NVIDIA/StepFun model IDs are registered. `/readyz` therefore reports `degraded` until those upstream models are added in 9Router (or the `DEEPSI_*_UPSTREAM_MODEL` mappings are pointed at exact registered IDs).
- 9Router 0.4.80 is affected by [GHSA-86m2-fcxq-5q7c / CVE-2026-55641](https://github.com/advisories/GHSA-86m2-fcxq-5q7c). Upgrade to 9Router 0.5.2 or newer before exposing it beyond loopback. See [Security](#security).

## Architecture

```text
Existing frontend (APIDeepSIService) / backend / external clients
                              |
                              v
                   DeepSI LLM API Gateway :8085
                              |
                    model alias resolution
                    identity prompt injection
                    validation / auth / limits
                              |
                              v
                   9Router :20128 (OpenAI-compatible)
                              |
             +----------------+----------------+
             |                |                |
       DeepSI-M1           DeepSI-S1       DeepSI-V1
       NVIDIA NIM           Step 5         NVIDIA NIM
       Nemotron VL          Preview        Nemotron Omni
             |                |                |
             +----------------+----------------+
                              |
                         DeepSI-E1
                         NVIDIA Embed
```

The gateway is stateless and keeps provider credentials and gateway client credentials separate.

## Public models

| Public model | Default upstream model ID | Endpoint | Role |
|---|---|---|---|
| `DeepSI-M1` | `nvidia/nemotron-3.5-super-vl-120b-a12b` | `/v1/chat/completions` | Fast, versatile chat, coding, reasoning, multimodal |
| `DeepSI-S1` | `stepfun-ai/step-5-preview` | `/v1/chat/completions` | Advanced reasoning and complex problem solving |
| `DeepSI-V1` | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning` | `/v1/chat/completions` | Omni and multimodal reasoning |
| `DeepSI-E1` | `nvidia/nemotron-3.5-embed-8b` | `/v1/embeddings` | Text/document embeddings, not conversational chat |

Upstream IDs are environment-configurable. The defaults above are initial values, not proof that 9Router has registered them. Public identifiers are case-insensitive and are the only model names accepted by the gateway; upstream IDs are never accepted as public aliases.

## Repository layout

```text
LLM-APIGATEWAY/
├── cmd/
│   ├── server/          # gateway process
│   └── doctor/          # 9Router/model-mapping diagnostic CLI
├── internal/
│   ├── config/          # environment loading and validation
│   ├── gateway/         # routing, prompt injection, streaming, response normalization
│   ├── httpx/           # OpenAI-compatible JSON errors and request IDs
│   ├── middleware/      # auth, rate limiting, CORS, body limit, logging, recovery
│   ├── models/          # centralized registry, aliases, identity prompts
│   ├── providers/       # 9Router HTTP client
│   └── server/          # routes, health/readiness, graceful shutdown
├── docs/API.md
├── scripts/build.ps1
├── .env.example
├── Makefile
└── README.md
```

## Workspace integration

This project is intentionally separate from the existing applications:

- `APIDeepSIService/` — existing React/Vite frontend, unchanged.
- `APIDeepSIService-ServerSide/` — existing Go backend on `127.0.0.1:18085`, unchanged.
- `deploy/` — existing static server and Cloudflare tunnel configuration, unchanged.
- `ecosystem.config.cjs` — one new PM2 application entry, `llm-api-gateway`, was added.
- The gateway listens on `127.0.0.1:8085`, which does not conflict with the backend (`18085`), frontend (`18086`), or 9Router (`20128`).

The existing Cloudflare tunnel still routes `service.deepsi.space/v1` to the existing backend. The new gateway is therefore local-only by design. To expose it, add a dedicated hostname or deliberately reroute the tunnel after deciding how the existing backend `/v1` surface should coexist.

## Prerequisites

- Go 1.24 or newer.
- Node.js and PM2 for the production process integration.
- A running 9Router instance with an OpenAI-compatible `/v1` API.
- Provider credentials registered **inside 9Router**: NVIDIA API credentials for the NVIDIA-hosted models, and the appropriate StepFun provider/catalog for Step 5 Preview.

## 9Router setup

The gateway only talks to 9Router. Before expecting live inference:

1. Open the 9Router dashboard (default `http://127.0.0.1:20128`) and add an NVIDIA NIM provider/catalog with a valid NVIDIA API key, plus the provider/catalog that serves Step 5 Preview.
2. Register/enable these exact upstream models, or determine the exact IDs that 9Router reports:
   - `nvidia/nemotron-3.5-super-vl-120b-a12b`
   - `stepfun-ai/step-5-preview`
   - `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning`
   - `nvidia/nemotron-3.5-embed-8b`
3. Verify the catalog:
   ```bash
   curl -sS http://127.0.0.1:20128/v1/models
   ```
4. If 9Router reports different identifiers, override the matching `DEEPSI_*_UPSTREAM_MODEL` values in `.env`.
5. Check the gateway's view of the mappings:
   ```bash
   cd LLM-APIGATEWAY
   go run ./cmd/doctor
   ```
6. Check readiness:
   ```bash
   curl -sS http://127.0.0.1:8085/readyz
   ```

If a model is missing, the gateway does **not** fall back to another model and does not bypass 9Router.

## Configuration

Copy the template and edit it:

```powershell
Copy-Item .env.example .env
```

| Variable | Default | Purpose |
|---|---|---|
| `APP_ENV` | `development` | `development` or `production`. Production requires a gateway API key. |
| `HOST` | `127.0.0.1` | Listen address. |
| `PORT` | `8085` | Listen port. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `NINE_ROUTER_BASE_URL` | `http://127.0.0.1:20128/v1` | 9Router base URL. `/v1` is appended when no path is supplied. |
| `NINE_ROUTER_API_KEY` | empty | Upstream credential sent only to 9Router. |
| `LLM_GATEWAY_API_KEY` | empty | Bearer key required from gateway clients. Required in production (min 16 chars). |
| `REQUEST_TIMEOUT_SECONDS` | `120` | Overall upstream request/stream timeout. |
| `MAX_REQUEST_BODY_BYTES` | `20971520` | Maximum client request body (20 MiB). |
| `RATE_LIMIT_PER_MINUTE` | `60` | Per-client fixed-window limit. `0` disables it. |
| `READINESS_REQUIRE_MODELS` | `false` | When `true`, `/readyz` returns 503 if any configured model is absent from 9Router. |
| `TRUST_PROXY_HEADERS` | `false` | Trust `X-Forwarded-For` for client-IP logging. Enable only behind a trusted proxy. |
| `CORS_ALLOWED_ORIGINS` | empty | Comma-separated exact origins. Wildcards are rejected. |
| `DEEPSI_M1_UPSTREAM_MODEL` | `nvidia/nemotron-3.5-super-vl-120b-a12b` | M1 upstream ID. |
| `DEEPSI_S1_UPSTREAM_MODEL` | `stepfun-ai/step-5-preview` | S1 upstream ID. |
| `DEEPSI_V1_UPSTREAM_MODEL` | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning` | V1 upstream ID. |
| `DEEPSI_E1_UPSTREAM_MODEL` | `nvidia/nemotron-3.5-embed-8b` | E1 upstream ID. |
| `DEEPSI_E1_DEFAULT_INPUT_TYPE` | `query` | Applied only when an embedding request omits `input_type`. |
| `DEEPSI_E1_DEFAULT_MODALITY` | empty | Optional modality default applied only when omitted. |

Never commit `.env`. Generate a strong production gateway key with `openssl rand -base64 48` or a password manager.

## Run locally

```powershell
cd LLM-APIGATEWAY
Copy-Item .env.example .env
# Edit .env: set LLM_GATEWAY_API_KEY (recommended) and verify 9Router/model IDs.
go run ./cmd/server
```

```bash
curl -sS http://127.0.0.1:8085/healthz
curl -sS http://127.0.0.1:8085/readyz
```

In development, an empty `LLM_GATEWAY_API_KEY` disables gateway authentication and emits a startup warning. Production (`APP_ENV=production`) refuses to start without a key of at least 16 characters.

## Build and test

```powershell
cd LLM-APIGATEWAY
gofmt -w .
go vet ./...
go test ./...
go build -trimpath -o bin/llm-api-gateway.exe ./cmd/server
```

or:

```powershell
.\scripts\build.ps1
# or, when make is available:
make build
```

The automated tests use mocked HTTP upstreams. They do not require live NVIDIA/StepFun credentials or paid API access.

## PM2 integration

The entry `llm-api-gateway` is part of the existing root `ecosystem.config.cjs`. It runs the compiled binary from `LLM-APIGATEWAY/bin/llm-api-gateway.exe`, listens on `127.0.0.1:8085`, restarts on failure, and allows graceful shutdown.

Build before starting:

```powershell
cd C:\Users\Admin\Desktop\Workspace\LLM-APIGATEWAY
.\scripts\build.ps1
```

Manage only the gateway (never restart unrelated processes):

```powershell
cd C:\Users\Admin\Desktop\Workspace
pm2 start ecosystem.config.cjs --only llm-api-gateway
pm2 restart llm-api-gateway
pm2 stop llm-api-gateway
pm2 delete llm-api-gateway
pm2 show llm-api-gateway
pm2 logs llm-api-gateway --lines 100
```

`pm2 start ecosystem.config.cjs --only llm-api-gateway` does not start or restart the existing frontend, backend, tunnel, or 9Router entries. Run `pm2 save` if the process list should survive a PM2 daemon restart.

The PM2 environment sets `APP_ENV=production`, `HOST=127.0.0.1`, and `PORT=8085`. Secrets and upstream model overrides stay in `LLM-APIGATEWAY/.env`.

## API

Base URL: `http://127.0.0.1:8085`

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/healthz` | No | Process liveness. |
| `GET` | `/readyz` | No | 9Router reachability and model-mapping readiness. |
| `GET` | `/v1/models` | Yes | Public DeepSI model list. |
| `POST` | `/v1/chat/completions` | Yes | Chat completions with streaming and tool support. |
| `POST` | `/v1/embeddings` | Yes | Embeddings through `DeepSI-E1`. |

Authentication: `Authorization: Bearer <LLM_GATEWAY_API_KEY>`.

### Model discovery

```bash
curl -sS http://127.0.0.1:8085/v1/models \
  -H "Authorization: Bearer $DEEPSI_API_KEY"
```

### Chat completion

```bash
curl -sS http://127.0.0.1:8085/v1/chat/completions \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "DeepSI-M1",
    "messages": [
      {"role": "user", "content": "Explain goroutines in two sentences."}
    ],
    "temperature": 0.3,
    "max_tokens": 256
  }'
```

### Streaming

```bash
curl -sS -N http://127.0.0.1:8085/v1/chat/completions \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "DeepSI-S1",
    "stream": true,
    "messages": [{"role": "user", "content": "Solve 17 * 23 step by step."}]
  }'
```

Streaming responses are relayed incrementally, preserve `[DONE]`, and normalize the model field in every chunk.

### Embeddings

```bash
curl -sS http://127.0.0.1:8085/v1/embeddings \
  -H "Authorization: Bearer $DEEPSI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "DeepSI-E1",
    "input": ["DeepSI platform", "retrieval document"],
    "input_type": "passage"
  }'
```

When `input_type` is omitted, the configured default (`query`) is applied. `modality` is passed through, and the configured default is applied only when omitted. `DeepSI-E1` cannot be used for chat completions, and chat models cannot be used for embeddings.

Full request/response and error details are in [`docs/API.md`](docs/API.md).

## Identity enforcement

Every `/v1/chat/completions` request for `DeepSI-M1`, `DeepSI-S1`, or `DeepSI-V1` is rewritten server-side:

1. The gateway prepends exactly one `system` message containing that model's DeepSI identity prompt.
2. Caller-supplied `system` and `developer` messages are preserved as **user-level application context** with an explicit subordinate marker.
3. All other messages, tool calls, multimodal parts, and parameters are preserved.
4. The transformation is applied before both streaming and non-streaming upstream calls.
5. Embedding requests never receive a chat system prompt.

This is deterministic: callers cannot create a second system message or place a conflicting system message after the enforced identity. Tests cover all three chat models, demotion of caller system/developer messages, multimodal system content, streaming, and response normalization.

## Security

- Gateway client keys and the 9Router upstream key are separate. Upstream credentials are never returned to clients or logged.
- Production requires `LLM_GATEWAY_API_KEY`; comparison uses SHA-256 and constant-time comparison.
- Request bodies are size-limited; JSON and model identifiers are validated.
- Upstream URLs come only from validated environment configuration. Client input cannot override the upstream URL. Redirects are not followed, preventing redirect-based SSRF.
- CORS origins must be explicit; wildcards are rejected.
- Error responses are sanitized: configured upstream model IDs are replaced with public IDs, and API-key patterns/secrets are redacted.
- Prompts, authorization headers, and request bodies are not logged. Request logs contain method, path, status, duration, byte count, client IP, and request ID.
- Optional fixed-window rate limiting is in-memory and keyed by a hash of the `Authorization` header when present, otherwise by remote IP. No database is required.
- The server supports graceful shutdown on interrupt/termination with a 15-second drain window.

### 9Router security warning

The inspected deployment runs 9Router **0.4.80**, which is affected by CVE-2026-55641: unauthenticated `/v1` proxy access via `Host` header spoofing, plus SSRF. It is currently bound to `127.0.0.1`, which limits remote exposure, but you should:

1. Upgrade 9Router to **0.5.2 or newer** before production use.
2. Keep 9Router bound to loopback or a private interface.
3. Enable 9Router API-key authentication for non-loopback access and set `NINE_ROUTER_API_KEY` in the gateway.
4. Restrict access to the 9Router dashboard and port at the host firewall/tunnel layer.

The gateway itself does not use a client-controlled `Host` header for authentication decisions and does not expose 9Router to clients.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `/readyz` returns `degraded` with missing models | One or more required upstream IDs are not registered in 9Router | Register them or override `DEEPSI_*_UPSTREAM_MODEL` with exact IDs from `/v1/models` |
| `/readyz` returns `unready`, upstream unreachable | 9Router is stopped, wrong port, or firewall | Start 9Router, verify `curl http://127.0.0.1:20128/v1/models` |
| `401 invalid_api_key` | Missing/wrong gateway bearer token | Send `Authorization: Bearer <LLM_GATEWAY_API_KEY>` |
| `502 upstream_authentication_error` | 9Router rejected its provider credential | Reconfigure the NVIDIA/StepFun provider credential in 9Router |
| `502 upstream_model_unavailable` | Configured upstream model is absent/rejected by 9Router | Check `go run ./cmd/doctor` and the 9Router catalog |
| `429 upstream_rate_limited` | Provider/9Router rate limit | Respect `Retry-After`; increase provider quota if persistent |
| `504 upstream_timeout` | Generation exceeded `REQUEST_TIMEOUT_SECONDS` | Increase the timeout or reduce request size |
| Gateway will not start in production | Missing or short `LLM_GATEWAY_API_KEY` | Generate a key of at least 16 characters |
| PM2 process restarts immediately | Missing binary or invalid `.env` | Run `.\scripts\build.ps1`, then `pm2 logs llm-api-gateway` |

## Known limitations

- Live DeepSI inference cannot be verified until the four upstream models are registered in 9Router. The test suite validates gateway behavior against mocked upstreams; readiness and catalog discovery were checked against the real local 9Router.
- The existing Cloudflare tunnel has not been rerouted to this gateway, because the existing backend already owns `service.deepsi.space/v1`.
- The gateway is intentionally stateless and has no usage database, billing ledger, or multi-tenant key store. The existing backend owns those concerns.
- 9Router 0.4.80 must be upgraded for a production deployment.
