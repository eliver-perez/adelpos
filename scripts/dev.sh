#!/usr/bin/env bash
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PROJECT_ROOT
source "$PROJECT_ROOT/scripts/env.sh"
cd "$PROJECT_ROOT"

build_server() {
  node scripts/sync-metadata.mjs
  mkdir -p bin
  (cd backend && go build -trimpath -o ../bin/ad-el-pos-server ./cmd/ad-el-pos-server)
}

case "${1:-help}" in
  sync) node scripts/sync-metadata.mjs ;;
  install) node scripts/sync-metadata.mjs; (cd desktop && npm ci); (cd backend && go mod download) ;;
  db-init|db-start|db-stop|backup) exec python3 scripts/database.py "$1" ;;
  build-server) build_server ;;
  migrate) build_server; exec bin/ad-el-pos-server migrate --config "${ADEL_SERVER_CONFIG:-.local/server.json}" ;;
  check-config) build_server; exec bin/ad-el-pos-server check-config --config "${ADEL_SERVER_CONFIG:-.local/server.json}" ;;
  server) build_server; exec bin/ad-el-pos-server serve --config "${ADEL_SERVER_CONFIG:-.local/server.json}" ;;
  server-start) build_server; exec python3 scripts/server-process.py start ;;
  server-stop) exec python3 scripts/server-process.py stop ;;
  status) exec python3 scripts/server-process.py status ;;
  desktop) node scripts/sync-metadata.mjs; cd desktop; exec npm run tauri -- dev ;;
  build-macos) node scripts/sync-metadata.mjs; cd desktop; exec npm run tauri -- build --bundles app ;;
  test)
    node scripts/sync-metadata.mjs --check
    (cd backend && go test -race ./... && go vet ./...)
    (cd desktop && npm test && npm run build)
    (cd desktop/src-tauri && cargo test --locked)
    ;;
  test-integration) exec python3 scripts/database.py test-integration ;;
  *) echo 'Uso: ./scripts/dev.sh {install|sync|db-init|db-start|db-stop|migrate|server|server-start|server-stop|status|desktop|build-server|build-macos|test|test-integration|backup|check-config}' ;;
esac
