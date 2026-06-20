#!/usr/bin/env bash
# GET /api/projects (list) and GET /api/projects/<id> (detail + units).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
api_login
api_get "/api/projects" "projects.json"

# detail of the first project returned
PID="$(python3 -c 'import json;d=json.load(open("'"$OUT_DIR"'/projects.json"));print(d["results"][0]["id"] if d["results"] else "")' 2>/dev/null || true)"
[ -n "$PID" ] && api_get "/api/projects/${PID}" "project_detail.json"
