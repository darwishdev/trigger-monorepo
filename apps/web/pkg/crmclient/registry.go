// Package crmclient owns the multi-provider CRM registry. Providers register
// their builder once at startup (in main.go); the registry resolves and caches
// a CRM client per tenant at request time.
package crmclient

import (
	"fmt"
	"sync"

	"trigger/apps/web/common/sales"
)

// ClientBuilder constructs a CRM client from a tenant's config. It returns an
// error so malformed tenant configs fail fast instead of panicking on first
// use. Provider packages (e.g. odooclient) expose a function with this shape.
type ClientBuilder func(cfg sales.CRMConfig) (sales.CRM, error)

// Registry maps a provider name (e.g. "odoo") to its ClientBuilder and caches
// the built clients per tenant. It is safe for concurrent use.
type Registry struct {
	mu       sync.RWMutex
	builders map[string]ClientBuilder
	cache    map[string]sales.CRM // keyed by tenantID
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		builders: make(map[string]ClientBuilder),
		cache:    make(map[string]sales.CRM),
	}
}

// Register associates a provider name with its builder. Registration happens
// once at startup, before any Build call.
func (r *Registry) Register(name string, b ClientBuilder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[name] = b
}

// Build returns the cached client for tenantID, constructing it on first use.
// Subsequent calls with the same tenantID reuse the cached instance, so each
// tenant pays construction cost once per process. This is the single entry
// point - there is intentionally no uncached variant.
func (r *Registry) Build(tenantID string, cfg sales.CRMConfig) (sales.CRM, error) {
	r.mu.RLock()
	if c, ok := r.cache[tenantID]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	b, ok := r.builders[cfg.Provider]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("crmclient: unsupported provider %q", cfg.Provider)
	}

	c, err := b(cfg)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Another goroutine may have raced ahead; prefer the existing entry so the
	// whole process shares one client per tenant.
	if existing, ok := r.cache[tenantID]; ok {
		return existing, nil
	}
	r.cache[tenantID] = c
	return c, nil
}
