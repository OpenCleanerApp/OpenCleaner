# OpenCleaner — IPC + SSE notes

## Recommended job model
- `POST /api/v1/scan` (starts job) → returns result or job id (TBD)
- `GET /api/v1/progress/stream` (SSE) streams snapshot + deltas
- Prefer: create job via REST; subscribe via SSE (so reconnect does not restart work).

## SSE implementation rules (Go net/http)
- Headers:
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
- Require `http.Flusher`; flush after events
- Heartbeat comments `: ping\n\n` every 15–30s
- Cancellation: exit on `r.Context().Done()`
- Event framing: events are terminated by a blank line (`\n\n` or `\r\n\r\n`).
  - Current server emits `data: <json>\n\n` and heartbeat comments `: ping\n\n`.
- Transport: clients MUST handle arbitrary chunking (including HTTP `Transfer-Encoding: chunked`, which is expected for streaming responses) and MUST NOT assume a single read == a whole SSE event.
  - Buffer *bytes* and only decode UTF-8 after an event delimiter is found (reads may split multi-byte scalars).
  - Accept both `\n\n` and `\r\n\r\n` as delimiters; normalize CRLF to LF if needed.
  - Support multi-line `data:` fields (concatenate values with `\n` per SSE rules); ignore comment lines starting with `:`.
  - Chunked bodies may include optional trailers after the final `0\r\n` chunk.
- Server-side: write each complete event frame in a single `Write()` when possible (then `Flush()`) to reduce interleaving, but clients still cannot rely on write/read boundaries.

## Backpressure
- Use bounded channels per subscriber; drop/coalesce intermediate progress events
- Rate limit progress events (ex: 10Hz)
- Consider write deadlines to avoid goroutine pinning

## Local-only security
- Unix socket is best isolation (file perms 0600).
- Swift client: implemented via Network.framework (`NWConnection`) speaking HTTP/1.1 over a Unix domain socket (see `app/Packages/OpenCleanerClient`).
- If TCP loopback is ever used: bind to 127.0.0.1 only + require bearer token (store in Keychain).
