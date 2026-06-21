# url-shortener

Go URL shortener backed by Redis. Read `SPEC.md` before touching codegen, rate limits, or redirect behavior — those tradeoffs are deliberate.

## Commands

```bash
go build ./...        # compile
go vet ./...          # lint
go test ./...         # tests (miniredis — no real Redis needed)
go run ./cmd/server   # server (needs Redis — see below)
```

Redis locally: `brew install redis && redis-server --daemonize yes`

## Layout

```
cmd/server/main.go    wiring
internal/config       env
internal/store        all Redis ops (SPEC.md §10)
internal/codegen      counter → permuted → base62 (SPEC.md §4)
internal/auth         Argon2id + PASETO v4.public
internal/ratelimit    per-IP fixed-window, POST /api/links only
internal/safebrowsing Safe Browsing client, no-op if key unset
internal/api          handlers, middleware, routing
```

`POST /api/links`: `optionalAuth` → `rateLimitCreate` → `handleCreateLink`  
`GET /{code}`: no auth, no rate limit by design (SPEC.md §7-8)

## Tests

`internal/api/server_test.go` — real httptest + miniredis per test, exercises full route table and middleware. New endpoints: follow same pattern (`newTestServer(t)` + `httptest.NewServer(srv.Routes())`).

## Go style

Strictly follow [Effective Go](https://go.dev/doc/effective_go) and [Google Go Style Guide](https://google.github.io/styleguide/go/). Non-negotiables: idiomatic error wrapping (`fmt.Errorf("...: %w", err)`), table-driven tests, no naked returns, small focused functions, no stutter in package names.

**Commits:** stage specific files by name, never `git add -A`. Message format: imperative mood, ≤72 chars, body explains *why* not *what*.  
**PRs:** `gh pr create` with a Summary (bullets) + Test plan (checklist).

---

## Behavioral guidelines

### Think before coding
State assumptions explicitly. If multiple interpretations exist, present them — don't pick silently. If unclear, stop and ask before implementing.

### Simplicity first
Minimum code that solves the problem. No unrequested features, abstractions, configurability, or error handling for impossible scenarios. If 200 lines could be 50, rewrite it.

### Surgical changes
Touch only what the request requires. Don't improve adjacent code, refactor unbroken things, or delete pre-existing dead code — mention it instead. Remove only imports/vars/functions YOUR changes made unused.

### Goal-driven execution
Transform tasks into verifiable goals before starting. For multi-step work, state a plan with explicit verify steps. Loop until criteria are met — don't stop at "looks right."
