#!/usr/bin/env bash
# Etapa A1 (fases C y B; para las fases F y G): pruebas de extremo a extremo de la aplicación Android contra un
# servidor Go TEMPORAL en modo de pruebas, nunca contra el servicio real de la Mac. Corre los pasos de
# AccessOnGoTest (android/app/src/androidTest) uno por instrumentación (`am instrument`), en orden y con la aplicación
# instalada una sola vez: los datos pasan de un paso al siguiente y el proceso se reinicia en cada uno.
#
# Uso (desde cualquier carpeta):
#   ANDROID_SERIAL=emulator-5580 \
#   ADELPOS_E2E_LOCAL_URL=http://127.0.0.1:8194 \
#   ADELPOS_E2E_NETWORK_URL=https://127.0.0.1:8454 \
#   ADELPOS_E2E_PROVISIONING_FILE=/ruta/al/provisioning-token \
#   ADELPOS_E2E_SERVER_ID=<server_id> \
#   scripts/android-e2e.sh [--steps "1 2 3 4 5 7"] [--no-install] [--out <carpeta>]
#
# Variables:
#   ANDROID_SERIAL                  obligatorio: el emulador o dispositivo (nunca se corre en todos los conectados).
#   ADELPOS_E2E_LOCAL_URL           escucha local HTTP del Go temporal vista desde la Mac (127.0.0.1 o localhost).
#   ADELPOS_E2E_NETWORK_URL         escucha de red HTTPS del Go temporal vista desde la Mac (127.0.0.1 o localhost).
#   ADELPOS_E2E_DEVICE_HOST         cómo llega el dispositivo a la Mac (10.0.2.2 en el emulador, por omisión).
#   ADELPOS_E2E_PIN                 huella SPKI esperada (hexadecimal); si falta, se toma del certificado servido.
#   ADELPOS_E2E_SERVER_ID           server_id esperado (opcional; el paso 1 comprueba el emparejado).
#   ADELPOS_E2E_PROVISIONING        token de aprovisionamiento, o ADELPOS_E2E_PROVISIONING_FILE con su ruta: si el
#                                   servidor no está configurado, se hace el asistente con él.
#   ADELPOS_E2E_MAIN_DEVICE y ADELPOS_E2E_MAIN_TOKEN
#                                   terminal principal de un servidor ya configurado (la imprime una corrida anterior).
#   JAVA_HOME, ANDROID_HOME         como en scripts/dev.sh (JDK de Android Studio y SDK del usuario por omisión).
#
# Pasos: 1 emparejar con el código · 2 PIN incorrecto, bloqueo al quinto (cierra la entrada sin conexión), entrada
# y cabeceras de cada mesero · 3 entrada sin conexión (modo avión en Android 11+; antes, `adb root` + `svc wifi/data`
# desde la Mac: solo imágenes google_apis) · 4 PIN cambiado mientras el mesero trabaja: «Vuelve a escribir tu PIN»
# sin salir de Mesas · 5 versión vieja (426) · 6 certificado cambiado · 7 dispositivo retirado.
# El paso 6 necesita OTRO servidor temporal (otra identidad) en las MISMAS direcciones: correr primero
# `--steps "1 2 3 4 5"`, reemplazar el servidor y después `--no-install --steps "6 7"` con los datos del nuevo.
#
# Seguridad: se niega a correr con los puertos del servicio real (8181, 8443), con un host que no sea la propia Mac
# o si `GET version` de alguna de las dos escuchas no responde `test_mode: true`. Las pruebas usan el tipo NSD
# `_adelpostest._tcp` (AdelTestRunner): nunca buscan ni prueban el servidor real del restaurante.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STEPS="1 2 3 4 5 7"
INSTALL=1
OUT=""

while [ $# -gt 0 ]; do
    case "$1" in
        --steps) STEPS="${2:?pasos}"; shift 2 ;;
        --no-install) INSTALL=0; shift ;;
        --out) OUT="${2:?carpeta}"; shift 2 ;;
        *) echo "Argumento no reconocido: $1" >&2; exit 2 ;;
    esac
done

fail() {
    echo "android-e2e: $*" >&2
    exit 2
}

: "${ANDROID_SERIAL:?Falta ANDROID_SERIAL (el emulador o dispositivo donde correr)}"
LOCAL_URL="${ADELPOS_E2E_LOCAL_URL:?Falta ADELPOS_E2E_LOCAL_URL}"
NETWORK_URL="${ADELPOS_E2E_NETWORK_URL:?Falta ADELPOS_E2E_NETWORK_URL}"
DEVICE_HOST="${ADELPOS_E2E_DEVICE_HOST:-10.0.2.2}"
export ANDROID_SERIAL

if [ -z "${JAVA_HOME:-}" ]; then
    export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
fi
if [ -z "${ANDROID_HOME:-}" ]; then
    export ANDROID_HOME="$HOME/Library/Android/sdk"
fi
ADB="$ANDROID_HOME/platform-tools/adb"

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

LOCAL_PORT="$(check_url "$LOCAL_URL" http)"
NETWORK_PORT="$(check_url "$NETWORK_URL" https)"
LOCAL_URL="${LOCAL_URL%/}"
NETWORK_URL="${NETWORK_URL%/}"

case " $STEPS " in
    *" 6 "*)
        for earlier in 1 2 3 4 5; do
            case " $STEPS " in
                *" $earlier "*) fail "el paso 6 necesita otro servidor temporal: corre $earlier antes, por separado" ;;
            esac
        done
        ;;
esac

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
curl -fsS --max-time 5 "$LOCAL_URL/api/v1/version" 2>/dev/null | test_mode ||
    fail "$LOCAL_URL no responde test_mode: true (no es un servidor temporal de pruebas)"
# --insecure solo para esta consulta sin credenciales: la confianza la da la huella, que se compara abajo.
curl -fsS --max-time 5 --insecure "$NETWORK_URL/api/v1/version" 2>/dev/null | test_mode ||
    fail "$NETWORK_URL no responde test_mode: true (no es un servidor temporal de pruebas)"

SERVED_PIN="$(openssl s_client -connect "127.0.0.1:$NETWORK_PORT" </dev/null 2>/dev/null |
    openssl x509 -pubkey -noout | openssl pkey -pubin -outform DER | openssl dgst -sha256 -r | cut -d' ' -f1)"
PIN="${ADELPOS_E2E_PIN:-$SERVED_PIN}"
if [ "$PIN" != "$SERVED_PIN" ]; then
    fail "la huella de $NETWORK_URL ($SERVED_PIN) no es ADELPOS_E2E_PIN ($PIN)"
fi

# Terminal principal: la indicada o la del asistente de configuración (servidor sin configurar).
MAIN_DEVICE="${ADELPOS_E2E_MAIN_DEVICE:-}"
MAIN_TOKEN="${ADELPOS_E2E_MAIN_TOKEN:-}"
if [ -z "$MAIN_DEVICE" ] || [ -z "$MAIN_TOKEN" ]; then
    PROVISIONING="${ADELPOS_E2E_PROVISIONING:-}"
    if [ -z "$PROVISIONING" ] && [ -n "${ADELPOS_E2E_PROVISIONING_FILE:-}" ]; then
        PROVISIONING="$(cat "$ADELPOS_E2E_PROVISIONING_FILE")"
    fi
    [ -n "$PROVISIONING" ] ||
        fail "sin terminal principal: indica ADELPOS_E2E_MAIN_DEVICE/TOKEN o el token de aprovisionamiento"
    SETUP="$(curl -fsS --max-time 30 -X POST "$LOCAL_URL/api/v1/setup" -H "Content-Type: application/json" \
        -H "X-Provisioning-Token: $PROVISIONING" \
        -d '{"name":"Restaurante de prueba A1","timezone":"America/Monterrey","currency":"MXN","theme":"light","terminal_name":"Principal"}')" ||
        fail "el asistente de configuración no respondió (¿el servidor ya estaba configurado?)"
    MAIN_DEVICE="$(printf '%s' "$SETUP" | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["device_id"])')"
    MAIN_TOKEN="$(printf '%s' "$SETUP" | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["device_token"])')"
fi

OUT="${OUT:-$PROJECT_ROOT/android/app/build/outputs/e2e/$ANDROID_SERIAL}"
mkdir -p "$OUT"
SUMMARY="$OUT/summary.txt"
SDK="$("$ADB" -s "$ANDROID_SERIAL" shell getprop ro.build.version.sdk | tr -d '\r')"
{
    echo "android-e2e en $ANDROID_SERIAL (API $SDK), pasos: $STEPS"
    echo "servidor: $LOCAL_URL y $NETWORK_URL, huella $PIN"
    echo "terminal principal: $MAIN_DEVICE (ADELPOS_E2E_MAIN_DEVICE; el token está en $OUT/main-token)"
} | tee "$SUMMARY"
(umask 077 && printf '%s' "$MAIN_TOKEN" >"$OUT/main-token")

if [ "$INSTALL" = 1 ]; then
    "$ADB" -s "$ANDROID_SERIAL" uninstall local.adel.pos.android >/dev/null 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" uninstall local.adel.pos.android.test >/dev/null 2>&1 || true
    if [ ! -f "$PROJECT_ROOT/android/local.properties" ]; then
        echo "sdk.dir=$ANDROID_HOME" >"$PROJECT_ROOT/android/local.properties"
    fi
    (cd "$PROJECT_ROOT/android" && ./gradlew --console=plain -q :app:installDebug :app:installDebugAndroidTest) \
        >"$OUT/install.log" 2>&1 || {
        tail -20 "$OUT/install.log" >&2
        fail "no se pudo instalar la aplicación ni sus pruebas"
    }
fi

FAILED=0

# Un paso de AccessOnGoTest en su propia instrumentación; resultado según los códigos de am instrument.
run_step() {
    local method="$1"
    shift
    local log="$OUT/$method.txt"
    "$ADB" -s "$ANDROID_SERIAL" logcat -c >/dev/null 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" shell am instrument -w -r \
        -e class "local.adel.pos.android.AccessOnGoTest#$method" \
        -e local "http://$DEVICE_HOST:$LOCAL_PORT" -e address "$DEVICE_HOST:$NETWORK_PORT" -e pin "$PIN" \
        -e serverId "${ADELPOS_E2E_SERVER_ID:-}" -e mainDevice "$MAIN_DEVICE" -e mainToken "$MAIN_TOKEN" "$@" \
        local.adel.pos.android.test/local.adel.pos.android.AdelTestRunner >"$log" 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V TestRunner:V AndroidRuntime:E \
        >"$OUT/$method-logcat.txt" 2>/dev/null || true
    local codes verdict
    codes="$(grep -E '^INSTRUMENTATION_STATUS_CODE: ' "$log" | awk '{print $2}' | tr -d '\r' | tr '\n' ' ')"
    case "$codes" in
        *"-2"*) verdict="FALLA" ;;
        *"-4"* | *"-3"*) verdict="OMITIDA" ;;
        "1 0 ") verdict="APROBADA" ;;
        *) verdict="SIN RESULTADO" ;;
    esac
    echo "$method: $verdict" | tee -a "$SUMMARY"
    if [ "$verdict" != "APROBADA" ]; then
        FAILED=1
        grep -A12 'INSTRUMENTATION_STATUS: stack=' "$log" | head -14 | tee -a "$SUMMARY" || true
    fi
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

for step in $STEPS; do
    case "$step" in
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
        *) fail "paso desconocido: $step" ;;
    esac
done

if [ "$FAILED" = 0 ]; then
    echo "android-e2e: todos los pasos aprobados ($OUT)" | tee -a "$SUMMARY"
else
    echo "android-e2e: hubo pasos sin aprobar ($OUT)" | tee -a "$SUMMARY"
fi
exit "$FAILED"
