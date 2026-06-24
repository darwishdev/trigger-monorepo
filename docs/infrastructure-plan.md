# Platform Infrastructure Plan

Builds shared technical foundations used by identity, calls, sales, and future features.
This work belongs on a standalone infrastructure branch and must not deliver feature-
specific database schemas, routes, pages, or domain behavior.

Mandatory rules are defined in [security-invariants.md](security-invariants.md).

---

## Scope

This plan owns:

- PostgreSQL connection and lifecycle.
- Transactional, concurrency-safe migrations.
- sqlc configuration and generated-store conventions.
- Redis connection and generic cache primitives.
- Secret encryption primitives.
- Environment-aware configuration validation.
- Secure cookie construction and deletion.
- Strict Origin middleware for cookie-authenticated browser mutations.
- Trusted-proxy policy.
- Persistent job infrastructure.
- Shared health checks and infrastructure test conventions.

This plan does not own:

- Tenant, user, CRM, call, or enrichment tables.
- WorkOS login and authorization behavior.
- CRM settings.
- Upload, transcription, or AI workflow behavior.

---

## Step 1 — PostgreSQL pool

Dependencies:

```text
github.com/jackc/pgx/v5
github.com/jackc/pgx/v5/pgxpool
```

Add `pkg/db/db.go`:

```go
func New(ctx context.Context, dsn string) (*pgxpool.Pool, error)
```

Requirements:

- Parse and validate the DSN before constructing the pool.
- Ping during startup with a bounded context.
- Configure pool sizing and lifetime through environment variables.
- Close the pool during graceful shutdown.
- Development may use `sslmode=disable` locally.
- Production must use provider-appropriate TLS verification.

Done when:

- Unit tests validate configuration.
- An integration test connects, pings, and closes cleanly.
- Startup fails clearly when PostgreSQL is unavailable.

---

## Step 2 — Migration runner

Add `pkg/db/migrate.go` and an embedded `pkg/db/migrations` directory.

Each migration:

- has an immutable ordered filename;
- runs inside a transaction;
- is recorded by filename and SHA-256 checksum;
- is applied exactly once;
- causes startup failure if an already-applied file changes.

The runner:

- acquires a PostgreSQL advisory lock before reading or applying migrations;
- supports multiple application instances starting concurrently;
- releases the lock on success and failure;
- never marks a failed migration as applied.

Feature plans own migration contents. Infrastructure owns only execution mechanics.

Done when:

- Empty, first-run, repeat-run, failure, checksum-mismatch, and concurrent-run tests pass.

---

## Step 3 — sqlc foundation

Install and pin sqlc in development tooling. Add:

```text
pkg/db/sqlc.yaml
pkg/db/queries/
pkg/db/store/
```

Configuration:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema: "migrations/"
    gen:
      go:
        package: "store"
        out: "store/"
        sql_package: "pgx/v5"
        emit_pointers_for_null_types: true
```

Rules:

- Generated files are committed and never edited manually.
- Every query name follows `{Entity}{Action}`.
- Repositories receive shared `*store.Queries`.
- Transactional repositories use `Queries.WithTx`.
- sqlc and pgx types do not escape repository boundaries.

Provide repeatable commands:

```text
make sqlc
make sqlc-check
```

`sqlc-check` regenerates and fails when committed output differs.

Done when:

- Code generation works with an empty foundation migration/query set.
- CI can detect stale generated code.

---

## Step 4 — Redis and cache primitives

Dependency:

```text
github.com/redis/go-redis/v9
```

Add `pkg/cache` with:

- client construction from `REDIS_URL`;
- namespaced key helpers;
- typed JSON get/set/delete operations;
- bounded operation timeouts;
- explicit TTLs;
- best-effort batch eviction;
- health check support.

Cache policy:

- Redis is never an authorization source by itself.
- A cache miss or outage falls back to the authoritative store.
- Sensitive plaintext is never serialized.
- Keys must not contain access tokens, refresh tokens, API keys, or personal credentials.
- Callers own cache meaning and invalidation; `pkg/cache` owns mechanics only.

Done when:

- Hit, miss, expiry, malformed-value, deletion, and outage tests pass.

---

## Step 5 — Secret encryption

Add `pkg/secretbox`.

Baseline format:

- AES-256-GCM.
- Random nonce for every write.
- Base64-decoded 32-byte key.
- Format and key-version header.
- Authenticated additional data supplied by the caller.
- Typed errors without plaintext, ciphertext, or key material in logs.

Interface:

```go
type Box interface {
    Encrypt(plaintext []byte, additionalData []byte) ([]byte, error)
    Decrypt(ciphertext []byte, additionalData []byte) ([]byte, error)
}
```

`CRM_CREDENTIAL_KEY` is required wherever encrypted feature data is enabled. Production
may later replace the environment key with KMS envelope encryption behind this interface.

Done when:

- Round-trip, wrong-key, wrong-owner-data, tampering, nonce uniqueness, and version tests
  pass.

---

## Step 6 — Environment and transport policy

Extend application configuration with:

```text
STATE
BASE_URL
DATABASE_URL
REDIS_URL
TRUSTED_PROXY_CIDRS
```

Add:

```go
func (c Config) IsProd() bool
func (c Config) Validate() error
```

Validation rules:

- `STATE=prod` requires an HTTPS `BASE_URL`.
- Development HTTP is limited to loopback hosts.
- Production PostgreSQL requires TLS verification appropriate to the provider.
- Secrets must not use documented placeholders in production.
- Origins are normalized once at startup, not per request.
- Forwarded headers are trusted only when the direct peer belongs to configured proxy
  CIDRs.

Done when:

- Table-driven tests cover production HTTPS, development loopback HTTP, invalid origins,
  bad proxy CIDRs, placeholder secrets, and database TLS policy.

---

## Step 7 — Shared cookie policy

Add `pkg/httpsecurity` cookie helpers.

Session-style cookies:

- `Path=/`.
- `HttpOnly=true`.
- `SameSite=Lax`.
- `Secure=Config.IsProd()`.
- no broad `Domain` unless explicitly required.

Cookie deletion must use the same name, path, domain, secure, and SameSite attributes,
plus `MaxAge=-1` and an expiry in the past.

Feature code supplies cookie name, value, and lifetime. It does not construct policy
attributes independently.

Done when:

- Development and production creation/deletion tests pass.

---

## Step 8 — Strict Origin middleware

Add strict Origin validation for unsafe browser routes authenticated by cookies.

For `POST`, `PUT`, `PATCH`, and `DELETE`:

- require a valid `Origin`;
- require exact normalized scheme and host equality with `BASE_URL`;
- require `Sec-Fetch-Site: same-origin` when the header is present;
- return 403 on missing, malformed, or mismatched values.

Exclusions:

- OAuth callbacks use state and PKCE.
- Native/mobile APIs use bearer or device authentication and do not depend on browser
  Origin headers.
- Webhooks use signature verification.

The middleware does not infer public origin from untrusted forwarded headers.

Done when:

- Same-origin, missing-origin, cross-origin, malformed-origin, proxy, OAuth callback, and
  native API tests pass.

---

## Step 9 — Persistent job foundation

Use Redis with Asynq for durable background processing.

Add `pkg/jobs` with:

- client and worker construction;
- named queues;
- retry and backoff defaults;
- idempotency/task-uniqueness support;
- graceful shutdown;
- structured task metadata including tenant ID, user ID, resource ID, and trace ID;
- dead-letter/failure observability.

Infrastructure defines task transport and worker lifecycle. Feature plans define task
payloads and handlers.

Do not use an in-process goroutine pool for durable product workflows.

Done when:

- Enqueue, process, retry, duplicate, cancellation, shutdown, and Redis restart tests pass.

---

## Step 10 — Health and verification

Add health endpoints or checks for:

- application process;
- PostgreSQL;
- Redis;
- migration state;
- worker readiness where workers run separately.

Verification commands:

```text
go test ./pkg/db/...
go test ./pkg/cache/...
go test ./pkg/secretbox/...
go test ./pkg/httpsecurity/...
go test ./pkg/jobs/...
go test ./...
```

The infrastructure branch is complete when feature plans can consume these components
without adding alternative database, migration, cache, cookie, encryption, Origin, or job
implementations.
