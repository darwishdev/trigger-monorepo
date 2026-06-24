# Identity and Authentication Plan

Implements tenant identity, WorkOS authentication, request authorization, CRM credential
management, and authenticated CRM client resolution.

Prerequisites:

- [infrastructure-plan.md](infrastructure-plan.md) is complete.
- [security-invariants.md](security-invariants.md) applies throughout.

This plan owns identity behavior. It does not reimplement database, migration, Redis,
cookie, origin-validation, or encryption infrastructure.

---

## Identity decisions

- A WorkOS organization maps one-to-one to a Trigger tenant.
- A WorkOS user belongs to exactly one Trigger tenant.
- `users.workos_user_id` is globally unique.
- A login for an existing user under another organization is rejected. Tenant transfer is
  an explicit support operation, never an automatic login side effect.
- Production tenants must normally be pre-provisioned. Self-service tenant creation is an
  explicit configuration option intended primarily for development.
- The first user provisioned for a tenant becomes `admin`; later users become `member`.
- Tenant identity is derived only from authenticated WorkOS claims.
- A personal CRM token takes precedence over the tenant CRM key.

---

## Step 1 — Identity migrations

Add `pkg/db/migrations/001_identity.sql`:

```sql
CREATE TABLE tenants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    workos_org_id TEXT UNIQUE NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    workos_user_id TEXT UNIQUE NOT NULL,
    name           TEXT NOT NULL,
    email          TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'member'
                   CHECK (role IN ('admin', 'member')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id)
);

CREATE INDEX users_tenant_id_idx ON users (tenant_id);
```

Add `pkg/db/migrations/002_crm_config.sql`:

```sql
CREATE TABLE crm_configs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL UNIQUE
                      REFERENCES tenants(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL DEFAULT 'none'
                      CHECK (provider IN ('none', 'odoo')),
    base_url          TEXT,
    api_key_encrypted BYTEA,
    version           BIGINT NOT NULL DEFAULT 1,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_crm_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL UNIQUE
                    REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('odoo')),
    token_encrypted BYTEA,
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

All ownership columns use foreign keys. Deletion behavior is deliberate:

- Tenants cannot be deleted while users exist.
- CRM configuration is deleted with its tenant.
- Personal CRM tokens are deleted with their user.

Done when:

- Migration tests verify uniqueness, checks, foreign keys, and deletion behavior.
- A WorkOS user cannot be inserted into two tenants.

---

## Step 2 — sqlc identity queries

Add `pkg/db/queries/identity.sql` with these queries:

```text
TenantFindByWorkOSOrgID
TenantCreate
UserFindByWorkOSID
UserIdentityGet
UserCreate
UserUpdateProfile
UserFindByID
CRMConfigGet
CRMConfigUpsert
CRMConfigAPIKeyClear
UserCRMTokenGet
UserCRMTokenUpsert
UserCRMTokenClear
UserAuthRecordGet
UserListByTenantID
```

Required query behavior:

- `UserIdentityGet` matches both WorkOS user ID and WorkOS organization ID.
- Credential upserts increment their version.
- An empty submitted secret preserves the existing secret.
- Removing a secret is a separate explicit query and increments the version.
- `UserAuthRecordGet` returns encrypted credential columns, never plaintext.
- The repository resolves personal-token-over-tenant-key precedence.

Done when:

- `sqlc generate` succeeds.
- Generated code is committed.
- Query integration tests pass against PostgreSQL.

---

## Step 3 — Identity domain and repository

Add domain types under `common/identity`:

```go
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

type UserIdentity struct {
    UserID       string
    TenantID     string
    WorkOSUserID string
    WorkOSOrgID  string
}

type CRMConfig struct {
    ID        string
    TenantID  string
    Provider  string
    BaseURL   string
    HasAPIKey bool
    Version   int64
}

type UserCRMToken struct {
    ID       string
    UserID   string
    Provider string
    HasToken bool
    Version  int64
}

type EncryptedAuthRecord struct {
    UserID              string
    TenantID            string
    WorkOSUserID        string
    WorkOSOrgID         string
    Role                Role
    CRMProvider         string
    CRMBaseURL          string
    EncryptedCRMToken   []byte
    CRMConfigVersion    int64
    UserCRMTokenVersion int64
}
```

Read models exposed to handlers contain only `HasAPIKey` and `HasToken`. Stored secrets
are never returned to HTML.

The identity repository receives shared `*store.Queries`, transaction support, and the
shared secret-encryption service from the infrastructure layer.

Required methods:

```go
TenantFindByWorkOSOrgID(ctx, orgID string) (identity.Tenant, error)
TenantFindOrCreate(ctx, name, workosOrgID string) (identity.Tenant, error)

UserFindByID(ctx, id string) (identity.User, error)
UserFindByWorkOSID(ctx, workosUserID string) (identity.User, error)
UserIdentityGet(ctx, workosUserID, workosOrgID string) (identity.UserIdentity, error)
UserListByTenantID(ctx, tenantID string) ([]identity.User, error)

IdentityProvision(
    ctx,
    workosOrgID,
    orgName,
    workosUserID,
    name,
    email string,
) (identity.User, error)

CRMConfigGet(ctx, tenantID string) (identity.CRMConfig, error)
CRMConfigUpsert(ctx, tenantID, provider, baseURL, apiKey string) error
CRMConfigAPIKeyClear(ctx, tenantID string) error

UserCRMTokenGet(ctx, userID string) (identity.UserCRMToken, error)
UserCRMTokenUpsert(ctx, userID, provider, token string) error
UserCRMTokenClear(ctx, userID string) error

UserAuthRecordGet(
    ctx,
    workosUserID,
    workosOrgID string,
) (identity.EncryptedAuthRecord, error)
```

`IdentityProvision` runs in one transaction:

1. Find the tenant by WorkOS organization ID.
2. Create it only when self-service provisioning is enabled.
3. Lock the tenant row while determining the bootstrap role.
4. Find the globally unique WorkOS user.
5. Reject an organization mismatch without changing `tenant_id`.
6. Create the first tenant user as admin or later users as members.
7. Update profile fields for an existing user without changing tenant or role.

Done when:

- Concurrent first logins cannot create multiple bootstrap admins.
- Organization mismatch returns a typed error.
- No `store.*` type leaves the repository boundary.

---

## Step 4 — WorkOS AuthKit flow

Pin the SDK version:

```text
github.com/workos/workos-go/v9 v9.3.0
```

Configuration:

```text
WORKOS_API_KEY=sk_...
WORKOS_CLIENT_ID=client_...
WORKOS_REDIRECT_URI=http://localhost:8080/auth/callback
WORKOS_COOKIE_PASSWORD=<64-character random hex value>
ALLOW_SELF_SERVICE_TENANTS=false
```

Initialize one shared client:

```go
wosClient := workos.NewClient(
    cfg.WorkOSAPIKey,
    workos.WithClientID(cfg.WorkOSClientID),
)
```

Routes:

```text
GET  /login
GET  /auth/callback
POST /logout
```

Login:

1. Call `GetAuthKitPKCEAuthorizationURL`.
2. Seal the generated state, code verifier, and five-minute expiry with `workos.Seal`.
3. Store that sealed transaction in a short-lived cookie using shared cookie helpers.
4. Redirect to the SDK-generated URL.

Callback:

1. Unseal and expire-check the login transaction.
2. Constant-time compare the returned state.
3. Delete the transaction cookie before exchanging the one-time code.
4. Call `AuthKitPKCECodeExchange`.
5. Require a WorkOS organization ID.
6. Fetch the organization through the SDK.
7. Call `IdentityProvision`.
8. Seal access and refresh tokens with `SealSessionFromAuthResponse`.
9. Set the session cookie using shared environment-aware cookie policy.

Logout:

1. Require an authenticated POST with strict Origin validation.
2. Build a WorkOS `Session` from the sealed cookie.
3. Call `GetLogoutURL`.
4. Clear the local cookie.
5. Redirect to the WorkOS-hosted logout URL.

Done when:

- Login survives page reload.
- Wrong, expired, missing, or replayed state is rejected.
- Missing organization is rejected.
- An existing user cannot authenticate into another organization.
- Development HTTP and production HTTPS behave according to infrastructure policy.

---

## Step 5 — Session middleware and auth context

Add `common/auth`:

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

func NewCtx(ctx context.Context, user AuthUser) context.Context
func FromCtx(ctx context.Context) (AuthUser, bool)
func MustFromCtx(ctx context.Context) AuthUser
```

Middleware order:

1. Read the sealed session cookie.
2. Call SDK `Session.Authenticate`.
3. Refresh only when the SDK reports `NeedsRefresh`.
4. Require WorkOS user and organization claims.
5. Query `UserIdentityGet` using both WorkOS IDs.
6. Build the shared auth-cache key from internal tenant ID and WorkOS user ID.
7. Read or populate an encrypted `CachedAuthRecord`.
8. Verify cached identity fields against the authenticated claims.
9. Decrypt the selected CRM credential into request memory.
10. Inject `AuthUser` into the request context.

Cache key:

```text
SHA-256(internal tenant ID + NUL + WorkOS user ID)
```

Redis stores encrypted credential material only. It never serializes `AuthUser`.
Redis failure falls back to PostgreSQL.

`AdminMiddleware` returns 403 unless `AuthUser.Role == admin`.

Public routes:

```text
/login
/auth/callback
/health
```

`POST /logout` remains authenticated.

SDK behavior: local session authentication verifies the SDK-created AES-GCM envelope and
token expiry. Remote revocation is observed at refresh/logout. If immediate revocation is
later required, consume WorkOS session webhooks into a short-lived local denylist.

Done when:

- Every protected request validates the WorkOS session before reading Redis.
- A WorkOS organization mismatch returns 403.
- Redis contains no plaintext CRM secret.
- Role and credential changes invalidate affected auth-cache entries.

---

## Step 6 — CRM settings

Routes:

```text
GET  /settings
POST /settings/crm
POST /settings/crm/key/remove
POST /settings/token
POST /settings/token/remove
```

All POST routes use shared strict-Origin middleware.

Tenant CRM configuration:

- Admin only.
- Provider: `none` or `odoo`.
- Existing key is represented only by a `Configured` indicator.
- Empty key input preserves the current key.
- Removing a key is explicit.
- Selecting `none` clears the base URL and tenant key transactionally.

Personal CRM token:

- Available to all authenticated users.
- Existing token is represented only by a `Configured` indicator.
- Empty input preserves the token.
- Removal is explicit.

CRM URL validation:

- Production requires HTTPS.
- Development permits HTTP for loopback/private test instances.
- Reject user info, fragments, and unsupported schemes.
- Revalidate redirects and prevent HTTPS downgrade.
- Production denies loopback, link-local, metadata, and private destinations unless an
  explicit deployment-level allowlist permits an internal CRM.

After updates:

- Personal changes evict that user’s auth record and CRM client.
- Tenant changes evict all tenant users’ auth records and CRM clients.
- Credential versions prevent stale reuse if an eviction is missed.

Done when:

- Settings HTML never contains a stored secret.
- Cross-origin mutations return 403.
- Credential rotation cannot reuse an old client.

---

## Step 7 — Auth-aware CRM resolution

Update the CRM registry to key clients by:

```text
internal tenant ID
+ WorkOS user ID
+ tenant CRM config version
+ personal CRM token version
```

Usecase call:

```go
user := auth.MustFromCtx(ctx)

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

Never include the credential itself in a cache key.

Done when:

- Two users in one tenant can use different personal tokens safely.
- Tenant and personal credential rotation produce new clients.
- A tenant without CRM configuration receives a normal empty state.
