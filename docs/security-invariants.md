# Security Invariants

Mandatory rules for every implementation plan and architectural change.

## Identity and tenancy

- Tenant identity comes only from authenticated server-verified identity.
- Request parameters, form values, headers, object keys, and cache values never select a
  tenant.
- Every tenant-owned query includes tenant scope.
- User-owned resources include both tenant and user scope.
- WorkOS organizations map one-to-one to Trigger tenants.
- A WorkOS user belongs to exactly one Trigger tenant unless this product decision is
  explicitly changed through a reviewed migration.
- Authorization defaults to deny.

## Database

- Every relational ownership column uses a PostgreSQL foreign key.
- Foreign-key deletion behavior is explicit.
- Roles, providers, statuses, and other closed sets use database checks or equivalent
  constrained types.
- Migrations are transactional, ordered, checksummed, and concurrency-safe.
- Cross-tenant access tests are required for every tenant-owned repository.

## Sessions and browser requests

- Session cookies are `HttpOnly` and `SameSite=Lax`.
- Session-cookie `Secure` is mandatory in production.
- Plain HTTP is allowed only for loopback development.
- Unsafe cookie-authenticated browser routes require exact Origin validation.
- OAuth uses state and PKCE.
- Logout is an authenticated POST.
- Cookie deletion matches creation attributes.

## Native clients, webhooks, and APIs

- Native applications do not send browser session cookies as bearer tokens.
- Native APIs use a dedicated bearer/device credential design.
- Browser Origin checks are not treated as native-client authentication.
- Webhooks require provider signature verification and replay protection.

## Secrets

- Credentials are encrypted at rest.
- Plaintext credentials are never logged, rendered, committed, or serialized into Redis.
- Existing secrets are never returned to settings forms.
- Secret removal is explicit; an empty form field does not silently erase a secret.
- Cryptographic ciphertext is bound to its owner through authenticated additional data.
- Production secrets must not use development placeholders.

## Caches

- A cache never replaces authentication or authoritative authorization checks.
- Cache keys never contain credentials.
- User-specific CRM clients are isolated by tenant, WorkOS user, and credential versions.
- Cache outages fall back to the authoritative store or fail closed.
- Role and credential changes define invalidation behavior.

## Network access

- Production external service URLs use HTTPS.
- Redirects are revalidated and cannot silently downgrade transport.
- User-configurable outbound URLs are protected against SSRF.
- Loopback, link-local, metadata, and private destinations are denied in production unless
  explicitly allowlisted for the deployment.
- Forwarded headers are trusted only from configured proxies.

## Files and jobs

- Object storage is private by default.
- Access uses short-lived presigned URLs or authenticated streaming.
- Object keys include trusted tenant and user identifiers.
- Durable workflows use a persistent retry-aware queue.
- Job handlers are idempotent and re-check resource ownership.
- Job payloads do not contain long-lived plaintext secrets.

## Logging and observability

- Logs use structured fields and trace IDs.
- Authentication failures avoid sensitive detail in user responses.
- Logs never contain passwords, tokens, cookies, authorization codes, raw credentials, or
  decrypted secret values.
- Security-relevant state changes produce auditable events.
