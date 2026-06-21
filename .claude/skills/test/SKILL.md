---
name: test
description: >
  Run the url-shortener test suite (go vet + go test ./...). No real Redis
  needed — handler tests use an in-memory miniredis fake. Use when asked to
  run tests, check the build, or verify changes before considering work done.
---

Run the project's automated checks. No external services required.

## Steps

```bash
cd /Users/pathaoltd/practice/url-shortener
go build ./...
go vet ./...
go test ./... -v
```

All three must pass before calling a change done. `go test` spins up an
in-memory `miniredis` instance per test (see `internal/api/server_test.go`),
so it works in any environment, sandboxed or not — there is no dependency
on a real Redis server.

## When a test fails

Read the failure output, locate the failing assertion in the test file,
and check the corresponding implementation in `internal/`. Don't relax an
assertion to make a test pass — fix the implementation, unless the test
itself is asserting the wrong thing per `SPEC.md`.

## Boundaries

This skill only runs checks — it does not fix failures or modify code. For
running the server itself, use the `run` skill.
