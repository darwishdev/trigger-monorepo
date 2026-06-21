package main

import (
	"embed"
	"log"
	"net/http"

	"trigger/apps/web/api"
	"trigger/apps/web/app/sales/usecase"
	"trigger/apps/web/common/sales"
	"trigger/apps/web/config"
	"trigger/apps/web/pkg/crmclient"
	"trigger/apps/web/pkg/crmclient/odooclient"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var tmplFS embed.FS

func main() {
	cfg, err := config.LoadConfig("config")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// CRM registry: provider registration happens once here. This is the only
	// place that knows about specific CRM implementations.
	crmRegistry := crmclient.NewRegistry()
	crmRegistry.Register("odoo", odooclient.NewClient)

	// Default tenant CRM config (config-driven until the identity domain lands
	// and supplies per-tenant configs from the tenant store).
	crmCfg := sales.CRMConfig{
		Provider: cfg.CRMProvider,
		BaseURL:  cfg.CRMBaseURL,
		APIKey:   cfg.CRMAPIKey,
	}

	// Sales usecase: the registry is injected here, and only the usecase is
	// injected into the server.
	salesUC := usecase.New(crmRegistry, crmCfg)

	// Server owns templates, rendering, and all handlers.
	server := api.NewServer(salesUC, tmplFS, staticFS)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	addr := ":" + cfg.Port
	log.Printf("listening on http://localhost%s (base URL %s)", addr, cfg.BaseURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
