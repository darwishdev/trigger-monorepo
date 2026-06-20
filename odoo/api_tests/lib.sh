#!/usr/bin/env bash
#
# Shared helpers for the Trigger CRM API test scripts. Source it, don't run it.
#   source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
#
# Config via environment (defaults shown):
#   BASE_URL=http://localhost:8069  DB=trigger_test7  LOGIN=admin  PASSWORD=admin
#   ODOO_API_KEY=<personal-api-key> skips session login and sends Bearer auth
#
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8069}"
DB="${DB:-trigger_test7}"
LOGIN="${LOGIN:-admin}"
PASSWORD="${PASSWORD:-admin}"
ODOO_API_KEY="${ODOO_API_KEY:-}"

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${LIB_DIR}/responses"
mkdir -p "$OUT_DIR"

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

api_login() {
  if [ -n "$ODOO_API_KEY" ]; then
    echo "  using Odoo API key bearer auth on ${DB}"
    return 0
  fi

  local resp uid
  resp="$(curl -sS -c "$COOKIE_JAR" -H 'Content-Type: application/json' \
    -X POST "${BASE_URL}/web/session/authenticate" \
    -d "{\"jsonrpc\":\"2.0\",\"params\":{\"db\":\"${DB}\",\"login\":\"${LOGIN}\",\"password\":\"${PASSWORD}\"}}")"
  uid="$(printf '%s' "$resp" | python3 -c 'import sys,json;print((json.load(sys.stdin).get("result") or {}).get("uid") or "")' 2>/dev/null || true)"
  [ -n "$uid" ] || { echo "✗ auth failed: $resp" >&2; exit 1; }
  echo "  authenticated as ${LOGIN} (uid=${uid}) on ${DB}"
}

_finish() {  # <http_code> <outfile>
  local code="$1" out="$2"
  if [ "$code" != "200" ]; then echo "✗ HTTP ${code}:" >&2; cat "$out" >&2; echo >&2; return 1; fi
  command -v python3 >/dev/null 2>&1 && { python3 -m json.tool "$out" >"${out}.tmp" && mv "${out}.tmp" "$out"; }
  echo "✓ HTTP 200 → ${out}"
}

api_get() {  # <path> <outfile-basename>
  local out="${OUT_DIR}/$2" code
  echo "→ GET $1"
  if [ -n "$ODOO_API_KEY" ]; then
    code="$(curl -sS -H "Authorization: Bearer ${ODOO_API_KEY}" -w '%{http_code}' "${BASE_URL}$1" -o "$out")"
  else
    code="$(curl -sS -b "$COOKIE_JAR" -w '%{http_code}' "${BASE_URL}$1" -o "$out")"
  fi
  _finish "$code" "$out"
}

api_post() {  # <path> <outfile-basename>
  local out="${OUT_DIR}/$2" code
  echo "→ POST $1"
  if [ -n "$ODOO_API_KEY" ]; then
    code="$(curl -sS -H "Authorization: Bearer ${ODOO_API_KEY}" -w '%{http_code}' -X POST "${BASE_URL}$1" -o "$out")"
  else
    code="$(curl -sS -b "$COOKIE_JAR" -w '%{http_code}' -X POST "${BASE_URL}$1" -o "$out")"
  fi
  _finish "$code" "$out"
}
