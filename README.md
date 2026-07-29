# flagkit

Agentic-first feature flag management service. Plain text API, agent-driven, single Go binary with JSON file storage.

## Quick Start

```bash
make build
./flagkit
# → flagkit starting on :7707 (db: flagkit.json)
```

## Philosophy

flagkit is built for AI agents, not humans. No UI, no SDK — just clean text APIs.

- **The agent IS the interface** — every endpoint is designed for an AI agent to call
- **Plain text by default** — one labeled, grepable line per record
- **Instructive errors** — every 4xx includes a hint telling the agent what to do next
- **Self-documenting** — `GET /help` returns a one-page operating manual
- **Simple auth** — OTP via email → long-lived bearer token
- **Single binary** — Go + JSON file storage, zero external dependencies
- **Zero config** — runs out of the box with sensible defaults

## API Reference

### Auth

| Method | Path | Body | Description |
|--------|------|------|-------------|
| POST | `/auth/request` | `email=user@example.com` | Request OTP code |
| POST | `/auth/verify` | `email=user@example.com&code=123456` | Verify OTP, get bearer token |

### Flags

| Method | Path | Body | Description |
|--------|------|------|-------------|
| POST | `/flags` | `name=my_flag&type=boolean` | Create a flag |
| GET | `/flags` | — | List all flags |
| GET | `/flags/{handle}` | — | Get a specific flag |
| PATCH | `/flags/{handle}` | `enabled=false` | Update a flag |
| DELETE | `/flags/{handle}` | — | Delete a flag |
| POST | `/flags/{handle}/evaluate` | `context=user123` | Evaluate a flag |

### Other

| Method | Path | Description |
|--------|------|-------------|
| GET | `/help` | Operating manual for agents |
| GET | `/audit` | Audit log of flag changes |
| POST | `/mcp` | MCP endpoint for chat clients |

### Flag Types

- **boolean** — Simple on/off flag
- **percentage** — Percentage rollout (0-100%), uses context key for consistent bucketing
- **variant** — A/B testing with multiple variants, context key determines which variant

### Response Format

Default (plain text):
```
handle=flag_k7m2q name=my_flag type=boolean enabled=true
```

JSON (via `Accept: application/json` or `?format=json`):
```json
{"handle":"flag_k7m2q","name":"my_flag","type":"boolean","enabled":true}
```

### Errors

All 4xx responses include a hint:
```
error: missing auth token | hint: call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token
```

## Configuration

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | `FLAGKIT_ADDR` | `:7707` | Listen address |
| `-db` | `FLAGKIT_DB` | `flagkit.json` | Database file path |
| `-secret` | `FLAGKIT_SECRET` | (auto-generated) | Token signing secret |

Config priority: defaults < env vars < CLI flags

## Build

```bash
make build    # CGO_ENABLED=0, single binary
make test     # go test -race
make vet      # go vet
```

## License

MIT
