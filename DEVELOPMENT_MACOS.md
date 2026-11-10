# Desarrollo macOS · Etapa 7

## Entorno preparado

Mac Apple Silicon arm64, macOS 26.5.2 y Command Line Tools. Herramientas locales: Go 1.27.3, Node 24.22.2 y Rust 1.99.1 en `.tools`. `scripts/dev.sh` selecciona sus rutas y cachés sin sustituir Go/Node globales. PostgreSQL 18.6 está instalado por Homebrew; el proyecto utiliza exclusivamente `.local/postgres`, puerto 55432, autenticación SCRAM y rol `adel_app` sin privilegios de superusuario.

No se inicia el clúster predeterminado de Homebrew, no se sustituyen bases ajenas y no hay launchd instalado. Tras reiniciar macOS se deben iniciar PostgreSQL y Go explícitamente. La ejecución local no necesita Internet; descargar dependencias sí.

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

En la primera ventana: Terminales → Autorizar otra terminal. Introducir el código y un nombre en la segunda ventana antes de diez minutos. Luego iniciar sesión. El código no crea una sesión automáticamente. Si se usa admin en la segunda, se invalida la sesión admin anterior; la primera ventana lo informa en su siguiente comprobación y sus operaciones ya son rechazadas por Go.

`credential_namespace` debe diferir entre terminales lógicas en un mismo usuario OS. Cambiarlo selecciona otra identidad local; no autoriza nada por sí mismo. No copiar secretos del Keychain entre equipos. La segunda terminal no necesita Go ni PostgreSQL locales.

## LAN con TLS

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

La plantilla launchd no está instalada. Un LaunchAgent depende de la sesión; para servicio comercial sin login se diseñará un LaunchDaemon con usuario y directorios propios. No iniciar simultáneamente launchd y server-start en el mismo puerto.

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

Versión 0.7.1/esquema 9. La migración `0009_operation_tables_cash_and_reasons.sql` **no se ha aplicado a la instalación real**: requiere autorización expresa del propietario. Procedimiento: `backup`, `python3 scripts/restore-check.py backups/NOMBRE.dump`, `server-stop`, `migrate`, `server-start`, comprobaciones de solo lectura, `build-macos` y `open-app`. La 0009 no modifica 0001–0008 ni registros existentes (los descuentos anteriores quedan sin motivo del catálogo), no concede permisos nuevos y su única siembra son los cinco motivos de descuento (Atención, Promoción, Demora, Cortesía de la casa y Otro); no crea datos ficticios.
