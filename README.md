# Switchboard Go

Switchboard Go is a small, dependency-free Go reverse proxy for the
OpenCode Go API. It exposes an OpenAI-compatible local endpoint, protects it
with your own API key, and cycles through multiple upstream OpenCode Go API keys
when one is exhausted.

It is designed for running on a trusted server in your local network so OpenAI-
compatible tools can use one stable base URL while the proxy handles key failover.

## Features

- OpenAI-compatible `/v1/*` reverse proxy
- Works with common OpenAI-compatible clients and tools via `base_url`
- Local proxy authentication with `Authorization: Bearer <PROXY_API_KEY>`
- Upstream OpenCode Go key rotation from `OPENCODE_GO_API_KEYS`
- Automatic failover on quota/usage-exhausted `429` responses
- Cyclic key selection: after the last key, rotation loops back to the first
- Inferred key status endpoint at `/admin/status`
- SMTP notifications when a key is exhausted and when all keys are exhausted
- Streaming-friendly proxying: no full-response timeout on upstream responses
- No third-party Go dependencies
- Single static-ish Go binary deployment

## How it works

Clients call this proxy as if it were an OpenAI-compatible API:

```text
OpenAI-compatible tool -> Switchboard Go -> https://opencode.ai/zen/go/v1
```

Incoming `/v1` paths are stripped before forwarding to the configured upstream
base URL:

```text
GET  /v1/models           -> GET  https://opencode.ai/zen/go/v1/models
POST /v1/chat/completions -> POST https://opencode.ai/zen/go/v1/chat/completions
```

When an upstream key returns a quota/usage-exhausted `429`, the proxy marks that
key as exhausted, sends a best-effort SMTP notification, and retries the same
request with the next non-exhausted key. If every key is exhausted, the proxy
returns an OpenAI-style `429` JSON error.

## Requirements

- Go 1.22 or newer
- One or more OpenCode Go API keys
- Optional SMTP server for notifications

## Install

### From source

```bash
git clone https://github.com/YOUR_USERNAME/switchboard-go.git
cd switchboard-go
go build -o switchboard-go .
```

### Run directly during development

```bash
go run .
```

## Configuration

All configuration is provided through environment variables.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `PROXY_API_KEY` | Yes | | API key clients must use to access this proxy. |
| `OPENCODE_GO_API_KEYS` | Yes | | Comma-separated OpenCode Go API keys. |
| `LISTEN_ADDR` | No | `:8080` | HTTP listen address. Use `127.0.0.1:8080` for local-only access. |
| `UPSTREAM_BASE_URL` | No | `https://opencode.ai/zen/go/v1` | OpenAI-compatible OpenCode Go upstream base URL. |
| `SMTP_HOST` | No | | SMTP host for notifications. |
| `SMTP_PORT` | No | `25` | SMTP port. |
| `SMTP_USERNAME` | No | | SMTP username. If empty, SMTP AUTH is skipped. |
| `SMTP_PASSWORD` | No | | SMTP password. |
| `SMTP_FROM` | No | | Sender email address. Required for notifications. |
| `SMTP_TO` | No | | Recipient email address. Required for notifications. |
| `SMTP_TLS` | No | `false` | Use implicit TLS when connecting to SMTP. |
| `SMTP_STARTTLS` | No | `false` | Use STARTTLS if the server advertises it. |

Notifications are disabled unless `SMTP_HOST`, `SMTP_FROM`, and `SMTP_TO` are
all set.

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"
export LISTEN_ADDR="127.0.0.1:8080"

go run .
```

In another terminal:

```bash
curl http://127.0.0.1:8080/healthz

curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

## OpenAI-compatible client setup

Point your client at the proxy and use the proxy API key, not your upstream
OpenCode Go key.

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

Example chat completion:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "messages": [{"role": "user", "content": "Say hello"}],
    "max_tokens": 100
  }'
```

Streaming requests are proxied as normal HTTP streams:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "stream": true,
    "messages": [{"role": "user", "content": "Write a short poem"}]
  }'
```

## Admin status

`/admin/status` returns inferred key state and the active key index.

```bash
curl http://127.0.0.1:8080/admin/status \
  -H "Authorization: Bearer $PROXY_API_KEY"
```

Example response:

```json
{
  "current_key_index": 1,
  "keys": [
    {
      "index": 0,
      "state": "exhausted",
      "last_429_time": "2026-06-19T11:48:29Z",
      "current": false
    },
    {
      "index": 1,
      "state": "available",
      "current": true
    }
  ],
  "note": "Remaining usage is unavailable from opencode-go API."
}
```

Key states are inferred by this proxy:

- `unknown`: key has not yet been proven available or exhausted
- `available`: key is currently selected and not marked exhausted
- `exhausted`: key returned a quota/usage-exhausted `429`

## Usage and quota visibility

OpenCode Go currently does not appear to expose a public API endpoint for
remaining quota or usage by API key. Switchboard Go therefore cannot show
exact remaining usage. It tracks inferred state based on upstream responses and
reports that through `/admin/status`.

If a public usage/quota endpoint becomes available, it can be added without
changing the client-facing OpenAI-compatible API.

## SMTP notifications

Set SMTP variables to receive email when the proxy switches away from an
exhausted key and when all keys have been exhausted.

Example with STARTTLS:

```bash
export SMTP_HOST="smtp.example.com"
export SMTP_PORT="587"
export SMTP_USERNAME="alerts@example.com"
export SMTP_PASSWORD="your-smtp-password"
export SMTP_FROM="alerts@example.com"
export SMTP_TO="you@example.com"
export SMTP_STARTTLS="true"
```

SMTP sending is asynchronous and best effort. Notification failures are logged
but do not fail client requests.

## Deployment

### Build a binary

```bash
go build -o switchboard-go .
sudo install -m 0755 switchboard-go /usr/local/bin/switchboard-go
```

### systemd example

Create `/etc/systemd/system/switchboard-go.service`:

```ini
[Unit]
Description=Switchboard Go OpenAI-compatible OpenCode Go key-cycling proxy
After=network.target

[Service]
Type=simple
Environment=PROXY_API_KEY=replace-with-a-long-random-local-key
Environment=OPENCODE_GO_API_KEYS=sk-first,sk-second,sk-third
Environment=LISTEN_ADDR=0.0.0.0:8080
Environment=SMTP_HOST=smtp.example.com
Environment=SMTP_PORT=587
Environment=SMTP_USERNAME=alerts@example.com
Environment=SMTP_PASSWORD=replace-me
Environment=SMTP_FROM=alerts@example.com
Environment=SMTP_TO=you@example.com
Environment=SMTP_STARTTLS=true
ExecStart=/usr/local/bin/switchboard-go
Restart=always
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Then enable it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now switchboard-go
sudo systemctl status switchboard-go
```

For production, prefer storing secrets in an environment file with restrictive
permissions instead of directly in the unit file.

## Security notes

- Treat both `PROXY_API_KEY` and `OPENCODE_GO_API_KEYS` as secrets.
- Use a long, random `PROXY_API_KEY`.
- Bind to `127.0.0.1` unless other machines on your LAN need access.
- If exposing beyond a trusted LAN, put the service behind TLS and a firewall.
- Do not commit API keys, SMTP credentials, or systemd files containing secrets.
- `/healthz` is intentionally unauthenticated for health checks.
- `/v1/*` and `/admin/*` require authentication.

## Operational behavior

- Request bodies larger than 20 MiB are rejected with HTTP `413`.
- Hop-by-hop and proxy headers are stripped when forwarding.
- The proxy sets a compatible default upstream `User-Agent` if the client does
  not provide one. This avoids upstream blocks of generic HTTP clients.
- If an upstream stream begins successfully and then later emits an error, the
  proxy does not recover mid-stream; it only retries before a response is sent.
- Exhausted keys remain exhausted until the process restarts.

## Development

Run tests:

```bash
go test ./...
```

Format code:

```bash
gofmt -w .
```

Build:

```bash
go build ./...
```

## License

MIT. See [LICENSE](LICENSE).
