#!/usr/bin/env bash
# Etapas A1 (fases C, B, D, E y F), A2 y A3: pruebas de extremo a extremo de la aplicación Android contra un servidor Go
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
#   scripts/android-e2e.sh [--suite access|catalog|waiter|field|takeaway|precheck|alerts|adjustments|license|unions|legacy|connection|kitchen|endtoend|all] \
#       [--steps "…"] \
#       [--no-install] [--out <carpeta>]
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
#   ADELPOS_E2E_CONNECTION_MINUTES  solo con --suite connection: cuántos minutos sin uso (10 por omisión; la
#                                   Fase E de la A3 corre 60).
#   ADELPOS_E2E_NEW_SERVER          solo con --suite all: orden que detiene el servidor temporal anterior y levanta
#                                   uno NUEVO (otra identidad y base vacía) en las MISMAS direcciones e imprime líneas
#                                   VARIABLE=valor con las ADELPOS_E2E_* que cambian (provisioning, base, reloj…).
#   ADELPOS_E2E_LICENSE_FIXTURES    etapa L1: el vectors.json del contrato de licencias con que se firman las licencias
#                                   de prueba (por omisión, contracts/licencias/v1/fixtures/vectors.json de este árbol).
#                                   El servidor temporal debe confiar en esas llaves (ADEL_TEST_LICENSE_TRUST).
#   ADELPOS_E2E_GO                  etapa L1: el `go` con que se compila backend/cmd/license-test (por omisión, el del
#                                   PATH).
#   ADELPOS_E2E_CLOCK_FILE          etapa L1, solo la suite license: el archivo del desfase del reloj del servidor
#                                   temporal (su ADEL_TEST_CLOCK_FILE). Sin él, los pasos de la hora salen como OMITIDA.
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
# y pagado sale de la lista · t9 en horizontal la navegación va en un riel lateral y la captura muestra los productos
# (capturas de pantalla de las dos orientaciones en <salida>/screens).
#
# Suite `precheck` (etapa A2, Fase C, PrecheckOnGoTest, con un servidor temporal nuevo; la prueba hace de principal y
# del agente de impresión de una terminal de prueba «Caja 2»): p1 restaurante, la terminal con su impresora USB y una
# impresora de red que no responde, emparejar, entrar y enviar una cuenta (sin `payments.tickets.print`, el botón
# deshabilitado con su motivo) · p2 sin impresora asignada (no se encola nada) · p3 la USB de «Caja 2» desconectada
# y, al volver, los bytes ESC/POS de la precuenta (título, «Atendió:», folio, mesa y partidas) · p4 el nombre visible
# (Ticket de venta, Cuenta) en el botón y en el impreso · p5 un pedido para llevar con su folio · p6 (SIN red) el
# botón lo dice · p7 la impresora de red no responde y «Reintentar» sin motivo. La impresora de red usa el puerto 9106
# de la Mac: nada debe escuchar ahí.
#
# La red la corta y la devuelve este guion entre pasos (modo avión desde la Mac en Android 11+; `adb root` + `svc
# wifi/data` en Android 8–10). Si no se puede cortar (por ejemplo, `adb root` no responde tras tres intentos), el
# guion lo dice y ese paso sale como FALLA; nunca termina en silencio ni corre «sin red» con red.
#
# Suite `alerts` (etapa A2, Fase D, AlertsOnGoTest con `readyAlerts=on`, con un servidor temporal nuevo): r1 preparar
# (Ana y Beto, cocina en pantalla, mesas), emparejar y entrar: arranca el servicio con «AD-EL POS conectado» y la
# explicación de los permisos · r2 aviso con la aplicación cerrada (tarea quitada de recientes), la pantalla apagada y
# Doze forzado · r3 no llega a otro mesero ni a otro dispositivo (un segundo android emparejado por la API) y
# transferir la cuenta cambia el destinatario · r4 este guion corta la red, marca «Listo» desde la Mac durante el
# corte y la devuelve (en Android 8–10, cuando el canal ya vio el corte): llega una sola vez · r5 agrupado por cuenta y resumen · r6 en «Selecciona tu usuario», al tocarlo pide
# el PIN y abre la cuenta; con la cuenta a la vista, aviso dentro de la aplicación · r7 sonido y vibración apagados
# desde Terminales y aviso apagado que detiene el servicio · r8 (r8a + r8b) este guion abre la aplicación (con la
# wifi del emulador apagada en Android 11+: el emulador alcanza 10.0.2.2 por la red móvil y no ve el anuncio real),
# mata el proceso, espera el reinicio pegajoso, marca «Listo» desde la Mac y busca el aviso con `dumpsys
# notification --noredact` · r10 (antes de r9) en «Selecciona tu usuario» la aplicación no usa la sesión del último mesero
# (Go no renueva su `last_seen_at` en 5,5 minutos con la pantalla encendida y 2 apagada) · r9 al retirar el
# dispositivo el servicio se detiene. «Listo» desde la Mac usa la sesión
# del administrador de la terminal principal por la escucha local (127.0.0.1).
#
# Suite `adjustments` (etapa A3, §4, AdjustmentsOnGoTest, con un servidor temporal nuevo; en el teléfono y en la
# tableta): a1 restaurante (dos áreas, mesas, catálogo sin imágenes y una cuenta vacía), emparejar y entrar · a2 Mesas
# compacta en vertical y horizontal (una fila con «Lista | Mapa», áreas y «Pendientes»; el mapa llena el espacio; tocar
# la mesa abre su panel; cuentas en dos renglones) · a3 «Imprimir precuenta» y «Descartar cuenta vacía» desde el panel
# · a4 la captura (título de la mesa, «Buscar producto», tarjetas sin imagen, «Ver la cuenta · 1 por enviar», la flecha
# con su aviso y «Pendientes») · a5 la entrada («Ingresar», logotipo, «Selecciona tu usuario») · a6 la pantalla inicial
# Para llevar desde Terminales · a7 el teclado del PIN completo en vertical y horizontal (la entrada y «Vuelve a
# escribir tu PIN»). Capturas de pantalla de cada paso en <salida>/screens.
#
# Suite `connection` (etapa A3, §2 y §6, ConnectionOnGoTest con `readyAlerts=on`, con un servidor temporal nuevo; NO
# entra en `all` porque dura): c1 empareja, entra un mesero y deja el dispositivo sin uso durante
# ADELPOS_E2E_CONNECTION_MINUTES minutos con el bloqueo por inactividad de fábrica (Mesas, «Selecciona tu usuario» a los 2
# minutos y el mesero vuelve a entrar 45 s después, una y otra vez). Exige cero: ningún segundo con «Sin conexión» a
# la vista (barra, «Selecciona tu usuario», Mesas, notificación del canal de avisos), ninguna petición sin respuesta y ninguna
# lectura repetida por una conexión muerta. El registro de cada cambio queda en <salida>/c1_…-logcat.txt
# (etiquetas AdelConexion y AdelLink). Con la Mac sin carga y sin Gradle compilando.
#
# Suite `license` (etapa L1, §8, LicenseOnGoTest, con un servidor temporal nuevo que confía en las llaves de
# vectors.json y, para l5 y l6, con ADELPOS_E2E_CLOCK_FILE; el guion importa cada licencia de prueba y mueve el reloj
# del servidor entre pasos, nunca el de la Mac ni el del emulador): l1 (licencia de 0 comanderos) restaurante,
# emparejar y entrar; capturar dice «La licencia no incluye comanderos (0 de 0)…» y lo enviado queda en «Requiere tu
# revisión» · l2 (SIN red) un pedido «Por enviar» · l3 (con red) se envía solo, Go lo rechaza por cupo y queda en
# revisión con su motivo, sin reintentos · l4 (licencia con cupos) «Enviar de nuevo» con la misma clave: entra una vez ·
# l5 (reloj del servidor 2 h atrás) el aviso fijo de la hora en todas las pantallas y el pedido rechazado con su texto ·
# l6 (reloj corregido) el aviso se quita y el pedido se envía de nuevo · l7 (licencia en tolerancia) el aviso discreto
# «Licencia: quedan 3 días» en la barra · l8 (licencia vencida) «La licencia del sistema no permite capturar pedidos».
# Capturas de pantalla de cada paso en <salida>/screens.
#
# Suite `unions` (etapa M1, Fase C, UnionsOnGoTest, con un servidor temporal nuevo; en las tabletas): u1 restaurante
# (Salón M1 con las mesas 5 a 10, Terraza M1, hamburguesa con ruta a cocina y refresco sin ruta), emparejar y entrar ·
# u2 una sola tarjeta por cuenta con «Tu cuenta» (§5.5) y el tiempo que tarda el panel en dejar «$0.00 · Borrador»
# después de «Enviar a cocina» (§5.6, registro AdelOnGo «M1 · retraso») · u3 (SIN red) el pedido sin conexión es su
# propia tarjeta «Por enviar» · u4 (con red) sale y su tarjeta pasa a ser la de la cuenta · uniones (con
# table-unions-v1): u5 «Unir con…» desde una mesa ocupada, el panel, la lista y el contorno · u6 la unión que hace la
# principal se ve y «Quitar mesa» la deshace · u7 la unión en pantalla nace con el primer producto · u8 (SIN red) una
# unión en pantalla nunca va a la cola (M1-D13) · u9 (con red) el pedido va solo a la mesa y el atajo del grupo con una
# sección ocupada · u10 un producto sin preparación sin etiqueta de cocina (§5.2) · u11 la unión cambia en otra terminal
# mientras se captura (M1-D32, TABLE_UNION_CONFLICT) · u12 el título completo (M1-D36, para el teléfono). Capturas en
# <salida>/screens.
#
# Licencia (etapa L1): desde la Fase C, un servidor sin licencia no empareja ni deja capturar ni administrar. Por eso,
# en un solo lugar (prepare_server → ensure_test_license), si el servidor temporal dice «Sin activar», el guion le
# importa una licencia de prueba perpetua con cupos de sobra (premium: 5 cajas, 10 comanderos y 5 pantallas de
# cocina), firmada con las llaves de prueba de vectors.json por backend/cmd/license-test para la identidad de ESE
# servidor (la lee de su solicitud sin internet). Nunca habla con un servidor de licencias.
#
# Suite `all`: con ADELPOS_E2E_NEW_SERVER, un servidor temporal nuevo para cada parte: access 1–5, access 6–7 (otra
# identidad, sin reinstalar), catalog, waiter, field, takeaway, precheck, alerts, adjustments y license. Resultados en
# <salida>/<suite>.
# La suite `connection` se corre aparte.
#
# Suite `kitchen` (etapa K1, KitchenOnGoTest de la APLICACIÓN DE COCINA, com.adelpos.cocina, con su CocinaTestRunner y el
# mismo tipo NSD de pruebas; con un servidor temporal nuevo): k1 emparejar la tableta como pantalla de cocina con el
# código (la prueba hace de computadora principal por la escucha local) · k2 conectar (`kitchen-display/connect`, el
# cupo de cocina) y la pantalla de pedidos vacía, «No hay pedidos» · k3 pedidos reales creados en Go (mesa con un
# modificador y una nota, para llevar, un agregado y una cancelación) que aparecen sin tocar nada · k4 marcar un
# platillo, el cierre automático y el botón del pedido; Go lo registra y deja el aviso al mesero (`ready_notices`, en
# la base temporal: con ADELPOS_E2E_DATABASE_URL; sin ella, ese paso queda omitido) · k5 «Recientes» y «Recuperar»
# (el aviso del platillo recuperado se quita) · k6 el tamaño de letra y el tema de `PATCH devices/{id}` llegan al
# momento · Fase D (§5): k7 cortes de 1 a 5 s del Go temporal (`kill -STOP`/`-CONT` solo a su PID, el que escucha en
# los dos puertos de la prueba) sin ningún aviso · k8 una marca durante un corte de 18 s se repite sola y Go la registra
# una vez · k9 un corte de 45 s muestra el aviso, no deja marcar y se recupera solo · k10 cortes de red de 1, 3 y 5 s
# y uno de 35 s (modo avión en Android 11+; wifi y datos con root antes), con lo que tarda Android en volver a tener
# red y el aviso solo cuando pasa del margen de 10 s (K1-D33) · k11 los ajustes con la contraseña del administrador ·
# k13 «Volver a emparejar» suelta el cupo. Se corre aparte (no entra en `all`, que es de la aplicación de meseros).
#
# Suite `endtoend` (etapa K1, §8): la prueba de punta a punta con las dos aplicaciones en el mismo dispositivo y un
# servidor temporal nuevo: m1 (meseros, AlertsOnGoTest r1) prepara el restaurante y entra Ana con su servicio de
# avisos · c1 (cocina, k1) empareja la pantalla de cocina · m2 Ana envía a cocina desde la aplicación de meseros y
# espera el aviso mientras el guion corre la pantalla de cocina (k12), que marca listo el pedido.
#
# Seguridad: se niega a correr con los puertos del servicio real (8181, 8443), con un host que no sea la propia Mac
# o si `GET version` de alguna de las dos escuchas no responde `test_mode: true`; la base solo con las guardas de
# arriba. Las pruebas usan el tipo NSD `_adelpostest._tcp` (AdelTestRunner): nunca buscan ni prueban el servidor real
# del restaurante.
#
# Aplicación: `com.adelpos.meseros` (A2-D5) con su paquete de pruebas `com.adelpos.meseros.test`. Antes de instalar
# se desinstalan también los de la 0.1.0 (`local.adel.pos.android` y su `.test`): para Android son otra aplicación y
# no deben quedar en el emulador junto a la nueva. La suite `kitchen` usa la aplicación de cocina (`com.adelpos.cocina`,
# proyecto Gradle cocina/, etapa K1) con su paquete de pruebas `com.adelpos.cocina.test`.
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
        precheck) echo "PrecheckOnGoTest" ;;
        alerts) echo "AlertsOnGoTest" ;;
        adjustments) echo "AdjustmentsOnGoTest" ;;
        license) echo "LicenseOnGoTest" ;;
        unions) echo "UnionsOnGoTest" ;;
        legacy) echo "LegacyUnionOnGoTest" ;;
        connection) echo "ConnectionOnGoTest" ;;
        kitchen) echo "KitchenOnGoTest" ;;
        endtoend) echo "AlertsOnGoTest" ;;
        *) fail "suite desconocida: $1 (access, catalog, waiter, field, takeaway, precheck, alerts, adjustments, license, unions, legacy, connection, kitchen, endtoend o all)" ;;
    esac
}

suite_steps() {
    case "$1" in
        access) echo "1 2 3 4 5 7" ;;
        catalog) echo "a b c" ;;
        waiter) echo "w1 w2 w3 w4 w5 w6 w7a w7b w7c w8 w9 w10 w11 w12a w12b w12c w13a w13b w13c" ;;
        field) echo "f1 f2 f3 f4 f5a f5b f6 f7 f8 f9 f10" ;;
        takeaway) echo "t1 t2 t3 t4 t5 t6 t7 t8 t9" ;;
        precheck) echo "p1 p2 p3 p4 p5 p6 p7" ;;
        alerts) echo "r1 r2 r3 r4 r5 r6 r7 r8 r10 r9" ;;
        adjustments) echo "a1 a2 a3 a4 a5 a6 a7" ;;
        license) echo "l1 l2 l3 l4 l5 l6 l7 l8" ;;
        unions) echo "u1 u2 u3 u4 u5 u6 u7 u8 u9 u10 u11 u12" ;;
        legacy) echo "v1 v2" ;;
        connection) echo "c1" ;;
        kitchen) echo "k1 k2 k3 k4 k5 k6 k7 k8 k9 k10 k11 k13" ;;
        endtoend) echo "m1 c1 m2" ;;
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
# Proyecto Gradle de la aplicación que se instala (etapa K1: android para meseros, cocina para la de cocina).
GRADLE_DIR="android"

# Etapa K1: la suite `kitchen` es de la aplicación de cocina, con su ejecutor (el mismo tipo NSD de pruebas). No hay una
# versión anterior de cocina que desinstalar.
select_app() {
    if [ "$1" = kitchen ]; then
        APP_ID="com.adelpos.cocina"
        TEST_PACKAGE="$APP_ID.test"
        TEST_RUNNER="$TEST_PACKAGE/$APP_ID.CocinaTestRunner"
        LEGACY_APP_ID=""
        GRADLE_DIR="cocina"
    else
        APP_ID="com.adelpos.meseros"
        TEST_PACKAGE="$APP_ID.test"
        TEST_RUNNER="$TEST_PACKAGE/$APP_ID.AdelTestRunner"
        LEGACY_APP_ID="local.adel.pos.android"
        GRADLE_DIR="android"
    fi
}

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
    # Etapa L1: sin licencia, Go no empareja ni deja capturar ni administrar.
    ensure_test_license
}

# --- Licencia de prueba (etapa L1) ---

LICENSE_FIXTURES="${ADELPOS_E2E_LICENSE_FIXTURES:-$PROJECT_ROOT/contracts/licencias/v1/fixtures/vectors.json}"
LICENSE_TOOL=""

# Sesión del administrador de la terminal principal por la escucha local (la del asistente de configuración).
admin_session() {
    curl -fsS --max-time 10 -X POST "$LOCAL_URL/api/v1/auth/login" -H "Content-Type: application/json" \
        -H "X-Device-ID: $MAIN_DEVICE" -H "X-Device-Token: $MAIN_TOKEN" \
        -d '{"username":"admin","password":"admin"}' | json_field session_token
}

# Una petición a la escucha local con la sesión [session]: deja el estado HTTP en LOCAL_STATUS y la respuesta en
# LOCAL_BODY (sin subcapa, para que lleguen a quien llama). Distinto de cero si no hubo respuesta.
local_call() {
    local session="$1" method="$2" path="$3" body="${4:-}" answer
    local data=()
    if [ -n "$body" ]; then
        data=(-H "Content-Type: application/json" -d "$body")
    fi
    LOCAL_STATUS=""
    LOCAL_BODY=""
    answer="$(curl -sS --max-time 30 -X "$method" "$LOCAL_URL/api/v1/$path" -H "X-Device-ID: $MAIN_DEVICE" \
        -H "X-Device-Token: $MAIN_TOKEN" -H "Authorization: Bearer $session" ${data[@]+"${data[@]}"} \
        -w $'\n%{http_code}')" || return 1
    LOCAL_BODY="${answer%$'\n'*}"
    LOCAL_STATUS="${answer##*$'\n'}"
}

# backend/cmd/license-test compilado una vez por corrida en LICENSE_TOOL (solo firma con las llaves de prueba de
# vectors.json; nunca llega a un instalador).
ensure_license_tool() {
    if [ -n "$LICENSE_TOOL" ]; then
        return 0
    fi
    [ -f "$LICENSE_FIXTURES" ] || fail "no existe $LICENSE_FIXTURES (ADELPOS_E2E_LICENSE_FIXTURES)"
    (cd "$PROJECT_ROOT/backend" && "${ADELPOS_E2E_GO:-go}" build -o "$OUT/license-test" ./cmd/license-test) \
        >"$OUT/license-test-build.log" 2>&1 || {
        tail -5 "$OUT/license-test-build.log" >&2
        fail "no se pudo compilar backend/cmd/license-test (ADELPOS_E2E_GO)"
    }
    LICENSE_TOOL="$OUT/license-test"
}

# Importa en el servidor temporal una licencia de prueba para SU identidad, con las opciones de `license-test issue`
# (sin opciones: perpetua, premium). La identidad (server_id y llave pública) sale de su solicitud sin internet de
# activar; la licencia entra por `POST license/import`, como un archivo .lic. Devuelve distinto de cero si Go la
# rechaza (por ejemplo, porque el servidor no confía en las llaves de vectors.json).
import_test_license() {
    local session identity issued license
    session="$(admin_session)" || { echo "android-e2e: sin sesión del administrador para la licencia" >&2; return 1; }
    local_call "$session" POST license/offline-request '{"action":"activate"}' || return 1
    if [ "$LOCAL_STATUS" != 200 ]; then
        echo "android-e2e: license/offline-request respondió $LOCAL_STATUS: $LOCAL_BODY" >&2
        return 1
    fi
    identity="$(printf '%s' "$LOCAL_BODY" | python3 -I -c '
import base64, json, sys
content = json.loads(json.load(sys.stdin)["content"])
encoded = content["payload_b64u"]
payload = json.loads(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
print(payload["server_id"], payload["installation_public_key"])')" || return 1
    ensure_license_tool
    issued="$(ADEL_TEST_MODE=1 "$LICENSE_TOOL" issue --fixtures "$LICENSE_FIXTURES" --server-id "${identity% *}" \
        --public-key "${identity#* }" "$@")" || { echo "android-e2e: license-test issue falló: $issued" >&2; return 1; }
    license="$(printf '%s' "$issued" | json_field output)" || return 1
    local_call "$session" POST license/import \
        "$(python3 -I -c 'import json, sys; print(json.dumps({"license": sys.argv[1]}))' "$license")" || return 1
    if [ "$LOCAL_STATUS" != 200 ]; then
        echo "android-e2e: license/import respondió $LOCAL_STATUS: $LOCAL_BODY" >&2
        echo "android-e2e: ¿el servidor temporal arrancó con ADEL_TEST_LICENSE_TRUST=$LICENSE_FIXTURES?" >&2
        return 1
    fi
    echo "licencia de prueba importada (license-test issue $*): $(printf '%s' "$LOCAL_BODY" | python3 -I -c '
import json, sys
answer = json.load(sys.stdin)
limits = (answer.get("license") or {}).get("limits") or {}
print(answer.get("state"), "cajas", limits.get("cash_registers"), "comanderos", limits.get("order_takers"),
      "cocina", limits.get("kitchen_displays"))')" | tee -a "${SUMMARY:-/dev/null}"
}

# Un servidor «Sin activar» queda activado con una licencia de prueba con cupos de sobra (el único lugar donde las
# suites anteriores a la etapa L1 obtienen su licencia). Uno ya activado se deja como está.
ensure_test_license() {
    local session state
    session="$(admin_session)" || fail "sin sesión del administrador de la terminal principal"
    local_call "$session" GET license/status || fail "license/status no respondió"
    [ "$LOCAL_STATUS" = 200 ] || fail "license/status respondió $LOCAL_STATUS: $LOCAL_BODY"
    state="$(printf '%s' "$LOCAL_BODY" | json_field state)"
    case "$state" in
        not_activated | not_activated_with_data)
            import_test_license --plan premium ||
                fail "no se pudo activar el servidor temporal con una licencia de prueba"
            ;;
    esac
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
    local packages=("$APP_ID" "$TEST_PACKAGE")
    if [ -n "$LEGACY_APP_ID" ]; then
        packages+=("$LEGACY_APP_ID" "$LEGACY_APP_ID.test")
    fi
    for package in "${packages[@]}"; do
        "$ADB" -s "$ANDROID_SERIAL" uninstall "$package" >/dev/null 2>&1 || true
    done
    if [ ! -f "$PROJECT_ROOT/$GRADLE_DIR/local.properties" ]; then
        echo "sdk.dir=$ANDROID_HOME" >"$PROJECT_ROOT/$GRADLE_DIR/local.properties"
    fi
    (cd "$PROJECT_ROOT/$GRADLE_DIR" && ./gradlew --console=plain -q :app:installDebug :app:installDebugAndroidTest) \
        >"$SUITE_OUT/install.log" 2>&1 || {
        tail -20 "$SUITE_OUT/install.log" >&2
        fail "no se pudo instalar la aplicación ni sus pruebas"
    }
}

FAILED=0

# Tiempo límite de un paso (segundos; vacío: sin límite, como siempre). La suite connection lo pone según sus minutos.
STEP_TIMEOUT_SECONDS=""

# Un paso de la suite en su propia instrumentación; resultado según los códigos de am instrument.
run_step() {
    local method="$1"
    shift
    local log="$SUITE_OUT/$method.txt"
    local limit=()
    if [ -n "$STEP_TIMEOUT_SECONDS" ]; then
        limit=(perl -e 'alarm shift; exec @ARGV' "$STEP_TIMEOUT_SECONDS")
    fi
    "$ADB" -s "$ANDROID_SERIAL" logcat -c >/dev/null 2>&1 || true
    # adb shell une los argumentos en una sola línea: un valor vacío desaparecería y correría los demás
    # (am termina leyendo basura), así que serverId solo se pasa si existe.
    local server_id=()
    if [ -n "${ADELPOS_E2E_SERVER_ID:-}" ]; then
        server_id=(-e serverId "$ADELPOS_E2E_SERVER_ID")
    fi
    ${limit[@]+"${limit[@]}"} "$ADB" -s "$ANDROID_SERIAL" shell am instrument -w -r \
        -e class "$APP_ID.$TEST_CLASS#$method" \
        -e local "http://$DEVICE_HOST:$LOCAL_PORT" -e address "$DEVICE_HOST:$NETWORK_PORT" \
        -e net "https://$DEVICE_HOST:$NETWORK_PORT" -e pin "$PIN" \
        ${server_id[@]+"${server_id[@]}"} -e mainDevice "$MAIN_DEVICE" -e mainToken "$MAIN_TOKEN" "$@" \
        "$TEST_RUNNER" >"$log" 2>&1 || true
    "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V CatalogOnGoTest:V TestRunner:V AndroidRuntime:E AdelAvisos:V \
        AdelConexion:V AdelLink:V >"$SUITE_OUT/$method-logcat.txt" 2>/dev/null || true
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

# `adb root` (Android 8 a 10, imágenes google_apis) con hasta tres intentos: adbd se reinicia y, con la Mac cargada,
# puede no volver a la primera. Comprueba que de verdad sea root. Si no se puede, lo dice (nunca corta el guion en
# silencio) y devuelve un código distinto de cero: el paso que lo necesitaba falla con ese motivo.
take_root() {
    local attempt
    for attempt in 1 2 3; do
        if "$ADB" -s "$ANDROID_SERIAL" root >/dev/null 2>&1 &&
            "$ADB" -s "$ANDROID_SERIAL" wait-for-device &&
            [ "$("$ADB" -s "$ANDROID_SERIAL" shell id -u 2>/dev/null | tr -d '\r')" = "0" ]; then
            return 0
        fi
        sleep 3
    done
    echo "android-e2e: no se pudo tomar root en $ANDROID_SERIAL («adb root», 3 intentos);" \
        "sin root no se corta ni se devuelve la red en Android 8–10" | tee -a "$SUMMARY" >&2
    return 1
}

# Android 8 a 10: el modo avión no existe como orden de shell; la red se corta desde la Mac con root. Cada orden que
# falla se informa con un código distinto de cero (quien llama decide; nunca se corta el guion en silencio).
host_network() {
    take_root || return 1
    "$ADB" -s "$ANDROID_SERIAL" shell svc wifi "$1" || return 1
    "$ADB" -s "$ANDROID_SERIAL" shell svc data "$1" || return 1
    sleep 3
    "$ADB" -s "$ANDROID_SERIAL" shell dumpsys connectivity | grep -m1 'Active default network' | tee -a "$SUMMARY" || true
}

# Corta (off) o devuelve (on) la red del dispositivo desde la Mac, según su versión; código distinto de cero si no pudo.
device_network() {
    if [ "$SDK" -lt 30 ]; then
        if [ "$1" = off ]; then
            host_network disable || return 1
        else
            host_network enable || return 1
            "$ADB" -s "$ANDROID_SERIAL" unroot >/dev/null 2>&1 || true
            "$ADB" -s "$ANDROID_SERIAL" wait-for-device || true
        fi
    else
        if [ "$1" = off ]; then
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode enable || return 1
        else
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode disable || return 1
        fi
        sleep 5
        "$ADB" -s "$ANDROID_SERIAL" shell dumpsys connectivity | grep -m1 'Active default network' | tee -a "$SUMMARY" || true
    fi
}

# Un paso SIN red: la corta, corre el paso y la devuelve. Si no se pudo cortar, el paso falla con su motivo (nunca corre
# «sin red» con red ni el guion se corta en silencio) y se sigue con el siguiente; si no se pudo devolver, se dice.
offline_step() {
    local method="$1"
    shift
    if ! device_network off; then
        echo "$SUITE_NAME $method: FALLA (no se pudo cortar la red del dispositivo; ver el mensaje de arriba)" |
            tee -a "$SUMMARY"
        FAILED=1
        device_network on || true
        return 0
    fi
    run_step "$method" "$@"
    if ! device_network on; then
        echo "$SUITE_NAME $method: no se pudo devolver la red del dispositivo; los pasos siguientes pueden fallar" |
            tee -a "$SUMMARY"
        FAILED=1
    fi
}

access_step() {
    case "$1" in
        1) run_step paso1_emparejarConElCodigo ;;
        2) run_step paso2_pinIncorrectoBloqueoALosCincoYEntrada ;;
        3)
            if [ "$SDK" -lt 30 ]; then
                offline_step paso3_entradaSinConexion -e hostNetworkCut true
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
        w3) offline_step w3_capturaSinConexion ;;
        w4) run_step w4_alVolverLaRedSeEnviaSinEntrarYSinDuplicar -e lostOfflineResponses 1 ;;
        w5) run_step w5_precioCambiadoYAgotadoConLaConfirmacionDelMesero ;;
        w6) run_step w6_mesaCobradaMientrasTanto ;;
        w7a) run_step w7a_borradorGuardadoEnGo ;;
        w7b) offline_step w7b_sinConexionSeAgregaYSeEnvia ;;
        w7c) run_step w7c_elPedidoSustituyeElBorradorSinDuplicar ;;
        w8) run_step w8_cancelarYModificarLoEnviado ;;
        w9) run_step w9_guardadoSinRespuestaAntesDeEnviar -e lostDraftResponses 1 ;;
        w10) run_step w10_confirmacionAplicadaSinRespuestaNoDuplica -e lostConfirmResponses 2 ;;
        w11) run_step w11_descartarConConexionQuitaDeLaCuenta ;;
        w12a) run_step w12a_capturaGuardadaParaDescartar ;;
        w12b) offline_step w12b_descartarSinConexionQuedaPendiente ;;
        w12c) run_step w12c_alVolverSeQuitaDeLaCuenta ;;
        w13a) run_step w13a_capturaEnLaCuentaQueSeCombinara ;;
        w13b) offline_step w13b_sinConexionSeEnviaALaCuenta ;;
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
        f5a) offline_step f5a_sinConexionLosPedidosDeBetoImpidenCambiar ;;
        f5b) run_step f5b_cadaPedidoSaleConLaCredencialDeSuMesero ;;
        f6) run_step f6_transferirYCombinarConLaCuentaAbierta ;;
        f7)
            if database_allowed; then
                advance_pin_series
                run_step f7a_bloqueoLargoDelPin
                offline_step f7b_bloqueoLargoCierraLaEntradaSinConexion
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
        t4) offline_step t4_sinConexionNaceAlEnviarseYElClienteSeEditaSinConexion ;;
        t5) run_step t5_alVolverLaRedSaleElPedidoYDespuesElCliente ;;
        t6) run_step t6_nombreYTelefonoDespuesEnLinea ;;
        t7) run_step t7_listaConPreparacionYPagoAlMomento ;;
        t8) run_step t8_entregaBloqueadaSinPagoYPermitidaPagado ;;
        t9)
            run_step t9_navegacionLateralEnHorizontalYProductosALaVista
            mkdir -p "$SUITE_OUT/screens"
            "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
                "$SUITE_OUT/screens" >/dev/null 2>&1 || echo "sin capturas que copiar" | tee -a "$SUMMARY"
            ;;
        *) fail "paso desconocido de la suite takeaway: $1" ;;
    esac
}

# Etapa A2, Fase C: imprimir la precuenta desde el dispositivo.
precheck_step() {
    case "$1" in
        p1) run_step p1_prepararEnviarACocinaYSinPermisoElBotonDiceElMotivo ;;
        p2) run_step p2_sinImpresoraAsignadaNoSeImprime ;;
        p3) run_step p3_terminalDesconectadaYLosBytesEscPos ;;
        p4) run_step p4_elNombreVisibleEnElBotonYEnElImpreso ;;
        p5) run_step p5_unPedidoParaLlevarConSuFolio ;;
        p6) offline_step p6_sinConexionElBotonLoDice ;;
        p7) run_step p7_laImpresoraNoRespondeYReintentarSinMotivo ;;
        *) fail "paso desconocido de la suite precheck: $1" ;;
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
    if [ "$SDK" -lt 30 ]; then
        # Android 8–10 (Fase E de la A2): `svc … disable` no siempre corta la conexión ya abierta (el modo avión de 11+
        # sí), y entonces el canal de avisos solo la da por muerta tras 45 s sin un byte. El corte dura hasta que la
        # prueba anota que el canal lo vio (75 s como mucho desde el «Listo»): el «Listo» ocurre de verdad sin red y la
        # reconexión posterior es la que se prueba.
        local seen_deadline=$((SECONDS + 75))
        until grep -q "r4 corte visto por el canal" "$logcat" 2>/dev/null || [ "$SECONDS" -ge "$seen_deadline" ]; do
            sleep 2
            "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V >"$logcat" 2>/dev/null || true
        done
        if grep -q "r4 corte visto por el canal" "$logcat" 2>/dev/null; then
            echo "r4: el canal vio el corte" >>"$log"
        else
            echo "r4: el canal no vio el corte en 75 s; se devuelve la red" >>"$log"
        fi
    else
        sleep 6
    fi
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
        sleep 1
        if [ "$("$ADB" -s "$ANDROID_SERIAL" shell pidof "$APP_ID" | tr -d '\r' || true)" = "$pid" ]; then
            # Algunas imágenes no dejan a run-as matar el proceso (Android 13: «unknown pid»): con root
            # (imágenes google_apis), fuera de la instrumentación.
            echo "r8b: run-as no pudo; kill -9 con root" >>"$log"
            "$ADB" -s "$ANDROID_SERIAL" root >>"$log" 2>&1 || true
            "$ADB" -s "$ANDROID_SERIAL" wait-for-device
            "$ADB" -s "$ANDROID_SERIAL" shell kill -9 "$pid" >>"$log" 2>&1 || true
            "$ADB" -s "$ANDROID_SERIAL" unroot >>"$log" 2>&1 || true
            "$ADB" -s "$ANDROID_SERIAL" wait-for-device
        fi
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
            if [ "$SDK" -lt 30 ] && ! take_root; then
                echo "$SUITE_NAME r4_corteDeRedDuranteElListoLlegaUnaSolaVez: FALLA (sin root no se corta la red)" |
                    tee -a "$SUMMARY"
                FAILED=1
                return 0
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
        r10) run_step r10_enQuienEresNoSeUsaLaSesionDelUltimoMesero -e readyAlerts on ;;
        r8)
            run_step r8a_servicioConUnaCuentaEnCocinaParaElReinicioPegajoso -e readyAlerts on
            alerts_sticky_restart
            ;;
        r9) run_step r9_alRetirarElDispositivoSeDetieneElServicio -e readyAlerts on ;;
        *) fail "paso desconocido de la suite alerts: $1" ;;
    esac
}

# Etapa M1, Fase C: la tarjeta única de la cuenta, el retraso después de «Enviar a cocina» y las uniones de mesas.
unions_step() {
    case "$1" in
        u1) run_step u1_prepararEmparejarYEntrar ;;
        u2) run_step u2_unaTarjetaPorCuentaYElRetrasoDespuesDeEnviar ;;
        u3) offline_step u3_sinConexionElPedidoEsSuPropiaTarjeta ;;
        u4) run_step u4_alVolverLaRedLaTarjetaEsLaDeLaCuenta ;;
        u5) run_step u5_unirDesdeUnaMesaOcupadaContornoYUnaSolaEntrada ;;
        u6) run_step u6_laUnionDeOtraTerminalSeVeYQuitarMesaLaDeshace ;;
        u7) run_step u7_unionEnPantallaNaceConElPrimerProducto ;;
        u8) offline_step u8_sinConexionLaUnionEnPantallaNoVaALaCola ;;
        u9) run_step u9_alVolverLaRedVaSoloALaMesaYElAtajoDelGrupoLoDiceGo ;;
        u10) run_step u10_loQueNoNecesitaPreparacionNoLlevaEtiqueta ;;
        u11) run_step u11_laUnionCambiaEnOtraTerminalYSeConservaLoCapturado ;;
        u12) run_step u12_elTituloNoSeCorta ;;
        *) fail "paso desconocido de la suite unions: $1" ;;
    esac
    mkdir -p "$SUITE_OUT/screens"
    "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
        "$SUITE_OUT/screens" >/dev/null 2>&1 || true
}

# Etapa M1, Fase C: la 0.4.1 frente a una unión (v1, con el APK de pruebas de la 0.4.1 que lleva el paso) y la 0.5.0
# instalada encima (v2). Las dos con --no-install: el guion no instala, se instala cada versión con adb entre pasos.
legacy_step() {
    case "$1" in
        v1) run_step v1_laVersionAnteriorFrenteAUnaUnion ;;
        v2) run_step v2_alActualizarALa050LaUnionEsUnaSolaEntrada ;;
        *) fail "paso desconocido de la suite legacy: $1" ;;
    esac
    mkdir -p "$SUITE_OUT/screens"
    "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
        "$SUITE_OUT/screens" >/dev/null 2>&1 || true
}

# Etapa A3, §4: los ajustes de la aplicación en vertical y horizontal, con sus capturas de pantalla.
adjustments_step() {
    case "$1" in
        a1) run_step a1_prepararEmparejarYEntrar ;;
        a2) run_step a2_mesasCompactaEnLasDosOrientaciones ;;
        a3) run_step a3_precuentaYDescartarCuentaVaciaDesdeElPanel ;;
        a4) run_step a4_capturaConTituloBuscadorFlechaYPendientes ;;
        a5) run_step a5_entradaConLosTextosNuevos ;;
        a6) run_step a6_pantallaInicialParaLlevar ;;
        a7) run_step a7_tecladoDelPinCompletoEnVerticalYHorizontal ;;
        *) fail "paso desconocido de la suite adjustments: $1" ;;
    esac
    mkdir -p "$SUITE_OUT/screens"
    "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
        "$SUITE_OUT/screens" >/dev/null 2>&1 || echo "sin capturas que copiar" | tee -a "$SUMMARY"
}

# Etapa L1, §8: la licencia, los cupos y la hora del servidor temporal cambian desde aquí entre pasos (cada licencia de
# prueba con su license_id nuevo; el reloj con su archivo de desfase), nunca la hora de la Mac ni la del emulador.
server_clock() {
    [ -n "${ADELPOS_E2E_CLOCK_FILE:-}" ] || return 1
    printf '%s\n' "$1" >"$ADELPOS_E2E_CLOCK_FILE"
    echo "reloj del servidor temporal: «$1»" | tee -a "$SUMMARY"
}

license_import_step() {
    local method="$1"
    shift
    if import_test_license "$@"; then
        run_step "$method"
    else
        echo "$SUITE_NAME $method: FALLA (no se pudo importar la licencia de prueba: license-test issue $*)" |
            tee -a "$SUMMARY"
        FAILED=1
    fi
}

license_step() {
    case "$1" in
        l1) license_import_step l1_sinComanderosElMensajeYLaRevision --plan premium --order-takers 0 ;;
        l2) offline_step l2_pedidoSinConexionPorEnviar ;;
        l3) run_step l3_alVolverLaRedQuedaEnRevisionSinReintentos ;;
        l4) license_import_step l4_volvioElCupoYSeEnviaDeNuevoConLaMismaClave --plan premium ;;
        l5)
            if server_clock "-2h"; then
                run_step l5_horaIncorrectaAvisoFijoYRechazo
            else
                skip_step l5_horaIncorrectaAvisoFijoYRechazo "sin ADELPOS_E2E_CLOCK_FILE: el reloj del servidor no se mueve"
            fi
            ;;
        l6)
            if server_clock ""; then
                run_step l6_horaCorregidaSeQuitaElAvisoYSeEnvia
            else
                skip_step l6_horaCorregidaSeQuitaElAvisoYSeEnvia "sin ADELPOS_E2E_CLOCK_FILE: el reloj del servidor no se mueve"
            fi
            ;;
        l7) license_import_step l7_toleranciaAvisoDiscretoEnLaBarra --type subscription --expires-at -2d ;;
        l8) license_import_step l8_vencidaNoPermiteCapturar --type subscription --expires-at -6d ;;
        *) fail "paso desconocido de la suite license: $1" ;;
    esac
    mkdir -p "$SUITE_OUT/screens"
    "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
        "$SUITE_OUT/screens" >/dev/null 2>&1 || echo "sin capturas que copiar" | tee -a "$SUMMARY"
}

# Etapa A3: la conexión tolerante, minutos sin uso sin avisos falsos (con el aviso de platillos listos encendido, como
# en la tableta del restaurante). El registro del emulador se agranda para que quepa la corrida completa.
connection_step() {
    case "$1" in
        c1)
            local minutes="${ADELPOS_E2E_CONNECTION_MINUTES:-10}"
            [[ "$minutes" =~ ^[0-9]+$ ]] && [ "$minutes" -ge 1 ] ||
                fail "ADELPOS_E2E_CONNECTION_MINUTES debe ser un número de minutos (1 o más)"
            "$ADB" -s "$ANDROID_SERIAL" logcat -G 16M >/dev/null 2>&1 || true
            STEP_TIMEOUT_SECONDS=$((minutes * 60 + 900))
            run_step c1_sinUsoNingunAvisoFalsoDeSinConexion -e readyAlerts on -e minutes "$minutes"
            STEP_TIMEOUT_SECONDS=""
            grep -h 'resumen:' "$SUITE_OUT/c1_sinUsoNingunAvisoFalsoDeSinConexion-logcat.txt" 2>/dev/null |
                tail -1 | tee -a "$SUMMARY" || true
            ;;
        *) fail "paso desconocido de la suite connection: $1" ;;
    esac
}

# Etapa K1: los avisos al mesero que deja marcar en la pantalla de cocina (app.ready_notices, los mismos de la A2).
# Go no los da por su API a la principal: se cuentan en la base temporal, después de comprobar que es la de ESTE
# servidor. Las cuentas son las que la prueba anotó en su registro (k4).
ready_notices_of() {
    test_sql "SELECT 1" >/dev/null
    "$PSQL" "$ADELPOS_E2E_DATABASE_URL" -X -tAq -v ON_ERROR_STOP=1 \
        -c "SELECT count(*) FROM app.ready_notices WHERE account_id = '$1'"
}

# k4: marcar en la pantalla deja el aviso de las dos cuentas; k5: recuperar el platillo quita el suyo.
kitchen_notices() {
    local step="$1" log="$SUITE_OUT/k4_marcarPlatilloCierreAutomaticoYPedido-logcat.txt"
    local table takeaway counts="$SUITE_OUT/k4-avisos.txt" before now verdict
    if ! database_allowed; then
        skip_step "$step" "sin ADELPOS_E2E_DATABASE_URL: los avisos al mesero solo se ven en la base"
        return
    fi
    table="$(logged_account "k4 mesa cuenta=" "$log")"
    takeaway="$(logged_account "k4 llevar cuenta=" "$log")"
    if [ -z "$table" ] || [ -z "$takeaway" ]; then
        echo "$SUITE_NAME $step: FALLA (k4 no anotó sus cuentas)" | tee -a "$SUMMARY"
        FAILED=1
        return
    fi
    if [ "$step" = k4_avisosAlMesero ]; then
        before="$(ready_notices_of "$table") $(ready_notices_of "$takeaway")"
        echo "$before" >"$counts"
        verdict="APROBADA"
        case "$before" in 0\ * | *\ 0) verdict="FALLA" ;; esac
        echo "$SUITE_NAME $step: $verdict (avisos en ready_notices: mesa ${before% *}, para llevar ${before#* })" |
            tee -a "$SUMMARY"
    else
        before="$(cut -d' ' -f2 "$counts" 2>/dev/null)"
        now="$(ready_notices_of "$takeaway")"
        verdict="FALLA"
        if [ -n "$before" ] && [ "$now" -lt "$before" ]; then
            verdict="APROBADA"
        fi
        echo "$SUITE_NAME $step: $verdict (avisos del pedido para llevar: $before antes de recuperar, $now después)" |
            tee -a "$SUMMARY"
    fi
    [ "$verdict" = APROBADA ] || FAILED=1
}

# Etapa K1, Fase D: el PID del Go temporal que se prueba, el que escucha **a la vez** en el puerto local y en el de red
# (comprobado con lsof antes de cada pausa). Los puertos del servicio real (8181 y 8443) ya se rechazaron al empezar.
kitchen_go_pid() {
    local local_pid network_pid
    local_pid="$(lsof -nP -iTCP:"$LOCAL_PORT" -sTCP:LISTEN -t 2>/dev/null | head -1)"
    network_pid="$(lsof -nP -iTCP:"$NETWORK_PORT" -sTCP:LISTEN -t 2>/dev/null | head -1)"
    if [ -z "$local_pid" ] || [ "$local_pid" != "$network_pid" ]; then
        return 1
    fi
    echo "$local_pid"
}

# Espera la marca [1] en el registro de la prueba (AdelOnGo), como r4.
kitchen_wait_log() {
    local mark="$1" logcat="$2" deadline=$((SECONDS + 180))
    while [ "$SECONDS" -lt "$deadline" ]; do
        "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V >"$logcat" 2>/dev/null || true
        if grep -q "$mark" "$logcat"; then
            return 0
        fi
        sleep 1
    done
    return 1
}

# Pausa el Go temporal [2] segundos (kill -STOP) y lo reanuda (kill -CONT). Quien llama deja un trap con -CONT.
kitchen_pause_go() {
    local pid="$1" seconds="$2" log="$3"
    echo "$(date +%T) pausa de ${seconds} s del Go temporal (PID $pid)" >>"$log"
    kill -STOP "$pid"
    sleep "$seconds"
    kill -CONT "$pid"
    echo "$(date +%T) reanudado" >>"$log"
}

# Corta (off) o devuelve (on) la red del emulador: modo avión en Android 11+; antes, wifi y datos con root (tomado
# antes del paso).
kitchen_network() {
    if [ "$SDK" -ge 30 ]; then
        if [ "$1" = off ]; then
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode enable
        else
            "$ADB" -s "$ANDROID_SERIAL" shell cmd connectivity airplane-mode disable
        fi
    else
        # Al cortar, primero los datos y después la wifi (y al revés al devolverla): así Android pasa de una red a
        # ninguna una sola vez, sin un momento con los datos del emulador.
        if [ "$1" = off ]; then
            "$ADB" -s "$ANDROID_SERIAL" shell svc data disable && "$ADB" -s "$ANDROID_SERIAL" shell svc wifi disable
        else
            "$ADB" -s "$ANDROID_SERIAL" shell svc wifi enable && "$ADB" -s "$ANDROID_SERIAL" shell svc data enable
        fi
    fi
}

# Espera a que Android tenga otra vez una red predeterminada (como mucho 90 s) y anota cuánto tardó.
kitchen_wait_network() {
    local log="$1" started=$SECONDS
    while [ $((SECONDS - started)) -lt 90 ]; do
        if ! "$ADB" -s "$ANDROID_SERIAL" shell dumpsys connectivity 2>/dev/null |
            grep -m1 'Active default network' | grep -q 'none'; then
            echo "$(date +%T) red de vuelta para Android $((SECONDS - started)) s después de devolverla" >>"$log"
            return 0
        fi
        sleep 1
    done
    echo "$(date +%T) la red no volvió en 90 s" >>"$log"
    return 1
}

# Los cortes de un paso, en segundo plano mientras corre la prueba; al terminar crea el archivo [2] que la prueba espera.
kitchen_cuts() {
    local step="$1" done_file="$2" log="$SUITE_OUT/$1-guion.txt" logcat="$SUITE_OUT/$1-logcat-vivo.txt" pid seconds
    : >"$log"
    if ! kitchen_wait_log "$step listo para" "$logcat"; then
        echo "$step: la prueba no pidió los cortes" >>"$log"
        return 1
    fi
    case "$step" in
        k7 | k8 | k9)
            if ! pid="$(kitchen_go_pid)"; then
                echo "$step: no se encontró el Go temporal en $LOCAL_PORT y $NETWORK_PORT" >>"$log"
                return 1
            fi
            # Pase lo que pase, el Go temporal sigue después de este guion.
            trap 'kill -CONT '"$pid"' 2>/dev/null' EXIT
            case "$step" in
                k7)
                    for seconds in 1 2 3 4 5; do
                        kitchen_pause_go "$pid" "$seconds" "$log"
                        sleep 5
                    done
                    ;;
                k8)
                    echo "$(date +%T) pausa de 18 s del Go temporal (PID $pid)" >>"$log"
                    kill -STOP "$pid"
                    "$ADB" -s "$ANDROID_SERIAL" shell touch /data/local/tmp/adelpos-k8-pausado
                    sleep 18
                    kill -CONT "$pid"
                    echo "$(date +%T) reanudado" >>"$log"
                    sleep 3
                    ;;
                k9) kitchen_pause_go "$pid" 45 "$log" ;;
            esac
            ;;
        k10)
            # Pase lo que pase, la red vuelve.
            trap 'kitchen_network on >/dev/null 2>&1' EXIT
            for seconds in 1 3 5; do
                echo "$(date +%T) corte de red de ${seconds} s" >>"$log"
                kitchen_network off >>"$log" 2>&1
                sleep "$seconds"
                kitchen_network on >>"$log" 2>&1
                kitchen_wait_network "$log" || true
                sleep 15
            done
            echo "$(date +%T) corte de red de 35 s" >>"$log"
            kitchen_network off >>"$log" 2>&1
            sleep 35
            kitchen_network on >>"$log" 2>&1
            kitchen_wait_network "$log" || true
            sleep 20
            ;;
    esac
    "$ADB" -s "$ANDROID_SERIAL" shell touch "$done_file"
    echo "$(date +%T) fin de los cortes" >>"$log"
}

# Un paso con cortes: borra los archivos de la corrida anterior, corre los cortes en segundo plano y la prueba.
kitchen_step_with_cuts() {
    local step="$1" method="$2" done_file="$3" helper
    "$ADB" -s "$ANDROID_SERIAL" shell rm -f "$done_file" /data/local/tmp/adelpos-k8-pausado >/dev/null 2>&1 || true
    kitchen_cuts "$step" "$done_file" &
    helper=$!
    run_step "$method"
    wait "$helper" || echo "$step: el guion no pudo hacer sus cortes ($SUITE_OUT/$step-guion.txt)" | tee -a "$SUMMARY"
}

# Etapa K1, la prueba de punta a punta (§8): las dos aplicaciones en el mismo dispositivo y el mismo Go temporal.
# m1 (meseros, AlertsOnGoTest r1) prepara el restaurante, empareja y entra Ana con el servicio de avisos; c1 (cocina,
# KitchenOnGoTest k1) empareja la pantalla de cocina; m2 (meseros, e1) Ana envía a cocina desde la aplicación y espera
# el aviso mientras este guion corre, en segundo plano, la pantalla de cocina (k12) que lo marca listo.
endtoend_use() {
    select_app "$1"
    TEST_CLASS="$2"
}

# k12 de la pantalla de cocina, en segundo plano mientras m2 espera el aviso: espera la cuenta que anota m2 y corre la
# instrumentación de cocina con ella (sin limpiar el registro, que es de la otra prueba).
endtoend_kitchen_marks() {
    local log="$SUITE_OUT/k12_marcaListoElPedidoDelMesero.txt" logcat="$SUITE_OUT/k12-logcat-vivo.txt" account=""
    local deadline=$((SECONDS + 300)) codes verdict
    while [ -z "$account" ] && [ "$SECONDS" -lt "$deadline" ]; do
        sleep 2
        "$ADB" -s "$ANDROID_SERIAL" logcat -d -s AdelOnGo:V >"$logcat" 2>/dev/null || true
        account="$(logged_account "e1 enviado a cocina cuenta=" "$logcat")"
    done
    if [ -z "$account" ]; then
        echo "$SUITE_NAME k12_marcaListoElPedidoDelMesero: SIN RESULTADO (la aplicación de meseros no envió)" |
            tee -a "$SUMMARY"
        return 1
    fi
    "$ADB" -s "$ANDROID_SERIAL" shell am instrument -w -r \
        -e class "com.adelpos.cocina.KitchenOnGoTest#k12_marcaListoElPedidoDelMesero" \
        -e local "http://$DEVICE_HOST:$LOCAL_PORT" -e address "$DEVICE_HOST:$NETWORK_PORT" \
        -e mainDevice "$MAIN_DEVICE" -e mainToken "$MAIN_TOKEN" -e account "$account" \
        com.adelpos.cocina.test/com.adelpos.cocina.CocinaTestRunner >"$log" 2>&1 || true
    codes="$(grep -E '^INSTRUMENTATION_STATUS_CODE: ' "$log" | awk '{print $2}' | tr -d '\r' | tr '\n' ' ' || true)"
    case "$codes" in
        *"-2"*) verdict="FALLA" ;;
        *"-4"* | *"-3"*) verdict="OMITIDA" ;;
        "1 0 ") verdict="APROBADA" ;;
        *) verdict="SIN RESULTADO" ;;
    esac
    echo "$SUITE_NAME k12_marcaListoElPedidoDelMesero: $verdict (cuenta $account)" | tee -a "$SUMMARY"
    [ "$verdict" = APROBADA ]
}

endtoend_step() {
    local helper
    case "$1" in
        m1)
            endtoend_use waiter AlertsOnGoTest
            run_step r1_prepararEmparejarYEntrarArrancaElServicio -e readyAlerts on -e alertPermissions ask
            ;;
        c1)
            endtoend_use kitchen KitchenOnGoTest
            run_step k1_emparejarComoPantallaDeCocina -e restaurant existing
            ;;
        m2)
            endtoend_use waiter AlertsOnGoTest
            endtoend_kitchen_marks &
            helper=$!
            run_step e1_enviaACocinaYLlegaElAvisoDeLaPantallaDeCocina -e readyAlerts on
            wait "$helper" || FAILED=1
            ;;
        *) fail "paso desconocido de la suite endtoend: $1" ;;
    esac
}

# Etapa K1: la aplicación de cocina (KitchenOnGoTest). Las capturas de cada paso se copian a <salida>/screens.
kitchen_step() {
    case "$1" in
        k1) run_step k1_emparejarComoPantallaDeCocina ;;
        k2) run_step k2_conectarYPantallaDePedidosVacia ;;
        k3) run_step k3_pedidosRealesAparecenSolos ;;
        k4)
            run_step k4_marcarPlatilloCierreAutomaticoYPedido
            kitchen_notices k4_avisosAlMesero
            ;;
        k5)
            run_step k5_recientesYRecuperar
            kitchen_notices k5_avisosRecuperados
            ;;
        k6) run_step k6_letraYTemaDesdeTerminalesAlMomento ;;
        k7) kitchen_step_with_cuts k7 k7_cortesCortosDelGoSinNingunAviso /data/local/tmp/adelpos-k7-fin ;;
        k8)
            kitchen_step_with_cuts k8 k8_unaMarcaDuranteUnCorteSeRepiteYGoLaRegistraUnaVez \
                /data/local/tmp/adelpos-k8-fin
            ;;
        k9) kitchen_step_with_cuts k9 k9_corteLargoAvisaNoDejaMarcarYSeRecuperaSolo /data/local/tmp/adelpos-k9-fin ;;
        k10)
            # Android 8–10: la red se corta con root, que se toma antes de la prueba (reiniciar adbd la cortaría).
            if [ "$SDK" -lt 30 ] && ! take_root; then
                echo "$SUITE_NAME k10_cortesDeRedCortosSinAvisoYUnoLargoConAviso: FALLA (sin root no se corta la red)" |
                    tee -a "$SUMMARY"
                FAILED=1
            else
                kitchen_step_with_cuts k10 k10_cortesDeRedCortosSinAvisoYUnoLargoConAviso \
                    /data/local/tmp/adelpos-k10-fin
                kitchen_network on >/dev/null 2>&1 || true
                kitchen_wait_network "$SUITE_OUT/k10-guion.txt" || true
                if [ "$SDK" -lt 30 ]; then
                    "$ADB" -s "$ANDROID_SERIAL" unroot >/dev/null 2>&1 || true
                    "$ADB" -s "$ANDROID_SERIAL" wait-for-device
                fi
                grep -h 'k10 sin red' "$SUITE_OUT"/k10_*-logcat.txt 2>/dev/null | tail -1 | tee -a "$SUMMARY" || true
            fi
            ;;
        k11) run_step k11_ajustesConLaContrasenaDelAdministrador ;;
        k13) run_step k13_volverAEmparejarSueltaElCupo ;;
        *) fail "paso desconocido de la suite kitchen: $1" ;;
    esac
    mkdir -p "$SUITE_OUT/screens"
    "$ADB" -s "$ANDROID_SERIAL" pull "/sdcard/Android/data/$APP_ID/files/screens/." \
        "$SUITE_OUT/screens" >/dev/null 2>&1 || echo "sin capturas que copiar" | tee -a "$SUMMARY"
}

# Corre los pasos [steps] de [suite] contra el servidor ya preparado.
run_suite() {
    local suite="$1" steps="$2" install="$3"
    SUITE_NAME="$suite"
    TEST_CLASS="$(suite_class "$suite")"
    select_app "$suite"
    SUITE_OUT="$4"
    mkdir -p "$SUITE_OUT"
    {
        echo "android-e2e en $ANDROID_SERIAL (API $SDK), suite $suite, pasos: $steps"
        echo "servidor: $LOCAL_URL y $NETWORK_URL, huella $PIN"
        echo "terminal principal: $MAIN_DEVICE (ADELPOS_E2E_MAIN_DEVICE; el token está en $SUITE_OUT/main-token)"
    } | tee -a "$SUMMARY"
    (umask 077 && printf '%s' "$MAIN_TOKEN" >"$SUITE_OUT/main-token")
    if [ "$install" = 1 ]; then
        if [ "$suite" = endtoend ]; then
            # Las dos aplicaciones en el mismo dispositivo.
            select_app kitchen
            install_app
            select_app "$suite"
        fi
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
            precheck) precheck_step "$step" ;;
            alerts) alerts_step "$step" ;;
            adjustments) adjustments_step "$step" ;;
            license) license_step "$step" ;;
            unions) unions_step "$step" ;;
            legacy) legacy_step "$step" ;;
            connection) connection_step "$step" ;;
            kitchen) kitchen_step "$step" ;;
            endtoend) endtoend_step "$step" ;;
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
                ADELPOS_E2E_DATABASE_URL | ADELPOS_E2E_SERVER_ID | ADELPOS_E2E_CLOCK_FILE)
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
    for part in catalog waiter field takeaway precheck alerts adjustments license; do
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
