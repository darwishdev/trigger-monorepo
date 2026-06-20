package sales

import (
	"context"
	"errors"
)

// CRMConfig holds the credentials needed to reach a tenant's CRM instance.
type CRMConfig struct {
	Provider string // "odoo", "engaz", ...
	BaseURL  string
	APIKey   string
}

// Errors surfaced by every CRM adapter. Adapters wrap these with provider
// context via fmt.Errorf("%w: ...", ...) so callers can use errors.Is.
var (
	// ErrNotImplemented is returned when a provider does not support a method.
	ErrNotImplemented = errors.New("sales: method not implemented for this provider")
	// ErrAuth means the CRM rejected the credentials.
	ErrAuth = errors.New("sales: authentication failed")
	// ErrNotFound means the requested resource does not exist.
	ErrNotFound = errors.New("sales: resource not found")
	// ErrValidation means the request was malformed or rejected by the CRM.
	ErrValidation = errors.New("sales: invalid request")
	// ErrServer means the CRM returned a server-side failure.
	ErrServer = errors.New("sales: crm server error")
)

// CRM is the provider-agnostic contract every CRM adapter must satisfy.
// Providers that do not yet support a method return ErrNotImplemented rather
// than being forced into a capability-split design.
//
// Method names follow the {Singular}{Action} convention so every method on a
// resource groups together (LeadList, LeadFind, ActivitySchedule, ...).
type CRM interface {
	LeadList(ctx context.Context, f LeadFilter) (Page[Lead], error)
	LeadFind(ctx context.Context, id string) (LeadDetail, error)
	ProjectList(ctx context.Context, f ProjectFilter) (Page[Project], error)
	ProjectFind(ctx context.Context, id string) (ProjectDetail, error)
	UnitList(ctx context.Context, f UnitFilter) (Page[Unit], error)
	ActivityList(ctx context.Context, f ActivityFilter) (Page[Activity], error)
	ActivitySchedule(ctx context.Context, draft ActivityDraft) (Activity, error)
	ActivityComplete(ctx context.Context, id, feedback string) (ActivityResult, error)
}
