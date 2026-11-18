#!/usr/bin/env bash
# Etapas A1 (fases C, B, D, E y F) y A2: pruebas de extremo a extremo de la aplicación Android contra un servidor Go
# TEMPORAL en modo de pruebas, nunca contra el servicio real de la Mac. Cada paso corre en su propia instrumentación
# (`am instrument`), en orden y con la aplicación instalada una sola vez por suite: los datos pasan de un paso al
# siguiente y el proceso se reinicia en cada uno.
#
# Uso (desde cualquier carpeta):
#   ANDROID_SERIAL=emulator-5580 \
#   ADELPOS_E2E_LOCAL_URL=http://127.0.0.1:8194 \
#   ADELPOS_E2E_NETWORK_URL=https://127.0.0.1:8454 \
#   ADELPOS_E2E_PROVISIONING_FILE=/ruta/al/provisioning-token \
#   [ADELPOS_E2E_DATABASE_URL=postgresql://usuario:clave@127.0.0.1:55499/adel_pos_test_…] \
#   scripts/android-e2e.sh [--suite access|catalog|waiter|field|takeaway|alerts|all] [--steps "…"] [--no-install] \
#       [--out <carpeta>]
#
# Variables:
#   ANDROID_SERIAL                  obligatorio: el emulador o dispositivo (nunca se corre en todos los conectados).
#   ADELPOS_E2E_LOCAL_URL           escucha local HTTP del Go temporal vista desde la Mac (127.0.0.1 o localhost).
#   ADELPOS_E2E_NETWORK_URL         escucha de red HTTPS del Go temporal vista desde la Mac (127.0.0.1 o localhost).
#   ADELPOS_E2E_DEVICE_HOST         cómo llega el dispositivo a la Mac (10.0.2.2 en el emulador, por omisión).
#   ADELPOS_E2E_PIN                 huella SPKI esperada (hexadecimal); si falta, se toma del certificado servido.
#   ADELPOS_E2E_SERVER_ID           server_id esperado (opcional; el paso 1 de access comprueba el emparejado).
#   ADELPOS_E2E_PROVISIONING        token de aprovisionamiento, o ADELPOS_E2E_PROVISIONING_FILE con su ruta: si el
#                                   servidor no está configurado, se hace el asistente con él.
#   ADELPOS_E2E_MAIN_DEVICE y ADELPOS_E2E_MAIN_TOKEN
#                                   terminal principal de un servidor ya configurado (la imprime una corrida anterior).
#   ADELPOS_E2E_DATABASE_URL        opcional: la base TEMPORAL de ese servidor, solo para lo que Go no permite por su
#                                   API (la época del catálogo de la etapa `c` y la serie de bloqueos del PIN de `f7`).
#                                   Solo se acepta en 127.0.0.1/localhost, nunca en el puerto 55432 (PostgreSQL del
#                                   servicio real) y con una base `adel_pos_test_…` que tenga la terminal principal de
#                                   este servidor. Sin ella, esos pasos salen como OMITIDA.
#   ADELPOS_E2E_PSQL                psql para lo anterior (por omisión el del PATH o el de Homebrew postgresql@18).
#   ADELPOS_E2E_NEW_SERVER          solo con --suite all: orden que detiene el servidor temporal anterior y levanta
#                                   uno NUEVO (otra identidad y base vacía) en las MISMAS direcciones e imprime líneas
#                                   VARIABLE=valor con las ADELPOS_E2E_* que cambian (provisioning, base…).
#   JAVA_HOME, ANDROID_HOME         como en scripts/dev.sh (JDK de Android Studio y SDK del usuario por omisión).
#
# Suite `access` (AccessOnGoTest): 1 emparejar con el código · 2 PIN incorrecto, bloqueo al quinto (cierra la entrada
# sin conexión), entrada y cabeceras de cada mesero · 3 entrada sin conexión (modo avión en Android 11+; antes,
# `adb root` + `svc wifi/data` desde la Mac: solo imágenes google_apis) · 4 PIN cambiado mientras el mesero trabaja:
# «Vuelve a escribir tu PIN» sin salir de Mesas · 5 versión vieja (426) · 6 certificado cambiado · 7 dispositivo
# retirado. El paso 6 necesita OTRO servidor temporal (otra identidad) en las MISMAS direcciones: correr primero
# `--steps "1 2 3 4 5"`, reemplazar el servidor y después `--no-install --steps "6 7"` con los datos del nuevo.
#
# Suite `catalog` (CatalogOnGoTest, Fase D, con un servidor temporal nuevo): a catálogo completo (~300 productos),
# cambios por el canal y por descarga, agotado e imagen (304) · b la principal quita el producto de prueba con
# «Quitar datos de prueba» (su API) y la baja llega en `deleted` · c (este guion cambia la época del catálogo en la
# base temporal, como una restauración: Go no tiene API para eso) Go responde `reset: true` y se reemplaza todo.
#
# Suite `waiter` (Fase E, WaiterModeOnGoTest, con un servidor temporal nuevo): w1 preparar el restaurante, emparejar y
# entrar · w2 captura en línea, rotación y «Enviar a cocina», y cambios de otra terminal al momento · w3 (SIN red)
# captura sin conexión: «Por enviar» · w4 (con red) WorkManager envía sin entrar y la primera respuesta se pierde: el
# reintento no duplica · w5 precio cambiado y agotado con la confirmación del mesero · w6 mesa cobrada mientras tanto ·
# w7a borrador guardado en Go, w7b (SIN red) se agrega y se envía sin conexión, w7c (con red) el pedido sustituye el
# borrador (A1-D6) · w8 cancelar y modificar unidades enviadas · revisión R6 (A1-D19): w9 el guardado pierde su
# respuesta antes de «Enviar a cocina» · w10 la confirmación en línea entra y pierde sus respuestas: sin duplicar ·
# w11 descartar con conexión · w12a/w12b (SIN red)/w12c descartar sin conexión y terminarlo al volver ·
# w13a/w13b (SIN red)/w13c la cuenta se combina y el pedido va a la cuenta en que se combinó.
#
# Suite `field` (Fase F, FieldOnGoTest, con un servidor temporal nuevo): f1 restaurante con cocina, 40 mesas y ~300
# productos, emparejar y entrar · f2 rendimiento (arranque en frío, Mesas, captura, «Enviar a cocina», memoria) ·
# f3 «En preparación» y «Listo» de cocina al momento · f4 dos meseros en el mismo dispositivo con una captura en curso
# · f5a (SIN red) pedido de Beto «Por enviar» que impide cambiar de mesero · f5b (con red) cada pedido sale con la
# credencial de su mesero · f6 transferir y combinar desde la principal con la cuenta abierta · f7 bloqueo largo del
# PIN (A1-D16; este guion adelanta la serie en la base temporal) que cierra también la entrada sin conexión · f8
# muerte de la actividad a mitad de una captura y del personalizador · f9 Atrás y bloqueo por inactividad al volver ·
# f10 tema oscuro: capturas de las pantallas principales y botones de 48 dp (las copia a <salida>/screens).
#
# Suite `takeaway` (etapa A2, Fase B, TakeawayOnGoTest, con un servidor temporal nuevo): t1 restaurante con cocina
# y el rol de meseros con takeaway.*, emparejar y entrar (navegación «Mesas» y «Para llevar») · t2 «Nuevo pedido» sin
# productos no deja nada · t3 captura y envío en línea (nace con el primer producto, con nombre y teléfono) · t4 (SIN
# red) lo último que se vio con su hora, pedido nuevo «Por enviar» y nombre y teléfono editados sin conexión · t5 (con
# red) sale el pedido (`destination.takeaway`) y después las ediciones del cliente · t6 nombre y teléfono en línea ·
# t7 preparación de cocina y pago de la caja al momento · t8 entrega bloqueada sin pago y permitida pagado; entregado
# y pagado sale de la lista.
#
# La red la corta y la devuelve este guion entre pasos (modo avión desde la Mac en Android 11+; `adb root` + `svc
# wifi/data` en Android 8–10).
#
# Suite `alerts` (etapa A2, Fase D, AlertsOnGoTest con `readyAlerts=on`, con un servidor temporal nuevo): r1 preparar
# (Ana y Beto, cocina en pantalla, mesas), emparejar y entrar: arranca el servicio con «AD-EL POS conectado» y la
# explicación de los permisos · r2 aviso con la aplicación cerrada (tarea quitada de recientes), la pantalla apagada y
# Doze forzado · r3 no llega a otro mesero ni a otro dispositivo (un segundo android emparejado por la API) y
# transferir la cuenta cambia el destinatario · r4 este guion corta la red, marca «Listo» desde la Mac durante el
# corte y la devuelve: llega una sola vez · r5 agrupado por cuenta y resumen · r6 en «¿Quién eres?», al tocarlo pide
# el PIN y abre la cuenta; con la cuenta a la vista, aviso dentro de la aplicación · r7 sonido y vibración apagados
# desde Terminales y aviso apagado que detiene el servicio · r8 (r8a + r8b) este guion abre la aplicación (con la
# wifi del emulador apagada en Android 11+: el emulador alcanza 10.0.2.2 por la red móvil y no ve el anuncio real),
# mata el proceso, espera el reinicio pegajoso, marca «Listo» desde la Mac y busca el aviso con `dumpsys
# notification --noredact` · r9 al retirar el dispositivo el servicio se detiene. «Listo» desde la Mac usa la sesión
# del administrador de la terminal principal por la escucha local (127.0.0.1).
#
# Suite `all`: con ADELPOS_E2E_NEW_SERVER, un servidor temporal nuevo para cada parte: access 1–5, access 6–7 (otra
# identidad, sin reinstalar), catalog, waiter, field, takeaway y alerts. Resultados en <salida>/<suite>.
#
# Seguridad: se niega a correr con los puertos del servicio real (8181, 8443), con un host que no sea la propia Mac
# o si `GET version` de alguna de las dos escuchas no responde `test_mode: true`; la base solo con las guardas de
# arriba. Las pruebas usan el tipo NSD `_adelpostest._tcp` (AdelTestRunner): nunca buscan ni prueban el servidor real
# del restaurante.
#
# Aplicación: `com.adelpos.meseros` (A2-D5) con su paquete de pruebas `com.adelpos.meseros.test`. Antes de instalar
# se desinstalan también los de la 0.1.0 (`local.adel.pos.android` y su `.test`): para Android son otra aplicación y
# no deben quedar en el emulador junto a la nueva.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUITE="access"
STEPS=""
INSTALL=1
OUT=""

while [ $# -gt 0 ]; do
    case "$1" in
        --steps) STEPS="${2:?pasos}"; shift 2 ;;
        --suite) SUITE="${2:?suite}"; shift 2 ;;
        --no-install) INSTALL=0; shift ;;
        --out) OUT="${2:?carpeta}"; shift 2 ;;
        *) echo "Argumento no reconocido: $1" >&2; exit 2 ;;
    esac
done

fail() {
    echo "android-e2e: $*" >&2
    exit 2
}

# Clase de prueba y pasos por omisión de cada suite.
suite_class() {
    case "$1" in
        access) echo "AccessOnGoTest" ;;
        catalog) echo "CatalogOnGoTest" ;;
        waiter) echo "WaiterModeOnGoTest" ;;
        field) echo "FieldOnGoTest" ;;
        takeaway) echo "TakeawayOnGoTest" ;;
        alerts) echo "AlertsOnGoTest" ;;
        *) fail "suite desconocida: $1 (access, catalog, waiter, field, takeaway, alerts o all)" ;;
    esac
}

suite_steps() {
    case "$1" in
        access) echo "1 2 3 4 5 7" ;;
        catalog) echo "a b c" ;;
        waiter) echo "w1 w2 w3 w4 w5 w6 w7a w7b w7c w8 w9 w10 w11 w12a w12b w12c w13a w13b w13c" ;;
        field) echo "f1 f2 f3 f4 f5a f5b f6 f7 f8 f9 f10" ;;
        takeaway) echo "t1 t2 t3 t4 t5 t6 t7 t8" ;;
        alerts) echo "r1 r2 r3 r4 r5 r6 r7 r8 r9" ;;
    esac
}

case "$SUITE" in
    all)
        [ -z "$STEPS" ] || fail "--suite all corre todos los pasos de cada suite (sin --steps)"
        [ -n "${ADELPOS_E2E_NEW_SERVER:-}" ] ||
            fail "--suite all necesita ADELPOS_E2E_NEW_SERVER (un servidor temporal nuevo para cada suite)"
        ;;
    *)
        suite_class "$SUITE" >/dev/null
        STEPS="${STEPS:-$(suite_steps "$SUITE")}"
        ;;
esac

: "${ANDROID_SERIAL:?Falta ANDROID_SERIAL (el emulador o dispositivo donde correr)}"
DEVICE_HOST="${ADELPOS_E2E_DEVICE_HOST:-10.0.2.2}"
export ANDROID_SERIAL

if [ -z "${JAVA_HOME:-}" ]; then
    export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
fi
if [ -z "${ANDROID_HOME:-}" ]; then
    export ANDROID_HOME="$HOME/Library/Android/sdk"
fi
ADB="$ANDROID_HOME/platform-tools/adb"

# Identificador de la aplicación (A2-D5): el mismo prefijo que el namespace y los paquetes de Kotlin, así que las
# clases de prueba (AccessOnGoTest…) y el ejecutor AdelTestRunner viven en el paquete APP_ID. La 0.1.0 usaba
# LEGACY_APP_ID; se nombra solo para desinstalarla.
APP_ID="com.adelpos.meseros"
TEST_PACKAGE="$APP_ID.test"
TEST_RUNNER="$TEST_PACKAGE/$APP_ID.AdelTestRunner"
LEGACY_APP_ID="local.adel.pos.android"

# Host y puerto de una URL «esquema://host:puerto»; solo la propia Mac y nunca los puertos del servicio real.
check_url() {
    local url="$1" scheme="$2" rest host port
    case "$url" in
        "$scheme"://*) rest="${url#"$scheme"://}" ;;
        *) fail "$url debe empezar con $scheme://" ;;
    esac
    rest="${rest%/}"
    host="${rest%%:*}"
    port="${rest##*:}"
    if [ "$host" = "$rest" ] || [ -z "$port" ] || ! [[ "$port" =~ ^[0-9]+$ ]]; then
        fail "$url debe llevar el puerto"
    fi
    case "$host" in
        127.0.0.1 | localhost) ;;
        *) fail "$url no es la propia Mac (solo 127.0.0.1 o localhost)" ;;
    esac
    case "$port" in
        8181 | 8443) fail "$url usa un puerto del servicio real de la Mac (8181 u 8443): nunca se prueba contra él" ;;
    esac
    echo "$port"
}

# Solo un servidor en modo de pruebas: las dos escuchas deben decir test_mode: true.
test_mode() {
    python3 -I -c '
import json, sys
try:
    answer = json.load(sys.stdin)
except ValueError:
    sys.exit(1)
sys.exit(0 if isinstance(answer, dict) and answer.get("test_mode") is True else 1)'
}

json_field() {
    python3 -I -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

# Comprueba el servidor de las variables ADELPOS_E2E_* y deja listos LOCAL_URL, NETWORK_URL, LOCAL_PORT,
# NETWORK_PORT, PIN, MAIN_DEVICE y MAIN_TOKEN (haciendo el asistente si hace falta).
prepare_server() {
    LOCAL_URL="${ADELPOS_E2E_LOCAL_URL:?Falta ADELPOS_E2E_LOCAL_URL}"
    NETWORK_URL="${ADELPOS_E2E_NETWORK_URL:?Falta ADELPOS_E2E_NETWORK_URL}"
    LOCAL_PORT="$(check_url "$LOCAL_URL" http)"
    NETWORK_PORT="$(check_url "$NETWORK_URL" https)"
    LOCAL_URL="${LOCAL_URL%/}"
    NETWORK_URL="${NETWORK_URL%/}"
    curl -fsS --max-time 5 "$LOCAL_URL/api/v1/version" 2>/dev/null | test_mode ||
        fail "$LOCAL_URL no responde test_mode: true (no es un servidor temporal de pruebas)"
    # --insecure solo para esta consulta sin credenciales: la confianza la da la huella, que se compara abajo.
    curl -fsS --max-time 5 --insecure "$NETWORK_URL/api/v1/version" 2>/dev/null | test_mode ||
        fail "$NETWORK_URL no responde test_mode: true (no es un servidor temporal de pruebas)"

    local served
    served="$(openssl s_client -connect "127.0.0.1:$NETWORK_PORT" </dev/null 2>/dev/null |
        openssl x509 -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r | cut -d' ' -f1)"
    PIN="${ADELPOS_E2E_PIN:-$served}"
    if [ "$PIN" != "$served" ]; then
        fail "la huella de $NETWORK_URL ($served) no es ADELPOS_E2E_PIN ($PIN)"
    fi

    # Terminal principal: la indicada o la del asistente de configuración (servidor sin configurar).
    MAIN_DEVICE="${ADELPOS_E2E_MAIN_DEVICE:-}"
    MAIN_TOKEN="${ADELPOS_E2E_MAIN_TOKEN:-}"
    if [ -z "$MAIN_DEVICE" ] || [ -z "$MAIN_TOKEN" ]; then
        local provisioning="${ADELPOS_E2E_PROVISIONING:-}" setup
        if [ -z "$provisioning" ] && [ -n "${ADELPOS_E2E_PROVISIONING_FILE:-}" ]; then
            provisioning="$(cat "$ADELPOS_E2E_PROVISIONING_FILE")"
        fi
        [ -n "$provisioning" ] ||
            fail "sin terminal principal: indica ADELPOS_E2E_MAIN_DEVICE/TOKEN o el token de aprovisionamiento"
        setup="$(curl -fsS --max-time 30 -X POST "$LOCAL_URL/api/v1/setup" -H "Content-Type: application/json" \
            -H "X-Provisioning-Token: $provisioning" \
            -d '{"name":"Restaurante de prueba A1","timezone":"America/Monterrey","currency":"MXN","theme":"light","terminal_name":"Principal"}')" ||
            fail "el asistente de configuración no respondió (¿el servidor ya estaba configurado?)"
        MAIN_DEVICE="$(printf '%s' "$setup" | json_field device_id)"
        MAIN_TOKEN="$(printf '%s' "$setup" | json_field device_token)"
    fi
}

# --- Base temporal (solo lo que Go no permite por su API) ---

PSQL="${ADELPOS_E2E_PSQL:-}"
if [ -z "$PSQL" ]; then
    if command -v psql >/dev/null 2>&1; then
        PSQL="$(command -v psql)"
    else
        PSQL="/opt/homebrew/opt/postgresql@18/bin/psql"
    fi
fi

# Revisa ADELPOS_E2E_DATABASE_URL antes de cualquier SQL: solo la Mac, nunca el PostgreSQL del servicio real
# (55432) y solo una base de pruebas `adel_pos_test_…`. Sale con error si la URL existe pero no es aceptable.
database_allowed() {
    [ -n "${ADELPOS_E2E_DATABASE_URL:-}" ] || return 1
    python3 -I - "$ADELPOS_E2E_DATABASE_URL" <<'PY' || fail "ADELPOS_E2E_DATABASE_URL no es una base temporal de pruebas"
import re, sys
from urllib.parse import urlsplit
url = urlsplit(sys.argv[1])
ok = (
    url.scheme in ("postgresql", "postgres")
    and url.hostname in ("127.0.0.1", "localhost")
    and url.port is not None
    and url.port != 55432
    and re.fullmatch(r"/adel_pos_test_[a-z0-9_]+", url.path or "") is not None
)
sys.exit(0 if ok else 1)
PY
}

# SQL de prueba en la base temporal, después de comprobar que es la de ESTE servidor (tiene su terminal principal).
test_sql() {
    local owner
    owner="$("$PSQL" "$ADELPOS_E2E_DATABASE_URL" -X -tAq -v ON_ERROR_STOP=1 \
        -c "SELECT count(*) FROM app.devices WHERE id = '$MAIN_DEVICE'")" ||
        fail "no se pudo leer la base temporal"
    [ "$owner" = "1" ] || fail "la base de ADELPOS_E2E_DATABASE_URL no es la del servidor que se prueba"
    "$PSQL" "$ADELPOS_E2E_DATABASE_URL" -X -q -v ON_ERROR_STOP=1 -c "$1"
}

# --- Dispositivo ---

install_app() {
    # Se desinstalan la aplicación y sus pruebas (una copia con otra firma haría fallar la instalación) y también
    # las de la 0.1.0, que para Android son otra aplicación.
    local package
    for package in "$APP_ID" "$TEST_PACKAGE" "$LEGACY_APP_ID" "$LEGACY_APP_ID.test"; do
        "$ADB" -s "$ANDROID_SERIAL" uninstall "$package" >/dev/null 2>&1 || true
    done
    if [ ! -f "$PROJECT_ROOT/android/local.properties" ]; then
        echo "sdk.dir=$ANDROID_HOME" >"$PROJECT_ROOT/android/local.properties"
    fi
    (cd "$PROJECT_ROOT/android" && ./gradlew --console=plain -q :app:installDebug :app:installDebugAndroidTest) \
        >"$SUITE_OUT/install.log" 2>&1 || {
        tail -20 "$SUITE_OUT/install.log" >&2
        fail "no se pudo instalar la aplicación ni sus pruebas"
    }
}

FAILED=0

# Un paso de la suite en su propia instrumentación; resultado según los códigos de am instrument.
run_step() {
    local method="$1"
    shift
    local log="$SUITE_OUT/$method.txt"
    "$ADB" -s "$ANDROID_SERIAL" logcat -c >/dev/null 2>&1 || true
    # adb shell une los argumentos en una sola línea: un valor vacío desaparecería y correría los demás
    # (am termina leyendo basura), así que serverId solo se pasa si existe.
    local server_id=()
    if [ -n "${ADELPOS_E2E_SERVER_ID:-}" ]; then
        server_id=(-e serverId "$ADELPOS_E2E_SERVER_ID")
    fi
    "$ADB" -s "$ANDROID_SERIAL" shell am instrument -w -r \
        -e class "$APP_ID.$TEST_CLASS#$method" \
        -e local "http://$DEVICE_HOST:$LOCAL_PORT" -e address "$DEVICE_HOST:$NETWORK_PORT" \
        -e net "https://$DEVICE_HOST:$NETWORK_PORT" -e pin "$PIN" \
        ${server_id[@]+"${server_id[@]}"} -e mainDevice "$MAIN_DEVICE" -e mainToken "$MAIN_TOKEN" "$@" \
        "$TEST_RUNNER" >"$log" 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V CatalogOnGoTest:V TestRunner:V AndroidRuntime:E AdelAvisos:V \
        >"$SUITE_OUT/$method-logcat.txt" 2>/dev/null || true
    local codes verdict
    # Sin ningún código (am no llegó a correr la prueba) grep no encuentra nada: «SIN RESULTADO».
    codes="$(grep -E '^INSTRUMENTATION_STATUS_CODE: ' "$log" | awk '{print $2}' | tr -d '\r' | tr '\n' ' ' || true)"
    case "$codes" in
        *"-2"*) verdict="FALLA" ;;
        *"-4"* | *"-3"*) verdict="OMITIDA" ;;
        "1 0 ") verdict="APROBADA" ;;
        *) verdict="SIN RESULTADO" ;;
    esac
    echo "$SUITE_NAME $method: $verdict" | tee -a "$SUMMARY"
    if [ "$verdict" != "APROBADA" ]; then
        FAILED=1
        grep -A12 'INSTRUMENTATION_STATUS: stack=' "$log" | head -14 | tee -a "$SUMMARY" || true
        grep -m3 -E '^(Error|onError):' "$log" | tee -a "$SUMMARY" || true
    fi
}

# Un paso que necesita la base temporal y no la tiene: se informa como omitido (no aprobado).
skip_step() {
    echo "$SUITE_NAME $1: OMITIDA ($2)" | tee -a "$SUMMARY"
    FAILED=1
}

# Android 8 a 10: el modo avión no existe como orden de shell; la red se corta desde la Mac con root.
host_network() {
    "$ADB" -s "$ANDROID_SERIAL" root >/dev/null 2>&1
    "$ADB" -s "$ANDROID_SERIAL" wait-for-device
    "$ADB" -s "$ANDROID_SERIAL" shell svc wifi "$1"
    "$ADB" -s "$ANDROID_SERIAL" shell svc data "$1"
    sleep 3
    "$ADB" -s "$ANDROID_SERIAL" shell dumpsys connectivity | grep -m1 'Active default network' | tee -a "$SUMMARY" || true
}

# Corta (off) o devuelve (on) la red del dispositivo desde la Mac, según su versión.
device_network() {
    if [ "$SDK" -lt 30 ]; then
        if [ "$1" = off ]; then
            host_network disable
        else
            host_network enable
            "$ADB" -s "$ANDROID_SERIAL" unroot >/dev/null 2>&1 || true
            "$ADB" -s "$ANDROID_SERIAL" wait-for-device
        fi
    else
        if [ "$1" = off ]; then
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode enable
        else
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode disable
        fi
        sleep 5
        "$ADB" -s "$ANDROID_SERIAL" shell dumpsys connectivity | grep -m1 'Active default network' | tee -a "$SUMMARY" || true
    fi
}

access_step() {
    case "$1" in
        1) run_step paso1_emparejarConElCodigo ;;
        2) run_step paso2_pinIncorrectoBloqueoALosCincoYEntrada ;;
        3)
            if [ "$SDK" -lt 30 ]; then
                host_network disable
                run_step paso3_entradaSinConexion -e hostNetworkCut true
                host_network enable
                "$ADB" -s "$ANDROID_SERIAL" unroot >/dev/null 2>&1 || true
                "$ADB" -s "$ANDROID_SERIAL" wait-for-device
            else
                run_step paso3_entradaSinConexion
            fi
            ;;
        4) run_step paso4_pinCambiadoPideVolverAEscribirloSinSalirDeMesas ;;
        5) run_step paso5_versionViejaPideActualizarLaAplicacion -e clientVersion 0.0.9 ;;
        6) run_step paso6_certificadoCambiadoNoConectaYPideEmparejarDeNuevo ;;
        7) run_step paso7_dispositivoRetiradoBorraTodo ;;
        *) fail "paso desconocido de la suite access: $1" ;;
    esac
}

catalog_step() {
    case "$1" in
        a) run_step a_catalogoCompletoCambiosAgotadoEImagen -e stage a ;;
        b) run_step b_unaBajaLlegaPorCambios -e stage b ;;
        c)
            # Go cambia la época sola cuando la base se restaura (otro OID u otro clúster) y no tiene API para
            # provocarlo: se cambia el identificador guardado, como lo haría una restauración. Solo en la base temporal.
            if database_allowed; then
                test_sql "UPDATE app.catalog_epoch SET epoch = gen_random_uuid()"
                run_step c_epocaDistintaReemplazaTodo -e stage c
            else
                skip_step c_epocaDistintaReemplazaTodo "sin ADELPOS_E2E_DATABASE_URL: la época solo cambia en la base"
            fi
            ;;
        *) fail "paso desconocido de la suite catalog: $1" ;;
    esac
}

waiter_step() {
    case "$1" in
        w1) run_step w1_prepararEmparejarYEntrar ;;
        w2) run_step w2_capturaEnLineaRotacionYEnvioACocina ;;
        w3) device_network off; run_step w3_capturaSinConexion; device_network on ;;
        w4) run_step w4_alVolverLaRedSeEnviaSinEntrarYSinDuplicar -e lostOfflineResponses 1 ;;
        w5) run_step w5_precioCambiadoYAgotadoConLaConfirmacionDelMesero ;;
        w6) run_step w6_mesaCobradaMientrasTanto ;;
        w7a) run_step w7a_borradorGuardadoEnGo ;;
        w7b) device_network off; run_step w7b_sinConexionSeAgregaYSeEnvia; device_network on ;;
        w7c) run_step w7c_elPedidoSustituyeElBorradorSinDuplicar ;;
        w8) run_step w8_cancelarYModificarLoEnviado ;;
        w9) run_step w9_guardadoSinRespuestaAntesDeEnviar -e lostDraftResponses 1 ;;
        w10) run_step w10_confirmacionAplicadaSinRespuestaNoDuplica -e lostConfirmResponses 2 ;;
        w11) run_step w11_descartarConConexionQuitaDeLaCuenta ;;
        w12a) run_step w12a_capturaGuardadaParaDescartar ;;
        w12b) device_network off; run_step w12b_descartarSinConexionQuedaPendiente; device_network on ;;
        w12c) run_step w12c_alVolverSeQuitaDeLaCuenta ;;
        w13a) run_step w13a_capturaEnLaCuentaQueSeCombinara ;;
        w13b) device_network off; run_step w13b_sinConexionSeEnviaALaCuenta; device_network on ;;
        w13c) run_step w13c_cuentaCombinadaSeEnviaALaCuentaEnQueSeCombino ;;
        *) fail "paso desconocido de la suite waiter: $1" ;;
    esac
}

# A1-D16: el tercer bloqueo seguido deja el PIN bloqueado hasta que lo desbloquee un administrador. Esperar dos
# bloqueos reales de 5 minutos es demasiado lento para cada emulador: se dejan en la base temporal los dos primeros
# bloqueos de la serie (ya vencidos, empezada hace 20 minutos) del usuario de prueba `beto.f`, como si hubieran ocurrido;
# el tercero lo provoca la prueba con cinco PIN incorrectos desde la aplicación.
advance_pin_series() {
    test_sql "INSERT INTO app.pin_attempts AS a (user_id, failures, window_start, locked_until, locks, lock_series_started_at, requires_unlock)
        SELECT id, 0, now(), NULL, 2, now() - interval '20 minutes', false FROM app.users WHERE username = 'beto.f'
        ON CONFLICT (user_id) DO UPDATE SET failures = 0, window_start = now(), locked_until = NULL, locks = 2,
            lock_series_started_at = now() - interval '20 minutes', requires_unlock = false"
}

field_step() {
    case "$1" in
        f1) run_step f1_prepararRestauranteConCocinaEmparejarYEntrar ;;
        f2) run_step f2_rendimiento ;;
        f3) run_step f3_preparacionYListoAlMomento ;;
        f4) run_step f4_dosMeserosConCapturaEnCurso ;;
        f5a) device_network off; run_step f5a_sinConexionLosPedidosDeBetoImpidenCambiar; device_network on ;;
        f5b) run_step f5b_cadaPedidoSaleConLaCredencialDeSuMesero ;;
        f6) run_step f6_transferirYCombinarConLaCuentaAbierta ;;
        f7)
            if database_allowed; then
                advance_pin_series
                run_step f7a_bloqueoLargoDelPin
                device_network off
                run_step f7b_bloqueoLargoCierraLaEntradaSinConexion
                device_network on
                run_step f7c_unAdministradorLoDesbloquea
            else
                skip_step f7 "sin ADELPOS_E2E_DATABASE_URL: la serie de bloqueos solo se adelanta en la base"
            fi
            ;;
        f8) run_step f8_muerteDeLaActividadEnCapturaYPersonalizador ;;
        f9) run_step f9_atrasYBloqueoPorInactividadAlVolver ;;
        f10)
            run_step f10_temaOscuroYBotones
            mkdir -p "$SUITE_OUT/screens"
            "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
                "$SUITE_OUT/screens" >/dev/null 2>&1 || echo "sin capturas que copiar" | tee -a "$SUMMARY"
            ;;
        *) fail "paso desconocido de la suite field: $1" ;;
    esac
}

# Etapa A2, Fase B: Para llevar en el dispositivo.
takeaway_step() {
    case "$1" in
        t1) run_step t1_prepararEmparejarYEntrar ;;
        t2) run_step t2_nuevoPedidoSinProductosNoDejaNada ;;
        t3) run_step t3_capturaYEnvioEnLinea ;;
        t4) device_network off; run_step t4_sinConexionNaceAlEnviarseYElClienteSeEditaSinConexion; device_network on ;;
        t5) run_step t5_alVolverLaRedSaleElPedidoYDespuesElCliente ;;
        t6) run_step t6_nombreYTelefonoDespuesEnLinea ;;
        t7) run_step t7_listaConPreparacionYPagoAlMomento ;;
        t8) run_step t8_entregaBloqueadaSinPagoYPermitidaPagado ;;
        *) fail "paso desconocido de la suite takeaway: $1" ;;
    esac
}

# Etapa A2, Fase D: aviso de platillos listos. «Listo» de cocina desde la Mac (por la escucha local, con la sesión del
# administrador de la terminal principal) para lo que la prueba no puede hacer desde el dispositivo: durante un corte
# de red (r4) o con el proceso de la aplicación muerto (r8).
main_session() {
    curl -fsS --max-time 10 -X POST "$LOCAL_URL/api/v1/auth/login" -H "Content-Type: application/json" \
        -H "X-Device-ID: $MAIN_DEVICE" -H "X-Device-Token: $MAIN_TOKEN" \
        -d '{"username":"admin","password":"admin"}' | json_field session_token
}

kitchen_ready() {
    local account="$1" session tasks operation
    session="$(main_session)" || return 1
    tasks="$(curl -fsS --max-time 10 "$LOCAL_URL/api/v1/kitchen/board?state=all&account_id=$account" \
        -H "X-Device-ID: $MAIN_DEVICE" -H "X-Device-Token: $MAIN_TOKEN" -H "Authorization: Bearer $session" |
        python3 -I -c 'import json, sys
tasks = json.load(sys.stdin)["tasks"]
print(json.dumps([{"id": t["id"], "revision": t["revision"]} for t in tasks if t["state"] not in ("cancelled", "ready")]))')" ||
        return 1
    [ "$tasks" != "[]" ] || return 1
    operation="$(uuidgen | tr '[:upper:]' '[:lower:]')"
    curl -fsS --max-time 10 -X POST "$LOCAL_URL/api/v1/kitchen/states" -H "Content-Type: application/json" \
        -H "X-Device-ID: $MAIN_DEVICE" -H "X-Device-Token: $MAIN_TOKEN" -H "Authorization: Bearer $session" \
        -d "{\"operation_id\":\"$operation\",\"state\":\"ready\",\"tasks\":$tasks}" >/dev/null
}

# La cuenta que la prueba anotó en su registro (AdelOnGo) con la marca [mark] («r4 listo para cortar cuenta=»…).
logged_account() {
    sed -n "s/.*$1\([0-9a-f-]*\).*/\1/p" "$2" 2>/dev/null | tail -1
}

# r4 (en segundo plano mientras corre la prueba): espera a que la prueba pida el corte, corta la red, marca «Listo»
# desde la Mac durante el corte, devuelve la red y se lo dice a la prueba con un archivo (adb llega al emulador sin
# red). En Android 8–10 la red se corta con root, que se toma antes de la prueba (reiniciar adbd la cortaría).
alerts_cut_during_ready() {
    local log="$SUITE_OUT/r4-guion.txt" account="" deadline=$((SECONDS + 180)) logcat="$SUITE_OUT/r4-logcat-vivo.txt"
    : >"$log"
    while [ -z "$account" ] && [ "$SECONDS" -lt "$deadline" ]; do
        sleep 2
        "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V >"$logcat" 2>/dev/null || true
        account="$(logged_account "r4 listo para cortar cuenta=" "$logcat")"
    done
    if [ -z "$account" ]; then
        echo "r4: la prueba no pidió el corte" >>"$log"
        return 1
    fi
    echo "r4: corte de red (cuenta $account)" >>"$log"
    if [ "$SDK" -lt 30 ]; then
        host_network disable >>"$log" 2>&1 || true
    else
        "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode enable >>"$log" 2>&1 || true
    fi
    sleep 6
    if kitchen_ready "$account" >>"$log" 2>&1; then
        echo "r4: «Listo» marcado desde la Mac durante el corte" >>"$log"
    else
        echo "r4: no se pudo marcar «Listo»" >>"$log"
    fi
    sleep 6
    if [ "$SDK" -lt 30 ]; then
        host_network enable >>"$log" 2>&1 || true
    else
        "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode disable >>"$log" 2>&1 || true
    fi
    "$ADB" -s "$ANDROID_SERIAL" shell touch /data/local/tmp/adelpos-r4-listo || true
    echo "r4: red devuelta" >>"$log"
}

# r8b: reinicio pegajoso tras matar el proceso (fuera de la instrumentación, que detiene la aplicación al terminar).
alerts_sticky_restart() {
    local account pid newpid deadline verdict="FALLA" reason="" service="$APP_ID/.ready.ReadyAlertsService"
    local log="$SUITE_OUT/r8b_reinicioPegajoso.txt"
    : >"$log"
    account="$(logged_account "r8 cuenta=" "$SUITE_OUT/r8a_servicioConUnaCuentaEnCocinaParaElReinicioPegajoso-logcat.txt")"
    if [ -z "$account" ]; then
        echo "$SUITE_NAME r8b_reinicioPegajoso: FALLA (r8a no dejó su cuenta)" | tee -a "$SUMMARY"
        FAILED=1
        return
    fi
    # La aplicación normal (ya emparejada con el Go temporal por su dirección: no busca nada); con la wifi apagada en
    # Android 11+ el emulador no ve el anuncio del servicio real y alcanza 10.0.2.2 por la red móvil simulada.
    if [ "$SDK" -ge 30 ]; then
        "$ADB" -s "$ANDROID_SERIAL" shell cmd wifi set-wifi-enabled disabled >>"$log" 2>&1 || true
    fi
    "$ADB" -s "$ANDROID_SERIAL" logcat -c >/dev/null 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" shell am start -W -n "$APP_ID/.MainActivity" >>"$log" 2>&1 || true
    deadline=$((SECONDS + 45))
    until "$ADB" -s "$ANDROID_SERIAL" shell dumpsys activity services "$service" | grep -q "isForeground=true"; do
        [ "$SECONDS" -lt "$deadline" ] || { reason="el servicio no arrancó al abrir la aplicación"; break; }
        sleep 2
    done
    if [ -z "$reason" ]; then
        "$ADB" -s "$ANDROID_SERIAL" shell input keyevent KEYCODE_HOME || true
        sleep 2
        # Con `set -e`, cada orden que puede fallar (pidof sin proceso, kill) lleva su `|| true`.
        pid="$("$ADB" -s "$ANDROID_SERIAL" shell pidof "$APP_ID" | tr -d '\r' || true)"
        echo "r8b: proceso $pid; kill -9" >>"$log"
        "$ADB" -s "$ANDROID_SERIAL" shell run-as "$APP_ID" kill -9 "$pid" >>"$log" 2>&1 || true
        deadline=$((SECONDS + 90))
        while :; do
            newpid="$("$ADB" -s "$ANDROID_SERIAL" shell pidof "$APP_ID" | tr -d '\r' || true)"
            if [ -n "$newpid" ] && [ "$newpid" != "$pid" ] &&
                "$ADB" -s "$ANDROID_SERIAL" shell dumpsys activity services "$service" | grep -q "isForeground=true"; then
                echo "r8b: reinicio pegajoso en el proceso $newpid" >>"$log"
                break
            fi
            [ "$SECONDS" -lt "$deadline" ] || { reason="Android no reinició el servicio"; break; }
            sleep 2
        done
    fi
    if [ -z "$reason" ]; then
        sleep 5
        kitchen_ready "$account" >>"$log" 2>&1 || reason="no se pudo marcar «Listo» desde la Mac"
    fi
    if [ -z "$reason" ]; then
        deadline=$((SECONDS + 60))
        until "$ADB" -s "$ANDROID_SERIAL" shell dumpsys notification --noredact |
            grep -q "android.title=String (Mesa R10 · 2 platillos listos)"; do
            [ "$SECONDS" -lt "$deadline" ] || { reason="el aviso no llegó tras el reinicio"; break; }
            sleep 2
        done
    fi
    [ -n "$reason" ] || verdict="APROBADA"
    "$ADB" -s "$ANDROID_SERIAL" shell dumpsys notification --noredact | grep -E "pkg=$APP_ID|android.title=" >>"$log" 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelAvisos:V AndroidRuntime:E >"$SUITE_OUT/r8b_reinicioPegajoso-logcat.txt" 2>/dev/null || true
    "$ADB" -s "$ANDROID_SERIAL" shell am force-stop "$APP_ID" >/dev/null 2>&1 || true
    if [ "$SDK" -ge 30 ]; then
        "$ADB" -s "$ANDROID_SERIAL" shell cmd wifi set-wifi-enabled enabled >>"$log" 2>&1 || true
    fi
    if [ "$verdict" = "APROBADA" ]; then
        echo "$SUITE_NAME r8b_reinicioPegajoso: APROBADA" | tee -a "$SUMMARY"
    else
        echo "$SUITE_NAME r8b_reinicioPegajoso: FALLA ($reason)" | tee -a "$SUMMARY"
        FAILED=1
    fi
}

# Etapa A2, Fase D: aviso de platillos listos (el aviso va encendido solo en esta suite: readyAlerts=on).
alerts_step() {
    case "$1" in
        r1) run_step r1_prepararEmparejarYEntrarArrancaElServicio -e readyAlerts on -e alertPermissions ask ;;
        r2) run_step r2_llegaConLaPantallaApagadaYLaAplicacionCerrada -e readyAlerts on ;;
        r3) run_step r3_noLlegaAOtroMeseroNiAOtroDispositivoYTransferirCambiaElDestinatario -e readyAlerts on ;;
        r4)
            if [ "$SDK" -lt 30 ]; then
                "$ADB" -s "$ANDROID_SERIAL" root >/dev/null 2>&1
                "$ADB" -s "$ANDROID_SERIAL" wait-for-device
            fi
            alerts_cut_during_ready &
            local helper=$!
            run_step r4_corteDeRedDuranteElListoLlegaUnaSolaVez -e readyAlerts on
            wait "$helper" || echo "r4: el guion no pudo cortar la red ($SUITE_OUT/r4-guion.txt)" | tee -a "$SUMMARY"
            if [ "$SDK" -lt 30 ]; then
                "$ADB" -s "$ANDROID_SERIAL" unroot >/dev/null 2>&1 || true
                "$ADB" -s "$ANDROID_SERIAL" wait-for-device
            else
                "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode disable >/dev/null 2>&1 || true
            fi
            ;;
        r5) run_step r5_agrupadoPorCuentaYResumenConVariasCuentas -e readyAlerts on ;;
        r6) run_step r6_enQuienEresAlTocarloPideElPinYAbreLaCuenta -e readyAlerts on ;;
        r7) run_step r7_sonidoYVibracionApagadosDesdeTerminalesYAvisoApagado -e readyAlerts on ;;
        r8)
            run_step r8a_servicioConUnaCuentaEnCocinaParaElReinicioPegajoso -e readyAlerts on
            alerts_sticky_restart
            ;;
        r9) run_step r9_alRetirarElDispositivoSeDetieneElServicio -e readyAlerts on ;;
        *) fail "paso desconocido de la suite alerts: $1" ;;
    esac
}

# Corre los pasos [steps] de [suite] contra el servidor ya preparado.
run_suite() {
    local suite="$1" steps="$2" install="$3"
    SUITE_NAME="$suite"
    TEST_CLASS="$(suite_class "$suite")"
    SUITE_OUT="$4"
    mkdir -p "$SUITE_OUT"
    {
        echo "android-e2e en $ANDROID_SERIAL (API $SDK), suite $suite, pasos: $steps"
        echo "servidor: $LOCAL_URL y $NETWORK_URL, huella $PIN"
        echo "terminal principal: $MAIN_DEVICE (ADELPOS_E2E_MAIN_DEVICE; el token está en $SUITE_OUT/main-token)"
    } | tee -a "$SUMMARY"
    (umask 077 && printf '%s' "$MAIN_TOKEN" >"$SUITE_OUT/main-token")
    if [ "$install" = 1 ]; then
        install_app
    fi
    local step
    for step in $steps; do
        case "$suite" in
            access) access_step "$step" ;;
            catalog) catalog_step "$step" ;;
            waiter) waiter_step "$step" ;;
            field) field_step "$step" ;;
            takeaway) takeaway_step "$step" ;;
            alerts) alerts_step "$step" ;;
        esac
    done
}

# --suite all: detiene el servidor anterior y levanta uno nuevo con la orden indicada; sus líneas VARIABLE=valor
# (solo ADELPOS_E2E_*) reemplazan a las anteriores. La terminal principal y la huella se vuelven a obtener.
new_server() {
    local output line name
    output="$(sh -c "$ADELPOS_E2E_NEW_SERVER")" || fail "ADELPOS_E2E_NEW_SERVER falló"
    unset ADELPOS_E2E_MAIN_DEVICE ADELPOS_E2E_MAIN_TOKEN ADELPOS_E2E_PIN ADELPOS_E2E_SERVER_ID
    while IFS= read -r line; do
        name="${line%%=*}"
        case "$name" in
            ADELPOS_E2E_LOCAL_URL | ADELPOS_E2E_NETWORK_URL | ADELPOS_E2E_PROVISIONING | ADELPOS_E2E_PROVISIONING_FILE | \
                ADELPOS_E2E_DATABASE_URL | ADELPOS_E2E_SERVER_ID)
                export "$name=${line#*=}"
                ;;
        esac
    done <<<"$output"
    prepare_server
}

SDK="$("$ADB" -s "$ANDROID_SERIAL" shell getprop ro.build.version.sdk | tr -d '\r')"
[[ "$SDK" =~ ^[0-9]+$ ]] || fail "no responde $ANDROID_SERIAL"
OUT="${OUT:-$PROJECT_ROOT/android/app/build/outputs/e2e/$ANDROID_SERIAL}"
mkdir -p "$OUT"
SUMMARY="$OUT/summary.txt"
: >"$SUMMARY"

if [ "$SUITE" = all ]; then
    new_server
    run_suite access "1 2 3 4 5" 1 "$OUT/access"
    new_server
    run_suite access "6 7" 0 "$OUT/access"
    for part in catalog waiter field takeaway alerts; do
        new_server
        run_suite "$part" "$(suite_steps "$part")" 1 "$OUT/$part"
    done
else
    case " $STEPS " in
        *" 6 "*)
            if [ "$SUITE" = access ]; then
                for earlier in 1 2 3 4 5; do
                    case " $STEPS " in
                        *" $earlier "*) fail "el paso 6 necesita otro servidor temporal: corre $earlier antes, por separado" ;;
                    esac
                done
            fi
            ;;
    esac
    prepare_server
    run_suite "$SUITE" "$STEPS" "$INSTALL" "$OUT"
fi

if [ "$FAILED" = 0 ]; then
    echo "android-e2e: todos los pasos aprobados ($OUT)" | tee -a "$SUMMARY"
else
    echo "android-e2e: hubo pasos sin aprobar ($OUT)" | tee -a "$SUMMARY"
fi
exit "$FAILED"
