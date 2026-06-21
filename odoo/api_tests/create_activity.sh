#!/usr/bin/env bash
# POST /api/activities — schedule a follow-up on a lead.
# Usage: ./create_activity.sh [lead_id]   (defaults to the first lead returned)
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login

LEAD="${1:-}"
if [ -z "$LEAD" ]; then
  api_get "/api/leads?limit=1" "_probe.json" >/dev/null
  LEAD="$(python3 -c 'import json;d=json.load(open("'"$OUT_DIR"'/_probe.json"));print(d["results"][0]["id"] if d["results"] else "")' 2>/dev/null || true)"
fi
[ -n "$LEAD" ] || { echo "✗ no lead to attach to" >&2; exit 1; }

echo "  creating follow-up on lead id=${LEAD}"
BODY="{\"lead\":${LEAD},\"activity_type\":\"mail.mail_activity_data_call\",\"summary\":\"Send floor plans for Mivida units\",\"note\":\"Follow up after the intro call.\",\"date_deadline\":\"$(date -v+2d +%Y-%m-%d 2>/dev/null || date -d '+2 days' +%Y-%m-%d)\"}"
out="${OUT_DIR}/create_activity.json"
echo "→ POST /api/activities  ${BODY}"
code="$(curl -sS -b "$COOKIE_JAR" -w '%{http_code}' -H 'Content-Type: application/json' \
  -X POST "${BASE_URL}/api/activities" -d "$BODY" -o "$out")"
[ "$code" = "201" ] || { echo "✗ HTTP ${code}:" >&2; cat "$out" >&2; echo >&2; exit 1; }
python3 -m json.tool "$out" >"${out}.tmp" && mv "${out}.tmp" "$out"
echo "✓ HTTP 201 (created) → ${out}"
