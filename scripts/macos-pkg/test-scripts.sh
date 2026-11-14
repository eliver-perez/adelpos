#!/usr/bin/env bash
# Prueba de los scripts del instalador de macOS (etapa 9G, fase A) sin instalar nada: los ejecuta SIN root
# con un destino falso dentro de una carpeta temporal y un launchctl simulado (ver common.sh). Comprueba
# la carpeta de datos (dueño y 0700), la modalidad, el archivo de habilitación, el LaunchDaemon generado
# (valores, permisos y plutil -lint) y las órdenes que habría recibido launchctl.
#
#   scripts/macos-pkg/test-scripts.sh [carpeta temporal]
set -euo pipefail
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SOURCE/../.." && pwd)"
if [ "$(id -u)" -eq 0 ]; then
    echo "Esta prueba no se ejecuta como root." >&2
    exit 1
fi
base="$(mktemp -d "${1:-${TMPDIR:-/tmp}}/adel-pkg-test.XXXXXX")"
trap 'rm -rf "$base"' EXIT
failures=0

check() {
    local description="$1"
    shift
    if "$@"; then
        echo "  ok   $description"
    else
        echo "  FALLA $description"
        failures=$((failures + 1))
    fi
}

# Un escenario: copia los scripts como los arma build-macos-pkg.sh, crea la aplicación y PostgreSQL falsos
# y prepara el launchctl simulado.
scenario() {
    local name="$1"
    root="$base/$name/root"
    state="$base/$name/launchctl"
    scripts="$base/$name/scripts"
    mkdir -p "$root/Applications/AD-EL POS.app/Contents/MacOS" "$state" "$base/$name/pg/bin"
    for package in app server service; do
        mkdir -p "$scripts/$package"
        cp "$SOURCE/common.sh" "$scripts/$package/"
    done
    cp "$SOURCE/app/preinstall" "$scripts/app/"
    cp "$SOURCE/server/preinstall" "$SOURCE/server/postinstall" "$scripts/server/"
    cp "$SOURCE/service/postinstall" "$scripts/service/"
    cp "$PROJECT_ROOT/ops/launchd/local.adel.pos.server.plist" "$scripts/service/"
    printf '#!/bin/sh\nexit 0\n' >"$root/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server"
    printf '#!/bin/sh\nexit 0\n' >"$base/$name/pg/bin/initdb"
    printf '#!/bin/sh\nexit 0\n' >"$base/$name/pg/bin/pg_ctl"
    chmod 0755 "$root/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" "$base/$name/pg/bin/"*
    cat >"$state/launchctl" <<EOF
#!/bin/bash
echo "\$*" >>"$state/log"
case "\$1" in
    print) [ -f "$state/loaded" ] ;;
    bootstrap) touch "$state/loaded" ;;
    bootout) rm -f "$state/loaded" ;;
esac
EOF
    chmod 0755 "$state/launchctl"
    export ADEL_PKG_TEST_ROOT="$root"
    export ADEL_PKG_TEST_LAUNCHCTL="$state/launchctl"
    export ADEL_PKG_TEST_PG_BIN="$base/$name/pg/bin"
    unset ADEL_PKG_TEST_USER
    data="$root/Library/Application Support/AD-EL POS"
    daemon="$root/Library/LaunchDaemons/local.adel.pos.server.plist"
}

run() {
    "$scripts/$1" "$base/AD-EL POS.pkg" / / / >"$base/last.log" 2>&1
}

mode_is() {
    grep -q "\"installation_mode\":\"$1\"" "$data/installation-mode.json"
}

permissions_are() {
    [ "$(stat -f %Lp "$1")" = "$2" ] && [ "$(stat -f %Su "$1")" = "$(id -un)" ]
}

plist_value() {
    plutil -extract "$1" raw -o - "$daemon"
}

echo "Servidor y terminal (instalación nueva)"
scenario server
run app/preinstall
run server/preinstall
run server/postinstall
run service/postinstall
check "carpeta de datos 0700 de la cuenta" permissions_are "$data" 700
check "carpeta de bitácoras 0700" permissions_are "$data/logs" 700
check "bitácoras del servicio 0600 de la cuenta" permissions_are "$data/logs/service.log" 600
check "modalidad «Servidor y terminal»" mode_is server-terminal
check "modalidad 0600" permissions_are "$data/installation-mode.json" 600
check "archivo de habilitación 0600" permissions_are "$data/service-enabled" 600
check "LaunchDaemon 0644" test "$(stat -f %Lp "$daemon")" = 644
check "LaunchDaemon válido" plutil -lint -s "$daemon"
check "UserName = cuenta que instala" test "$(plist_value UserName)" = "$(id -un)"
check "servidor de dentro de la aplicación" test "$(plist_value ProgramArguments.0)" = "$root/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server"
check "comando host" test "$(plist_value ProgramArguments.1)" = host
check "carpeta de datos del servicio" test "$(plist_value ProgramArguments.3)" = "$data"
check "RunAtLoad" test "$(plist_value RunAtLoad)" = true
check "KeepAlive con PathState del archivo de habilitación" \
    test "$(plutil -extract KeepAlive.PathState xml1 -o - "$daemon" | grep -c "<key>$data/service-enabled</key>")" = 1
check "ThrottleInterval" test "$(plist_value ThrottleInterval)" = 10
check "bitácora en logs" test "$(plist_value StandardOutPath)" = "$data/logs/service.log"
check "sin marcadores" bash -c "! grep -q __ADEL_ '$daemon'"
check "launchctl bootstrap system" grep -qx "bootstrap system $daemon" "$state/log"
check "servicio cargado" test -f "$state/loaded"

echo "Actualización: detiene el servicio y conserva «Solo servidor»"
printf '{"installation_mode":"server-only"}\n' >"$data/installation-mode.json"
: >"$state/log"
run app/preinstall
check "preinstall descarga el servicio anterior" grep -qx "bootout system/local.adel.pos.server" "$state/log"
check "servicio descargado" test ! -f "$state/loaded"
run server/postinstall
run service/postinstall
check "conserva «Solo servidor»" mode_is server-only
check "vuelve a cargar el servicio" test -f "$state/loaded"

echo "Solo terminal (opción desmarcada)"
scenario terminal
run app/preinstall
run service/postinstall
check "modalidad «Solo terminal»" mode_is terminal-only
check "sin archivo de habilitación" test ! -e "$data/service-enabled"
check "servicio registrado (termina enseguida sin PostgreSQL ni Go)" test -f "$state/loaded"

echo "Rechazos"
scenario refusals
export ADEL_PKG_TEST_PG_BIN="$base/refusals/sin-postgresql"
check "sin PostgreSQL 18 no se instala el servidor" bash -c "! '$scripts/server/preinstall' >'$base/last.log' 2>&1"
check "el aviso explica qué falta" grep -q "falta PostgreSQL 18" "$base/last.log"
export ADEL_PKG_TEST_PG_BIN="$base/refusals/pg/bin"
run server/postinstall
export ADEL_PKG_TEST_USER=nobody
check "no toma una carpeta de datos de otra cuenta" bash -c "! '$scripts/service/postinstall' >'$base/last.log' 2>&1"
check "el aviso nombra a la dueña" grep -q "pertenece a la cuenta $(id -un)" "$base/last.log"
unset ADEL_PKG_TEST_USER
check "sin destino de prueba no corre sin root" \
    bash -c "! env -u ADEL_PKG_TEST_ROOT '$scripts/server/postinstall' >'$base/last.log' 2>&1"

if [ "$failures" -ne 0 ]; then
    echo "$failures comprobaciones fallaron"
    exit 1
fi
echo "Scripts del instalador: todo correcto"
