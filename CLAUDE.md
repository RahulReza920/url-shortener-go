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

## Env

| Var | Default | Notes |
|---|---|---|
| `PORT` | 8080 | |
| `REDIS_ADDR` | localhost:6379 | |
| `PASETO_SECRET_KEY` | (generated) | Unset = new key on every restart, all tokens invalidate |
| `SAFE_BROWSING_API_KEY` | (off) | Unset = malicious-URL check silently skipped |

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

## Invariants

- `codegen.Code` is a pure bijection over 42-bit space. Change permutation constants → rerun `TestCode_NoCollisionsAndNotSequential` (brute-forces 100k values)
- Aliases and generated codes share `url:{code}` namespace — `SETNX` is the collision guard
- Redis only. No second store without updating SPEC.md (tradeoff is explicit, tracked there)
- Redirect is `302` not `301`. See SPEC.md §6 — click-count accuracy depends on it

## Tests

`internal/api/server_test.go` — real httptest + miniredis per test, exercises full route table and middleware. New endpoints: follow same pattern (`newTestServer(t)` + `httptest.NewServer(srv.Routes())`).

## Go style

Strictly follow [Effective Go](https://go.dev/doc/effective_go) and [Google Go Style Guide](https://google.github.io/styleguide/go/). Non-negotiables: idiomatic error wrapping (`fmt.Errorf("...: %w", err)`), table-driven tests, no naked returns, small focused functions, no stutter in package names.

## Claude skills

| Skill | What it does |
|---|---|
| `/test` | `go vet ./...` + `go test ./...` |
| `/run` | starts Redis if needed, then `go run ./cmd/server` |
| `/code-review` | reviews current diff for bugs and cleanups |
| `/code-review --fix` | applies review findings to the working tree |
| `/simplify` | cleanup-only pass (no bug hunting) |

**Commits:** stage specific files by name, never `git add -A`. Message format: imperative mood, ≤72 chars, body explains *why* not *what*.  
**PRs:** `gh pr create` with a Summary (bullets) + Test plan (checklist). Always include `🤖 Generated with Claude Code`.

## Hooks (automatic, no action needed)

- **PostToolUse on Edit/Write** → `gofmt -w` + `go vet ./...` on every `.go` file saved
- **PreToolUse on Bash `git commit`** → `go test ./...` must pass before any commit lands
