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
    sync)
        node scripts/sync-metadata.mjs
        ;;
    install)
        (cd desktop && npm ci)
        node scripts/sync-metadata.mjs
        (cd backend && go mod download)
        ;;
    format)
        desktop/node_modules/.bin/prettier . --write
        gofmt -w backend
        (cd desktop/src-tauri && cargo fmt)
        ;;
    format-check)
        desktop/node_modules/.bin/prettier . --check
        unformatted_go="$(gofmt -l backend)"
        if [ -n "$unformatted_go" ]; then
            echo "$unformatted_go"
            exit 1
        fi
        (cd desktop/src-tauri && cargo fmt --check)
        ;;
    db-init|db-start|db-stop|backup)
        exec python3 scripts/database.py "$1"
        ;;
    build-server)
        build_server
        ;;
    migrate|check-config|prepare-setup|provision-sa|reset-sa|tls-init|create-enrollment)
        build_server
        command_name="$1"
        shift
        exec bin/ad-el-pos-server "$command_name" --config "${ADEL_SERVER_CONFIG:-.local/server.json}" "$@"
        ;;
    server)
        build_server
        exec bin/ad-el-pos-server serve --config "${ADEL_SERVER_CONFIG:-.local/server.json}"
        ;;
    server-start)
        build_server
        exec python3 scripts/server-process.py start
        ;;
    server-stop)
        exec python3 scripts/server-process.py stop
        ;;
    status)
        exec python3 scripts/server-process.py status
        ;;
    desktop|open-app)
        node scripts/sync-metadata.mjs
        if [ -f "$PROJECT_ROOT/.local/provisioning-token" ]; then
            export ADEL_PROVISIONING_FILE="$PROJECT_ROOT/.local/provisioning-token"
        fi
        if [ -f "$PROJECT_ROOT/.local/enrollment-code" ]; then
            export ADEL_ENROLLMENT_FILE="$PROJECT_ROOT/.local/enrollment-code"
        fi
        if [ "$1" = "open-app" ]; then
            exec "desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app/Contents/MacOS/ad-el-pos-desktop"
        fi
        cd desktop
        exec npm run tauri -- dev
        ;;
    build-macos)
        node scripts/sync-metadata.mjs
        cd desktop
        exec npm run tauri -- build --bundles app
        ;;
    build-macos-pkg)
        # Etapa 9G: instalador .pkg con el servidor como servicio (no instala nada; ver el script).
        shift
        exec scripts/build-macos-pkg.sh "$@"
        ;;
    test)
        "$PROJECT_ROOT/scripts/dev.sh" format-check
        node scripts/sync-metadata.mjs --check
        (cd backend && go test -race ./... && go vet ./...)
        (cd desktop && npm test && npm run build)
        (cd desktop/src-tauri && cargo test --locked)
        ;;
    test-browser)
        exec python3 scripts/browser-tests.py
        ;;
    test-integration)
        exec python3 scripts/database.py test-integration
        ;;
    *)
        echo 'Consulta README.md para los comandos de desarrollo, formato, aprovisionamiento y pruebas.'
        ;;
esac
