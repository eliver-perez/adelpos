#!/bin/bash
# Funciones comunes de los scripts del instalador de macOS (etapa 9G, fase A; decisión D3). El script de
# compilación (scripts/build-macos-pkg.sh) copia este archivo junto a cada preinstall y postinstall.
#
# El Instalador de macOS ejecuta los scripts como root, con el disco de destino en $3. Fuera del Instalador
# (pruebas) solo corren SIN root y con un destino falso:
#
#   ADEL_PKG_TEST_ROOT       prefijo de destino (obligatorio): /Library, /Applications… quedan debajo
#   ADEL_PKG_TEST_LAUNCHCTL  programa que sustituye a /bin/launchctl (obligatorio)
#   ADEL_PKG_TEST_USER       cuenta que «instala» (por omisión, la actual)
#   ADEL_PKG_TEST_PG_BIN     carpeta de PostgreSQL que se comprueba (por omisión, la de Homebrew)
#
# Como root esas variables se ignoran: nunca cambian el destino real.
set -euo pipefail

readonly ADEL_LABEL="local.adel.pos.server"
readonly ADEL_PRODUCT="AD-EL POS"
readonly ADEL_SERVER_BINARY="ad-el-pos-server"
readonly ADEL_MODE_FILE="installation-mode.json"
readonly ADEL_ENABLE_FILE="service-enabled"

adel_fail() {
    echo "AD-EL POS: $*" >&2
    exit 1
}

adel_log() {
    echo "AD-EL POS: $*"
}

if [ "$(id -u)" -eq 0 ]; then
    # Solo el disco de arranque: el servicio del sistema vive en él.
    if [ -n "${3:-}" ] && [ "${3:-}" != "/" ]; then
        adel_fail "instala AD-EL POS en el disco de arranque de la computadora"
    fi
    ADEL_ROOT=""
    ADEL_LAUNCHCTL="/bin/launchctl"
    ADEL_TEST=0
    # La cuenta que instala es la que tiene la sesión gráfica (el Instalador corre como root).
    ADEL_USER="$(/usr/bin/stat -f %Su /dev/console)"
    ADEL_PG_BINS="/opt/homebrew/opt/postgresql@18/bin /usr/local/opt/postgresql@18/bin"
else
    if [ -z "${ADEL_PKG_TEST_ROOT:-}" ] || [ -z "${ADEL_PKG_TEST_LAUNCHCTL:-}" ]; then
        adel_fail "estos scripts solo corren en el Instalador de macOS o en pruebas con ADEL_PKG_TEST_ROOT y ADEL_PKG_TEST_LAUNCHCTL"
    fi
    case "$ADEL_PKG_TEST_ROOT" in
        /*) ;;
        *) adel_fail "ADEL_PKG_TEST_ROOT debe ser una ruta absoluta" ;;
    esac
    ADEL_ROOT="${ADEL_PKG_TEST_ROOT%/}"
    ADEL_LAUNCHCTL="$ADEL_PKG_TEST_LAUNCHCTL"
    ADEL_TEST=1
    ADEL_USER="${ADEL_PKG_TEST_USER:-$(id -un)}"
    ADEL_PG_BINS="${ADEL_PKG_TEST_PG_BIN:-/opt/homebrew/opt/postgresql@18/bin /usr/local/opt/postgresql@18/bin}"
fi

readonly ADEL_DATA_DIR="$ADEL_ROOT/Library/Application Support/$ADEL_PRODUCT"
readonly ADEL_APP="$ADEL_ROOT/Applications/$ADEL_PRODUCT.app"
readonly ADEL_SERVER="$ADEL_APP/Contents/MacOS/$ADEL_SERVER_BINARY"
readonly ADEL_DAEMON="$ADEL_ROOT/Library/LaunchDaemons/$ADEL_LABEL.plist"

# adel_check_user exige una cuenta de usuario real (no root ni la del asistente de configuración). Los
# nombres cortos de macOS solo tienen letras, números, punto, guion y guion bajo: así el nombre puede ir al
# plist sin escaparlo.
adel_check_user() {
    case "$ADEL_USER" in
        "" | root | _mbsetupuser | loginwindow)
            adel_fail "no se encontró la cuenta del usuario que instala; inicia sesión y vuelve a abrir el instalador"
            ;;
    esac
    if ! printf '%s' "$ADEL_USER" | /usr/bin/grep -Eq '^[A-Za-z0-9_][A-Za-z0-9_.-]{0,254}$'; then
        adel_fail "el nombre de la cuenta que instala no es válido"
    fi
    if ! /usr/bin/id "$ADEL_USER" >/dev/null 2>&1; then
        adel_fail "la cuenta $ADEL_USER no existe"
    fi
}

# adel_chown cambia el dueño (solo como root; en pruebas todo ya es de la cuenta actual).
adel_chown() {
    if [ "$ADEL_TEST" -eq 0 ]; then
        /usr/sbin/chown "$@"
    fi
}

# adel_prepare_data_dir crea la carpeta de datos de la cuenta que instala, con permisos 0700, y su carpeta
# de bitácoras. Si ya existe y es de otra cuenta, no la toca: los datos del restaurante nunca cambian de
# dueño en silencio.
adel_prepare_data_dir() {
    adel_check_user
    local group
    group="$(/usr/bin/id -gn "$ADEL_USER")"
    if [ -e "$ADEL_DATA_DIR" ] || [ -L "$ADEL_DATA_DIR" ]; then
        if [ -L "$ADEL_DATA_DIR" ] || [ ! -d "$ADEL_DATA_DIR" ]; then
            adel_fail "la carpeta de datos no es una carpeta normal; revisión manual necesaria"
        fi
        local owner
        owner="$(/usr/bin/stat -f %Su "$ADEL_DATA_DIR")"
        if [ "$owner" != "$ADEL_USER" ]; then
            adel_fail "la carpeta de datos pertenece a la cuenta $owner; instala AD-EL POS con esa cuenta"
        fi
    else
        /bin/mkdir -p "$(/usr/bin/dirname "$ADEL_DATA_DIR")"
        /bin/mkdir -m 0700 "$ADEL_DATA_DIR"
        adel_chown "$ADEL_USER:$group" "$ADEL_DATA_DIR"
    fi
    /bin/chmod 0700 "$ADEL_DATA_DIR"
    if [ ! -d "$ADEL_DATA_DIR/logs" ]; then
        /bin/mkdir -m 0700 "$ADEL_DATA_DIR/logs"
        adel_chown "$ADEL_USER:$group" "$ADEL_DATA_DIR/logs"
    fi
    # Las bitácoras de launchd (salida estándar y de errores del servicio) se crean de la cuenta que
    # instala para que pueda leerlas sin permisos de administrador.
    local log
    for log in service.log service-error.log; do
        if [ ! -e "$ADEL_DATA_DIR/logs/$log" ] && [ ! -L "$ADEL_DATA_DIR/logs/$log" ]; then
            : >"$ADEL_DATA_DIR/logs/$log"
            /bin/chmod 0600 "$ADEL_DATA_DIR/logs/$log"
            adel_chown "$ADEL_USER:$group" "$ADEL_DATA_DIR/logs/$log"
        fi
    done
}

# adel_write_user_file escribe un archivo 0600 de la cuenta que instala con reemplazo atómico.
adel_write_user_file() {
    local target="$1" content="$2" temporary group
    group="$(/usr/bin/id -gn "$ADEL_USER")"
    temporary="$(/usr/bin/mktemp "$ADEL_DATA_DIR/.adel-pkg.XXXXXX")"
    printf '%s' "$content" >"$temporary"
    /bin/chmod 0600 "$temporary"
    adel_chown "$ADEL_USER:$group" "$temporary"
    /bin/mv -f "$temporary" "$target"
}

# adel_current_mode lee la modalidad guardada (vacía si no hay archivo). El archivo lo escriben este
# instalador, la aplicación (Rust) y nadie más; aquí basta reconocer los tres valores.
adel_current_mode() {
    local file="$ADEL_DATA_DIR/$ADEL_MODE_FILE"
    if [ ! -f "$file" ]; then
        echo ""
        return
    fi
    local mode
    for mode in server-terminal server-only terminal-only; do
        if /usr/bin/grep -q "\"$mode\"" "$file"; then
            echo "$mode"
            return
        fi
    done
    echo ""
}

adel_write_mode() {
    adel_write_user_file "$ADEL_DATA_DIR/$ADEL_MODE_FILE" "{\"installation_mode\":\"$1\"}
"
}

# adel_find_postgres comprueba que PostgreSQL 18 esté instalado (decisión D3: no viaja en el paquete).
adel_find_postgres() {
    local folder
    for folder in $ADEL_PG_BINS; do
        if [ -x "$folder/initdb" ] && [ -x "$folder/pg_ctl" ]; then
            echo "$folder"
            return 0
        fi
    done
    return 1
}

# adel_service_loaded indica si el servicio está cargado en launchd.
adel_service_loaded() {
    "$ADEL_LAUNCHCTL" print "system/$ADEL_LABEL" >/dev/null 2>&1
}

# adel_stop_service descarga el servicio (launchd le envía SIGTERM: Go se cierra en orden y detiene
# PostgreSQL) y espera a que termine.
adel_stop_service() {
    if ! adel_service_loaded; then
        return 0
    fi
    adel_log "deteniendo el servidor anterior"
    "$ADEL_LAUNCHCTL" bootout "system/$ADEL_LABEL" || true
    local attempt
    for attempt in $(/usr/bin/seq 1 120); do
        if ! adel_service_loaded; then
            return 0
        fi
        /bin/sleep 1
    done
    adel_fail "el servidor anterior no se detuvo; reinicia la computadora y vuelve a abrir el instalador"
}
