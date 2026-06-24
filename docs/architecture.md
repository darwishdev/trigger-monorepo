# Trigger AI — Architecture Reference

This document is the permanent architectural reference for the Trigger monorepo.
It describes what exists and the rules that govern how code is organised.
It is not a roadmap — planned work lives in feature plan files.

---

## What Trigger Is

Trigger is a sales intelligence layer that sits on top of a CRM (Odoo today, others later).
It records phone calls made by salespeople on Android, transcribes them, enriches them with
AI, and surfaces the results alongside the CRM's scheduled activity queue.

The CRM is the system of record for tasks (who needs to call whom, by when).
Trigger is the system of record for what happened (recording, transcript, enrichment).
Ownership transfers from CRM to Trigger the moment a call is made.

---

## Monorepo Layout

```
trigger-monorepo/
  apps/
    web/          Go HTMX web server (this document's primary subject)
  odoo/           Odoo 18 Docker setup + custom addons
  docs/           Architecture + feature plans (you are here)
```

---

## Stack

| Layer | Choice | Reason |
|-------|--------|--------|
| Language | Go 1.25 | Standard library covers routing, templates, embed |
| Frontend | HTMX 2.0 + `html/template` | Server-rendered, no JS build step |
| PWA | manifest + sw.js + icons | Installable on mobile without an app store |
| CRM | Odoo 18 (initial) | First provider behind the CRM interface |
| Auth | WorkOS | B2B multi-tenant, SSO, session management |
| DB | PostgreSQL 15 | Trigger's own state: tenants, users, call records, enrichments |
| File storage | Cloudflare R2 | Zero egress fees for continuous audio uploads |
| Transcription | Deepgram / Whisper | High-speed, accurate speaker diarisation |
| AI enrichment | Claude (Anthropic) | Summary, sentiment, outcome extraction |
| Job queue | Redis + Asynq | Persistent, retry-aware async pipeline |
| Observability | Sentry + structured JSON logs + Trace IDs | End-to-end trace: Upload → Queue → AI → CRM |

---

## Bounded Contexts

Named after business subdomains, not technical layers or HTTP resources.

| Context | Owns | Status |
|---------|------|--------|
| `sales` | CRM pipeline (Lead, Opportunity), follow-ups (Activity), catalog (Project, Unit) | Active |
| `calls` | Recorded call asset: audio file, transcript, enrichment artifacts | Planned |
| `identity` | Tenant, User, CRM config, session — WorkOS-backed | Planned |
| `insights` | Dashboards, pipeline analytics, coaching signals | Future |

Deliberately **not** contexts: the AI engine (a service producing artifacts owned by `calls`),
R2/TUS storage (plumbing — the Recording asset lives in `calls`), Redis/SSE/Sentry (pure infra).

---

## Folder Layout (`apps/web/`)

```
apps/web/
  main.go                        # composition root only — wires everything, owns nothing
  config/                        # viper-based config; reads env vars
  common/
    auth/                        # AuthUser struct, FromCtx, NewCtx — no imports from app/pkg
    sales/                       # CRM interface + domain DTOs (Lead, Activity, Project, Unit, Page[T])
    calls/                       # CallRecord, CallEnrichment domain types
    identity/                    # Tenant, User domain types
  app/
    sales/
      usecase/                   # business orchestration against sales.CRM
      adapter/                   # request → domain filter; domain DTO → view struct
    calls/
      usecase/                   # merge logic, extraction trigger
      adapter/
      repo/                      # call_records + call_enrichments DB queries
    identity/
      usecase/                   # tenant/user management
      adapter/
      repo/                      # tenants, users, crm_configs, user_crm_tokens DB queries
  api/
    server.go                    # Server struct: injected usecases + parsed templates
    middleware.go                # SessionMiddleware, AdminMiddleware
    calls.go                     # /calls handler
    config.go                    # /settings handler
    upload.go                    # /calls/upload handler (Android APK endpoint)
  pkg/
    crmclient/
      registry.go                # provider registry + per-tenant cache
      odooclient/                # Odoo REST client implementing sales.CRM
    db/                          # pgx pool init + migration runner
    r2/                          # Cloudflare R2 upload client
    workos/                      # WorkOS SDK wrapper (auth URL, code exchange)
    jobs/                        # in-process goroutine job pool
  templates/
    layout.html                  # full layout: nav, main#content, footer, inline CSS
    partials.html                # nav, footer template blocks
    activities.html              # activity list page
    calls.html                   # calls page (merged view)
    config.html                  # CRM config + user token screen
  static/                        # logo, icons, sw.js, manifest.json
```

---

## Naming Rules

To keep the codebase predictable and unified, we adhere strictly to the **`{Entity}{Action}`** naming convention across all interfaces, database queries, repository layers, and usecases. We always prefix the name with the primary business entity/noun, followed by the action/verb.

### SQL Queries (sqlc name annotations)
Always prefix the query name with the entity name:
* `-- name: TenantFindByWorkOSOrgID :one` (not `FindTenantByWorkOSOrgID`)
* `-- name: UserFindByWorkOSID :one` (not `FindUserByWorkOSID`)
* `-- name: UserCreate :one` (not `CreateUser`)

### Repository / Usecase Methods
Always name methods using `{Entity}{Action}` structure:
* `UserFindOrCreate(...)` (not `FindOrCreateUser(...)`)
* `CRMConfigGet(...)` (not `GetCRMConfig(...)`)
* `UserCRMTokenUpsert(...)` (not `UpsertUserCRMToken(...)`)

### Adapter / Conversion Helper Functions
Follow `{Subject}{Target}From{Source}` structure:
* `TenantDtoFromSql`
* `CrmConfigUpsertSqlFromDto`

---

## Dependency Rule

```
api                    → app/*/usecase, common/auth, common/*
app/*/usecase          → app/*/adapter, app/*/repo, pkg/crmclient, common/*
app/*/adapter          → common/*  only
app/*/repo             → common/*  only  (receives pgx pool, returns domain types)
pkg/crmclient          → common/sales
pkg/crmclient/*client  → common/sales, common/httpclient
pkg/db, pkg/r2,
pkg/workos, pkg/jobs   → stdlib + third-party only
common/*               → stdlib only  ← keystone invariant; enforce with depguard
```

`common/*` importing nothing internal is the invariant that keeps domain types stable
and prevents cycles. All Odoo-specific JSON shapes and field mappings are confined to
`pkg/crmclient/odooclient/types.go` and `adapter.go` and never escape that package.

---

## Auth Context Pattern

Every authenticated request carries a typed `AuthUser` injected into `context.Context`
by `SessionMiddleware`. Any layer — handler, usecase, repo — reads it without needing
auth parameters in function signatures.

```go
// common/auth/auth.go
type AuthUser struct {
    UserID      string
    TenantID    string
    Role        Role   // admin | member
    CRMProvider string // odoo | none
    CRMBaseURL  string
    CRMToken    string // user-level token > tenant-level token
}

func NewCtx(ctx context.Context, u AuthUser) context.Context
func FromCtx(ctx context.Context) (AuthUser, bool)
func MustFromCtx(ctx context.Context) AuthUser  // panics if missing
```

CRM credentials are resolved **once in the middleware** (user token > tenant token > none)
so usecases never query the DB for tokens — they call `auth.MustFromCtx(ctx)` and build
the CRM client directly from the credentials already in the context.

The CRM **client** is never stored in the context — only the credentials. The client is
built from the registry inside the usecase.

---

## CRM Interface + Registry Pattern

`common/sales.CRM` is the single interface all CRM providers implement.

```go
type CRM interface {
    LeadList(ctx, LeadFilter) (Page[Lead], error)
    LeadFind(ctx, id) (LeadDetail, error)
    ProjectList(ctx, ProjectFilter) (Page[Project], error)
    ProjectFind(ctx, id) (ProjectDetail, error)
    UnitList(ctx, UnitFilter) (Page[Unit], error)
    ActivityList(ctx, ActivityFilter) (Page[Activity], error)
    ActivitySchedule(ctx, ActivityDraft) (Activity, error)
    ActivityComplete(ctx, id, feedback) (ActivityResult, error)
}
```

Providers that don't support a method return `sales.ErrNotImplemented`.
The registry builds and caches a client per tenant:

```go
// usecase resolves the client per request
user := auth.MustFromCtx(ctx)
crm, err := u.reg.Build(sales.CRMConfig{
    Provider: user.CRMProvider,
    BaseURL:  user.CRMBaseURL,
    APIKey:   user.CRMToken,
})
```

---

## HTMX Rendering Patterns

Three patterns are used across the app. All rendering is server-side.

### 1. Content swap (navigation)
`HX-Request: true` → server renders only the `{{define "content"}}` block.
`hx-target="#content" hx-swap="innerHTML" hx-push-url="true"` on nav links and chips.

### 2. Form-change trigger (filters)
A `<form hx-get="..." hx-trigger="change">` wraps all filter selects.
Any select change fires a GET with all form fields serialised as query params.
A hidden input carries filter values that live outside the form (e.g. state from chips).

### 3. Sentinel infinite scroll
A `<div hx-trigger="intersect once" hx-swap="outerHTML">` at the bottom of each page.
When it enters the viewport it fires a GET with `scroll_token` and replaces itself with
the next batch of rows plus a new sentinel (if more pages exist).
An `hx-swap-oob` element in the response updates out-of-band elements (e.g. the counter).

---

## Odoo Addon (`odoo/addons/trigger_crm_api/`)

The Odoo side is a Python addon that exposes a REST API consumed by the Go server.
All serialisation lives in `controllers/main.py` (`_serialize_*` helpers).
The Go client mirrors those shapes exactly in `odooclient/types.go`.

Key constraint: the Go app never reads Odoo's PostgreSQL directly.
All CRM data flows through the REST API with bearer token auth.

---

## Data Boundary

| Data | Owner | Storage |
|------|-------|---------|
| Scheduled tasks (activities) | CRM (Odoo) | Odoo PostgreSQL |
| Lead / contact data | CRM (Odoo) | Odoo PostgreSQL |
| Call recording (audio file) | Trigger | Cloudflare R2 |
| Call record metadata | Trigger | Trigger PostgreSQL |
| Transcript, summary, enrichment | Trigger | Trigger PostgreSQL |
| Tenant / user / auth config | Trigger | Trigger PostgreSQL |

Trigger never writes enrichment back to the CRM.
The only write path to the CRM is completing or scheduling an activity.
