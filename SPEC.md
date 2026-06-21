# URL Shortener Service — Spec

## 1. Overview

A high-traffic URL shortener written in Go, backed by Redis as the sole
data store. Supports anonymous and authenticated link creation, custom
aliases, optional TTL, basic click counting, and malicious-URL screening
at creation time.

## 2. Stack

- **Language/framework:** Go (stdlib `net/http` or a minimal router like
  `chi`; no heavy framework needed for a redirect-and-create service).
- **Data store:** Redis only. No relational/durable store in v1. Redis
  persistence (AOF/RDB) is **not** configured in v1 — see Out of Scope.
  No storage abstraction layer is built; code talks to Redis directly.
- **Auth:** PASETO tokens (v2/v4 local or public, pick one and be
  consistent — recommend `v4.public` for stateless verification without
  shared secret distribution). Service owns signup/login/token issuance.

## 3. Data Model (Redis keys)

```
seq:code                          -> INCR counter, source of short-code IDs
url:{code}                        -> destination URL (string)
url:{code}:meta                   -> hash {owner_id, created_at, ttl, expires_at}
url:{code}:clicks                 -> INCR counter, click count
alias:reserved                    -> SET of blocklisted words (admin, api, login, ...)
user:{id}                         -> hash {email, password_hash, created_at}
user:email:{email}                -> id lookup (uniqueness)
user:{id}:links                   -> SET of codes owned by this user
```

TTL on `url:{code}` and `url:{code}:meta` set via Redis `EXPIRE` when a
per-link TTL is specified at creation. No TTL = no expiry (persists
until manually removed or Redis loses the key).

## 4. Short Code Generation

- **Source of uniqueness:** Redis `INCR seq:code` — monotonic, collision-free,
  no retry loop, no central bottleneck beyond a single atomic INCR.
- **Unguessability:** the raw counter is sequential and therefore
  guessable. Before base62-encoding, the counter value is passed through
  a reversible bit permutation (fixed-key XOR + bit-rotation over the
  62-bit space, or equivalent Feistel-style scramble). This makes
  `code(n)` and `code(n+1)` unrelated-looking while remaining a pure
  bijection — no collision risk, no extra lookup needed, and the
  permutation is reversible only if you have the key (not exposed).
- **Encoding:** scrambled integer → base62 (`[0-9a-zA-Z]`), no padding,
  giving short codes (~6-7 chars covers billions of IDs).
- **Custom aliases:** validated against:
  - charset: alphanumeric + hyphens
  - length: 3-30 chars
  - reserved-word blocklist (`alias:reserved` set)
  - uniqueness: `SETNX url:{code}` to claim atomically; reject with 409 if taken.
  Custom aliases live in the **same namespace** as generated codes
  (`url:{code}`), so a user can't claim a code that collides with an
  already-issued generated one and vice versa — both check/set against
  the same key.

## 5. API

| Method | Path              | Auth      | Description |
|--------|-------------------|-----------|--------------|
| POST   | `/api/signup`      | none      | create account, returns PASETO token |
| POST   | `/api/login`       | none      | verify credentials, returns PASETO token |
| POST   | `/api/links`        | optional  | create short link. Body: `{url, alias?, ttl_seconds?}`. Anonymous allowed at lower rate limit; authenticated links are owned and listable. |
| GET    | `/api/links`        | required  | list the authenticated user's own links |
| GET    | `/{code}`           | none      | 302 redirect to destination, increments click counter |

No edit/delete endpoints in v1 (see scope decision below).

## 6. Redirect Behavior

- **Status code:** `302 Found` (not 301) — deliberately not cached
  aggressively by clients, so every visit hits the server and the click
  counter stays accurate.
- **Click tracking:** `INCR url:{code}:clicks` performed inline in the
  same request before responding (single atomic Redis op, negligible
  latency — no async pipeline needed for a bare counter increment).
- **Miss handling:** unknown or expired code → `404`.

## 7. Caching / Performance

- Redis itself is the cache — there is no separate datastore for it to
  front, so no additional caching layer (no app-level LRU, no CDN edge
  cache) is introduced in v1. Redirect lookups are O(1) `GET` against
  Redis.
- Designed for high traffic: stateless Go service instances behind a
  load balancer, horizontally scalable; Redis can move to Cluster mode
  later if a single instance becomes the bottleneck (not built in v1,
  but the key scheme above doesn't require multi-key transactions, so
  it's cluster-compatible later without redesign).

## 8. Abuse Prevention

- **Rate limiting:** per-IP, on `POST /api/links` only (e.g. token
  bucket, 10 creates/min for anonymous, higher for authenticated).
  The redirect path (`GET /{code}`) is intentionally unthrottled since
  it must scale freely with click traffic.
- **Malicious URL screening:** at creation time, destination URL is
  checked against the Google Safe Browsing API. A match rejects
  creation with `400`. This adds one external call + latency to the
  create path only (not the hot redirect path).

## 9. Auth

- PASETO tokens, issued on signup/login.
- Passwords hashed with Argon2id.
- Anonymous creation is allowed (lower rate limit, link has no owner,
  not listable/manageable later).
- Authenticated creation: link is associated with `user_id`, added to
  `user:{id}:links`, retrievable via `GET /api/links`.

## 10. Out of Scope (v1)

- **Durable storage.** Redis has no AOF/RDB persistence configured.
  A server restart or crash means **all links and accounts are lost**.
  This is an explicit, accepted gap for v1 — revisit by either enabling
  Redis persistence or migrating to Redis + a durable store (Postgres)
  later.
- Link edit (changing destination URL) and delete/deactivate.
- Rich analytics: referrer, user-agent, geo, device/browser parsing, UTM
  tracking. v1 only tracks a raw click count.
- Async analytics pipeline / event queue — not needed since v1 analytics
  is a single atomic counter increment.
- CDN/edge caching, app-level LRU cache in front of Redis.
- Redis Cluster / replication — single-node Redis for v1; scheme is
  cluster-compatible later without redesign.
- Password reset, email verification, account deletion.
- Admin tooling for the reserved-alias blocklist (it's a static seeded set).
- Reactive abuse takedown workflow (relying solely on Safe Browsing
  check at creation time).

## 11. End-to-End Verification

Proves the full create → redirect → click-count flow works against a
running instance.

```bash
# 1. Start Redis (if not already running)
redis-server --daemonize yes

# 2. Start the service
go run ./cmd/server &
SERVER_PID=$!
sleep 1

# 3. Create a link anonymously
RESP=$(curl -s -X POST http://localhost:8080/api/links \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com/some/long/path"}')
echo "create response: $RESP"
CODE=$(echo "$RESP" | jq -r '.code')

# 4. Follow the redirect, verify it lands on the original URL
LOCATION=$(curl -s -o /dev/null -w '%{redirect_url}' http://localhost:8080/$CODE)
test "$LOCATION" = "https://example.com/some/long/path" \
  && echo "PASS: redirect target matches" \
  || echo "FAIL: got $LOCATION"

# 5. Verify status code is 302, not 301
STATUS=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/$CODE)
test "$STATUS" = "302" \
  && echo "PASS: status is 302" \
  || echo "FAIL: status was $STATUS"

# 6. Hit it again, verify click count incremented to 2
redis-cli GET "url:$CODE:clicks"   # expect 2

# 7. Unknown code returns 404
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/doesnotexist  # expect 404

kill $SERVER_PID
```

A pass on all five checks (target match, 302 status, click count = 2,
404 on unknown code, and the initial create returning a valid code)
demonstrates the redirect flow end-to-end.
