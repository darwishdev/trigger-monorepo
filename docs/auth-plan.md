# Auth Plan — PostgreSQL + WorkOS + CRM Config

Covers everything needed before the calls page can be built:
database foundation, identity repos, WorkOS login, session middleware,
CRM config screens, and wiring auth context into the existing CRM layer.

Each step has a clear completion check. Do them in order — each one unblocks the next.

---

## Step 1 — PostgreSQL Setup

**Goal:** PostgreSQL running locally; Go can connect and run migrations.

### Docker Compose

Add a `postgres` service to `apps/web/docker-compose.yml` (create if it doesn't exist):

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: trigger
      POSTGRES_USER: trigger
      POSTGRES_PASSWORD: trigger
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
volumes:
  pgdata:
```

### Go packages

```
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
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

Embeds SQL files from `pkg/db/migrations/` and runs them in order on startup.
Use a `schema_migrations` table to track applied versions (simple, no ORM).

### Migration files

`pkg/db/migrations/001_identity.sql`
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

`pkg/db/migrations/002_crm_config.sql`
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

### Config additions (`config/dev.env.example`)

```
DATABASE_URL=postgres://trigger:trigger@localhost:5432/trigger
```

### Done when

`go test ./pkg/db/...` connects, runs migrations, and passes against the local DB.

---

## Step 2 — Identity Domain Types

**Goal:** Clean domain types for tenant, user, CRM config — no DB details.

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
    ID            string
    TenantID      string
    WorkOSUserID  string
    Name          string
    Email         string
    Role          Role
    CreatedAt     time.Time
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

## Step 3 — Identity Repos

**Goal:** Typed DB query functions for all identity tables.

### `app/identity/repo/repo.go`

```go
package identityrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }
```

### Functions to implement

```go
// tenant resolution
FindTenantByWorkOSOrgID(ctx, orgID string) (identity.Tenant, error)
CreateTenant(ctx, name, workosOrgID string) (identity.Tenant, error)

// user resolution
FindUserByWorkOSID(ctx, workosUserID string) (identity.User, error)
FindOrCreateUser(ctx, workosUserID, tenantID, name, email string) (identity.User, error)

// CRM config (admin manages, middleware reads)
GetCRMConfig(ctx, tenantID string) (identity.CRMConfig, error)
UpsertCRMConfig(ctx, tenantID, provider, baseURL, apiKey string) error

// per-user token (user manages, middleware reads)
GetUserCRMToken(ctx, userID string) (identity.UserCRMToken, error)
UpsertUserCRMToken(ctx, userID, provider, token string) error
```

All functions return domain types from `common/identity` — no pgx types leak out.

### Done when

Integration tests for all functions pass against the local DB.

---

## Step 4 — WorkOS Auth Flow

**Goal:** Users can log in via WorkOS; Go exchanges the code and gets a user profile.

### `pkg/workos/workos.go`

```go
package workos

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
)

type Client struct {
    apiKey   string
    clientID string
    appURL   string
    http     *http.Client
}

type WorkOSUser struct {
    ID             string
    Email          string
    FirstName      string
    LastName       string
    OrganizationID string // maps to tenant
}

func New(apiKey, clientID, appURL string) *Client

// AuthURL builds the WorkOS OAuth authorization URL.
func (c *Client) AuthURL(state string) string

// ExchangeCode exchanges an OAuth code for a WorkOS user profile.
func (c *Client) ExchangeCode(ctx context.Context, code string) (WorkOSUser, error)
```

### Routes in `api/server.go`

```
GET  /login           → redirect to WorkOS AuthURL
GET  /auth/callback   → ExchangeCode → FindOrCreateUser → set session cookie → redirect /
GET  /logout          → clear session cookie → redirect /login
```

### Session cookie

- Signed with `SESSION_SECRET` (HMAC-SHA256 over `userID:tenantID:expiry`)
- HTTP-only, Secure, SameSite=Lax
- 24h expiry; refresh on each request

### Config additions

```
WORKOS_API_KEY=sk_...
WORKOS_CLIENT_ID=client_...
APP_URL=http://localhost:8080
SESSION_SECRET=32-random-bytes-hex
```

### Done when

Browser login flow works end-to-end: click login → WorkOS → callback → session set → redirect to `/activities`.
Session survives page reload.

---

## Step 5 — Session Middleware + Auth Context

**Goal:** Every authenticated request carries `AuthUser` in context; unauthenticated requests redirect to `/login`.

### `api/middleware.go`

```go
// SessionMiddleware validates the session cookie, loads the user and CRM
// credentials from the DB, and injects AuthUser into the request context.
// Unauthenticated requests are redirected to /login.
func (s *Server) SessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID, tenantID, err := s.parseSessionCookie(r)
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }

        // load user from DB
        user, err := s.identityRepo.FindUserByID(r.Context(), userID)
        // load CRM config for tenant
        crmConfig, err := s.identityRepo.GetCRMConfig(r.Context(), tenantID)
        // load user's personal token (may be empty)
        userToken, _ := s.identityRepo.GetUserCRMToken(r.Context(), userID)

        // resolve: user token > tenant token
        token := crmConfig.APIKey
        if userToken.Token != "" {
            token = userToken.Token
        }

        authUser := auth.AuthUser{
            UserID:      user.ID,
            TenantID:    tenantID,
            Role:        auth.Role(user.Role),
            CRMProvider: crmConfig.Provider,
            CRMBaseURL:  crmConfig.BaseURL,
            CRMToken:    token,
        }

        ctx := auth.NewCtx(r.Context(), authUser)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// AdminMiddleware rejects non-admin users with 403.
func (s *Server) AdminMiddleware(next http.Handler) http.Handler
```

### Route wrapping in `RegisterRoutes`

```go
protected := s.SessionMiddleware(mux)   // wraps all app routes
// /login, /auth/callback, /logout, /health are exempt
```

### Done when

- Visiting `/activities` without a session redirects to `/login`
- After login, `/activities` loads with real data scoped to the logged-in user's tenant

---

## Step 6 — CRM Config Screen

**Goal:** Admin can connect their Odoo instance; change takes effect on next request.

### `api/config.go`

```
GET  /settings              renders config.html with current crm_config + user_crm_token
POST /settings/crm          (admin only) upsert crm_config; invalidate registry cache for tenant
POST /settings/token        upsert user_crm_token for logged-in user
```

### `templates/config.html`

Two HTMX forms on the same page:

**Tenant CRM config** (admin only — hide form if `AuthUser.Role != admin`):
- Provider select: `None` | `Odoo`
- Base URL input (shown when Odoo selected)
- API Key input
- Submit → `POST /settings/crm` → HTMX swaps form with success message

**Personal Odoo token** (all users):
- Token input
- Submit → `POST /settings/token` → HTMX swaps with success message

### Registry cache invalidation

After `POST /settings/crm` succeeds, evict the cached CRM client for the tenant:
```go
s.registry.Evict(tenantID)
```

Add `Evict(tenantID string)` to the registry.

### Done when

Admin saves Odoo config → activities page loads from that Odoo instance.
User saves personal token → their requests use that token.

---

## Step 7 — Auth-Aware CRM Resolution

**Goal:** Remove the hardcoded `defaultTenant` from the sales usecase; read credentials from context.

### Change in `app/sales/usecase/usecase.go`

```go
// Before
const defaultTenant = "default"
crm, err := u.reg.Build(defaultTenant, u.cfg)

// After
user := auth.MustFromCtx(ctx)
crm, err := u.reg.Build(sales.CRMConfig{
    Provider: user.CRMProvider,
    BaseURL:  user.CRMBaseURL,
    APIKey:   user.CRMToken,
})
```

No changes to adapter, templates, or Odoo client — only the usecase's credential source changes.

### Done when

Two users with different Odoo API tokens each see their own CRM data.
A user with no CRM config sees an empty state (no error, just no data).
