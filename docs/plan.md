# Trigger AI — Full Implementation Plan

## What We Are Building

Trigger is a sales intelligence layer that sits on top of a CRM (Odoo today, others later).
It records phone calls made by salespeople, transcribes them, enriches them with AI, and
surfaces the results alongside the CRM's scheduled activity queue.

The central screen is the **Calls page**: a merged view of CRM call tasks and recorded call
artifacts, with an extraction pipeline that adds transcript, summary, and outcome to each
matched call.

---

## Architecture Principles

- **CRM is read/write for tasks, Trigger is read/write for enrichment.** Odoo owns who needs
  to call whom and by when. Trigger owns what was said and what happened.
- **Domain-driven boundaries.** Bounded contexts are named after business subdomains:
  `sales`, `calls`, `identity`, `insights`. External systems live in `pkg/` and never leak
  into domain code.
- **Auth context flows through every layer.** Middleware injects a typed `AuthUser` struct
  into `context.Context`. Any layer — handler, usecase, repo — reads it via `auth.FromCtx(ctx)`.
  No auth params in function signatures.
- **CRM client is resolved per-request from context.** The registry builds a client from the
  credentials in `AuthUser` (user token > tenant token > none). Usecases never query the DB
  for tokens.
- **One flat `sales.CRM` interface.** Providers that don't support a method return
  `ErrNotImplemented`. No capability splitting.

---

## Current State (already built)

| Area | Status |
|------|--------|
| Go web app skeleton (config, main, static, templates) | Done |
| `common/sales` domain DTOs + `CRM` interface | Done |
| `common/httpclient` HTTP caller | Done |
| `pkg/crmclient` registry + per-tenant cache | Done |
| `pkg/crmclient/odooclient` — full Odoo client (leads, projects, units, activities) | Done |
| `app/sales` usecase + adapter | Done |
| `api/server.go` HTMX server | Done |
| Odoo keyset pagination (scroll_token) for activities | Done |
| Activity list page — state/type/sort/dir filters, infinite scroll, live counter | Done |
| Odoo `trigger_crm_api` addon (leads, projects, units, activities) | Done |

---

## Database Schema

```sql
-- Multi-tenant identity
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    workos_org_id TEXT UNIQUE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    workos_user_id  TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'member', -- admin | member
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- CRM configuration
CREATE TABLE crm_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL UNIQUE REFERENCES tenants(id),
    provider    TEXT NOT NULL DEFAULT 'none', -- odoo | none
    base_url    TEXT,
    api_key     TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_crm_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL UNIQUE REFERENCES users(id),
    provider    TEXT NOT NULL,
    token       TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Call records (uploaded by Android APK via Go API → Cloudflare R2)
CREATE TABLE call_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    user_id      UUID NOT NULL REFERENCES users(id),
    phone        TEXT NOT NULL,        -- normalized E.164
    duration_sec INT  NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL, -- from APK metadata
    r2_url       TEXT NOT NULL,        -- public/presigned URL
    r2_key       TEXT NOT NULL,        -- internal R2 object key
    status       TEXT NOT NULL DEFAULT 'uploaded',
                 -- uploaded | transcribing | transcribed | failed
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enrichment (attached to a call_record once extraction runs)
CREATE TABLE call_enrichments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_record_id  UUID NOT NULL UNIQUE REFERENCES call_records(id),
    transcript_text TEXT,
    transcript_url  TEXT,   -- R2 URL of raw transcript file
    summary         TEXT,   -- Claude-generated
    sentiment       TEXT,   -- positive | neutral | negative
    outcome         TEXT,   -- interested | not_interested | follow_up | no_answer
    extracted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON call_records (tenant_id, user_id, started_at DESC);
CREATE INDEX ON call_records (phone, started_at DESC);
```

---

## Auth Context (flows through every layer)

### `common/auth/auth.go`

```go
package auth

import "context"

type Role string
const (
    RoleAdmin  Role = "admin"
    RoleMember Role = "member"
)

// AuthUser is injected into every request context by SessionMiddleware.
// It carries enough to resolve the CRM client and scope all DB queries.
type AuthUser struct {
    UserID   string
    TenantID string
    Role     Role
    // CRM credentials — resolved once in middleware, ready to use
    CRMProvider string // odoo | none
    CRMBaseURL  string
    CRMToken    string // user-level token takes priority over tenant-level
}

type ctxKey struct{}

func NewCtx(ctx context.Context, u AuthUser) context.Context {
    return context.WithValue(ctx, ctxKey{}, u)
}

func FromCtx(ctx context.Context) (AuthUser, bool) {
    u, ok := ctx.Value(ctxKey{}).(AuthUser)
    return u, ok
}

// MustFromCtx panics if the context has no AuthUser.
// Use only inside handlers that are guaranteed to run after SessionMiddleware.
func MustFromCtx(ctx context.Context) AuthUser {
    u, ok := FromCtx(ctx)
    if !ok {
        panic("auth: no AuthUser in context — missing SessionMiddleware?")
    }
    return u
}
```

### Middleware chain

```
request
  │
  ▼ SessionMiddleware
  │  validates session cookie
  │  loads user + tenant from DB
  │  resolves CRM credentials (user token > tenant token)
  │  injects AuthUser into ctx
  │  → 401 if no valid session
  │
  ▼ AdminMiddleware  (applied only to admin routes)
  │  reads AuthUser from ctx
  │  → 403 if role != admin
  │
  ▼ handler
```

---

## Folder Layout (target)

```
apps/web/
  main.go                        # composition root
  config/
  common/
    auth/                        # AuthUser, FromCtx, NewCtx
    sales/                       # CRM interface + domain DTOs (done)
    calls/                       # CallRecord, CallEnrichment domain types
    identity/                    # Tenant, User domain types
  app/
    sales/
      usecase/                   # done
      adapter/                   # done
    calls/
      usecase/                   # merge logic, extraction trigger
      adapter/
      repo/                      # call_records + call_enrichments queries
    identity/
      usecase/                   # tenant/user management
      adapter/
      repo/                      # tenants, users, crm_configs, user_crm_tokens
  api/
    server.go                    # done
    middleware.go                # SessionMiddleware, AdminMiddleware
    calls.go                     # calls page handler
    config.go                    # CRM config screen handler
    upload.go                    # R2 upload endpoint (called by Android APK)
  pkg/
    crmclient/                   # done
    db/                          # pgx pool init, migration runner
    r2/                          # Cloudflare R2 upload client
    workos/                      # WorkOS SDK wrapper (auth code exchange, user profile)
  templates/
    layout.html partials.html    # done
    calls.html                   # calls page
    config.html                  # CRM config + user token screen
    login.html                   # auth screens (or WorkOS hosted)
```

### Dependency rule

```
api                → app/*/usecase, common/auth, common/*
app/*/usecase      → app/*/adapter, app/*/repo, pkg/crmclient, common/*
app/*/adapter      → common/*  only
app/*/repo         → common/*  only  (receives pgx pool, returns domain types)
pkg/crmclient      → common/sales
pkg/db, pkg/r2, pkg/workos → stdlib + third-party only
common/*           → stdlib only  (keystone: nothing internal)
```

---

## Build Phases

---

### Phase 1 — Database + Migrations

**Goal:** PostgreSQL running locally with all tables, accessible from Go.

- Set up PostgreSQL (Docker Compose service in `apps/web/docker-compose.yml`)
- Add `pgx/v5` to `go.mod`
- `pkg/db/db.go` — connection pool init from config (`DATABASE_URL`)
- `pkg/db/migrate.go` — embed and run SQL migrations from `pkg/db/migrations/`
- Migration files: `001_tenants_users.sql`, `002_crm_configs.sql`, `003_call_records.sql`
- Config: add `DATABASE_URL` to `dev.env.example`
- Verification: `go test ./pkg/db/...` connects and runs migrations against a test DB

---

### Phase 2 — Identity Repos

**Goal:** Go structs and DB queries for tenants, users, CRM configs, user tokens.

- `common/identity/` — `Tenant`, `User`, `CRMConfig`, `UserCRMToken` domain types
- `app/identity/repo/` — typed query functions (pgx, no ORM):
  - `FindUserByWorkOSID(ctx, workosUserID) (identity.User, error)`
  - `FindOrCreateUser(ctx, workosUserID, tenantID, name, email) (identity.User, error)`
  - `GetCRMConfig(ctx, tenantID) (identity.CRMConfig, error)`
  - `UpsertCRMConfig(ctx, tenantID, provider, baseURL, apiKey) error`
  - `GetUserCRMToken(ctx, userID) (identity.UserCRMToken, error)`
  - `UpsertUserCRMToken(ctx, userID, provider, token) error`
- Verification: integration tests against test DB

---

### Phase 3 — WorkOS Auth

**Goal:** Users can log in via WorkOS; session cookie is set; context carries `AuthUser`.

- `pkg/workos/workos.go` — thin wrapper:
  - `AuthURL(redirectURI, state) string` — builds WorkOS OAuth URL
  - `ExchangeCode(ctx, code) (WorkOSUser, error)` — exchanges code for user profile
- `WorkOSUser` carries `ID`, `Email`, `Name`, `OrganizationID`
- `api/middleware.go`:
  - `SessionMiddleware` — validates signed session cookie, loads user from DB,
    resolves CRM credentials (user token > tenant token > none), injects `AuthUser` via `auth.NewCtx`
- Routes:
  - `GET /login` — redirects to WorkOS hosted login
  - `GET /auth/callback` — exchanges code, finds/creates user, sets session cookie, redirects to `/`
  - `GET /logout` — clears cookie
- All existing routes wrapped with `SessionMiddleware`
- Config: `WORKOS_API_KEY`, `WORKOS_CLIENT_ID`, `APP_URL`, `SESSION_SECRET` added to `dev.env.example`
- Verification: login flow works end-to-end in browser; session survives page reload

---

### Phase 4 — CRM Config Screen

**Goal:** Admin can connect their Odoo instance; users can add their personal token.

- `api/config.go`:
  - `GET /settings` — renders config page with current CRM config for the tenant
  - `POST /settings/crm` — admin saves tenant CRM config (provider, base_url, api_key)
  - `POST /settings/token` — user saves their personal Odoo API token
- `templates/config.html` — HTMX form; shows provider select (Odoo | None), base URL, API key
- `AdminMiddleware` guards `POST /settings/crm`
- After save, registry cache for this tenant is invalidated
- Verification: admin connects Odoo; activities page loads real data from their instance

---

### Phase 5 — Auth-Aware CRM Resolution

**Goal:** Every CRM call uses the logged-in user's credentials, not a hardcoded config.

- Update `app/sales/usecase/` to read `AuthUser` from ctx instead of `defaultTenant`
- `reg.Build(sales.CRMConfig{Provider: user.CRMProvider, BaseURL: user.CRMBaseURL, APIKey: user.CRMToken})`
- Remove `defaultTenant` constant; remove hardcoded config from usecase
- Verification: two users with different Odoo tokens see their own data

---

### Phase 6 — Call Records Upload (Android APK integration)

**Goal:** Android APK can upload a call recording to Trigger; a `call_record` row is created.

- `pkg/r2/r2.go` — Cloudflare R2 upload client:
  - `Upload(ctx, key, reader, contentType) (url string, err error)`
  - Uses AWS S3-compatible SDK (Cloudflare R2 is S3-compatible)
- `app/calls/repo/` — DB queries:
  - `CreateCallRecord(ctx, tenantID, userID, phone, durationSec, startedAt, r2URL, r2Key) (CallRecord, error)`
  - `ListCallRecords(ctx, tenantID, userID, since time.Time) ([]CallRecord, error)`
  - `GetCallRecord(ctx, id) (CallRecord, error)`
  - `UpdateCallStatus(ctx, id, status) error`
- `api/upload.go`:
  - `POST /calls/upload` — multipart form; fields: `phone`, `duration_sec`, `started_at`, `file`
  - Reads `AuthUser` from ctx (tenantID, userID)
  - Streams audio file to R2
  - Creates `call_record` row
  - Returns `{id, r2_url}` as JSON
- Config: `R2_ACCOUNT_ID`, `R2_ACCESS_KEY`, `R2_SECRET_KEY`, `R2_BUCKET` added to `dev.env.example`
- Auth: this endpoint is called by the APK with a user session token (Bearer), same `SessionMiddleware`
- Verification: `curl` upload creates a DB row and the file appears in R2

---

### Phase 7 — Calls Page (merge CRM + records)

**Goal:** Single page shows CRM call activities merged with recorded call files.

- `common/calls/` — domain types:
  ```go
  type CallRecord struct {
      ID          string
      Phone       string
      DurationSec int
      StartedAt   time.Time
      R2URL       string
      Status      string
      Enrichment  *CallEnrichment
  }

  type CallEnrichment struct {
      Transcript string
      Summary    string
      Sentiment  string
      Outcome    string
      ExtractedAt time.Time
  }

  // MergedCall is one row on the calls page
  type MergedCall struct {
      Activity    *sales.Activity  // nil if unscheduled call
      Record      *CallRecord      // nil if no recording found
  }
  ```
- `app/calls/usecase/calls.go` — merge logic:
  ```
  fetch activities (type=Call) from CRM via sales.CRM
  fetch call_records from DB (user + tenant scoped)
  for each activity: find record where phone matches lead.phone
                     AND record.started_at within activity.deadline ± 4h
  return []MergedCall sorted by started_at desc / deadline asc
  ```
- `api/calls.go` — `GET /calls` handler; keyset-paged; HTMX partial swap
- `templates/calls.html` — merged list:
  - Row with both sides → show recording duration, enable Extract button
  - Row with only activity → "Not recorded" badge
  - Row with only record → "Unscheduled call" label
- Verification: a seeded call_record whose phone matches a CRM lead shows as merged

---

### Phase 8 — Extraction Pipeline

**Goal:** User clicks Extract; transcript and AI enrichment are added to the call record.

- Background job runner — simple goroutine pool in `pkg/jobs/`:
  - `Submit(fn func(ctx context.Context))` — queues work
  - No external queue yet; in-process goroutines are sufficient at pre-scale
- Extraction steps (run sequentially in background):
  1. Fetch audio from R2 (presigned URL)
  2. Send to transcription provider (Deepgram or Whisper API)
  3. Save raw transcript to `call_enrichments.transcript_text`
  4. Send transcript to Claude: prompt returns `{summary, sentiment, outcome}`
  5. Save enrichment fields; set `call_records.status = transcribed`
- `api/calls.go`:
  - `POST /calls/:id/extract` — validates ownership, sets status to `transcribing`,
    submits background job, returns 202 Accepted
  - `GET /calls/:id/status` — returns current status as JSON (polled by HTMX)
- `templates/calls.html` — Extract button:
  - On click: `hx-post="/calls/:id/extract"` → button becomes spinner
  - Spinner polls `GET /calls/:id/status` every 3s via `hx-trigger="every 3s"`
  - On `transcribed`: swap row with enriched view (summary, sentiment badge, outcome)
- Config: `DEEPGRAM_API_KEY` (or `OPENAI_API_KEY` for Whisper), `ANTHROPIC_API_KEY`
- Verification: full cycle — upload → extract → enrichment visible on calls page

---

### Phase 9 — Complete Activity Cycle

**Goal:** User completes a CRM activity through Trigger (not directly in Odoo).

- `api/calls.go`:
  - `POST /calls/:id/complete` — body: `{feedback, outcome}`
  - Calls `crm.ActivityComplete(ctx, activityID, feedback)` (already implemented in odooclient)
  - Writes `outcome` to `call_enrichments` if a linked record exists
  - Returns HTMX fragment updating the row (removes Complete button, shows outcome)
- Verification: completing via Trigger removes the activity from Odoo's open list

---

## Config Reference (all env vars)

| Key | Phase | Description |
|-----|-------|-------------|
| `BASE_URL` | 1 | Odoo base URL (legacy single-tenant, removed in Phase 5) |
| `API_KEY` | 1 | Odoo API key (legacy, removed in Phase 5) |
| `DATABASE_URL` | 1 | PostgreSQL connection string |
| `WORKOS_API_KEY` | 3 | WorkOS secret key |
| `WORKOS_CLIENT_ID` | 3 | WorkOS OAuth client ID |
| `APP_URL` | 3 | Public app URL for OAuth redirect |
| `SESSION_SECRET` | 3 | 32-byte secret for signing session cookies |
| `R2_ACCOUNT_ID` | 6 | Cloudflare account ID |
| `R2_ACCESS_KEY` | 6 | R2 access key |
| `R2_SECRET_KEY` | 6 | R2 secret key |
| `R2_BUCKET` | 6 | R2 bucket name |
| `DEEPGRAM_API_KEY` | 8 | Transcription API key |
| `ANTHROPIC_API_KEY` | 8 | Claude API key for enrichment |

---

## What Is Explicitly Out of Scope

- Engaz or any second CRM provider (the interface is ready; the client is not)
- Lead, Project, Unit pages (domain + client exists; no UI planned yet)
- Push notifications to Android APK
- Multi-region or multi-instance deployment
- Background job persistence (in-process goroutine pool is enough pre-scale)
- Real-time SSE (polling is sufficient for extraction status at this scale)
