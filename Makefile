SHELL := /usr/bin/env bash

WEB_DIR := apps/web
WEB_CONFIG_DIR := $(WEB_DIR)/config
ODOO_DIR := odoo
GO_CACHE := /tmp/trigger-go-build
ODOO_BASE_URL ?= http://localhost:8069
ODOO_API_KEY ?=
DB ?= trigger_test
LOGIN ?= admin
PASSWORD ?= admin

include $(WEB_CONFIG_DIR)/state.env
include $(WEB_CONFIG_DIR)/shared.env
include $(WEB_CONFIG_DIR)/$(shell awk -F= '/^STATE=/ { print tolower($$2) }' $(WEB_CONFIG_DIR)/state.env).env
export STATE PORT BASE_URL DATABASE_URL WORKOS_API_KEY

.DEFAULT_GOAL := help

.PHONY: help
help:
	@printf 'Trigger development commands\n\n'
	@printf 'Web:\n'
	@printf '  make web-run        Run the Go/HTMX web app on :%s\n' "$(PORT)"
	@printf '  make web-test       Run Go tests for the web app\n'
	@printf '  make web-fmt        Format Go files\n'
	@printf '  make health         Call the local web health endpoint\n'
	@printf '  make env-print      Print loaded app environment\n\n'
	@printf 'Odoo:\n'
	@printf '  make odoo-up        Start Odoo and Postgres with Docker Compose\n'
	@printf '  make odoo-down      Stop Odoo and Postgres\n'
	@printf '  make odoo-logs      Tail Odoo logs\n'
	@printf '  make odoo-shell     Open a shell in the Odoo container\n\n'
	@printf 'Odoo modules:\n'
	@printf '  make odoo-install   Install Trigger modules into DB=%s\n' "$(DB)"
	@printf '  make odoo-update    Update Trigger modules in DB=%s\n\n' "$(DB)"
	@printf 'API tests:\n'
	@printf '  make api-leads      Test /api/leads\n'
	@printf '  make api-projects   Test /api/projects\n'
	@printf '  make api-units      Test /api/units\n'
	@printf '  make api-activities Test /api/activities\n'

.PHONY: web-run
web-run:
	cd $(WEB_DIR) && GOCACHE=$(GO_CACHE) go run .

.PHONY: web-test
web-test:
	cd $(WEB_DIR) && GOCACHE=$(GO_CACHE) go test ./...

.PHONY: web-fmt
web-fmt:
	cd $(WEB_DIR) && gofmt -w main.go

.PHONY: health
health:
	curl -fsS http://127.0.0.1:$(PORT)/health
	@printf '\n'

.PHONY: env-print
env-print:
	@printf 'STATE=%s\n' "$(STATE)"
	@printf 'PORT=%s\n' "$(PORT)"
	@printf 'BASE_URL=%s\n' "$(BASE_URL)"
	@printf 'DATABASE_URL=%s\n' "$(DATABASE_URL)"
	@printf 'WORKOS_API_KEY=%s\n' "$$(if [ -n "$(WORKOS_API_KEY)" ]; then printf '<set>'; else printf '<empty>'; fi)"
	@printf 'ODOO_BASE_URL=%s\n' "$(ODOO_BASE_URL)"
	@printf 'ODOO_API_KEY=%s\n' "$$(if [ -n "$(ODOO_API_KEY)" ]; then printf '<set>'; else printf '<empty>'; fi)"

.PHONY: odoo-up
odoo-up:
	cd $(ODOO_DIR) && docker compose up -d

.PHONY: odoo-down
odoo-down:
	cd $(ODOO_DIR) && docker compose down

.PHONY: odoo-logs
odoo-logs:
	cd $(ODOO_DIR) && docker compose logs -f odoo

.PHONY: odoo-shell
odoo-shell:
	cd $(ODOO_DIR) && docker compose exec odoo bash

.PHONY: odoo-install
odoo-install:
	cd $(ODOO_DIR) && docker compose exec odoo odoo -d $(DB) -i trigger_estate,trigger_crm_api,trigger_crm_demo --stop-after-init

.PHONY: odoo-update
odoo-update:
	cd $(ODOO_DIR) && docker compose exec odoo odoo -d $(DB) -u trigger_estate,trigger_crm_api,trigger_crm_demo --stop-after-init

.PHONY: api-leads
api-leads:
	@cd $(ODOO_DIR) && BASE_URL=$(ODOO_BASE_URL) DB=$(DB) LOGIN=$(LOGIN) PASSWORD=$(PASSWORD) ODOO_API_KEY=$(ODOO_API_KEY) ./api_tests/leads.sh

.PHONY: api-projects
api-projects:
	@cd $(ODOO_DIR) && BASE_URL=$(ODOO_BASE_URL) DB=$(DB) LOGIN=$(LOGIN) PASSWORD=$(PASSWORD) ODOO_API_KEY=$(ODOO_API_KEY) ./api_tests/projects.sh

.PHONY: api-units
api-units:
	@cd $(ODOO_DIR) && BASE_URL=$(ODOO_BASE_URL) DB=$(DB) LOGIN=$(LOGIN) PASSWORD=$(PASSWORD) ODOO_API_KEY=$(ODOO_API_KEY) ./api_tests/units.sh

.PHONY: api-activities
api-activities:
	@cd $(ODOO_DIR) && BASE_URL=$(ODOO_BASE_URL) DB=$(DB) LOGIN=$(LOGIN) PASSWORD=$(PASSWORD) ODOO_API_KEY=$(ODOO_API_KEY) ./api_tests/activities.sh
