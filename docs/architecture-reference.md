# Trigger AI — Architecture Reference

Permanent architectural rules for the Trigger monorepo. Planned work belongs in plan
documents; implemented-versus-remaining detail belongs in status documents.

See [security-invariants.md](security-invariants.md) for mandatory security constraints.

---

## Product boundary

Trigger is a sales-intelligence layer over external CRMs.

- The CRM owns scheduled work, leads, contacts, and pipeline state.
- Trigger owns recordings, transcripts, enrichments, and processing state.
- Trigger accesses CRM data through provider APIs and never reads a CRM database directly.
- Completing or scheduling an activity is the intentional CRM write path.

---

## Monorepo

```text
trigger-monorepo/
  apps/
    web/       Go HTMX server and background workers
  odoo/        Odoo 18 setup and custom integration addons
  docs/        Product, architecture, plans, and status
```

---

## Stack

| Layer | Choice |
|------|--------|
| Language | Go 1.25 |
| Web UI | HTMX 2 + `html/template` |
| Browser app | PWA |
| Identity | WorkOS AuthKit |
| Trigger database | PostgreSQL 15+ |
| Cache and queue transport | Redis |
| Durable jobs | Asynq |
| CRM | Provider interface; Odoo 18 first |
| Recording storage | Private Cloudflare R2 |
| Resumable upload | TUS |
| Transcription | Deepgram initially; Whisper-compatible alternative later |
| Enrichment | Anthropic Claude initially, behind a service boundary |
| Browser live updates | SSE or bounded HTMX polling |
| Observability | Structured logs, trace IDs, Sentry |

---

## Bounded contexts

| Context | Owns |
|---------|------|
| `sales` | CRM-facing leads, opportunities, activities, projects, and units |
| `identity` | Tenants, users, roles, CRM configuration, and personal CRM tokens |
| `calls` | Recording metadata, transcript, enrichment, matching, and processing state |
| `insights` | Future analytics and coaching signals |

Infrastructure is not a bounded context. PostgreSQL, Redis, R2, TUS, WorkOS SDK plumbing,
HTTP security, and job transport live under `pkg`.

---

## Web application layout

```text
apps/web/
  main.go
  config/
  common/
    auth/
    identity/
    sales/
    calls/
  app/
    identity/
      adapter/
      repo/
      usecase/
    sales/
      adapter/
      usecase/
    calls/
      adapter/
      repo/
      usecase/
  api/
    server.go
    middleware.go
    auth.go
    settings.go
    calls.go
    upload.go
  pkg/
    cache/
    crmclient/
      odooclient/
    db/
      migrations/
      queries/
      store/
    httpsecurity/
    jobs/
    r2/
    secretbox/
  templates/
  static/
```

`main.go` is the composition root. It builds shared infrastructure and injects domain
repositories and usecases.

---

## Dependency rules

```text
api                    → app/*/usecase, common/*
app/*/usecase          → app/*/repo, app/*/adapter, pkg/*, common/*
app/*/adapter          → common/* and generated input/output types when conversion requires it
app/*/repo             → common/*, pkg/db/store, infrastructure interfaces
pkg/crmclient          → common/sales
pkg/crmclient/provider → common/sales, common/httpclient
pkg/* infrastructure  → stdlib and third-party packages
common/*               → stdlib only
```

Domain DTOs do not depend on sqlc, pgx, Odoo JSON types, WorkOS SDK types, Redis types, or
AWS SDK types.

---

## Naming

Use `{Entity}{Action}` for queries, repository methods, and usecases:

```text
TenantFindByWorkOSOrgID
UserIdentityGet
CRMConfigUpsert
CallRecordList
ActivityComplete
```

Conversion functions use `{Subject}{Target}From{Source}` where practical:

```text
TenantDtoFromSql
ActivityDtoFromOdoo
CRMConfigUpsertSqlFromEncrypted
```

---

## Database and repositories

- PostgreSQL is Trigger’s system of record.
- Migrations are embedded, transactional, checksummed, and advisory-locked.
- sqlc generates one shared `store` package.
- Repositories receive `*store.Queries`; transactions use `WithTx`.
- Every ownership relationship uses a foreign key.
- Every tenant-owned query is tenant-scoped.
- Repository boundaries return domain types or internal encrypted records, not generated
  sqlc types.

---

## Identity and sessions

WorkOS organizations map one-to-one to Trigger tenants. A WorkOS user maps to exactly one
Trigger tenant.

Browser authentication:

- server-side AuthKit authorization-code flow;
- SDK PKCE and state helpers;
- SDK-sealed HTTP-only session cookie;
- production `Secure` cookies and development loopback HTTP support;
- WorkOS user and organization claims validated before application cache use.

Native recorder authentication is separate. The Android recorder never sends the browser
sealed-session cookie as a bearer token. Native endpoints use dedicated device/bearer
credentials and produce the same trusted internal user/tenant context after validation.

---

## Auth context

Protected application requests carry:

```go
type AuthUser struct {
    UserID              string
    TenantID            string
    WorkOSUserID        string
    WorkOSOrgID         string
    Role                Role
    CRMProvider         string
    CRMBaseURL          string
    CRMToken            string
    CRMConfigVersion    int64
    UserCRMTokenVersion int64
}
```

Plaintext `CRMToken` exists only in request memory. PostgreSQL and Redis retain encrypted
credential material.

---

## CRM registry

`common/sales.CRM` is the provider-neutral interface.

CRM clients are cached by:

```text
internal tenant ID
+ WorkOS user ID
+ tenant CRM config version
+ personal CRM token version
```

Tenant-only caching is forbidden because users may have different personal CRM tokens.
Credentials themselves never appear in cache keys.

Usecases resolve clients from `AuthUser`:

```go
crm, err := registry.Build(
    user.TenantID,
    user.WorkOSUserID,
    user.CRMConfigVersion,
    user.UserCRMTokenVersion,
    sales.CRMConfig{
        Provider: user.CRMProvider,
        BaseURL:  user.CRMBaseURL,
        APIKey:   user.CRMToken,
    },
)
```

---

## Browser request security

Unsafe cookie-authenticated browser routes require exact Origin validation against
`BASE_URL`. OAuth callbacks use state and PKCE instead.

Native APIs and signed webhooks are not subject to browser Origin requirements; they use
their own authentication.

User-configurable CRM URLs are validated against scheme, redirect, and SSRF policy before
client construction and during redirects.

---

## Recording ingestion

The native Android recorder uploads through a resumable TUS endpoint.

- Authentication is native-device/bearer authentication.
- Object keys are derived from authenticated tenant and user IDs.
- Audio is streamed to private R2 storage.
- PostgreSQL stores an opaque R2 object key, not a public URL.
- Reading audio uses a short-lived presigned URL or authenticated stream.
- Upload finalization creates the call record idempotently.

---

## Background processing

Durable workflows use Redis and Asynq.

- Task payloads contain identifiers, not large blobs or plaintext long-lived secrets.
- Handlers reload and authorize current state.
- Handlers are idempotent.
- Retries use bounded backoff.
- Terminal failures are recorded in domain state and observability systems.
- In-process goroutine pools are not used for durable transcription or enrichment.

---

## HTMX patterns

1. Navigation swaps the `content` block and pushes browser history.
2. Filter forms trigger GET requests on change.
3. Infinite scrolling uses a sentinel and opaque cursor.
4. Out-of-band fragments update counters or shared page state.
5. Unsafe forms pass through session authentication and strict Origin middleware.

---

## Data ownership

| Data | Owner | Storage |
|------|-------|---------|
| Leads, contacts, pipeline | CRM | CRM storage |
| Scheduled activities | CRM | CRM storage |
| Tenant and user identity | Trigger | Trigger PostgreSQL |
| CRM configuration | Trigger | Trigger PostgreSQL, encrypted credentials |
| Recording object | Trigger | Private R2 |
| Recording metadata | Trigger | Trigger PostgreSQL |
| Transcript and enrichment | Trigger | Trigger PostgreSQL/private R2 |
| Cache | Derived only | Redis |
| Job state | Processing | Redis plus durable domain status in PostgreSQL |
