# Desarrollo macOS · Etapa 9G

## Entorno preparado

Mac Apple Silicon arm64, macOS 26.5.2 y Command Line Tools. Herramientas locales: Go 1.27.3, Node 24.22.2 y Rust 1.99.1 en `.tools`. `scripts/dev.sh` selecciona sus rutas y cachés sin sustituir Go/Node globales. PostgreSQL 18.6 está instalado por Homebrew; el proyecto utiliza exclusivamente `.local/postgres`, puerto 55432, autenticación SCRAM y rol `adel_app` sin privilegios de superusuario.

No se inicia el clúster predeterminado de Homebrew y no se sustituyen bases ajenas. En desarrollo (y en la instalación real hasta pasar sus datos al servicio de la etapa 9G) no hay launchd instalado: tras reiniciar macOS se deben iniciar PostgreSQL y Go explícitamente. Desde la 9G la computadora principal instalada con el `.pkg` tiene el servidor como servicio del sistema (ver «Servidor del restaurante como servicio · etapa 9G» al final). La ejecución local no necesita Internet; descargar dependencias sí.

## Preparación en otro Mac

Revisar primero `uname -m`, `sw_vers`, `xcode-select -p`, Go, Node, Cargo y cualquier PostgreSQL existente. Instalar solo requisitos ausentes y no actualizar clústeres existentes sin revisar datos y respaldos. Homebrew puede proporcionar `postgresql@18`, Go, `node@24` y Rustup; Apple proporciona Command Line Tools. Rustfmt se requiere para verificar formato (`rustup component add rustfmt`).

```bash
cd "/ruta/a/ad-el-pos"
./scripts/dev.sh install
./scripts/dev.sh db-init
./scripts/dev.sh migrate
./scripts/dev.sh prepare-setup
./scripts/dev.sh server-start
./scripts/dev.sh desktop
```

`db-init` crea el clúster exclusivo y secretos aleatorios en archivos 0600 dentro del directorio privado `.local`. Repetirlo no reinicializa datos ni rota contraseñas. `ADEL_PG_BIN` permite usar otra instalación PostgreSQL; por defecto `/opt/homebrew/opt/postgresql@18/bin` en ARM y `/usr/local/opt/postgresql@18/bin` en Intel. No se ha probado Intel.

`migrate` aplica la siguiente migración de forma explícita. `prepare-setup` genera una autorización local privada sin mostrar su valor. El asistente consume esa autorización desde el puente nativo, únicamente por loopback. No enviar ese archivo a otra máquina.

## Primera configuración y acceso

En el asistente, el nombre inicial es **ADEL POS**. La zona horaria proviene de `Intl.DateTimeFormat().resolvedOptions().timeZone` del sistema operativo y se puede editar; Go valida el identificador IANA. Se guardan MXN, contacto opcional, apariencia y nombre de terminal. Caja 1 se crea sin fondos, turnos o movimientos.

Después de guardar: usuario **admin**, contraseña **admin**. La advertencia de contraseña inicial permite seguir trabajando. Cambiarla en Cambiar contraseña; no se restablece al reiniciar. Las credenciales de terminal y sesión se guardan por Rust en Keychain del sistema, nunca en localStorage o archivos versionados. macOS puede pedir acceso a Keychain; si se deniega, el sistema muestra el fallo y no usa almacenamiento inseguro como alternativa.

El asistente solo aparece si PostgreSQL indica que no existe establecimiento. Repetir setup con autorización local no duplica registros ni recupera contraseñas; una instalación existente necesita acceso normal.

## Aprovisionamiento privado de soporte

La identidad reservada sa existe desde la migración 0002, sin contraseña utilizable. El propietario define el secreto personalmente:

```bash
./scripts/dev.sh provision-sa
```

Debe ejecutarse en Terminal local interactiva. Solicita contraseña y confirmación sin eco (mínimo 12 caracteres); no acepta el secreto como flag, entorno o stdin redirigido. Solo guarda Argon2id y registra el aprovisionamiento en auditoría. No hay contraseña universal. Si ya existe credencial, el comando se niega a reemplazarla.

Recuperación excepcional:

```bash
./scripts/dev.sh reset-sa
```

Requiere escribir `RESET sa` y definir una nueva contraseña con entrada oculta. Revoca sus sesiones y queda auditado como intervención del propietario local. Con la terminal autorizada, sa inicia sesión en el formulario normal escribiendo su usuario; no existe un botón público de soporte. Una sesión administrativa ordinaria no puede modificarlo ni asignar su rol.

## Ejecución diaria y procesos

```bash
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

`open-app` ejecuta el `.app` compilado y transmite solo las rutas de archivos privados de aprovisionamiento local cuando existen. Para desarrollo usar `desktop`. Go puede funcionar solo:

```bash
./scripts/dev.sh server
# O el binario ya construido:
./bin/ad-el-pos-server serve --config .local/server.json
```

`server` en primer plano depende de esa terminal. `server-start` usa un proceso separado, PID y logs `.local/server.log`. Cerrar Tauri no envía señales a Go ni a PostgreSQL. `server-stop` y `db-stop` los detienen por separado; el script comprueba la propiedad del PID y no mata procesos ajenos.

Go usa `.local/server.json` o `ADEL_SERVER_CONFIG`; `ADEL_DB_PASSWORD` puede sustituir el secreto DB. Parámetros técnicos locales (puerto, TLS, pool, modo, perfil) no son la configuración del restaurante. Producción sigue requiriendo el proveedor comercial pendiente.

## Segunda terminal en el mismo Mac

Usar otro perfil de credenciales para que tenga identidad de dispositivo propia:

```bash
cp config/terminal.second-local.example.json .local/terminal-second.json
ADEL_TERMINAL_CONFIG="$PWD/.local/terminal-second.json" \
  "desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app/Contents/MacOS/ad-el-pos-desktop"
```

En la primera ventana: Terminales → Código para esta computadora (hasta la 9F, «Autorizar otra terminal»; desde la 9G el código de 43 caracteres solo sirve en la propia computadora, que es el caso de un segundo perfil en el mismo Mac; una terminal de otra computadora se empareja con el código de 6 dígitos). Introducir el código y un nombre en la segunda ventana antes de diez minutos. Luego iniciar sesión. El código no crea una sesión automáticamente. Si se usa admin en la segunda, se invalida la sesión admin anterior; la primera ventana lo informa en su siguiente comprobación y sus operaciones ya son rechazadas por Go.

`credential_namespace` debe diferir entre terminales lógicas en un mismo usuario OS. Cambiarlo selecciona otra identidad local; no autoriza nada por sí mismo. No copiar secretos del Keychain entre equipos. La segunda terminal no necesita Go ni PostgreSQL locales.

## LAN con TLS (camino anterior a la etapa 9G)

Desde la etapa 9G las terminales de otra computadora se **emparejan con código** contra la escucha HTTPS `:8443` del servicio, con el certificado propio del servidor fijado por huella (ver la sección de la 9G y [docs/INSTALAR_EN_RED.md](docs/INSTALAR_EN_RED.md)). Lo que sigue (`tls-init`, IP fija, `allow_lan`, CA copiada a la terminal) se conserva sin cambios y sin uso.

La URL HTTP solo admite loopback. Para LAN utilizar una IP privada específica y HTTPS. Generación local de certificados, sin modificar confianza global:

```bash
./scripts/dev.sh tls-init --lan-ip 192.168.1.10
```

Sustituir la IP por la real del servidor. Crea `.local/tls/ca.pem`, `ca.key`, `server.pem`, `server.key` con permisos privados y rechaza reemplazarlos si ya existen. No ejecutar otra vez para rotarlos sin un procedimiento deliberado.

En una copia privada del JSON del servidor: `listen_address` con esa IP, `allow_lan=true`, `tls_certificate` y `tls_private_key` con rutas absolutas a server.pem/server.key. Mantener DB en loopback. Reiniciar Go con esa configuración. No escuchar en 0.0.0.0 ni mapear puertos del router.

En el otro Mac: copiar **solo ca.pem** mediante un canal local cuya identidad verifique el propietario, usar `config/terminal.only.example.json` como plantilla, poner URL HTTPS, nombre de perfil y ruta absoluta `ca_certificate`. No compartir ca.key, server.key, configuración DB ni el archivo de aprovisionamiento. El cliente verifica CA, SAN/hostname y fechas; no existe un modo de desactivar TLS.

Para generar certificados con otra IP o renovarlos debe planificarse su rotación. No hay descubrimiento automático, instalador de certificados ni segundo equipo físico probado en esta entrega.

## Recuperar una terminal local perdida

Desde una sesión OS autorizada en el servidor, con su configuración privada:

```bash
./scripts/dev.sh create-enrollment
./scripts/dev.sh open-app
```

Se emite y audita una autorización de diez minutos en `.local/enrollment-code`, sin mostrarla. En la pantalla de autorización, indicar el nombre y usar **Usar autorización local**. No crea ni restablece cuentas y no permite acceso remoto anónimo. Sirve cuando se perdió la credencial de terminal y no queda una ventana administradora disponible. Requiere acceso local al mismo usuario OS que administra el proyecto.

## Pruebas, formato y compilación

```bash
./scripts/dev.sh format
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh build-macos
```

`test` incluye formato, Go con detector de carreras/vet, frontend/TypeScript/build y Rust. Las suites de migraciones y administración usan bases aleatorias distintas `adel_pos_test_*`, eliminadas al terminar, y no escriben en adel_pos.

`python3 tests/runtime_smoke.py` detiene/recupera temporalmente servicios propios y cierra/reabre Tauri. Solo desarrollo, una ventana nativa y sin trabajo real en curso. Deja servicios y escritorio activos. `--terminal-only` comprueba también el modo técnico de solo terminal. No es una prueba de todos los controles visuales.

El `.app` ARM64 declara macOS 14 mínimo; solo se probó macOS 26.5.2. No se genera DMG ni instalador Windows. Firma Apple/notarización, universal Intel/ARM y servicio comercial siguen pendientes.

## Respaldo y operación

```bash
./scripts/dev.sh backup
./scripts/dev.sh migrate
```

Antes de migraciones sobre información importante, respaldar y restaurar en una base temporal distinta. `backup` genera pg_dump custom y comprueba el catálogo con pg_restore --list; no sustituye probar una restauración completa. Nunca editar checksums ni migraciones aplicadas para ocultar una incompatibilidad.

Hasta la etapa 9F la plantilla launchd no se instalaba. Desde la 9G el `.pkg` instala el LaunchDaemon `local.adel.pos.server` (sección de la 9G). No iniciar simultáneamente el servicio y `server-start` en el mismo puerto: tras pasar los datos al servicio, los comandos de `dev.sh` que actúan sobre la base ya no son la instalación real.

Errores habituales: servidor desconectado → revisar status/log y URL; DB no disponible → db-start/puerto 55432; esquema incompatible → migrate con el binario correcto; credencial inválida → login o nueva autorización de terminal; fallo de Keychain → revisar acceso OS, sin borrar datos; fallo TLS → CA/SAN/fecha/IP correctos, sin desactivar validación. Un entorno aislado de agente puede necesitar permiso para puertos, memoria compartida y apertura de la aplicación.

## Catálogo · etapa 3

La versión 0.3.0 utiliza esquema 3. Para actualizar una instalación configurada: respaldo, restauración de ensayo en base temporal, `server-stop`, `migrate` y `server-start`; después compilar con `build-macos` y abrir con `open-app`. No ejecutar setup ni borrar .local. Se conservan establecimiento, usuarios, terminales y credenciales Keychain. Los logos originales deben existir en assets/brand.

Desde la etapa 5 el `.app` inicia en pantalla completa nativa, con salida/regreso visibles y tamaño adaptable; 1280×720 solo es un escenario de prueba. Catálogo y Probar personalización aparecen según permisos. Las pruebas de integración incluyen `internal/catalog` en una tercera base temporal y eliminan solo las bases creadas por la prueba. Los productos de los ejemplos se crean únicamente en esas bases; no se siembran en la instalación real. Reglas operativas en [docs/CATALOG.md](docs/CATALOG.md).

Una compilación de desarrollo nueva puede solicitar autorización del Llavero de macOS para leer la credencial existente. Autorizar únicamente en el diálogo del sistema; nunca compartir la contraseña por chat ni sustituir el perfil para evitarlo. La recuperación se ejecuta en segundo plano y la ventana muestra **Revisa el acceso seguro** mientras espera; conserva disponible Diagnóstico técnico. Después de autorizar, vuelve a comprobar automáticamente. Si se denegó el acceso, cerrar y reabrir la aplicación tras revisar la autorización del sistema. Esta política no guarda tokens en archivos ni cambia sa.

## Cocina, impresión y eventos · etapa 5

Versión 0.5.0/esquema 5. Antes de actualizar: `backup`, `python3 scripts/restore-check.py backups/NOMBRE.dump`, `server-stop`, `migrate`, `server-start`, `build-macos` y `open-app`. El ensayo restaura todas las tablas en una base temporal y genera `.local/stage5-restore-verification.json`. No ejecutar setup ni eliminar credenciales para actualizar.

Configurar impresoras/estaciones/rutas desde Cocina y preparación; no se siembran datos de negocio. CUPS consulta y envía en el host de Go; no en el equipo cliente de una terminal remota. `lpstat -a` lista colas y `lpoptions -p NOMBRE -l` opciones reales. No asumir que la impresora predeterminada es térmica. ESC/POS exige IP privada y puerto 9100–9109; agente remoto permanece pendiente. Aceptación CUPS/TCP no confirma papel. Revisar resultados inciertos en la cola antes de autorizar otra copia.

Go mantiene dos trabajadores de impresión y un planificador de liberaciones; cerrar Tauri no los detiene. Los eventos usan SSE desde Rust con credenciales nativas, cursor y reconexión; no se requiere un servidor WebSocket separado ni acceso web alternativo. Los estados de preparación no cambian pago/entrega.

`test-browser` ejecuta Chromium y WebKit con Go/PostgreSQL temporales y un puente de pruebas solo en Node. Incluye 1280×720, 1920×1080, 2560×1440, 1024×768 y 900×1200, conservación de cantidad, temas, teclado y preparación/cola reales. No equivale a hardware táctil o a toda la ventana nativa.

La comprobación nativa opt-in `ADEL_WINDOW_VERIFICATION_FILE="$PWD/.local/stage5-window-verification.json" ./scripts/dev.sh open-app` consulta fullscreen inicial, sale a ventana y regresa mediante Tauri, guardando solo tamaños/resultados. Ejecutarla con una única ventana y sin trabajo real; en el arranque normal esa variable no existe y no cambia modos automáticamente.

La prueba CUPS es opt-in: `ADEL_TEST_PACKAGE=./internal/operations ADEL_TEST_RUN=TestControlledCUPSPrinter ADEL_HARDWARE_QUEUE=NOMBRE ./scripts/dev.sh test-integration` genera un documento sin enviarlo. Solo `ADEL_HARDWARE_SEND=1` autoriza el envío real de una copia técnica. `ADEL_HARDWARE_PREVIEW` y `ADEL_HARDWARE_REPORT` seleccionan archivos privados de evidencia. Nunca activar esa opción en pruebas ordinarias o CI. Ver detalles y límites en [docs/KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md).

## Cobros y pagos · etapa 6

Versión 0.6.2/esquema 7 (la 0.6.2 no agrega migraciones). La migración `0007_features_product_tax_and_usd_cash.sql` (etapa 6.1) se aplicó a la instalación real el 8 de noviembre de 2026 con autorización del propietario; las migraciones futuras también requieren autorización. La migración `0006_payments_and_refunds.sql` se aplicó a la instalación real el 8 de noviembre de 2026 con autorización del propietario; las migraciones futuras también requieren autorización. Procedimiento: `backup`, `python3 scripts/restore-check.py backups/NOMBRE.dump`, `server-stop`, `migrate`, `server-start`, `build-macos` y `open-app`. La migración asigna IVA 16 % a los productos ya confirmados, siembra cinco métodos de pago y concede los permisos nuevos al rol administrador con límite de descuento de 100 %; no crea turnos, pagos ni datos ficticios.

Para cobrar, la terminal debe estar asociada a una caja (Cajas físicas / Terminales) y abrir turno desde la pantalla de cobro. La impresora de tickets se elige en Cobros → Políticas entre las impresoras de cocina registradas. Las pruebas de integración incluyen `internal/payments` y los recorridos de cobro de `internal/operations`; las bases temporales se eliminan al terminar.

## Caja y cortes · etapa 7

Versión 0.7.0/esquema 8. La migración `0008_cash_shifts_and_reconciliation.sql` se aplicó a la instalación real el 9 de noviembre de 2026 con autorización del propietario; las migraciones futuras también requieren autorización. Procedimiento: `backup`, `python3 scripts/restore-check.py backups/NOMBRE.dump`, `server-stop`, `migrate`, `server-start`, comprobaciones de solo lectura, `build-macos` y `open-app`. La 0008 no modifica 0001–0007 ni registros históricos: los turnos existentes conservan su estado, la fecha comercial de un turno anterior se deriva al consultarlo de su apertura en la zona del establecimiento (la fila no se modifica), siembra las denominaciones habituales (billetes y monedas de pesos y billetes de dólares), la función `cash_limit` desactivada y concede los diecinueve permisos `cash.*` nuevos solo al rol administrador; no crea turnos, operaciones ni datos ficticios.

La pantalla **Caja** (permiso `cash.shifts.read`) administra el turno de la caja de la terminal. El cajón requiere una impresora registrada en Cocina y preparación y asociada en Caja → Configuración: ESC/POS por red o, desde la etapa 7.1, una cola CUPS que admita datos raw (USB, serie, paralelo o socket; Go lo comprueba solo con `lpstat` y `lpoptions` y explica por qué una cola no sirve). Las pruebas de integración de caja están en `internal/operations/cash_*_test.go` (encadenadas a `TestRestaurantAcceptance`) y usan el adaptador controlado de la cola: ninguna envía nada a CUPS, a una impresora ni a una conexión TCP.

## Ajustes de operación, mesas y caja · etapa 7.1

Versión 0.7.1/esquema 9. La migración `0009_operation_tables_cash_and_reasons.sql` se aplicó a la instalación real el 9 de noviembre de 2026 con autorización del propietario, con este procedimiento: `backup`, `python3 scripts/restore-check.py backups/NOMBRE.dump`, `server-stop`, `migrate`, `server-start`, comprobaciones de solo lectura, `build-macos` y `open-app`. La 0009 no modifica 0001–0008 ni registros existentes (los descuentos anteriores quedan sin motivo del catálogo), no concede permisos nuevos y su única siembra son los cinco motivos de descuento (Atención, Promoción, Demora, Cortesía de la casa y Otro); no crea datos ficticios.

## Inventarios, recetas y compras · etapa 8

Versión 0.8.0/esquema 14. Las migraciones `0010_inventory_core.sql` a `0014_inventory_control.sql` se aplicaron a la instalación
real el 10 de noviembre de 2026 con autorización del propietario, con el procedimiento de
[docs/ETAPA_8_AVANCE.md](docs/ETAPA_8_AVANCE.md) («Procedimiento para aplicar 0010–0014 a la instalación real»):

1. `backup`.
2. `python3 scripts/restore-check.py backups/NOMBRE.dump`.
3. Comprobación previa de solo lectura.
4. `server-stop`.
5. `migrate` (`applied:5`, y `applied:0` al repetirla).
6. `server-start`.
7. Comprobaciones de solo lectura.
8. `build-macos` y `open-app`.

No modifican registros existentes, conceden los 24 permisos de inventario solo al administrador y dejan «Control de
inventario» desactivado: la venta es idéntica a la 0.7.1 hasta activarlo. Resultado: `applied:5` y `applied:0`,
readiness 200 y servidor y `.app` 0.8.0.

## Reportes, indicadores y exportaciones · etapa 9A

Versión 0.9.0/esquema 17. Las migraciones `0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql`
y `0017_reports.sql` se aplicaron a la instalación real el 10 de noviembre de 2026 con autorización previa del
propietario, con el procedimiento de [docs/ETAPA_9A_AVANCE.md](docs/ETAPA_9A_AVANCE.md) («Procedimiento para aplicar
0015–0017 a la instalación real»):

1. `backup`.
2. `python3 scripts/restore-check.py backups/NOMBRE.dump`.
3. Comprobación previa de solo lectura.
4. `server-stop`.
5. `migrate` (`applied:3`, y `applied:0` al repetirla).
6. `server-start`.
7. Comprobaciones de solo lectura.
8. `build-macos` y `open-app`.

No modifican filas existentes: la opción «El precio puede bajar al cambiar complementos» queda desactivada en todos los
productos, las precuentas anteriores quedan sin revisión, se crea el proveedor «Compras de mostrador» sin compras, el
relleno inicial reconoce las cuentas ya cerradas y los 10 permisos `reports.*` se conceden solo al administrador. La
«Hora de inicio del día» y «Control de inventario» no cambian. Resultado: respaldo
`backups/adel_pos-20261111T050739Z-c64463.dump` con restauración verificada (126 tablas idénticas), `applied:3` y
`applied:0`, readiness 200, `/api/v1/version` 0.9.0 y esquema 17, 127 tablas en `app`, conteos de datos existentes sin
cambio, relleno de las 2 cuentas cerradas (9 renglones, $695.00) y servidor y `.app` 0.9.0. El recorrido en la ventana
nativa no se verificó.

## Servidor del restaurante como servicio · etapa 9G

Decisiones D3, D4 y D14 de [docs/ETAPA_9G_AVANCE.md](docs/ETAPA_9G_AVANCE.md); guía para el propietario en
[docs/INSTALAR_EN_RED.md](docs/INSTALAR_EN_RED.md).

**Instalador.** `./scripts/dev.sh build-macos-pkg [--reuse-app]` compila la aplicación con el servidor dentro
(`desktop/src-tauri/tauri.macos-pkg.conf.json`: `Contents/MacOS/ad-el-pos-server`), firma ad hoc la copia como paquete
completo, la verifica con `codesign --verify --deep --strict` y arma
`desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_<versión>.pkg` con `pkgbuild` y `productbuild`
(`scripts/macos-pkg/`). **No instala nada.** Tres paquetes: la aplicación en `/Applications` (siempre, no reubicable;
`preinstall` detiene el servicio anterior), «Servidor del restaurante» (marcada por omisión: exige PostgreSQL 18 de
Homebrew, crea la carpeta de datos de la cuenta que instala, guarda la modalidad y crea el archivo de habilitación;
desmarcarla es «Solo terminal») y el servicio (siempre, invisible: escribe el plist con la cuenta de la consola, lo
valida con `plutil` y lo carga con `launchctl bootstrap system`). Los scripts se prueban sin root con
`scripts/macos-pkg/test-scripts.sh` (`ADEL_PKG_TEST_ROOT` y `ADEL_PKG_TEST_LAUNCHCTL`). Sin firma de desarrollador ni
notarización; la arquitectura del instalador es la compilada (arm64).

**Servicio.** El LaunchDaemon `local.adel.pos.server` (plantilla `ops/launchd/local.adel.pos.server.plist`) corre
`ad-el-pos-server host --data-dir "/Library/Application Support/AD-EL POS"` con la cuenta de quien instaló,
`RunAtLoad`, `KeepAlive` → `PathState` del archivo `service-enabled`, `ThrottleInterval` 10, `ExitTimeOut` 90, `Umask`
077 y bitácoras en `logs/`. El `host` no hace nada sin el archivo de habilitación o en «Solo terminal»; si no, toma
`host.lock`, crea o adopta el clúster propio (127.0.0.1:55432, PostgreSQL de Homebrew), respalda antes de migrar, migra,
sirve en `127.0.0.1:8181` y en `:8443` (escucha HTTPS de la red local) y se anuncia en la red. Cerrar la ventana no lo
detiene; la aplicación solo crea o quita el archivo de habilitación al cambiar la modalidad, y «Reiniciar» de
Mantenimiento hace que launchd lo relance. Estado: Mantenimiento › «Servicio del servidor»; sin ventana:
`launchctl print system/local.adel.pos.server` y `logs/server.log` (solo lectura).

**Hasta que el propietario instale el `.pkg` y se pasen los datos, la instalación real de este Mac sigue siendo la de
`.local` con `dev.sh`** (secciones anteriores). El paso es una parada con el propietario (ventana de la contraseña de
administrador, aviso «Elementos en segundo plano añadidos», posible diálogo del Llavero y reinicio) y sigue el
procedimiento del avance (Fase A, «Procedimiento para la Fase G»):

1. `./scripts/dev.sh build-macos-pkg` (anotar el SHA-256).
2. `./scripts/dev.sh backup` y `python3 scripts/restore-check.py backups/<archivo>`, con la comprobación de solo lectura.
3. `./scripts/dev.sh server-stop` (PostgreSQL de `.local` sigue en marcha) y cerrar la aplicación anterior.
4. Instalar el `.pkg` **desmarcando «Servidor del restaurante»** (solo esta primera vez, para que el servicio no cree
   una base vacía).
5. `"/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" adopt-legacy --from-config "<repositorio>/.local/server.json"`
   (sin `sudo`): respaldo, clúster nuevo en un puerto temporal, restauración y conteos idénticos de todas las tablas
   (`legacy-adopted.json`). Nunca modifica `.local`.
6. `./scripts/dev.sh db-stop`.
7. Abrir la aplicación instalada y elegir «Servidor y terminal»: launchd lanza el servicio, que respalda antes de migrar,
   aplica las migraciones pendientes (la 0023, si no se aplicó antes) y sirve.
8. Verificar `health/ready` y `version`, Mantenimiento › «Servicio del servidor», cerrar la ventana (el servidor sigue),
   «Reiniciar» y reiniciar la Mac sin iniciar sesión.

<!-- PENDIENTE-COORDINADOR: resultado del procedimiento en la instalación real (fecha, respaldo, SHA-256 del .pkg,
conteos, versión 0.10.0 / esquema 23). -->

**Después de pasar los datos** el servicio es la instalación real: **no usar** `dev.sh db-start`, `server-start`,
`server`, `migrate`, `backup`, `restore-check.py`, `test-integration` ni `test-browser` con ella (actúan sobre la base
de `.local`, que queda como copia histórica, o necesitan su clúster en 55432, que ahora es el del servicio); para probar,
el procedimiento aislado de `CLAUDE.md` §6. Comandos del servidor instalado:

```bash
SERVER="/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server"
"$SERVER" backup                          # respaldo en /Library/Application Support/AD-EL POS/backups
"$SERVER" restore-check --file <respaldo> # ensayo en una base temporal
"$SERVER" restore --file <respaldo>       # pausa el servicio, respaldo de seguridad, restaura y lo reanuda
```

Además del respaldo manual, el servidor hace un **respaldo automático diario** (Mantenimiento › Respaldos; también con
`serve`, en `backup_directory` o `backups/` junto a la configuración). Desinstalar el servicio, si hiciera falta (pide
contraseña; lo hace el propietario): `sudo launchctl bootout system/local.adel.pos.server` y borrar
`/Library/LaunchDaemons/local.adel.pos.server.plist`; los datos se conservan.

**Red en desarrollo.** Con `serve`, la escucha de red solo se abre con `"network_listen_address": ":8443"` en
`.local/server.json` (nunca en los ejemplos de `config/`, que copian las pruebas); la identidad del servidor de
desarrollo vive en `.local/identity`. El permiso «Red local» de macOS se pide a la aplicación al buscar o conectarse a
un servidor de la red (con firma ad hoc puede volver a pedirse en cada compilación); el anuncio lo hace mDNSResponder.
