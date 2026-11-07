# Desarrollo macOS · Etapa 2

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
