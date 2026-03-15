# vibe-c2-http-channel

First production channel module for Vibe C2.

## Purpose

`vibe-c2-http-channel` implements HTTP transport for implant/session <-> C2 communication using:

- `github.com/vibe-c2/vibe-c2-golang-channel-core` (latest)
- `github.com/vibe-c2/vibe-c2-golang-protocol`

## Scope (v0)

- receive inbound HTTP requests
- resolve profile via channel-core matcher (`hint` -> `fallback`)
- pass canonical values into channel-core profile-aware runtime
- call C2 sync endpoint through channel-core runtime
- return outbound encrypted payload

## Configuration

Runtime config is read from environment variables:

- `CHANNEL_ID` (default: `http-main`)
- `LISTEN_ADDR` (default: `:8080`)
- `C2_SYNC_BASE_URL` (default: `http://localhost:9000`)
- `PROFILES_FILE` (default: `configs/profiles.example.yaml`)

`.env` fallback:

- Pass path with `--config <path-to-env-file>`
- `.env` is loaded as fallback only (existing environment variables win)

Profiles are loaded from YAML file and resolved via channel-core matcher.

Mapping refs support location prefixes:
- `body:<field>`
- `header:<name>`
- `query:<name>`
- `cookie:<name>`

Example: `id: query:agent_id`, `encrypted_data: header:X-Payload`.

## Run

```bash
go run ./cmd/http-channel --config .env
```

## Integration Tests

This module includes integration tests with a built-in `test-c2-core` simulator for `/api/channel/sync`.

Run:

```bash
go test ./...
```

Covered obfuscation profile scenarios (loaded from `examples/profiles/*.yaml`):

- `body` mapping
- `header` mapping
- `query` mapping
- `cookie` mapping
- hint-routed profile selection
- `base64` transform pipeline (`decode inbound` + `encode outbound`)
- ambiguous profile hint error handling
