# Odoo CRM Integration — Feature Status

This document tracks exactly what is built and what remains for the Odoo CRM integration.
It covers both layers: the Odoo Python controller and the Go client that consumes it.

---

## Odoo Controller (`odoo/addons/trigger_crm_api/controllers/main.py`)

### Endpoints — Done

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/leads` | limit/offset pagination; filters: `type`, `stage`, `include_archived` |
| `GET` | `/api/leads/:id` | detail: includes partner, sales team, deadline, open activities |
| `GET` | `/api/projects` | limit/offset pagination; filter: `location` |
| `GET` | `/api/projects/:id` | detail: includes nested units |
| `GET` | `/api/units` | limit/offset pagination; filters: `project`, `state`, `max_price`, `min_bedrooms` |
| `GET` | `/api/activities` | **keyset pagination**; filters below |
| `POST` | `/api/activities` | schedule a new activity on a lead |
| `POST` | `/api/activities/:id/done` | complete an activity; Odoo deletes it and logs a chatter message |

### Activity Endpoint — Detail

Pagination model: keyset (scroll_token), not limit/offset.

| Query param | Type | Behaviour |
|-------------|------|-----------|
| `scroll_token` | opaque base64 | cursor from previous response; omit for first page |
| `page_size` | int | default 50, clamped 1–500 |
| `sort` | `date_deadline` \| `id` | allowlisted; default `date_deadline` |
| `dir` | `asc` \| `desc` | default `asc` |
| `state` | `overdue` \| `today` \| `planned` | maps to `date_deadline` range; no filter = all |
| `activity_type` | string | matches `activity_type_id.name` e.g. `Call`, `Email`, `Meeting`, `To-Do` |
| `user` | int id | filter by assigned user |
| `lead` | int id | filter by specific lead |
| `model` | string | default `crm.lead` |

Response shape:
```json
{
  "count": 22,              // present only on first page (no scroll_token)
  "next_scroll_token": "…", // present only when more pages exist
  "results": [ ... ]
}
```

The cursor encodes `{sort, dir, last_value, last_id}` as base64 JSON.
It is validated on decode — a stale or tampered token silently restarts from the beginning.
State filter maps to `date_deadline` ranges because `mail.activity.state` is a non-stored
computed field that cannot be searched directly.

### Endpoints — Remaining

| Item | Notes |
|------|-------|
| `GET /api/units/:id` | Unit detail endpoint does not exist; units are only reachable via the project detail. Add if a unit detail page is needed. |
| Keyset pagination on leads / projects / units | Currently limit/offset. Migrate if those pages get infinite scroll. Not urgent. |

---

## Go Client (`apps/web/pkg/crmclient/odooclient/`)

### Done

**`types.go`** — Odoo JSON structs that mirror `_serialize_*` output exactly:
`odooLead`, `odooLeadDetail`, `odooProject`, `odooProjectDetail`, `odooUnit`,
`odooActivity`, `odooActivityListResponse` (keyset envelope with `*int` count),
`activityDoneResponse`, `scheduleActivityRequest`.

**`adapter.go`** — Pure conversion functions (no side effects, no I/O):
- `LeadListFilterOdooFromDto`, `ProjectListFilterOdooFromDto`, `UnitListFilterOdooFromDto`
- `ActivityListFilterOdooFromDto` — maps all keyset + filter params
- `LeadDtoFromOdoo`, `LeadDetailDtoFromOdoo`, `LeadListDtoFromOdoo`
- `ProjectDtoFromOdoo`, `ProjectDetailDtoFromOdoo`, `ProjectListDtoFromOdoo`
- `UnitDtoFromOdoo`, `UnitListDtoFromOdoo`
- `ActivityDtoFromOdoo`, `ActivityListDtoFromOdoo` — handles `*int` count + NextToken
- `ActivityResultDtoFromOdoo` — maps `message_id` → `AuditRef`
- `ActivityScheduleOdooFromDto`, `ActivityCompleteParamsFromDto`

**`client.go`** — All eight `sales.CRM` methods implemented:
`LeadList`, `LeadFind`, `ProjectList`, `ProjectFind`, `UnitList`,
`ActivityList`, `ActivitySchedule`, `ActivityComplete`.

**`client_test.go`** — Integration tests against live Odoo (require `ODOO_BASE_URL` + `ODOO_API_KEY`).
Covers: `LeadList`, `LeadFind`, `ProjectList`, `ProjectDetail`, `UnitList`,
`ActivityList`, `ActivitySchedule` + `ActivityComplete`.

### Remaining

| Item | Notes |
|------|-------|
| Auth-aware client resolution | Currently uses a hardcoded `defaultTenant` constant in the usecase. Must be replaced with credentials from `auth.MustFromCtx(ctx)` once the auth layer (auth plan) is complete. |
| Per-user token support | `user_crm_tokens` table and resolution logic (user token > tenant token) live in the auth plan. Once those exist, `ActivityListFilterOdooFromDto` and friends need no changes — only the usecase's `reg.Build(...)` call changes. |
| Cache invalidation | `registry.BuildCached` caches for process lifetime. When a tenant updates their CRM config (via the config screen), the cached client must be evicted. |

---

## Activity List Page (`apps/web/`)

### Done

**`common/sales/activity.go`**
- `Activity` — domain DTO
- `ActivityDraft`, `ActivityResult` — write-path DTOs
- `ActivityFilter` — keyset fields: `ScrollToken`, `Sort`, `Dir`, `PageSize`, `State`, `Type`, `UserID`, `LeadID`

**`common/sales/page.go`**
- `Page[T]` — generic envelope: `Results []T`, `Count *int` (nil on scroll pages), `NextToken string`

**`app/sales/adapter/activity.go`**
- `ActivityListReq` — presentation request with all filter + keyset fields
- `ActivityListResult` — `Items []ActivityView`, `Count *int`, `NextToken string`
- `ActivityListFilterDtoFromReq`, `ActivityViewFromDto`, `ActivityListViewFromDto`

**`app/sales/usecase/activity.go`**
- `ActivityList` — resolves CRM client, calls `crm.ActivityList`, maps result

**`api/server.go`** — `handleActivities`:
- Parses all query params: `scroll_token`, `sort`, `dir`, `page_size` (default 10), `state`, `type`, `user_id`, `lead_id`, `loaded`, `total`
- On scroll request (`scroll_token` + `HX-Request`): renders only `activity-rows` fragment
- Otherwise: renders full `content` block via `renderPage`

**`templates/activities.html`**
- State chips: All / Overdue / Today / Planned — chip links carry current `type`, `sort`, `dir`
- Filter form: type select (All / Call / Email / Meeting / To-Do), sort select, dir select — `hx-trigger="change"` on the form
- `activity-rows` define: range over items → `<article>` rows; scroll sentinel `<div>` with `intersect once`
- Live counter: `<p id="activity-count">Showing X of Y</p>` updated via `hx-swap-oob` on every scroll page
- `loaded` and `total` threaded through the sentinel URL so the counter accumulates correctly

### Remaining

| Item | Notes |
|------|-------|
| Schedule activity UI | `ActivitySchedule` is implemented in the client but there is no form or button on the page. Needed for the "create follow-up" flow. |
| Complete activity UI | `ActivityComplete` is implemented in the client but there is no button on the page. Will be built as part of the Calls feature (completing a call activity after the recording is matched). |
| Auth-aware activity fetch | Once the auth plan is implemented, `handleActivities` reads `AuthUser` from ctx and builds the CRM client from those credentials instead of `defaultTenant`. No template or filter changes needed. |
| Unscheduled call activities | The type filter currently shows all Call activities. After the Calls page is built, activity rows that have a matched call record will show enrichment status inline. |
