# vibe-c2-http-channel

First production channel module for Vibe C2.

## Purpose

`vibe-c2-http-channel` implements HTTP transport for implant/session <-> C2 communication using:

- `github.com/vibe-c2/vibe-c2-golang-channel-core`
- `github.com/vibe-c2/vibe-c2-golang-protocol`

## Scope (v0)

- receive inbound HTTP requests
- pass canonical `id` + `encrypted_data` into `vibe-c2-golang-channel-core`
- call C2 sync endpoint through channel-core runtime
- return outbound encrypted payload

## Environment

- `LISTEN_ADDR` (default `:8080`)
- `CHANNEL_ID` (default `http-main`)
- `C2_SYNC_BASE_URL` (default `http://localhost:9000`)

## Run

```bash
go run ./cmd/http-channel
```
