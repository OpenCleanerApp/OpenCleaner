# Research: Go daemon + REST + SSE (local-only)

## Recommended model
- Separate *job creation* from *event subscription*:
  - REST starts scan/clean job
  - SSE streams snapshot + deltas for that job
  - Avoid “SSE starts the job” (reconnect would restart work)

## SSE implementation (Go net/http)
- Headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
- Require flusher; flush after events
- Heartbeat comment `: ping\n\n` every 15–30s
- Exit on `r.Context().Done()`

## Backpressure
- Bounded per-subscriber buffers; drop/coalesce intermediate progress updates
- Rate limit progress events (e.g. 10Hz)
- Consider write deadlines to avoid goroutine pinning

## Unix socket vs 127.0.0.1
- Unix socket best isolation via filesystem perms (0600)
- Swift URLSession doesn’t natively do HTTP-over-unix; use Network.framework (NWConnection) + minimal HTTP
- If ever forced to TCP loopback: require bearer token (Keychain) + bind to 127.0.0.1 only
