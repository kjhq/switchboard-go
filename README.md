<p align="center">
  <img src="assets/logo.png" alt="Switchboard Go logo" width="220">
</p>

<h1 align="center">Switchboard Go</h1>

Switchboard Go is a small local proxy for the OpenCode Go API.

It gives OpenAI-compatible and Anthropic Messages-compatible tools one stable
local endpoint and automatically cycles through your upstream OpenCode Go API
keys when one is exhausted.

```text
OpenAI/Anthropic-compatible app -> http://127.0.0.1:8080/v1 -> OpenCode Go
```

## Why use it?

- One local `/v1/*` endpoint for OpenAI-compatible and Anthropic Messages requests
- One proxy API key for your tools
- Multiple upstream OpenCode Go keys behind the scenes
- Automatic failover when an upstream key is exhausted
- Web dashboard at `/dashboard/` for key management, monitoring, and settings
- JSON config file (no YAML), optional SMTP alerts

## Install

Download a binary from GitHub Releases:

```text
https://github.com/ArsalanDotMe/switchboard-go/releases
```

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"

# First run — seed initial keys (optional, can add via dashboard later)
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"

switchboard-go
```

Then open http://127.0.0.1:8080/dashboard/ in your browser.

## Settings

Only `PROXY_API_KEY` is required as an environment variable. Everything else is
configured through the dashboard or stored in `~/.config/switchboard-go/config.json`.

| Env var | Required | Description |
|---|---|---|
| `PROXY_API_KEY` | Yes | Key clients use to access Switchboard Go. |
| `OPENCODE_GO_API_KEYS` | No | Seeds keys on first run (ignored afterwards). |

On first run, if no config file exists, the proxy starts with default settings
and 0 upstream keys. Add keys through the dashboard. If `OPENCODE_GO_API_KEYS`
is set, those keys are seeded into the config file automatically.

## Dashboard

Open `http://127.0.0.1:8080/dashboard/` in your browser. You'll be prompted for
your `PROXY_API_KEY`. The dashboard has four pages:

- **Dashboard** — active key indicator, request rate sparkline, key health summary
- **Keys** — add/delete/reorder upstream API keys with clipboard validation
- **Requests** — sortable, paginated request log (method, path, key#, status, duration)
- **Settings** — listen address, upstream URL, SMTP config, dark/light theme

## Use it from an OpenAI-compatible client

Use the proxy key, not your upstream OpenCode Go key:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

## Use it from an Anthropic Messages-compatible client

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
export ANTHROPIC_API_KEY="$PROXY_API_KEY"
```

## Admin API

Use `Authorization: Bearer $PROXY_API_KEY`:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/admin/status` | Key states and active index |
| `GET` | `/admin/keys` | Full key list with states |
| `POST` | `/admin/keys` | Add a key (validates first) |
| `DELETE` | `/admin/keys/{index}` | Remove a key |
| `PUT` | `/admin/keys/reorder` | Reorder keys |
| `GET` | `/admin/settings` | Current config (password masked) |
| `PUT` | `/admin/settings` | Update config |
| `GET` | `/admin/requests` | Request log entries |
| `POST` | `/admin/validate-key` | Validate a single key |
| `POST` | `/admin/validate-keys` | Validate all configured keys |

Health checks:

- `GET /healthz`
- `GET /readyz`

## More docs

- [Agent/client setup](docs/agent-config.md)
- [Docker](docs/docker.md)
- [systemd deployment](docs/deployment.md)
- [SMTP notifications](docs/smtp.md)
- [Operations and security](docs/operations.md)

## Development

```bash
go test ./...
gofmt -w .
go build ./...
```

## License

MIT. See [LICENSE](LICENSE).
