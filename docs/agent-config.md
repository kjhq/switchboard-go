# Agent and client setup

Point OpenAI-compatible and Anthropic Messages-compatible clients at
Switchboard Go and use the proxy API key, not your upstream OpenCode Go key.

For a local Switchboard Go process:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="$PROXY_API_KEY"
```

## curl

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-5.1",
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
    "model": "glm-5.1",
    "stream": true,
    "messages": [{"role": "user", "content": "Write a short poem"}]
  }'
```

Anthropic Messages-compatible models use `/v1/messages` and `x-api-key`:

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: $PROXY_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-m3",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "Say hello"}]
  }'
```

## opencode

Log in to opencode's built-in OpenCode Go provider and use the Switchboard proxy
API key when prompted:

```bash
opencode auth login
```

Select `opencode-go`, then paste the Switchboard proxy API key, not an upstream
OpenCode Go key.

Create or edit `~/.config/opencode/opencode.json`. Preserve any existing keys
and merge in only the `opencode-go` provider override. This keeps opencode's
built-in OpenCode Go model list and only changes the endpoint:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "opencode-go": {
      "options": {
        "baseURL": "http://127.0.0.1:8080/v1"
      }
    }
  },
  "model": "opencode-go/glm-5.1"
}
```

Use your deployed Switchboard URL for `baseURL` if it is not running locally,
for example `https://switchboard.example.com/v1`.

Verify:

```bash
opencode models opencode-go
opencode run --model opencode-go/glm-5.1 "Reply with exactly: switchboard ok"
```

Restart opencode after changing config; running sessions keep the old config.

## Pi Coding Agent

Pi already has a built-in `opencode-go` provider with model metadata. Reuse that
provider and only override its endpoint and API key. This avoids maintaining a
long custom `models` list.

Create or edit `~/.pi/agent/models.json`:

```json
{
  "providers": {
    "opencode-go": {
      "baseUrl": "http://127.0.0.1:8080",
      "apiKey": "$SWITCHBOARD_PROXY_API_KEY",
      "compat": {
        "supportsDeveloperRole": false
      }
    }
  }
}
```

Then export the proxy key before starting Pi:

```bash
export SWITCHBOARD_PROXY_API_KEY="$PROXY_API_KEY"
```

Set defaults in `~/.pi/agent/settings.json`, preserving any existing settings
keys:

```json
{
  "defaultProvider": "opencode-go",
  "defaultModel": "glm-5.1"
}
```

Verify:

```bash
pi --no-extensions --provider opencode-go --model glm-5.1 --no-session --no-tools -p "Reply with exactly: switchboard ok"
```

Expected:

```text
switchboard ok
```

The `supportsDeveloperRole: false` compatibility flag tells Pi to send its
instruction message as a standard `system` message instead of OpenAI's newer
`developer` role. OpenCode Go currently rejects `developer` messages for the
OpenAI-compatible route, so this flag is required.

This setup exposes Pi's built-in `opencode-go` model list through Switchboard.
Models that Pi sends through its OpenAI-compatible implementation work with
Switchboard's `/chat/completions` or `/v1/chat/completions` endpoint. Models
that Pi sends through an Anthropic Messages implementation work with
Switchboard's `/v1/messages` endpoint.
