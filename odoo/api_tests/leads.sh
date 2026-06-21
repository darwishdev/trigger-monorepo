#!/usr/bin/env bash
# GET /api/leads — list leads & opportunities (incl. archived).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login
api_get "/api/leads?include_archived=1&limit=100" "leads.json"
