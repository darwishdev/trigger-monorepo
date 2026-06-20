package crmclient

import (
	"context"
	"errors"
	"testing"

	"trigger/apps/web/common/sales"
)

// stubCRM is a no-op sales.CRM used to verify registry caching behavior.
// The unexported field gives the struct non-zero size, so distinct &stubCRM{}
// allocations yield distinct addresses (a zero-size type shares one address).
type stubCRM struct{ _ int }

func (stubCRM) LeadList(context.Context, sales.LeadFilter) (sales.Page[sales.Lead], error) {
	return sales.Page[sales.Lead]{}, nil
}
func (stubCRM) LeadFind(context.Context, string) (sales.LeadDetail, error) {
	return sales.LeadDetail{}, nil
}
func (stubCRM) ProjectList(context.Context, sales.ProjectFilter) (sales.Page[sales.Project], error) {
	return sales.Page[sales.Project]{}, nil
}
func (stubCRM) ProjectFind(context.Context, string) (sales.ProjectDetail, error) {
	return sales.ProjectDetail{}, nil
}
func (stubCRM) UnitList(context.Context, sales.UnitFilter) (sales.Page[sales.Unit], error) {
	return sales.Page[sales.Unit]{}, nil
}
func (stubCRM) ActivityList(context.Context, sales.ActivityFilter) (sales.Page[sales.Activity], error) {
	return sales.Page[sales.Activity]{}, nil
}
func (stubCRM) ActivitySchedule(context.Context, sales.ActivityDraft) (sales.Activity, error) {
	return sales.Activity{}, nil
}
func (stubCRM) ActivityComplete(context.Context, string, string) (sales.ActivityResult, error) {
	return sales.ActivityResult{}, nil
}

func TestBuild_unknownProvider(t *testing.T) {
	r := NewRegistry()
	_, err := r.Build("tenant-a", sales.CRMConfig{Provider: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBuild_builderError(t *testing.T) {
	r := NewRegistry()
	boom := errors.New("boom")
	r.Register("broken", func(cfg sales.CRMConfig) (sales.CRM, error) {
		return nil, boom
	})
	if _, err := r.Build("tenant-a", sales.CRMConfig{Provider: "broken"}); !errors.Is(err, boom) {
		t.Fatalf("expected builder error, got %v", err)
	}
}

func TestBuild_reusesInstancePerTenant(t *testing.T) {
	r := NewRegistry()
	r.Register("odoo", func(cfg sales.CRMConfig) (sales.CRM, error) {
		return &stubCRM{}, nil // distinct pointer per call so identity checks work
	})
	cfg := sales.CRMConfig{Provider: "odoo"}

	a, err := r.Build("tenant-a", cfg)
	if err != nil {
		t.Fatalf("Build tenant-a: %v", err)
	}
	aAgain, err := r.Build("tenant-a", cfg)
	if err != nil {
		t.Fatalf("Build tenant-a (2): %v", err)
	}
	if a != aAgain {
		t.Error("Build should return the same instance for the same tenant")
	}

	b, err := r.Build("tenant-b", cfg)
	if err != nil {
		t.Fatalf("Build tenant-b: %v", err)
	}
	if a == b {
		t.Error("Build should return distinct instances per tenant")
	}
}
