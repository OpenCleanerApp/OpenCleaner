# Phase 05 — Daemon transport (unix socket HTTP) + SSE

## Goal
Implement local daemon exposing HTTP/JSON API over unix domain socket, with SSE progress.

## Requirements
- Bind only to unix socket file; permissions 0600
- Endpoints per PRD:
  - `POST /api/v1/scan`
  - `POST /api/v1/clean`
  - `GET /api/v1/status`
  - `GET /api/v1/progress/stream` (SSE)
- Job model: REST triggers work; SSE streams snapshot + deltas
- Bounded buffers, coalesced progress, heartbeat

## Files (Go)
Create:
- `go/internal/transport/server.go`
- `go/internal/transport/unix_http.go`
- `go/internal/stream/progress.go`

## Steps
1) Implement HTTP server listening on unix socket
2) Implement request handlers that call engine/cleaner
3) Implement progress publisher:
   - internal channel
   - per-subscriber bounded channel
   - periodic heartbeat
4) Implement cancellation with context + client disconnect

## Success criteria
- `curl` via unix-socket client can scan/clean
- SSE stream works and doesn’t leak goroutines on disconnect
