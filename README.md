# vibe-c2-http-channel

First production channel module for Vibe C2.

## Purpose

`vibe-c2-http-channel` implements HTTP transport for implant/session <-> C2 communication using:

- `github.com/vibe-c2/vibe-c2-golang-channel-core`
- `github.com/vibe-c2/vibe-c2-golang-protocol`

## Scope (v0)

- receive inbound HTTP requests
- extract canonical `id` + `encrypted_data` via profile mapping
- call C2 sync endpoint through channel-core runtime
- return outbound encrypted payload

## Run (placeholder)

```bash
go run ./cmd/http-channel
```
