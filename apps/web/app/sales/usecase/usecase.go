// Package usecase orchestrates sales-domain flows. It owns no presentation or
// CRM-adapter knowledge beyond the registry: it resolves the tenant's CRM
// client, calls domain-level CRM methods, and hands results to the adapter for
// presentation shaping. The CRM registry is injected here in main.go; only the
// Usecase is injected into the api layer.
package usecase

import (
	"trigger/apps/web/common/sales"
	"trigger/apps/web/pkg/crmclient"
)

// defaultTenant is the single tenant used until the identity domain lands and
// resolves the real tenant per request from the authenticated session. Once
// identity exists, New will take a tenant-config provider instead of a static
// CRMConfig.
const defaultTenant = "default"

// Usecase orchestrates sales-domain use cases.
type Usecase struct {
	reg *crmclient.Registry
	cfg sales.CRMConfig
}

// New returns a Usecase wired to the injected CRM registry. The cfg is the
// default tenant's CRM connection (config-driven for now).
func New(reg *crmclient.Registry, cfg sales.CRMConfig) *Usecase {
	return &Usecase{reg: reg, cfg: cfg}
}
