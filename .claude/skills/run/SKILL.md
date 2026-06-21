---
name: run
description: >
  Launch the url-shortener server for manual testing. Checks for a running
  Redis instance (starts one if missing), then runs `go run ./cmd/server`.
  Use when asked to run, start, or try out the URL shortener locally.
---

Launch the server so its behavior can be exercised manually (e.g. via curl).

## Steps

1. Check Redis is reachable:
   ```bash
   redis-cli -h localhost -p 6379 ping
   ```
   - If `redis-cli` is not installed: `brew install redis` (one-time).
   - If installed but not running: `redis-server --daemonize yes`.

2. Start the server:
   ```bash
   cd /Users/pathaoltd/practice/url-shortener
   go run ./cmd/server
   ```
   Default port 8080, default `REDIS_ADDR=localhost:6379`. Override with env
   vars (`PORT`, `REDIS_ADDR`, `PASETO_SECRET_KEY`, `SAFE_BROWSING_API_KEY`)
   if needed — see CLAUDE.md for the full list.

3. Confirm it's up:
   ```bash
   curl -s -X POST http://localhost:8080/api/links \
     -H 'Content-Type: application/json' \
     -d '{"url": "https://example.com"}'
   ```
   Expect a JSON response with a `code` field.

## Boundaries

This skill only launches the server for manual/local testing. It does not
deploy anywhere or modify production state. For automated correctness
checks, use the `test` skill instead.
