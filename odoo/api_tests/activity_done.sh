#!/usr/bin/env bash
# POST /api/activities/<id>/done — complete a follow-up task.
# Usage: ./activity_done.sh [activity_id]   (defaults to the first pending one)
#
# NOTE: Odoo deletes the activity on completion and logs a chatter message,
# so the response returns a snapshot + the resulting message_id for your audit.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login

ID="${1:-}"
if [ -z "$ID" ]; then
  api_get "/api/activities?limit=1" "_probe.json" >/dev/null
  ID="$(python3 -c 'import json;d=json.load(open("'"$OUT_DIR"'/_probe.json"));print(d["results"][0]["id"] if d["results"] else "")' 2>/dev/null || true)"
fi
[ -n "$ID" ] || { echo "✗ no pending activity to complete" >&2; exit 1; }

echo "  completing activity id=${ID}"
api_post "/api/activities/${ID}/done" "activity_done.json"
