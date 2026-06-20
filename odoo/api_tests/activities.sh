#!/usr/bin/env bash
# GET /api/activities — the pending follow-up task queue (mail.activity).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login
api_get "/api/activities" "activities.json"
