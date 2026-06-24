# Odoo CRM Integration — Status

Tracks implemented and remaining work for the Odoo controller, Go provider client, and
activity page.

Related plans:

- [infrastructure-plan.md](infrastructure-plan.md)
- [identity-auth-plan.md](identity-auth-plan.md)
- [calls-feature-plan.md](calls-feature-plan.md)

---

## Odoo controller

Implemented in `odoo/addons/trigger_crm_api/controllers/main.py`:

| Method | Path | Status |
|--------|------|--------|
| `GET` | `/api/leads` | Done |
| `GET` | `/api/leads/:id` | Done |
| `GET` | `/api/projects` | Done |
| `GET` | `/api/projects/:id` | Done |
| `GET` | `/api/units` | Done |
| `GET` | `/api/activities` | Done |
| `POST` | `/api/activities` | Done |
| `POST` | `/api/activities/:id/done` | Done |

Activity listing uses opaque keyset pagination with:

```text
scroll_token
page_size
sort
dir
state
activity_type
user
lead
model
```

The response includes `count` only on the first page and `next_scroll_token` while more
rows remain.

Remaining:

- `GET /api/units/:id` only if a standalone unit-detail page is required.
- Keyset pagination for leads, projects, and units only when those screens need infinite
  scrolling.
- Review tampered activity cursor behavior. Invalid cursors should return a clear client
  error rather than silently restart and risk duplicate UI results.

---

## Go Odoo client

Implemented under `apps/web/pkg/crmclient/odooclient`:

- Provider-specific request and response types.
- Pure adapters between Odoo shapes and `common/sales`.
- `LeadList`
- `LeadFind`
- `ProjectList`
- `ProjectFind`
- `UnitList`
- `ActivityList`
- `ActivitySchedule`
- `ActivityComplete`
- Live integration tests using Odoo base URL and API key configuration.

Provider code remains unaware of WorkOS, tenants, users, Redis, and database repositories.
It receives a validated `sales.CRMConfig`.

---

## CRM registry

Current state:

- Provider registration exists.
- Client construction exists.
- The current application still uses a hardcoded default tenant/configuration.
- Current process caching is tenant-only.

Required by [identity-auth-plan.md](identity-auth-plan.md):

- Resolve configuration from `AuthUser`.
- Cache by:

```text
internal tenant ID
+ WorkOS user ID
+ tenant CRM config version
+ personal CRM token version
```

- Add targeted user eviction.
- Add tenant-wide eviction for tenant configuration changes.
- Never include credentials in cache keys.
- Return a normal “CRM not configured” result for provider `none`.

No Odoo adapter changes are required for personal-token support.

---

## CRM URL and credential handling

Owned by [identity-auth-plan.md](identity-auth-plan.md), not the Odoo provider:

- Tenant and personal credentials are encrypted at rest.
- Existing secrets are never rendered.
- Production CRM URLs require HTTPS.
- Redirects are revalidated.
- SSRF-sensitive destinations are denied unless deployment allowlisting permits an
  internal CRM.

The Odoo HTTP client must preserve those guarantees by applying redirect checks for every
redirect and never logging bearer tokens.

---

## Activity list page

Implemented:

- Activity domain DTOs and filters.
- Generic page envelope.
- Request/view adapters.
- Activity list usecase.
- Query parsing.
- HTMX content rendering.
- State and type filters.
- Sort and direction controls.
- Opaque-cursor infinite scrolling.
- Out-of-band loaded-count updates.

Remaining:

- Replace hardcoded CRM config with auth-aware registry resolution.
- Add a schedule-activity UI when product scope requires it.
- Complete call activities through the calls workflow after a persisted call/activity
  match.
- Decide whether matched-call enrichment status also appears on the activity page.

---

## Blocking relationships

```text
infrastructure plan
        ↓
identity/auth plan
        ↓
auth-aware Odoo registry resolution
        ↓
calls matching and completion
```

When identity/auth implementation lands, update this document so auth-aware registry work
moves from “required” to “implemented.”
