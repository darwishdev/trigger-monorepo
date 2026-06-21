# Implementation Plan: Sales Domain & CRM-Agnostic Integration Layer

## Objective
Establish the first business domain (`sales`) in the Go backend behind a CRM-agnostic integration layer. The `sales` domain owns the sales team's world — pipeline (`Lead`/`Opportunity`), follow-ups (`Activity`), and the catalog reps sell from (`Project`/`Unit`). External CRM systems (Odoo first, Engaz and others next) are integrated through a swappable client in `pkg/`, so the domain logic never depends on any single CRM's shape.

This is the foundation for a multi-tenant, multi-CRM platform: each tenant resolves to its own `CRMConfig`, the registry builds (and caches) a client per tenant, and the usecase orchestrates business flows against the clean domain types defined in `common/`.

## Architectural Principles
- **Business-domain driven (DDD).** Bounded contexts are named after business subdomains in ubiquitous language, not technical layers or resources. `sales` is the first; `calls`, `identity`, and `insights` follow as the product grows.
- **External systems live in `pkg/`.** The Odoo/Engaz REST APIs are foreign systems. Their clients belong in `pkg/`, and all CRM-specific JSON/quirks die at that boundary — never escaping into `app/` or `api/`.
- **Shared contracts live in `common/`.** Domain DTOs are *our* clean, frontend-facing contracts, not external concerns. They import nothing internal.
- **One flat interface, `ErrNotImplemented` for unsupported methods.** A provider that does not yet support a method returns `ErrNotImplemented` rather than forcing a capability-split design.
- **Layered flow with two adapter hops.** `pkg/.../adapter.go` converts foreign JSON → domain DTO at the boundary; `app/sales/adapter/` shapes domain DTOs ↔ API request/response. Both intentional.
- **Server-owned rendering.** HTTP handlers, routing, and `html/template` execution live on a `Server` struct (replaces the package-level closures in the current `main.go`).

## Scope & Impact
- **New:**
  - `apps/web/common/sales/` — domain DTOs, `CRM` interface, `Page[T]`, `CRMConfig`, `ErrNotImplemented`.
  - `apps/web/app/sales/usecase/` — business orchestration.
  - `apps/web/app/sales/adapter/` — request/response shaping.
  - `apps/web/app/sales/repo/` — reserved for the domain's own persistence (not used for the CRM external calls).
  - `apps/web/pkg/crmclient/registry.go` — multi-provider registry with per-tenant caching.
  - `apps/web/pkg/crmclient/odooclient/` — Odoo REST client (`client.go`, `types.go`, `adapter.go`).
  - `apps/web/pkg/crmhttp/` — shared HTTP transport (timeout, retry, bearer auth, trace-id, typed errors).
  - `apps/web/api/server.go` + `api/{resource}.go` — `Server` struct and handlers.
- **Modified:** `apps/web/main.go` — wires the registry, registers the Odoo provider, constructs usecases, builds the `Server`, registers routes.
- **Future bounded contexts (reserved names, not implemented here):** `common/calls`, `common/identity`, `common/insights` (see Business-Domain Map).

## Folder Layout

```
apps/web/
  main.go                            # composition root: registry → usecase → server → routes
  config/                            # existing (viper-based)
  api/
    server.go                        # Server struct: templates, router, injected usecases
    leads.go                         # handlers per resource (auth + validate → usecase → render HTMX)
    activities.go
    projects.go
    units.go
  app/
    sales/
      usecase/
        leads.go                     # e.g. LeadUsecase.List(ctx, tenantID, req) → []sales.Lead
        activities.go
        adapter/
        leads.go                     # api-request → domain DTO; domain DTO → api-response struct
        activities.go
        repo/                        # reserved for own persistence (e.g. our Postgres); CRM is NOT here
  common/
    sales/                           # package sales — the shared, frontend-facing contract
      crm.go                         # CRM interface, CRMConfig, ErrNotImplemented
      lead.go activity.go project.go unit.go
      page.go                        # generic Page[T]
    calls/                           # (future) recorded calls, transcripts, analysis artifacts
    identity/                        # (future) org/tenant/user, WorkOS-backed
    insights/                        # (future) dashboards, analytics, coaching
  pkg/
    crmclient/
      registry.go                    # package crmclient: Registry, Register, Build, BuildCached
      odooclient/
        client.go                    # package odooclient: NewClient implements sales.CRM
        types.go                     # Odoo JSON structs (mirror of _serialize_* in the Odoo controller)
        adapter.go                   # Odoo JSON → sales DTOs (nullable handling, money, dates)
      engazclient/                   # (future)
    crmhttp/
      transport.go                   # shared: timeout, retry w/ backoff, bearer auth, trace-id, typed errors
  static/ templates/
```

### Dependency rule (who may import whom)
```
api                    → app/sales/usecase, common/sales, (templates)
app/sales/usecase      → app/sales/adapter, pkg/crmclient, common/sales
app/sales/adapter      → common/sales                (only)
pkg/crmclient          → common/sales                (registry only)
pkg/crmclient/odooclient → common/sales, pkg/crmhttp
pkg/crmhttp            → stdlib only
common/sales           → stdlib only                 (stable contract; imports nothing internal)
```
`common/sales` importing nothing internal is the keystone invariant — enforce later with `depguard`.

## Business-Domain Map
Named after business subdomains (ubiquitous language), not resources or technical layers:

| Context | Status | Owns | Source |
|---|---|---|---|
| `sales` | **now** | Pipeline (Lead/Opportunity), follow-ups (Activity), catalog (Project/Unit) | Pillars 2 + parts of 3 |
| `calls` | future | Recorded call asset: audio, transcript, speakers, analysis artifacts | Pillar 1 (Kotlin recorder feeds this) |
| `identity` | future | Organization↔Tenant mapping, users, teams, SSO sessions (WorkOS) | Multi-tenant core |
| `insights` | future | Dashboards, pipeline analytics, forecasting, coaching | Pillar 4 |

Deliberately **not** domains: the AI/LLM engine (a service producing artifacts owned by `calls`/`sales`), storage/TUS/R2 (plumbing — the *Recording* asset lives in `calls`), and the Redis queue / SSE / Sentry (pure infrastructure). Pillar 3 (Sales Productivity) is mostly *consumption* of `sales`+`calls` data and does not earn its own context yet.

## Proposed Solution

### 1. The Sales Domain Layer (`common/sales/`)
Stable, frontend-facing contract. Derived from the real Odoo controller (`trigger_crm_api/controllers/main.py`), cleaned of all Odoo leakage. Module path prefix is `trigger/apps/web/` (go.mod:1).

```go
package sales

import "time"

// Money groups an amount with its currency. Odoo models expected_revenue /
// list_price / price_from as float64 plus a currency name; we keep that shape.
type Money struct {
	Amount   float64
	Currency string
}

// Page is the generic list envelope every collection endpoint returns.
type Page[T any] struct {
	Count   int
	Limit   int
	Offset  int
	Results []T
}
```

**Lead** (mirrors `GET /api/leads`, detail adds partner/team/deadline/open activities):
```go
type Lead struct {
	ID              string
	Name            string
	Type            string // "lead" | "opportunity"
	ContactName     string
	Email           string
	Phone           string
	Stage           string
	Budget          Money   // Odoo: expected_revenue
	Probability     float64
	Priority        string
	Location        string
	Salesperson     string
	SalespersonID   string
	Tags            []string
	Active          bool
	SuggestedProjects []ProjectRef
}

type LeadDetail struct {
	Lead
	Partner        string
	SalesTeam      string
	Deadline       time.Time
	OpenActivities []Activity
}

type ProjectRef struct {
	ID       string
	Name     string
	Location string
}
```

**Project / Unit** (Project is the aggregate root; Units nest under it):
```go
type Project struct {
	ID             string
	Name           string
	Location       string
	Developer      string
	DeliveryDate   time.Time
	UnitCount      int
	AvailableUnits int
	PriceFrom      Money
}

type ProjectDetail struct {
	Project
	Units []Unit
}

type Unit struct {
	ID                string
	Code              string
	Name              string
	ProjectID         string
	Project           string
	Type              string // apartment|villa|townhouse|office|retail
	AreaSqm           float64
	Bedrooms          int
	Bathrooms         int
	Floor             string
	Price             Money // Odoo: list_price
	State             string // available|reserved|sold
}
```

**Activity** (the follow-up queue; completing it in Odoo deletes the row and writes a chatter message):
```go
type Activity struct {
	ID       string
	Summary  string
	Type     string
	Note     string
	Deadline time.Time
	State    string // overdue|today|planned
	UserID   string
	User     string
	LeadID   string
	Lead     string
}

type ActivityDraft struct {
	LeadID       string
	Summary      string
	Note         string
	Deadline     time.Time
	UserID       string         // optional assignee
	ActivityType string         // provider-resolved (Odoo xmlid or Engaz equivalent)
}

// ActivityResult is returned by CompleteActivity. Odoo maps message_id into
// AuditRef; Engaz will map its own task reference. The domain stays stable.
type ActivityResult struct {
	Completed Activity
	AuditRef  string
}
```

**Filters** (query-param shaped, provider-agnostic):
```go
type LeadFilter struct {
	Type, Stage     string
	IncludeArchived bool
	Limit, Offset   int
}
type ProjectFilter struct {
	Location      string
	Limit, Offset int
}
type UnitFilter struct {
	ProjectID, State string
	MaxPrice         float64
	MinBedrooms      int
	Limit, Offset    int
}
type ActivityFilter struct {
	UserID, LeadID, State string
	Limit, Offset         int
}
```

**Interface & config** — flat, with `ErrNotImplemented` for unsupported providers:
```go
type CRMConfig struct {
	Provider string // "odoo", "engaz", ...
	BaseURL  string
	APIKey   string
}

var ErrNotImplemented = errors.New("sales: method not implemented for this provider")

type CRM interface {
	ListLeads(ctx context.Context, f LeadFilter) (Page[Lead], error)
	GetLead(ctx context.Context, id string) (LeadDetail, error)
	ListProjects(ctx context.Context, f ProjectFilter) (Page[Project], error)
	GetProject(ctx context.Context, id string) (ProjectDetail, error)
	ListUnits(ctx context.Context, f UnitFilter) (Page[Unit], error)
	ListActivities(ctx context.Context, f ActivityFilter) (Page[Activity], error)
	ScheduleActivity(ctx context.Context, draft ActivityDraft) (Activity, error)
	CompleteActivity(ctx context.Context, id, feedback string) (ActivityResult, error)
}
```

### 2. The Registry (`pkg/crmclient/registry.go`)
Multi-provider switch with **per-tenant caching**. Wired once in `main.go`; `Build` is called per tenant at request time. The builder returns an error so malformed tenant configs fail fast.

```go
package crmclient

import (
	"fmt"
	"sync"

	"trigger/apps/web/common/sales"
)

type ClientBuilder func(cfg sales.CRMConfig) (sales.CRM, error)

type Registry struct {
	mu       sync.RWMutex
	builders map[string]ClientBuilder
	cache    map[string]sales.CRM // keyed by tenantID
}

func NewRegistry() *Registry {
	return &Registry{
		builders: make(map[string]ClientBuilder),
		cache:    make(map[string]sales.CRM),
	}
}

func (r *Registry) Register(name string, b ClientBuilder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[name] = b
}

// Build returns the client for a provider, constructing it on first use.
func (r *Registry) Build(cfg sales.CRMConfig) (sales.CRM, error) {
	r.mu.RLock()
	b, ok := r.builders[cfg.Provider]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("crmclient: unsupported provider %q", cfg.Provider)
	}
	return b(cfg)
}

// BuildCached returns the cached client for a tenant, building it on first use.
// Subsequent calls with the same tenantID reuse the cached instance.
func (r *Registry) BuildCached(tenantID string, cfg sales.CRMConfig) (sales.CRM, error) {
	r.mu.RLock()
	c, ok := r.cache[tenantID]
	r.mu.RUnlock()
	if ok {
		return c, nil
	}
	c, err := r.Build(cfg)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[tenantID] = c
	r.mu.Unlock()
	return c, nil
}
```

### 3. Shared HTTP Transport (`pkg/crmhttp/transport.go`)
One helper reused by every provider client: a configured `*http.Client` with timeout, retry-with-backoff, bearer auth header injection, trace-id propagation (project-overview.md:83 — Trace IDs flow Upload → Queue → AI → CRM), and typed error mapping from Odoo's `{"error": "..."}` + status code to sentinels (`ErrAuth`, `ErrNotFound`, `ErrValidation`, `ErrServer`). Keeps `odooclient` and the future `engazclient` thin.

### 4. The Odoo Client (`pkg/crmclient/odooclient/`)
**`types.go`** — Odoo JSON structs mirroring the controller's `_serialize_*` output exactly (`id` as int, nullable `contact_name` as `*string`, the `{count,limit,offset,results}` envelope, etc.).

**`adapter.go`** — pure functions converting Odoo structs → `sales` DTOs: int→string IDs, nullable unwrapping, `expected_revenue`+currency→`Money`, ISO date strings→`time.Time`, and the activity-done mapping (`message_id` → `ActivityResult.AuditRef`). This is the only place Odoo-isms live.

**`client.go`** — `NewClient(cfg sales.CRMConfig) (sales.CRM, error)` returns a `*Client` that satisfies `sales.CRM`. Uses `pkg/crmhttp` for transport. Compile-time guarantee:
```go
var _ sales.CRM = (*Client)(nil)
```

### 5. The Sales Usecase (`app/sales/usecase/`)
Orchestrates business flows. The registry is injected; the usecase resolves the tenant's cached CRM and calls interface methods, then hands results to the adapter for response shaping.

```go
package salesusecase

import (
	"context"

	"trigger/apps/web/app/sales/adapter"
	"trigger/apps/web/common/sales"
	"trigger/apps/web/pkg/crmclient"
)

type LeadUsecase struct {
	reg *crmclient.Registry
}

func NewLeadUsecase(reg *crmclient.Registry) *LeadUsecase {
	return &LeadUsecase{reg: reg}
}

// List is called by api/leads.go after auth + validation.
// tenantCfg is resolved upstream (identity/tenant store — future; config-driven for now).
func (u *LeadUsecase) List(ctx context.Context, tenantID string, cfg sales.CRMConfig, req LeadListReq) ([]LeadView, error) {
	crm, err := u.reg.BuildCached(tenantID, cfg)
	if err != nil {
		return nil, err
	}
	page, err := crm.ListLeads(ctx, adapter.ReqToLeadFilter(req))
	if err != nil {
		return nil, err
	}
	return adapter.LeadsToView(page.Results), nil
}
```

### 6. The Server Layer (`api/server.go`)
`Server` owns templates and injected usecases; handlers are methods. This replaces the package-level closures in the current `main.go:59-98`.

```go
package api

import (
	"html/template"
	"net/http"

	"trigger/apps/web/app/sales/usecase"
)

type Server struct {
	leads      *salesusecase.LeadUsecase
	activities *salesusecase.ActivityUsecase
	// future: calls, identity, insights usecases
	homeTmpl *template.Template
}

func NewServer(leads *salesusecase.LeadUsecase, activities *salesusecase.ActivityUsecase, tmpl *template.Template) *Server {
	return &Server{leads: leads, activities: activities, homeTmpl: tmpl}
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /leads", s.handleLeads) // HTMX: auth → validate → usecase → render
	// ...
}
```

### 7. Composition Root (`main.go`)
Registry is wired once here; per-tenant clients are built lazily on request.

```go
func main() {
	cfg, err := config.LoadConfig("config")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 1. CRM registry + provider registration (the only place that knows about Odoo)
	reg := crmclient.NewRegistry()
	reg.Register("odoo", odooclient.NewClient)
	// reg.Register("engaz", engazclient.NewClient) // future

	// 2. Usecases (only usecases are injected into the server)
	leadsUC := salesusecase.NewLeadUsecase(reg)
	activitiesUC := salesusecase.NewActivityUsecase(reg)

	// 3. Server (owns templates + rendering)
	srv := api.NewServer(leadsUC, activitiesUC, homeTmpl)

	mux := http.NewServeMux()
	srv.Routes(mux)

	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
```

### Request → HTMX response flow
```
GET /leads?stage=Negotiation
  │  (auth resolves tenantID; tenant CRMConfig fetched — identity store future, config now)
  ▼
api/leads.go          Server.handleLeads: auth + validate → build LeadListReq
  ▼
app/sales/usecase     LeadUsecase.List(ctx, tenantID, cfg, req)
  │                    adapter.ReqToLeadFilter(req) → sales.LeadFilter
  ▼
pkg/crmclient         Registry.BuildCached(tenantID, cfg) → sales.CRM
  ▼
pkg/crmclient/odoo    ListLeads → GET /api/leads → JSON
  │                    adapter.OdooToDomain → []sales.Lead   (Odoo leakage dies here)
  ▼
app/sales/usecase     receives []sales.Lead
  │                    adapter.LeadsToView(...) → []LeadView
  ▼
api/leads.go          render HTMX with []LeadView
```

## Known Gaps & Decisions
- **No `GET /api/units/{id}`** in the current Odoo controller. Unit detail must either be fetched via the nested `Units` array on `GetProject`, or we add a `GET /api/units/{id}` endpoint to `trigger_crm_api/controllers/main.py`. Until decided, the `CRM` interface omits `GetUnit`.
- **Per-tenant `CRMConfig` source.** Today it is config-driven; once the `identity` domain lands, tenant configs are fetched from the tenant store (WorkOS org → CRM mapping) and passed into the usecase by the API layer.
- **`app/sales/repo/` is reserved.** It is NOT used for the external CRM (that goes through `pkg/crmclient` directly from the usecase). It will hold the sales domain's own persistence (e.g. cached/denormalized data in our Postgres) when needed.
- **Cache invalidation.** `BuildCached` caches for the process lifetime. If tenant configs can change at runtime, add an invalidation/eviction path before that becomes a requirement.

## Verification
- **Factory/registry:** table-driven test — known provider returns a client; unknown provider returns the expected error; `BuildCached` returns the same instance for a repeated `tenantID`.
- **Adapter purity:** unit-test `odooclient/adapter.go` against JSON fixtures modeled on the controller's `_serialize_*` output — nullable `contact_name`, ISO date strings, the activity-done `message_id` → `AuditRef` mapping. Commit fixtures under `pkg/crmclient/odooclient/testdata/` (not the gitignored `api_tests/responses/`).
- **Compile-time interface satisfaction:** `var _ sales.CRM = (*odooclient.Client)(nil)` in each provider.
- **Dependency direction:** `go list -deps ./common/sales` shows no internal app/pkg imports; add a `depguard` rule to keep it that way.
- **Server wiring:** `go vet ./...` and `go test ./...` pass; the existing `/health` and `/` routes still respond; a new `/leads` route renders the HTMX view against a stubbed `sales.CRM`.
