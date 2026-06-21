<p align="center">
  <img src="assets/logo.png" alt="Switchboard Go logo" width="220">
</p>

<h1 align="center">Switchboard Go</h1>

<p align="center">
  <a href="https://github.com/kjhq/switchboard-go/releases"><img src="https://img.shields.io/github/v/release/kjhq/switchboard-go?style=flat-square" alt="Release"></a>
  <a href="https://github.com/kjhq/switchboard-go/actions/workflows/docker-publish.yml"><img src="https://img.shields.io/github/actions/workflow/status/kjhq/switchboard-go/docker-publish.yml?branch=main&style=flat-square&label=docker" alt="Docker"></a>
  <a href="https://github.com/kjhq/switchboard-go/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/kjhq/switchboard-go/release.yml?style=flat-square&label=release" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/kjhq/switchboard-go"><img src="https://goreportcard.com/badge/github.com/kjhq/switchboard-go?style=flat-square" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
</p>

<p align="center">
  Multi-key reverse proxy for OpenCode Go with a web dashboard.
  <br>
  Rotate API keys automatically. Manage everything from the browser.
</p>

A fork of [ArsalanDotMe/switchboard-go](https://github.com/ArsalanDotMe/switchboard-go) that adds a
full web dashboard for key management, monitoring, and settings.

Switchboard Go is a small local proxy for the OpenCode Go API. It gives
OpenAI-compatible tools one stable local endpoint and automatically cycles
through your upstream OpenCode Go API keys when one is exhausted.

```text
OpenAI-compatible app -> http://127.0.0.1:8080/v1 -> OpenCode Go
```

## Run with Docker (one-liner)

No clone, no install. Just Docker:

```bash
curl -sL https://raw.githubusercontent.com/kjhq/switchboard-go/main/docker-compose.yml \
  | PROXY_API_KEY=$(openssl rand -hex 32) docker compose -f - up -d
```

Open **http://127.0.0.1:8080/dashboard/** and log in with the generated key
(visible in `docker compose logs`).

## Why this fork?

The original is a CLI-only proxy. This fork adds:

- **Web dashboard** at `/dashboard/` — manage keys, monitor request logs, adjust settings
- **Key management UI** — add/delete/reorder keys with validation, naming, and clipboard paste
- **Request log** — live ring buffer of proxied requests with status, timing, and key assignment
- **Settings UI** — configure upstream URL, SMTP, and limits from the browser
- **JSON config** persisted to disk (no YAML dependency)
- Admin CRUD API for all operations

## Install

Download a binary from GitHub Releases:

```text
https://github.com/kjhq/switchboard-go/releases
```

## Quick start

```bash
export PROXY_API_KEY="replace-with-a-long-random-local-key"

# Optional: seed initial keys on first run (can also add via dashboard)
export OPENCODE_GO_API_KEYS="sk-first,sk-second,sk-third"

switchboard-go
```

Then open **http://127.0.0.1:8080/dashboard/** in your browser and log in with your
`PROXY_API_KEY`.

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
your `PROXY_API_KEY`. Four pages:

- **Dashboard** — active key indicator, request rate sparkline, key health summary
- **Keys** — add/delete/reorder upstream API keys with clipboard validation
- **Requests** — sortable, paginated request log (method, path, key#, status, duration)
- **Settings** — listen address, upstream URL, SMTP config

## Use it from an OpenAI-compatible client

Use the proxy key, not your upstream OpenCode Go key:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
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
