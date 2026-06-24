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
-- name: TenantFindByWorkOSOrgID :one
SELECT * FROM tenants WHERE workos_org_id = $1;

-- name: TenantCreate :one
INSERT INTO tenants (name, workos_org_id)
VALUES ($1, $2)
RETURNING *;

-- name: UserFindByWorkOSID :one
SELECT * FROM users WHERE workos_user_id = $1;

-- name: UserCreate :one
INSERT INTO users (tenant_id, workos_user_id, name, email, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UserFindByID :one
SELECT * FROM users WHERE id = $1;

-- name: CRMConfigGet :one
SELECT * FROM crm_configs WHERE tenant_id = $1;

-- name: CRMConfigUpsert :exec
INSERT INTO crm_configs (tenant_id, provider, base_url, api_key, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id) DO UPDATE
SET provider   = EXCLUDED.provider,
    base_url   = EXCLUDED.base_url,
    api_key    = EXCLUDED.api_key,
    updated_at = now();

-- name: UserCRMTokenGet :one
SELECT * FROM user_crm_tokens WHERE user_id = $1;

-- name: UserCRMTokenUpsert :exec
INSERT INTO user_crm_tokens (user_id, provider, token, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id) DO UPDATE
SET provider   = EXCLUDED.provider,
    token      = EXCLUDED.token,
    updated_at = now();

-- name: UserAuthContextGet :one
SELECT 
    u.id AS user_id,
    u.tenant_id,
    u.role,
    COALESCE(cc.provider, 'none')::TEXT AS crm_provider,
    COALESCE(cc.base_url, '')::TEXT AS crm_base_url,
    COALESCE(uct.token, cc.api_key, '')::TEXT AS crm_token
FROM users u
LEFT JOIN crm_configs cc ON cc.tenant_id = u.tenant_id
LEFT JOIN user_crm_tokens uct ON uct.user_id = u.id
WHERE u.workos_user_id = $1;
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

// Naming convention: {Subject}{Target}From{Source}
//   store row → domain DTO:  TenantDtoFromSql, UserDtoFromSql, ...
//   domain DTO → sql params: CrmConfigUpsertSqlFromDto, ...

// responses (source Sql, target Dto)
func TenantDtoFromSql(r store.Tenant) identity.Tenant
func UserDtoFromSql(r store.User) identity.User
func CrmConfigDtoFromSql(r store.CrmConfig) identity.CRMConfig
func UserCrmTokenDtoFromSql(r store.UserCrmToken) identity.UserCRMToken

// requests (source Dto, target Sql params)
func CrmConfigUpsertSqlFromDto(tenantID, provider, baseURL, apiKey string) store.CRMConfigUpsertParams
func UserCrmTokenUpsertSqlFromDto(userID, provider, token string) store.UserCRMTokenUpsertParams
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
TenantFindByWorkOSOrgID(ctx, orgID string) (identity.Tenant, error)
TenantCreate(ctx, name, workosOrgID string) (identity.Tenant, error)

UserFindByID(ctx, id string) (identity.User, error)
UserFindByWorkOSID(ctx, workosUserID string) (identity.User, error)

// UserFindOrCreate: calls UserFindByWorkOSID first;
// if pgx.ErrNoRows → calls UserCreate in the same transaction.
UserFindOrCreate(ctx, tenantID, workosUserID, name, email string) (identity.User, error)

CRMConfigGet(ctx, tenantID string) (identity.CRMConfig, error)
CRMConfigUpsert(ctx, tenantID, provider, baseURL, apiKey string) error

UserCRMTokenGet(ctx, userID string) (identity.UserCRMToken, error)
UserCRMTokenUpsert(ctx, userID, provider, token string) error

// UserAuthContextGet fetches complete user, role, and CRM credentials with token precedence in a single optimized DB roundtrip
UserAuthContextGet(ctx, workosUserID string) (auth.AuthUser, error)
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

**Goal:** Users log in via WorkOS AuthKit; the SDK manages the sealed session cookie.

### Package

No custom wrapper needed. Use the official SDK directly:

```
go get -u github.com/workos/workos-go/...
```

The SDK (v9+) uses a shared `workos.Client` with service accessors.
WorkOS AuthKit manages the sealed session cookie — there is no custom HMAC cookie,
no `SESSION_SECRET`, and no manual cookie rewriting.

### Initialise in `main.go`

```go
wosClient := workos.NewClient(cfg.WorkOSAPIKey)
```

Inject `wosClient` into the Server.

### Routes

```
GET /login          → redirect to WorkOS AuthorizationURL
GET /auth/callback  → AuthenticateWithCode → UserFindOrCreate → redirect /
GET /logout         → RevokeSession → redirect /login
```

### `/login` handler

```go
url, err := s.wos.UserManagement().GetAuthorizationURL(
    usermanagement.GetAuthorizationURLParams{
        ClientID:    cfg.WorkOSClientID,
        RedirectURI: cfg.WorkOSRedirectURI,
        Provider:    "authkit",
    },
)
http.Redirect(w, r, url, http.StatusFound)
```

### `/auth/callback` handler

```go
result, err := s.wos.UserManagement().AuthenticateWithCode(ctx,
    usermanagement.AuthenticateWithCodeParams{
        ClientID: cfg.WorkOSClientID,
        Code:     r.URL.Query().Get("code"),
    },
)
// result.User      → WorkOS user profile (ID, Email, FirstName, LastName)
// result.OrganizationID → maps to our tenant
// result.SealedSession  → set as the session cookie

user, err := s.identityRepo.UserFindOrCreate(ctx,
    result.OrganizationID,
    result.User.ID,
    result.User.FirstName+" "+result.User.LastName,
    result.User.Email,
)

http.SetCookie(w, &http.Cookie{
    Name:     "wos-session",
    Value:    result.SealedSession,
    Path:     "/",
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLaxMode,
})
http.Redirect(w, r, "/", http.StatusFound)
```

### `/logout` handler

```go
sessionCookie, _ := r.Cookie("wos-session")
s.wos.UserManagement().RevokeSession(ctx, usermanagement.RevokeSessionParams{
    SessionID: sessionCookie.Value,
})
http.SetCookie(w, &http.Cookie{Name: "wos-session", MaxAge: -1})
http.Redirect(w, r, "/login", http.StatusFound)
```

### Config additions

```
WORKOS_API_KEY=sk_...
WORKOS_CLIENT_ID=client_...
WORKOS_REDIRECT_URI=http://localhost:8080/auth/callback
```

### Done when

Login → WorkOS → callback → sealed session cookie set → redirect to `/activities`.
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

### `api/middleware.go` — Session Middleware & authenticateOrRefresh

Includes the Redis cache layers, pre-computed cache keys, and a dedicated token verification and refresh sub-function.

```go
type SessionAuthResult struct {
    UserID   string // The authenticated WorkOS User ID
    CacheKey string // The ready-to-use Redis cache key (pre-hashed)
}

func (s *Server) getSessionCacheKey(cookieVal string) string {
    return fmt.Sprintf("session:%x", sha256.Sum256([]byte(cookieVal)))
}

// authenticateOrRefresh validates the session. If expired, it refreshes the session,
// writes the updated cookie, and returns the pre-computed ready-to-use Redis CacheKey.
func (s *Server) authenticateOrRefresh(w http.ResponseWriter, r *http.Request, session *workos.Session, originalCookieVal string) (SessionAuthResult, error) {
    ctx := r.Context()

    // 1. Try local authentication (validates token locally via cached JWKS public keys)
    authResult, err := session.Authenticate(ctx)
    if err == nil && authResult.Authenticated {
        return SessionAuthResult{
            UserID:   authResult.User.ID,
            CacheKey: s.getSessionCacheKey(originalCookieVal),
        }, nil
    }

    // 2. Token Expired -> Attempt Refresh
    log.Printf("middleware: local token is expired/invalid. Attempting session refresh...")
    refreshResult, err := session.Refresh(ctx, workos.SessionRefreshParams{})
    if err != nil {
        return SessionAuthResult{}, err
    }
    if !refreshResult.Authenticated {
        return SessionAuthResult{}, errors.New("refreshed session is unauthenticated")
    }

    // 3. Write refreshed cookie to response writer
    http.SetCookie(w, &http.Cookie{
        Name:     "wos-session",
        Value:    refreshResult.SealedSession,
        Path:     "/",
        HttpOnly: true,
        Secure:   true,
        SameSite: http.SameSiteLaxMode,
    })

    return SessionAuthResult{
        UserID:   refreshResult.User.ID,
        CacheKey: s.getSessionCacheKey(refreshResult.SealedSession),
    }, nil
}

func (s *Server) SessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        // 1. Early Return: Validate cookie presence
        cookie, err := r.Cookie("wos-session")
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }
        cookieVal := cookie.Value

        // Generate the original cache key
        originalCacheKey := s.getSessionCacheKey(cookieVal)

        // 2. Fast Path: Read AuthUser directly from Redis
        if authUser, found := s.getCachedSession(ctx, originalCacheKey); found {
            authCtx := auth.NewCtx(ctx, authUser)
            next.ServeHTTP(w, r.WithContext(authCtx))
            return
        }

        // 3. Slow Path (Cache Miss): Load WorkOS session
        session := workos.NewSession(s.wos, cookieVal, s.cfg.WorkOSCookiePassword)

        // 4. Early Return: Authenticate/refresh session (returns ready-to-use pre-computed cache key)
        result, err := s.authenticateOrRefresh(w, r, session, cookieVal)
        if err != nil {
            log.Printf("middleware: session authentication failed: %v", err)
            s.clearSessionCookie(w)
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }

        // 5. Early Return: Fetch complete user auth context using consolidated SQL query
        authUser, err := s.identityRepo.UserAuthContextGet(ctx, result.UserID)
        if err != nil {
            if errors.Is(err, pgx.ErrNoRows) {
                log.Printf("middleware: authenticated WorkOS user %s has no internal DB record", result.UserID)
                http.Error(w, "Forbidden: User record not found.", http.StatusForbidden)
                return
            }
            log.Printf("middleware: DB error fetching user auth context: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        // 6. Save resolved session context to Redis using the helper's computed ready-to-use key
        s.setCachedSession(ctx, result.CacheKey, authUser)

        // 7. If cache key changed (which implicitly means cookie was refreshed), evict the old key
        if result.CacheKey != originalCacheKey {
            s.evictCachedSession(ctx, originalCacheKey)
        }

        // 8. Inject final AuthUser payload into the request context and proceed
        authCtx := auth.NewCtx(ctx, authUser)
        next.ServeHTTP(w, r.WithContext(authCtx))
    })
}

// AdminMiddleware rejects non-admin users with 403.
func (s *Server) AdminMiddleware(next http.Handler) http.Handler
```

`/login`, `/auth/callback`, `/logout`, and `/health` are exempt from `SessionMiddleware`.
All other routes are wrapped.

The cookie is only rewritten when WorkOS actually refreshes the access token
(`result.SealedSession != ""`), not on every request.

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
