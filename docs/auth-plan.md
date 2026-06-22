# Auth Plan — PostgreSQL + sqlc + WorkOS + CRM Config

Covers everything needed before the calls page can be built:
database foundation, identity repos, WorkOS login, session middleware,
CRM config screens, and wiring auth context into the existing CRM layer.

Each step has a clear completion check. Do them in order — each one unblocks the next.

---

## Step 1 — PostgreSQL + Migrations

**Goal:** Go connects to a local PostgreSQL instance and runs migrations on startup.

Trigger uses a native PostgreSQL instance on both development and production.
Locally you run Postgres directly on your machine. In production you use any hosted
PostgreSQL solution (Supabase, Neon, Railway, etc.). Only the `DATABASE_URL` changes
between environments.

### Go packages

```
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
```

### Config (`config/dev.env.example`)

```
DATABASE_URL=postgres://localhost:5432/trigger?sslmode=disable
```

### `pkg/db/db.go`

```go
package db

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
)

func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        return nil, fmt.Errorf("db: connect: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("db: ping: %w", err)
    }
    return pool, nil
}
```

### `pkg/db/migrate.go`

Embeds all `.sql` files from `pkg/db/migrations/` and runs them in filename order
on every startup. Tracks applied files in a `schema_migrations` table — no external
migration tool needed.

### `pkg/db/migrations/001_identity.sql`

```sql
CREATE TABLE tenants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    workos_org_id TEXT UNIQUE NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    workos_user_id  TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'member',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `pkg/db/migrations/002_crm_config.sql`

```sql
CREATE TABLE crm_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL UNIQUE REFERENCES tenants(id),
    provider    TEXT NOT NULL DEFAULT 'none',
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
```

### Done when

`go test ./pkg/db/...` connects to local Postgres, runs migrations, and passes.

---

## Step 2 — sqlc Setup

**Goal:** sqlc generates type-safe Go query functions from `.sql` files.
The generated code is never edited by hand.

### Install sqlc CLI

```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

sqlc is a codegen tool — it is not imported into Go code and has no runtime dependency.

### Folder layout

```
pkg/db/
  migrations/       SQL migration files  (run on startup by migrate.go)
  queries/          SQL query files      (read by sqlc at codegen time only)
    identity.sql
    calls.sql       (added when the calls feature is built)
  store/            generated Go package (never edit manually)
    db.go           DBTX interface + New(*pgxpool.Pool) *Queries
    models.go       generated row structs
    identity.sql.go generated query functions
    calls.sql.go
  sqlc.yaml
```

### `pkg/db/sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema:  "migrations/"
    gen:
      go:
        package:                      "store"
        out:                          "store/"
        sql_package:                  "pgx/v5"
        emit_pointers_for_null_types: true
```

### `pkg/db/queries/identity.sql`

```sql
-- name: FindTenantByWorkOSOrgID :one
SELECT * FROM tenants WHERE workos_org_id = $1;

-- name: CreateTenant :one
INSERT INTO tenants (name, workos_org_id)
VALUES ($1, $2)
RETURNING *;

-- name: FindUserByWorkOSID :one
SELECT * FROM users WHERE workos_user_id = $1;

-- name: CreateUser :one
INSERT INTO users (tenant_id, workos_user_id, name, email, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: FindUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetCRMConfig :one
SELECT * FROM crm_configs WHERE tenant_id = $1;

-- name: UpsertCRMConfig :exec
INSERT INTO crm_configs (tenant_id, provider, base_url, api_key, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id) DO UPDATE
SET provider   = EXCLUDED.provider,
    base_url   = EXCLUDED.base_url,
    api_key    = EXCLUDED.api_key,
    updated_at = now();

-- name: GetUserCRMToken :one
SELECT * FROM user_crm_tokens WHERE user_id = $1;

-- name: UpsertUserCRMToken :exec
INSERT INTO user_crm_tokens (user_id, provider, token, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id) DO UPDATE
SET provider   = EXCLUDED.provider,
    token      = EXCLUDED.token,
    updated_at = now();
```

### Generate

```
cd apps/web/pkg/db && sqlc generate
```

Re-run this command every time a query file changes. Commit the generated `store/` files.

### Done when

`sqlc generate` runs without errors. `go build ./...` passes.

---

## Step 3 — Identity Domain Types

**Goal:** Clean domain types that the rest of the app works with.
No sqlc or pgx types ever leave the repo layer.

### `common/identity/identity.go`

```go
package identity

import "time"

type Role string

const (
    RoleAdmin  Role = "admin"
    RoleMember Role = "member"
)

type Tenant struct {
    ID          string
    Name        string
    WorkOSOrgID string
    CreatedAt   time.Time
}

type User struct {
    ID           string
    TenantID     string
    WorkOSUserID string
    Name         string
    Email        string
    Role         Role
    CreatedAt    time.Time
}

type CRMConfig struct {
    ID       string
    TenantID string
    Provider string // odoo | none
    BaseURL  string
    APIKey   string
}

type UserCRMToken struct {
    ID       string
    UserID   string
    Provider string
    Token    string
}
```

---

## Step 4 — Identity Adapter + Repo

### `app/identity/adapter/adapter.go`

Pure functions that convert sqlc row structs → domain types and API request params →
domain inputs. All conversion for the identity bounded context lives here — the same
role that `odooclient/adapter.go` plays for the sales domain.

```go
package identityadapter

import (
    "trigger/apps/web/common/identity"
    "trigger/apps/web/pkg/db/store"
)

func TenantFromRow(r store.Tenant) identity.Tenant
func UserFromRow(r store.User) identity.User
func CRMConfigFromRow(r store.CrmConfig) identity.CRMConfig
func UserTokenFromRow(r store.UserCrmToken) identity.UserCRMToken
```

### `app/identity/repo/repo.go`

**Goal:** Raw DB operations only. The repo calls sqlc, passes results through the adapter,
and returns domain types. No conversion logic lives here.
The `*store.Queries` struct is injected — the repo never builds its own DB connection.

```go
package identityrepo

import (
    "trigger/apps/web/app/identity/adapter"
    "trigger/apps/web/pkg/db/store"
)

type Repo struct {
    q *store.Queries
}

func New(q *store.Queries) *Repo {
    return &Repo{q: q}
}
```

### Methods on `Repo`

```go
FindTenantByWorkOSOrgID(ctx, orgID string) (identity.Tenant, error)
CreateTenant(ctx, name, workosOrgID string) (identity.Tenant, error)

FindUserByID(ctx, id string) (identity.User, error)
FindUserByWorkOSID(ctx, workosUserID string) (identity.User, error)

// FindOrCreateUser: calls FindUserByWorkOSID first;
// if pgx.ErrNoRows → calls CreateUser in the same transaction.
FindOrCreateUser(ctx, tenantID, workosUserID, name, email string) (identity.User, error)

GetCRMConfig(ctx, tenantID string) (identity.CRMConfig, error)
UpsertCRMConfig(ctx, tenantID, provider, baseURL, apiKey string) error

GetUserCRMToken(ctx, userID string) (identity.UserCRMToken, error)
UpsertUserCRMToken(ctx, userID, provider, token string) error
```

### Wiring in `main.go`

```go
pool, _ := db.New(ctx, cfg.DatabaseURL)
q := store.New(pool)          // one store, shared

identityRepo := identityrepo.New(q)
callsRepo    := callsrepo.New(q)   // same store, added in calls plan
```

### Done when

Integration tests call each method against a local Postgres DB.
All methods return domain types — no `store.*` types are visible to callers.

---

## Step 5 — WorkOS Auth Flow

**Goal:** Users log in via WorkOS; Go exchanges the code and creates a session.

### `pkg/workos/workos.go`

```go
package workos

type Client struct {
    apiKey   string
    clientID string
    appURL   string
    http     *http.Client
}

type Profile struct {
    ID             string
    Email          string
    FirstName      string
    LastName       string
    OrganizationID string
}

func New(apiKey, clientID, appURL string) *Client

// AuthURL returns the WorkOS OAuth redirect URL.
func (c *Client) AuthURL(state string) string

// ExchangeCode exchanges the OAuth code from the callback for a user profile.
func (c *Client) ExchangeCode(ctx context.Context, code string) (Profile, error)
```

### Routes

```
GET /login          redirect to WorkOS AuthURL
GET /auth/callback  ExchangeCode → FindOrCreateUser → set session cookie → redirect /
GET /logout         clear session cookie → redirect /login
```

### Session cookie

- Signed with `SESSION_SECRET` using HMAC-SHA256
- Payload: `userID:tenantID:expiry`
- Flags: HTTP-only, Secure, SameSite=Lax
- Lifetime: 24 hours, refreshed on each request

### Config additions

```
WORKOS_API_KEY=sk_...
WORKOS_CLIENT_ID=client_...
APP_URL=http://localhost:8080
SESSION_SECRET=32-random-bytes-hex
```

### Done when

Login → WorkOS → callback → session cookie set → redirect to `/activities`.
Session survives a page reload.

---

## Step 6 — Session Middleware + Auth Context

**Goal:** Every authenticated request carries `AuthUser` in context.
Unauthenticated requests are redirected to `/login`.

### `common/auth/auth.go`

```go
package auth

import "context"

type Role string

const (
    RoleAdmin  Role = "admin"
    RoleMember Role = "member"
)

type AuthUser struct {
    UserID      string
    TenantID    string
    Role        Role
    CRMProvider string
    CRMBaseURL  string
    CRMToken    string // user-level token if set, otherwise tenant-level
}

type ctxKey struct{}

func NewCtx(ctx context.Context, u AuthUser) context.Context {
    return context.WithValue(ctx, ctxKey{}, u)
}

func FromCtx(ctx context.Context) (AuthUser, bool) {
    u, ok := ctx.Value(ctxKey{}).(AuthUser)
    return u, ok
}

func MustFromCtx(ctx context.Context) AuthUser {
    u, ok := FromCtx(ctx)
    if !ok {
        panic("auth: no AuthUser in context — missing SessionMiddleware?")
    }
    return u
}
```

### `api/middleware.go` — SessionMiddleware

```go
func (s *Server) SessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID, err := s.parseSessionCookie(r)
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }

        user, err := s.identityRepo.FindUserByID(r.Context(), userID)
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }

        crmConfig, _ := s.identityRepo.GetCRMConfig(r.Context(), user.TenantID)
        userToken, _ := s.identityRepo.GetUserCRMToken(r.Context(), userID)

        token := crmConfig.APIKey
        if userToken.Token != "" {
            token = userToken.Token
        }

        ctx := auth.NewCtx(r.Context(), auth.AuthUser{
            UserID:      user.ID,
            TenantID:    user.TenantID,
            Role:        auth.Role(user.Role),
            CRMProvider: crmConfig.Provider,
            CRMBaseURL:  crmConfig.BaseURL,
            CRMToken:    token,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// AdminMiddleware rejects non-admin users with 403.
func (s *Server) AdminMiddleware(next http.Handler) http.Handler
```

`/login`, `/auth/callback`, `/logout`, and `/health` are exempt from `SessionMiddleware`.
All other routes are wrapped.

### Done when

- Visiting `/activities` without a session redirects to `/login`
- After login, `/activities` loads real data scoped to the logged-in user's tenant

---

## Step 7 — CRM Config Screen

**Goal:** Admin connects the tenant Odoo instance; users add their personal token.

### Routes

```
GET  /settings         render config page showing current crm_config + user token
POST /settings/crm     admin only — upsert crm_config; evict tenant's cached CRM client
POST /settings/token   any user — upsert user_crm_tokens for the logged-in user
```

### `templates/config.html`

Two independent HTMX forms:

**Tenant CRM config** (rendered only when `AuthUser.Role == admin`):
- Provider select: `None` | `Odoo`
- Base URL input
- API Key input
- `hx-post="/settings/crm"` → swaps form area with a success confirmation

**Personal Odoo token** (all users):
- Token input
- `hx-post="/settings/token"` → swaps field with a success confirmation

After `POST /settings/crm` the registry evicts the cached client for this tenant
so the next request builds a fresh one with the new credentials:

```go
s.registry.Evict(auth.MustFromCtx(r.Context()).TenantID)
```

Add `Evict(tenantID string)` to `pkg/crmclient/registry.go`.

### Done when

Admin saves Odoo config → next visit to `/activities` fetches from that Odoo instance.
User saves personal token → their requests use it instead of the tenant key.

---

## Step 8 — Auth-Aware CRM Resolution

**Goal:** The sales usecase reads CRM credentials from context instead of a hardcoded value.

### `app/sales/usecase/usecase.go`

```go
user := auth.MustFromCtx(ctx)
crm, err := u.reg.Build(sales.CRMConfig{
    Provider: user.CRMProvider,
    BaseURL:  user.CRMBaseURL,
    APIKey:   user.CRMToken,
})
```

No changes to adapters, templates, Odoo client, or any other layer.

### Done when

Two users with different Odoo tokens each see their own CRM data.
A user whose tenant has no CRM config sees an empty state with no error.
