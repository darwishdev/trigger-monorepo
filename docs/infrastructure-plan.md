# Platform Infrastructure Plan

Builds shared technical foundations used by identity, calls, sales, and future features.
This work belongs on a standalone infrastructure branch and must not deliver feature-
specific database schemas, routes, pages, or domain behavior. The branch lands in small,
sequentially reviewable PRs (one or two steps each) and is merged incrementally into
`main`; long-lived feature branches rebase onto updated `main` rather than vendoring
infrastructure copies. Because embedded migrations are checksummed, any feature branch
that adds its own migration files must rebase after each infrastructure merge to keep the
migration sequence gap-free.

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
- Shared observability primitives (structured logging, trace IDs, error reporting).
- Application composition root and process lifecycle (construction order, signal
  handling, ordered graceful shutdown, and the seam feature plans plug into).
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
- Configure pool sizing and lifetime through explicit environment variables:
  `DB_MAX_CONNS`, `DB_MIN_CONNS`, `DB_MAX_CONN_LIFETIME`, `DB_MAX_CONN_IDLE_TIME`, and
  `DB_HEALTH_CHECK_PERIOD`. pgxpool defaults are CPU-derived and unsafe for containerized
  multi-instance deployments; every value must be set explicitly.
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
- causes startup failure if an already-applied file changes;
- applies files strictly by lexicographic order and fails startup on any gap between the
  highest applied sequence number and the next candidate, forbidding mid-sequence
  insertion of files after earlier ones have shipped.

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

Pin Go 1.25 via a `go.toolchain` directive in `apps/web/go.mod`, and pin sqlc in
development tooling. Add:

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
        emit_json_tags: false
        emit_interface: true
        emit_exact_table_names: false
```

Rules:

- Generated files are committed and never edited manually.
- `emit_*` options are one-way ratchets: chosen here before the first feature query ships
  and changed only through a dedicated migration PR that updates every dependent
  repository.
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
- prefix-scoped namespace eviction as the sole batch-invalidation primitive (no tag or
  pattern support); callers compose their own namespaces;
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
- `TRUSTED_PROXY_CIDRS` is consumed only for direct-peer validation before trusting
  `X-Forwarded-For` (real client IP for logging and rate-limiting) and `X-Forwarded-Proto`
  (scheme derivation); it never influences Origin or authorization decisions.

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
attributes independently. Opaque cookie values must originate from a CSPRNG or an
SDK-sealed token (e.g. the WorkOS session); feature code never invents its own
non-random session identifiers.

Done when:

- Development and production creation/deletion tests pass.

---

## Step 8 — Strict Origin middleware

Add strict Origin validation for unsafe browser routes authenticated by cookies.

For `POST`, `PUT`, `PATCH`, and `DELETE`:

- require a valid `Origin`;
- require exact normalized scheme and host equality with `BASE_URL`;
- require `Sec-Fetch-Site: same-origin` when the header is present;
- additionally require either a non-CORS-safelisted `Content-Type` (e.g.
  `application/json`) or an explicit custom request header, so that cross-site
  `text/plain`, `application/x-www-form-urlencoded`, and `multipart/form-data`
  submissions — which bypass CORS preflight — cannot rely on Origin forgery resistance
  alone;
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
- enqueue-time deduplication (`asynq.UniqueTTL`) and single-flight-by-resource-ID
  (`asynq.TaskID`) options exposed to feature callers;
- graceful shutdown with `ShutdownTimeout` sized to the longest expected task class
  (the 8s default is unsafe for transcription and enrichment jobs);
- structured task metadata including tenant ID, user ID, resource ID, and trace ID;
- dead-letter/failure observability.

Infrastructure defines task transport and worker lifecycle. Feature plans define task
payloads, handlers, and **handler idempotency**. `pkg/jobs` guarantees at-least-once
delivery with lease-based reprocessing on worker restart; it does not guarantee
exactly-once execution.

Do not use an in-process goroutine pool for durable product workflows.

Done when:

- Enqueue, process, retry, dedup-by-payload, single-flight-by-ID, cancellation,
  shutdown, and Redis restart tests pass.

---

## Step 10 — Observability foundation

Add `pkg/observability` owning the cross-cutting telemetry primitives shared by every
feature.

- Structured logger construction (`log/slog`) with JSON output in production and
  human-readable output in development;
- trace-ID propagation through request context, emitted on every log line and stamped
  onto every job payload;
- Sentry hook installation gated on `SENTRY_DSN`, with release tagging from build
  metadata;
- a request logger middleware that records method, path, status, duration, and trace ID
  and never logs cookies, tokens, or authorization headers;
- redaction helpers that scrub configured sensitive field names before serialization.

Feature plans consume these primitives; they do not instantiate their own loggers or
error reporters.

Done when:

- Structured output, trace-ID propagation across HTTP and job boundaries, Sentry
  forwarding, redaction, and "no-secret-in-log" assertion tests pass.

---

## Step 11 — Composition root and lifecycle

`apps/web/main.go` is the sole composition root. It builds every component from Steps 1–10
in dependency order, wires the feature application layers on top, serves until signaled,
and tears everything down in reverse order within a bounded shutdown budget.

This step owns orchestration only. It defines the seam that feature plans plug into; it
does not define feature repositories, usecases, or handlers.

Construction order — each layer receives only what its dependency rule permits:

```text
config.Config              validated once; normalized origins cached
pkg/observability          logger, trace-ID source, Sentry hook
pkg/db pool                pinged before anything reads it
pkg/db migrate             runs to completion before listener or workers start
*store.Queries             bound to the pool
pkg/cache client           pinged
pkg/secretbox.Box          key-version loaded
pkg/jobs Client + Server   configured, started only after migrations succeed
shared infra bundle        typed container passed to feature layers
feature repos              receive *store.Queries and infra interfaces
feature usecases           receive repos plus cache/box/jobs client
feature HTTP/job handlers  mounted onto the router and job server
http.Server                started last
```

Application layers communicate strictly by the dependency rules in
[architecture-reference.md](architecture-reference.md):
`api → app/*/usecase → app/*/repo → pkg/*`. Sibling features never import each other;
`main.go` is the only place that knows all layers exist.

Lifecycle surface (lives at `apps/web`, not under `pkg`):

```go
type App struct {
    // wired infrastructure plus the http.Server; fields unexported
}

func bootstrap(ctx context.Context) (*App, error) // build in order above; fail fast
func (a *App) Run(ctx context.Context) error      // serve until ctx is cancelled
func (a *App) Shutdown(ctx context.Context) error  // reverse-order teardown within ctx
```

Registration seam:

- A `Module` (or equivalent) interface lets each feature plan mount its HTTP routes onto
  the shared router and register its Asynq handlers on the job server, without the
  composition root knowing feature internals.
- The infra branch ships a runnable skeleton with no feature modules mounted: it boots,
  runs migrations, serves `/health`, 404s everything else, and shuts down cleanly.

Graceful shutdown:

- `signal.NotifyContext` traps `SIGINT`/`SIGTERM` and cancels the root context.
- Teardown runs in reverse construction order: HTTP `Shutdown` (drain in-flight requests)
  → job server `Shutdown` (drain active tasks within its `ShutdownTimeout`) → job client
  `Close` → Redis `Close` → pgxpool `Close` → logger/Sentry flush.
- A total `SHUTDOWN_BUDGET` bounds the whole sequence; each component receives a sub-budget
  derived from it.
- A partial startup failure tears down already-constructed components in reverse before
  exiting non-zero; `Shutdown` is idempotent and safe to call after a partial bootstrap.
- Workers start only after migrations succeed; the HTTP listener accepts traffic only after
  all health dependencies are ready.

Done when:

- Clean startup → serve → signal → drain → exit tests pass.
- Signal-during-startup, signal-during-active-request, and signal-during-active-job tests
  prove ordered drain within budget.
- A component construction failure leaves no leaked resources and exits non-zero.
- The skeleton binary with no feature modules mounted boots, migrates, serves `/health`,
  and shuts down cleanly.

---

## Step 12 — Health and verification

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
go test ./pkg/observability/...
go vet ./...
staticcheck ./...
gosec ./pkg/...
go test ./...
```

The infrastructure branch is complete when feature plans can consume these components
without adding alternative database, migration, cache, cookie, encryption, Origin, or job
implementations.
