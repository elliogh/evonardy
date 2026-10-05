#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .cache
go build -o .cache/evonardy-dev ./cmd/evonardy
.cache/evonardy-dev serve --addr 127.0.0.1:8080 --dev-origin http://127.0.0.1:5173 --data-dir "${EVONARDY_DATA_DIR:-./data}" &
backend_pid=$!
frontend_pid=""
cleanup() {
  if [[ -n "$frontend_pid" ]]; then kill "$frontend_pid" 2>/dev/null || true; wait "$frontend_pid" 2>/dev/null || true; fi
  kill "$backend_pid" 2>/dev/null || true
  wait "$backend_pid" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT TERM
node --input-type=module <<'JS'
for (let attempt = 0; attempt < 50; attempt++) {
  try { if ((await fetch("http://127.0.0.1:8080/api/bots")).ok) process.exit(0); } catch {}
  await new Promise(resolve => setTimeout(resolve, 100));
}
throw new Error("The backend did not start. Check its output above.");
JS
kill -0 "$backend_pid"
cd web
node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 5173 --strictPort &
frontend_pid=$!
wait "$frontend_pid"
