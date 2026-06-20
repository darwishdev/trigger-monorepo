#!/usr/bin/env bash
# GET /api/units — all units, plus a filtered (available only) variant.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login
api_get "/api/units" "units.json"
api_get "/api/units?state=available&max_price=20000000" "units_available_under_20m.json"
