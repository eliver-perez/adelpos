# Notas para Windows · AD-EL POS

Punto de partida para preparar el instalador de Windows 11 (hecho en la etapa 9E: ver la sección 0; lo que la etapa
9F, desarrollada en la Mac, deja por verificar en Windows está en la sección 6, y lo que la etapa 9G deja para su Fase W
—servicio de Windows, instalador para todo el equipo, terminales en red y pruebas cruzadas— en la sección 7). Recoge cómo se
empaqueta y se ejecuta hoy AD-EL POS en macOS y qué partes dependen de macOS, con el archivo y la línea de cada una.
Se escribió en la Fase 0 de la etapa 9D (versión 0.9.2, esquema 20) leyendo el código; **nada se compiló ni se probó en
Windows**. Lo marcado «por comprobar» es una expectativa razonable, no un hecho verificado.

## 0. Estado después de la etapa 9E (12/11/2026, versión 0.9.4)

La etapa 9E compiló, probó y empaquetó AD-EL POS en Windows 11 Pro de 64 bits. Las secciones 1 a 5 se conservan como el
análisis previo; lo que se hizo con cada punto (detalle en [ETAPA_9E_AVANCE.md](ETAPA_9E_AVANCE.md), cómo desarrollar en
[../DEVELOPMENT_WINDOWS.md](../DEVELOPMENT_WINDOWS.md) y la guía del propietario en
[INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md)):

| Punto                          | Resultado en la 9E                                                                                                                                                                                                                                                                                              |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.4 Datos y 2.1 PostgreSQL     | En la instalación, datos en `%LOCALAPPDATA%\AD-EL POS` (ACL privada) y PostgreSQL 18.6 de EDB incluido. **No se usa un servicio de Windows**: la aplicación lanza `ad-el-pos-server host`, que crea el clúster (`--locale=C`, SCRAM, UTC, solo `127.0.0.1:55432`, sin socket de Unix), lo arranca y lo detiene. |
| 2.2 Servidor Go como servicio  | Sin servicio: proceso oculto de la aplicación (`server_host.rs`). Parada con un evento con nombre (`stop --pid`) en lugar de `SIGTERM`; bitácora con `--log-file`. El apagado con SSE abierto ya no agota 5 s (corregido también para macOS).                                                                   |
| 2.3 Comandos interactivos      | `provision-sa` y `reset-sa` usan `CONIN$`/`CONOUT$` en Windows (`terminal_windows.go`); no se ejecutaron (son del propietario).                                                                                                                                                                                 |
| 2.4 Permisos                   | `internal/fileacl`: ACL propia sin herencia en Windows; la prueba de 0600 se separó por sistema.                                                                                                                                                                                                                |
| 2.5 Impresión y cajón          | La cola del equipo (`cups` en la base) es la impresora instalada en Windows con ESC/POS RAW por `winspool.drv`, con inspección previa; ESC/POS por red sin cambios. Textos por plataforma. Ver [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md), «Cola de impresión de Windows». Sin prueba física.           |
| 2.6 Keychain                   | Administrador de credenciales con el mismo `vault.rs`; la prueba real del almacén pasa en Windows. Textos neutros por sistema.                                                                                                                                                                                  |
| 2.7 Menú nativo                | Se compiló, se probó y **se omite en Windows** (barra clásica no táctil que repetía la barra superior).                                                                                                                                                                                                         |
| 2.8 Atajos y pantalla completa | Ctrl y F11 funcionan; la ventana recuerda tamaño y posición en modo ventana (`window_state.rs`).                                                                                                                                                                                                                |
| 2.9 WebView                    | WebView2; el instalador incluye su instalador de arranque. Los colores de la 9D y las barras siempre visibles del mapa se ven en WebView2 (capturas de la aplicación instalada). El mapa de mesas oscilaba al 125 % (corregido).                                                                                |
| 2.10 Empaquetado               | Instalador NSIS por usuario (`scripts/build-windows.ps1`, 36.7 MB), `icon.ico` y el símbolo de la marca como icono. Identificador sin cambiar (`local.adel.pos.dev`). La exportación de reportes con el diálogo nativo funcionó en la aplicación instalada.                                                     |
| 2.11 Servicios                 | Ninguno: Go y PostgreSQL viven mientras la aplicación está abierta.                                                                                                                                                                                                                                             |
| 2.12 Red local y TLS           | No preparado en Windows: el instalador solo usa `127.0.0.1` y no pide permisos del firewall.                                                                                                                                                                                                                    |
| 2.13 Scripts y pruebas         | `scripts/dev.ps1` y `env.ps1` (lógica en `scripts/windows/dev.mjs`), `.gitattributes` con LF y todas las suites aprobadas en Windows (Go sin `-race`).                                                                                                                                                          |
| 4.7 Migrar datos de una Mac    | Sigue sin herramienta: solo `pg_dump`/`pg_restore` custom.                                                                                                                                                                                                                                                      |

## 1. Cómo funciona hoy en macOS

### 1.1 Piezas

```text
AD-EL POS.app (Tauri 2: Rust + WebView)  ──HTTP JSON /api/v1 + SSE──▶  ad-el-pos-server (Go)  ──pgx──▶  PostgreSQL 18
   └─ Keychain (tokens de terminal y sesión)                                └─ CUPS (lp, lpstat, lpoptions) / ESC/POS TCP
```

- Son **tres procesos independientes**. La aplicación de escritorio **no incluye ni inicia** Go ni PostgreSQL
  (`desktop/src-tauri/tauri.conf.json` no declara `externalBin` ni recursos; regla de `ARCHITECTURE.md` y `CLAUDE.md` §3:
  «Tauri no inicia ni detiene Go ni PostgreSQL»). Cerrar la ventana no detiene el servidor ni la base.
- Modos de instalación: `server-terminal` (servidor y terminal en el mismo equipo), `terminal-only` (solo la
  aplicación, contra un servidor de la red local por HTTPS) y `server-only` (solo Go). La aplicación rechaza
  `server-only` (`desktop/src-tauri/src/connection.rs:22-27`).

### 1.2 Construcción

| Comando (`scripts/dev.sh`)            | Qué hace                                                                                                                                                                                                                                      | Resultado                                                            |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `sync` (`dev.sh:15-17`)               | `node scripts/sync-metadata.mjs`: copia nombre, versión e identificador de `config/product.json` a `tauri.conf.json`, `Cargo.toml`, `package.json`, `index.html` y a `backend/internal/product/metadata.json`, `desktop/src/generated/*.json` | metadatos                                                            |
| `build-server` (`dev.sh:8-12, 40-42`) | `go build -trimpath -o bin/ad-el-pos-server ./cmd/ad-el-pos-server`                                                                                                                                                                           | `bin/ad-el-pos-server` (binario único con las migraciones embebidas) |
| `build-macos` (`dev.sh:77-81`)        | `npm run tauri -- build --bundles app` (antes `npm run build`: TypeScript + Vite)                                                                                                                                                             | `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`        |

El `.app` es solo arm64, declara macOS 14.0 como mínimo (`tauri.conf.json` → `bundle.macOS.minimumSystemVersion`), usa
el identificador provisional `local.adel.pos.dev`, no está firmado ni notarizado y **no hay DMG ni instalador**
(`bundle.targets: ["app"]`). Las herramientas (Go 1.27.3, Node 24.22.2, Rust 1.99.1, Python) viven en `.tools/` con
binarios de macOS (por ejemplo `.tools/node-v24.22.2-darwin-arm64`); `scripts/env.sh` las antepone al `PATH`.

### 1.3 Arranque

| Paso             | Comando                                                                        | Detalle                                                                                                                                                                                                             |
| ---------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base             | `db-start` → `scripts/database.py start` (`database.py:63-73`)                 | `pg_ctl -D .local/postgres -l .local/postgres.log -w start`, binarios de Homebrew `/opt/homebrew/opt/postgresql@18/bin` (Intel: `/usr/local/…`; o `ADEL_PG_BIN`, `database.py:17-26`)                               |
| Servidor         | `server-start` → `scripts/server-process.py start` (`server-process.py:37-83`) | compila, lanza `bin/ad-el-pos-server serve --config .local/server.json` con `start_new_session=True`, guarda el PID en `.local/server.pid`, redirige la salida a `.local/server.log` y espera `/api/v1/health/live` |
| Detener servidor | `server-stop` (`server-process.py:86-109`)                                     | comprueba con `ps` que el PID es el binario del proyecto y envía `SIGTERM`; nunca fuerza                                                                                                                            |
| Aplicación       | `open-app` (`dev.sh:63-73`)                                                    | ejecuta directamente `…/AD-EL POS.app/Contents/MacOS/ad-el-pos-desktop` con `ADEL_PROVISIONING_FILE` y `ADEL_ENROLLMENT_FILE` si existen                                                                            |

Nada arranca solo al encender el equipo: tras reiniciar macOS hay que iniciar PostgreSQL y Go a mano
(`DEVELOPMENT_MACOS.md`, «Entorno preparado»). La plantilla `ops/launchd/local.adel.pos.server.plist.example`
(`RunAtLoad`, `KeepAlive` con `SuccessfulExit=false`, `ThrottleInterval` 10, logs `.local/service.log` y
`service-error.log`) **no está instalada**; un LaunchAgent dependería de la sesión del usuario.

### 1.4 Dónde se guardan datos, configuración y registros

Todo lo del servidor vive dentro de la carpeta del repositorio (excluida de git):

| Ruta                                                  | Contenido                                                                                                                               | Permisos      |
| ----------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ------------- |
| `.local/postgres/`                                    | clúster PostgreSQL exclusivo (puerto 55432, solo `127.0.0.1`, socket en `.local/socket`, `timezone='UTC'`, SCRAM; `database.py:89-112`) | 0700 (initdb) |
| `.local/postgres.log`                                 | registro de PostgreSQL                                                                                                                  | —             |
| `.local/server.json`                                  | configuración de Go (dirección, base, modo, orígenes, TLS) **con la contraseña de `adel_app`** (`database.py:127-134`)                  | 0600          |
| `.local/admin-password`, `.local/app-password`        | contraseñas del administrador del clúster (`adel_dev_admin`) y del rol de la aplicación                                                 | 0600          |
| `.local/cluster-owner.json`                           | marcador de propiedad del clúster (evita tocar uno ajeno)                                                                               | —             |
| `.local/provisioning-token`, `.local/enrollment-code` | autorizaciones locales del asistente y de enrolamiento (`cmd/ad-el-pos-server/local.go:17-33, 76-90`)                                   | 0600          |
| `.local/tls/`                                         | CA y certificados LAN (`tls-init`, `internal/localsetup/tls.go:28, 96`)                                                                 | 0700 / 0600   |
| `.local/server.log`, `.local/server.pid`              | salida JSON de Go (`slog` a stdout, `main.go:31`) y PID                                                                                 | —             |
| `backups/*.dump`                                      | respaldos `pg_dump` custom (`database.py:164-199`)                                                                                      | 0600          |
| `bin/ad-el-pos-server`                                | binario del servidor                                                                                                                    | —             |

Terminal (aplicación): configuración de conexión en este orden (`connection.rs:64-79`): `ADEL_TERMINAL_CONFIG`; si no,
`connection.json` en el directorio de configuración de la aplicación (`app_config_dir()` de Tauri; en esta Mac no
existe); si no, el `connection.json` embebido en la compilación (`http://127.0.0.1:8181`, `server-terminal`). Las
preferencias de usuario y de terminal se guardan en el servidor, no en el equipo.

### 1.5 Secretos

- **Servidor**: solo archivos 0600 dentro de `.local/` (tabla anterior). No usa Keychain. `ADEL_DB_PASSWORD` puede
  sustituir la contraseña de la base (`internal/config/config.go:54`).
- **Terminal**: identificador y token de la terminal y token de sesión en **Keychain** mediante el crate `keyring`
  (`desktop/src-tauri/src/vault.rs:18-63`): servicio = `bundle_id` de `product.json`, cuenta = `"{server_url}|{perfil}"`,
  valor JSON. Si el almacén falla **no** hay alternativa en archivos ni en `localStorage` (`vault.rs:54-57`). La apertura
  corre en otro hilo porque macOS puede pedir consentimiento para una compilación nueva (`main.rs:144-155`). Los tokens
  nunca llegan al WebView.
- `sa` se aprovisiona solo en una terminal interactiva (`provision-sa`/`reset-sa`, `/dev/tty`, ver 2.3).

### 1.6 Instalación y actualización hoy

No hay instalador: «instalar» es compilar en la carpeta del repositorio y ejecutar desde ahí. Primera vez:
`install` → `db-init` → `migrate` → `prepare-setup` → `server-start` → `desktop`/`open-app` y el asistente
(`DEVELOPMENT_MACOS.md`). Actualizar: `backup` → `python3 scripts/restore-check.py backups/…` → `server-stop` →
`migrate` → `server-start` → `build-macos` → `open-app`. Producción está bloqueada por diseño
(`licensing.CheckStartup`: solo `environment=development` con `license_mode=development`).

## 2. Qué depende de macOS y qué cambiar en Windows

Orden sugerido para quien prepare el instalador: base de datos y servicio de Go primero (sin ellos no hay nada que
probar), luego impresión y cajón, después la aplicación de escritorio y al final scripts y pruebas.

### 2.1 PostgreSQL

- **Hoy**: Homebrew `postgresql@18` en rutas fijas (`database.py:17-26`), `initdb --locale=C --auth=scram-sha-256`,
  `unix_socket_directories` en `.local/socket` (`database.py:106-112`), `pg_ctl` desde Python; comandos `pg_dump`,
  `pg_restore`, `createdb`, `dropdb`, `psql` desde `PG_BIN`.
- **Windows**: elegir binarios de PostgreSQL 18 para Windows (instalador o binarios incluidos en el instalador propio),
  registrar el clúster como **servicio de Windows** con un usuario propio, `listen_addresses = '127.0.0.1'` y sin socket
  Unix (la aplicación ya conecta por TCP `127.0.0.1:55432`). `ADEL_PG_BIN` ya permite otra ruta. Revisar que
  `--locale=C` y `timezone='UTC'` se conserven (las consultas y los reportes asumen UTC en la base).
- **Riesgo**: `database.py` usa `os.uname()` (no existe en Windows) y rutas Unix; hay que portar `db-init`, `backup`,
  `test-integration` o sustituirlos por el instalador.

### 2.2 Servidor Go como servicio

- **Hoy**: proceso independiente con `server-process.py` (`ps`, `SIGTERM`, `start_new_session`); Go escucha
  `os.Interrupt` y `syscall.SIGTERM` (`cmd/ad-el-pos-server/main.go:82`) y apaga con `Shutdown` de 5 s. Con un flujo SSE
  abierto el apagado agota esos 5 s y termina con código 1 (riesgo conocido, `CLAUDE.md` §19).
- **Windows**: `syscall.SIGTERM` compila pero Windows no lo envía; un servicio recibe la orden de parada del
  Administrador de servicios. Opciones: envolver `serve` con `golang.org/x/sys/windows/svc` (el módulo `golang.org/x/sys`
  ya está en `go.mod` como indirecto; su uso directo requiere cambiar `go.mod`) o un envoltorio externo. Antes conviene
  resolver el apagado con SSE abierto (código 1) para que el servicio no se marque como fallido.
- La salida es JSON por stdout: un servicio necesita redirigirla a archivo o al Visor de eventos.
- `--config` por omisión es la ruta **relativa** `.local/server.json` (`main.go:45`): el servicio debe pasar una ruta
  absoluta (p. ej. bajo `%ProgramData%`).

### 2.3 Comandos interactivos de soporte

`provision-sa` y `reset-sa` abren `/dev/tty` (`cmd/ad-el-pos-server/local.go:38-58`), que no existe en Windows; habría
que usar `CONIN$`/`CONOUT$` (por comprobar con `x/term` en Windows). Son solo del propietario.

### 2.4 Permisos de archivos

Go crea secretos con `0600`/`0700` (`local.go:25, 77, 82`; `localsetup/tls.go:28, 96`) y los scripts con
`os.open(…, 0o600)`. En Windows esos bits no protegen el archivo (solo influyen en «solo lectura»): el instalador debe
fijar ACL de la carpeta de datos (servicio + administradores). La prueba `internal/localsetup/tls_test.go:57` comprueba
`Perm() == 0600` y fallaría en Windows.

### 2.5 Impresión (CUPS) y cajón

- **Hoy**: todo pasa por la interfaz `kitchen.Transport` (`Submit`, `internal/kitchen/transport.go:26-28`) y la
  inspección por `QueueInspector` (`internal/kitchen/cups_raw.go:53-56`); la ejecución de comandos por `commandRunner`
  (`cups_raw.go:23-40`). La implementación del equipo (`HostTransport`) usa rutas fijas: `/usr/bin/lpstat -a`
  (descubrir colas, `transport.go:55`), `/usr/bin/lpoptions -p … -l` (67, 110), `/usr/bin/lpstat -p` (100),
  `/usr/bin/lp` para texto (139), para documentos con formato en raw (`transport_formatted.go:138`) y para el pulso del
  cajón (`transport.go:319`); detección de colas raw `cups_raw.go:86-89`. Archivos temporales con
  `os.CreateTemp("", …)` (`transport.go:127`). Usan CUPS además `payments/cash_drawer.go` (cajón) y
  `designer/samples.go:187` (muestras).
- Ya **independiente de macOS**: ESC/POS por TCP (`escpos_tcp`, puertos 9100-9109, IP privada) para tickets, comandas y
  cajón. El esquema reserva el transporte `agent` (`migrations/0005…sql:21`; hoy responde «agente de impresión futuro»,
  `transport.go:94`).
- **Windows**: implementar un `Transport` y un `QueueInspector` con la cola de impresión de Windows (datos RAW por la
  API del spooler) o restringir a ESC/POS TCP. El **cajón por USB** hoy solo funciona como cola CUPS raw: en Windows
  requiere el equivalente RAW del spooler. Las reglas (sin reintento automático, `uncertain` exige intervención,
  aceptación ≠ papel) no cambian.
- Textos visibles con «CUPS» o «macOS»: `pages/kitchen/PrintersAdmin.tsx:68-69, 81-82, 92, 119, 166, 184-185, 284`,
  `pages/cash/CashHardwareSettings.tsx:132, 142`, `pages/documents/IdentityPanel.tsx:113`,
  `pages/documents/designer/BlockProperties.tsx:190`, `SampleDialog.tsx:24`, `pages/kitchen/PrintQueue.tsx:46`. El
  campo `transport` guarda `cups` en la base: decidir si «cola del sistema» reutiliza ese valor o agrega otro (CHECK de
  `kitchen_printers.transport` y `kitchen_printers_accepts_escpos_check`).

### 2.6 Keychain

`keyring = "=3.6.3"` ya declara `features = ["apple-native", "windows-native"]` (`desktop/src-tauri/Cargo.toml`) y
`Cargo.lock` incluye `windows-sys` para ese crate: en Windows usaría el **Administrador de credenciales** sin cambiar
`vault.rs` (por comprobar: límites de tamaño del valor y nombre del destino). La prueba real de Keychain solo existe
para macOS (`vault.rs:66-98`). Textos: «Revisa el diálogo de macOS» (`main.rs:33`) y «acceso al Llavero»
(`desktop/src/App.tsx:281`) deben ser neutros.

### 2.7 Menú nativo

`desktop/src-tauri/src/menu.rs` arma la lista cerrada `MENU_ACTIONS`; el submenú «AD-EL POS» solo existe en macOS
(`Platform::current`, `menu.rs:189-203`; `layout` 248-…); los aceleradores usan `mac`/`other` por acción. En Windows el
menú **no se compiló ni probó**; los aceleradores pueden no llegar al menú con el foco en el WebView (la capa de
atajos de React los atiende, `commands/keyboard.ts`, y el despachador descarta la gemela en 700 ms). Decidir si en
Windows se muestra barra de menú.

### 2.8 Atajos y pantalla completa

`desktop/src/commands/platform.ts:5-38`: plataforma por `navigator.platform`/`userAgent`; ⌘ en macOS y Ctrl en el resto;
pantalla completa ⌃⌘F en macOS y **F11** en Windows. `tauri.conf.json` abre en pantalla completa (`fullscreen: true`);
`WindowControls` cambia con `window_set_fullscreen` (`main.rs:66-72`). Hay prueba de Vitest y de Rust de los atajos por
plataforma (`menu.rs:714`), no en Windows real.

### 2.9 WebView

macOS usa WKWebView; Windows usa **WebView2** (Chromium), que debe estar instalado o incluirse en el instalador. Las
pruebas Playwright Chromium se parecen más a Windows que las de WebKit. La CSP permite `ipc:` y `http://ipc.localhost`
(`tauri.conf.json`) y Go ya admite el origen `http://tauri.localhost` (`config/server.*.example.json`); por comprobar
en Windows.

### 2.10 Empaquetado de la aplicación

- `tauri.conf.json`: `bundle.targets` solo `app`; para Windows, `msi` o `nsis`. Solo hay `icons/icon.png`: el recurso de
  Windows suele requerir `icons/icon.ico` (por comprobar con `tauri build` en Windows).
- `main.rs:1` ya declara `windows_subsystem = "windows"` en compilación de liberación (sin consola).
- Identificador provisional `local.adel.pos.dev` (afecta el nombre del almacén de credenciales y el directorio de
  configuración): cambiarlo **pierde las credenciales guardadas** de las terminales; definir el definitivo antes de la
  primera instalación en Windows.
- Exportación de reportes: diálogo `tauri-plugin-dialog` (multiplataforma) y escritura atómica con temporal oculto
  `.adel-export-….part` y `fs::rename` (`export.rs:300-345`). En Windows el punto inicial no oculta el archivo y el
  reemplazo falla si el destino está abierto en otro programa (por comprobar).

### 2.11 Servicios en segundo plano

macOS: ninguno instalado (plantilla launchd sin instalar). Windows: PostgreSQL y Go como servicios con inicio
automático, en ese orden (Go espera la base: readiness `health/ready`). La aplicación no debe iniciarlos (regla de
arquitectura). Los trabajadores de impresión, el planificador de cocina y el de inventario viven dentro de Go
(`main.go:130-145`) y corren aunque nadie abra la aplicación. **Desde la etapa 9G** macOS sí tiene servicio
(LaunchDaemon `local.adel.pos.server` instalado por el `.pkg`, con el comando `host`); en Windows el servicio es de la
Fase W (sección 7).

### 2.12 Red local y TLS

`tls-init --lan-ip` crea la CA y el certificado en `.local/tls/` (`localsetup/tls.go`); la terminal remota confía solo
en el `ca_certificate` de su `connection.json` (`main.rs:115-129`). En Windows falta: regla del Firewall para el puerto
de Go en la red privada y ubicación de `ca.pem` en la terminal. Go rechaza escuchar en LAN fuera de desarrollo
(`config.go:77`). **Desde la etapa 9G** este camino queda como anterior: el servidor tiene su propia identidad
(certificado ECDSA y `server_id`), una escucha HTTPS de red en `:8443` con filtro de orígenes (`internal/lanlistener`)
y las terminales se emparejan con un código y fijan la huella del certificado (`pairing.rs`), sin CA que copiar. Lo que
falta en Windows está en la sección 7.

### 2.13 Scripts, herramientas y pruebas

- `scripts/dev.sh` (bash), `scripts/env.sh`, `scripts/database.py`, `server-process.py`, `browser-tests.py`,
  `restore-check.py` (Python con rutas y comandos Unix: `ps`, señales, `os.uname`).
- `tests/runtime_smoke.py`: `.app` de macOS (17), `osascript` para activar y cerrar (88, 104, 138, 189), `ps -axo` (51).
- `.tools/`: binarios de macOS; en Windows hay que instalar las mismas versiones (`docs/DEPENDENCIES.md`).
- `desktop/package.json` y Playwright no usan comandos de shell propios de Unix (`playwright.config.ts`).
- Pruebas Go con rutas `/usr/bin/lp*` usan ejecutores simulados (no ejecutan nada; pasan en cualquier sistema); la de
  permisos 0600 no (2.4). Las de integración necesitan PostgreSQL y `ADEL_TEST_DATABASE_URL`.

## 3. Abstracciones que ya existen

- **Rutas**: Go usa `path/filepath` (`main.go:80, 91`; `local.go:17, 76`); Rust `PathBuf` y `app_config_dir()`.
- **Impresión**: `kitchen.Transport`, `kitchen.QueueInspector`, `commandRunner`; transporte `agent` reservado en el
  esquema; ESC/POS TCP sin dependencias del sistema.
- **Credenciales**: `vault.rs` sobre `keyring` con el backend de Windows ya habilitado.
- **Plataforma en la interfaz**: `commands/platform.ts` (atajos y textos) y `menu.rs` (`Platform`, aceleradores por
  plataforma), con pruebas.
- **Configuración**: `ADEL_SERVER_CONFIG`, `ADEL_DB_PASSWORD`, `ADEL_TERMINAL_CONFIG`, `ADEL_PG_BIN`.
- **Separación de procesos**: la aplicación ya funciona contra un Go existente (modo `terminal-only`).

## 4. Riesgos y pendientes de comprobar

1. Apagado de Go con SSE abierto (5 s, código 1) frente al Administrador de servicios.
2. Impresión y cajón USB sin CUPS: requieren un transporte nuevo y prueba física.
3. ACL de la carpeta de datos (los bits 0600 no protegen en Windows).
4. Menú nativo, aceleradores y pantalla completa en WebView2: no probados.
5. Recurso de iconos `.ico` y destino del instalador (`msi`/`nsis`).
6. Identificador definitivo de la aplicación antes de la primera instalación (credenciales y configuración).
7. Migración de datos de una instalación macOS a Windows: hoy solo `pg_dump`/`pg_restore` custom; no hay herramienta.
8. Dependencias en línea: `go mod download`, `npm ci` y `cargo` necesitan red la primera vez (las cachés de macOS no
   sirven tal cual).

## 5. Detalles de la etapa 9D que afectan a Windows

Revisado en la fase F leyendo el código de las fases A–E y de las correcciones de la revisión (hasta `81ff531`).
**Nada de esto se probó en Windows.** La 9D no agrega dependencias, operaciones
nativas, permisos de Tauri ni textos con «macOS», «CUPS» o «Llavero».

- **Colores y cursor**: la paleta azul vive en los tokens de `desktop/src/styles.css` y usa CSS estándar; `color-mix()` ya
  se usaba antes de la 9D y requiere un WebView2 basado en Chromium 111 o posterior (por comprobar con el WebView2 que
  incluya el instalador). En Windows `cursor: not-allowed` es el círculo tachado del sistema y `cursor: progress` la
  flecha con el indicador de espera: comunican lo mismo que en macOS. Las pruebas solo compararon el estilo calculado en
  Chromium y WebKit, no el puntero real.
- **Barra superior a 1280 px**: la fuente de la interfaz es `-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`
  (regla `:root` de `styles.css`); en Windows se usará Segoe UI, con anchos de texto distintos a los de macOS. La barra se probó sin
  cortes a 1280 × 720 y 1920 × 1080 con las fuentes de macOS (Chromium y WebKit); en Windows hay que repetir esa prueba.
  Si falta espacio, solo se acorta el nombre del usuario (las cuatro secciones son columnas iguales).
- **Mapa de Mesas**: el desplazamiento por arrastre usa eventos de puntero con `setPointerCapture`
  (`components/restaurant/TableMap.tsx`) y el mapa operativo no fija `touch-action: none` (solo el diseñador), así que en
  una pantalla táctil el WebView puede tomar el desplazamiento nativo del contenedor (`overflow: auto`). Las barras siempre
  visibles usan `::-webkit-scrollbar` (`styles.css`, `.table-map-pannable`), que WebView2 admite mientras no se declaren
  `scrollbar-width` o `scrollbar-color` en ese elemento (Chromium ignora `::-webkit-scrollbar` si están). Por comprobar
  en una pantalla táctil con Windows 11 (tampoco se verificó en macOS con pantalla táctil).
- **Atajos**: sin cambios: siguen en `commands/platform.ts` (Ctrl en Windows; ⌘/Ctrl + 1–4 y ⌘5) y no se agregó ningún
  atajo que dependa de ⌘. Escape en Mesas vuelve al resumen del área y también cierra el menú de un producto del
  detalle de la cuenta (que se abre al tocar el renglón o «⋯», sin depender de hover).
- **Foco de los diálogos de Ayuda**: «Acerca de» y «Atajos de teclado» llevan el foco al diálogo antes de dibujar
  (`useLayoutEffect` en `commands/CommandDialog.tsx`) y tienen un oyente de Escape en el documento, por una carrera que se
  reprodujo en WebKit; en WebView2 no se probó.
- **«Sin conexión» y «Reintentar»**: dependen de que las peticiones del puente fallen (`api_request`), igual en cualquier
  sistema. Con Go como servicio de Windows (2.2), un servicio detenido se verá como «Sin conexión».
- **Migración 0021**: solo SQL de PostgreSQL 18 (`gen_random_uuid()`, `numeric(18,2)` y un disparador PL/pgSQL de solo
  inserción como los de la etapa 8); no depende del sistema. Aplicarla en Windows exige el mismo procedimiento de
  respaldo y ensayo de restauración, que hoy vive en scripts de macOS (`database.py`, `restore-check.py`; ver 2.1 y 2.13).
- **Cancelar cuenta y el aviso a cocina**: el aviso de cancelación de una cuenta viaja por la cola y los transportes de
  siempre (CUPS o ESC/POS TCP; ver 2.5): en Windows depende del transporte que se elija para la impresión. La autorización
  de supervisor es la de los cobros (Go), sin nada del sistema.
- **Exportación de los reportes nuevos** («Cuentas canceladas» y «Cancelaciones por motivo»): por la misma operación
  `export_report` y el diálogo de `tauri-plugin-dialog` (ver 2.10); sin cambios de plataforma.

## 6. Etapa 9F: qué verificar en Windows

La 9F (versión 0.9.5, esquema 22, migración `0022_printing_and_kitchen_stations.sql`) se desarrolló y se probó en la Mac
([ETAPA_9F_AVANCE.md](ETAPA_9F_AVANCE.md) y [VERIFICATION_STAGE9F.md](VERIFICATION_STAGE9F.md)). **Nada de la 9F se
ejecutó en Windows**: en cada fase con cambios de Go se compiló y se revisó el código para Windows
(`GOOS=windows GOARCH=amd64 go build ./...` y `go vet ./...`, también con `-tags=integration`), pero las pruebas que
solo existen en Windows (`internal/standalone/testmode_test.go`) solo compilaron y `installed-smoke.ps1` ni siquiera se
pudo analizar (en la Mac no hay PowerShell). Tampoco se generó el instalador 0.9.5. Lo propio de cada sistema sigue
detrás de condiciones por plataforma. Lista para la próxima sesión en Windows, en este orden:

1. **Suites en Windows** (`scripts\dev.ps1`): `test`, `test-integration` y `test-browser` con Chromium y WebKit.
   `scripts\windows\dev.mjs` escribe ahora `test_mode: true` y `backup_directory` en la configuración temporal de
   Playwright y el puente de las pruebas se niega a correr sin `test_mode`: comprobar que `test-browser` arranca y que
   `workflow-zz-test-data` corre al final. Ejecutar también `go test ./internal/standalone` (incluye
   `testmode_test.go`) y comprobar que la subprueba `9F_respaldo_real_con_pg_dump` de `administration` encuentra
   `pg_dump` (`ADEL_PG_BIN`) y no se omite.
2. **`host` con `ADEL_TEST_MODE`** (`internal/standalone/testmode.go` y `cmd/ad-el-pos-server/host_windows.go`):
   - Con `ADEL_TEST_MODE=1`, `host`, `backup`, `restore-check` y `restore` deben negarse a usar
     `%LOCALAPPDATA%\AD-EL POS`, también si la variable `LOCALAPPDATA` apunta a otro lugar (se compara con la carpeta
     conocida del sistema, `FOLDERID_LocalAppData`, no con la variable) y con otra forma de escribir la misma ruta.
   - Con otra carpeta, `host` arranca y `GET /api/v1/version` informa `test_mode: true`; con la variable vacía o
     ausente nada cambia; con otro valor, error.
   - `server_host.rs` no limpia el entorno del proceso hijo: la variable puesta al abrir `ad-el-pos-desktop.exe` debe
     llegar al `host`.
3. **Prueba de humo con carpeta temporal** (`scripts\windows\installed-smoke.ps1`, UTF-8 con BOM, y
   `desktop\e2e-installed\installed-smoke.spec.ts`):
   - Se niega si la aplicación real está abierta o si los puertos 8181, 55432 o 9222 están ocupados.
   - Abre la aplicación instalada con `LOCALAPPDATA` temporal, `ADEL_TERMINAL_CONFIG` con un perfil propio
     (`humo-<12 hex>`) y `ADEL_TEST_MODE=1`; la prueba exige `test_mode: true` antes del primer paso.
   - Al terminar, también si falla: cierra la aplicación en orden (fuerza lo que quede tras 60 s), borra la carpeta
     temporal y la credencial `http://127.0.0.1:8181|humo-….local.adel.pos.dev` (comprobar con `cmdkey /list`), avisa
     si el clúster real quedó en marcha e informa lo que no pudo limpiar.
   - Después, `%LOCALAPPDATA%\AD-EL POS` debe seguir igual (sin establecimiento, impresoras ni ventas nuevas) y la
     aplicación real debe abrir con su sesión de siempre.
   - Usa como impresora de tickets la de la aceptación («Tickets controlados E2E», cola del equipo
     `ADEL_E2E_SIN_IMPRESORA`, que no existe): comprobar que la etiqueta de ese destino en Windows se reconoce (se
     escribió con una expresión regular, sin ejecutarse).
4. **Actualizar la instalación de la 9E a la 0.9.5** (`scripts\build-windows.ps1` y el instalador encima de la 0.9.4):
   antes, revisar los roles propios que tengan «Actualizar preparación» (`kitchen.states.manage`), por el cambio de
   significado de las estaciones por terminal (punto 9). El `host` debe respaldar antes de migrar
   (`…-antes-de-migrar.dump`) y aplicar la 0022; después, `api/v1/version` con 0.9.5 y esquema 22, los datos intactos y
   las impresoras que ya existían con corte total y, si ya cortaban, avance «Como hasta ahora» (los bytes de siempre).
5. **«Quitar datos de prueba» sobre la instalación de Windows de la 9E** (Administración › Sistema › Mantenimiento; la
   ejecuta el propietario, con la 0.9.5 instalada):
   - **Qué revisa**: solo lo que coincide con una firma exacta de `backend/internal/maintenance/signatures.go`. En esa
     instalación, la impresora «Destino controlado E2E» (tipo agente, destino «Agente de aceptación no disponible»), las
     estaciones «Cocina caliente» y «Freidoras» que solo usan esa impresora, el producto «Filete de prueba» (`FILETE`),
     la categoría «Platillos» que lo contiene y la ruta de «Platillos» a esas dos estaciones.
   - **Qué hará**: lo que ya tiene registros relacionados (las ventas del filete, las tareas de cocina, los trabajos de
     impresión) **se desactiva** en lugar de borrarse (el producto además se archiva); lo que no tiene registros se
     elimina; se **desvinculan** la impresora de tickets de Cobros (que era ese destino) y lo que apunte a esos datos
     (cajón, impresora de respaldo, estaciones por terminal y por usuario). La vista previa dice qué se elimina, qué se
     desactiva, qué se desvincula y qué se conserva, con el motivo de cada uno: revisarla antes de confirmar.
   - **Qué conservará**: el establecimiento «Fonda Doña Ñeli», los usuarios, el área «Salón principal» con «Mesa 1» y
     «Mesa 2», «Pago en dólares» y toda la operación de la prueba de humo (turno, cobros en efectivo, dólares y tarjeta,
     descuento, cancelación, corte parcial y cierre): ventas, pagos, cortes y movimientos de inventario nunca cambian.
   - **No deja la instalación «limpia»**: seguirá teniendo el establecimiento de prueba y su operación. Lo limpio es
     reinstalar con otra carpeta de datos (decisión D1 del avance); el procedimiento para hacerlo en Windows conservando
     el respaldo de la instalación de prueba no está escrito ni probado.
   - Antes de quitar nada se crea un respaldo con la etiqueta `antes-de-quitar-datos-de-prueba` (`standalone.Backup`):
     comprobar que aparece en «Respaldos de AD-EL POS» y que el aviso final nombra el respaldo.
6. **Aviso sin impresora válida**:
   - Mientras la impresora de tickets siga siendo el destino de prueba (tipo agente), imprimir una precuenta debe avisar
     en el momento («Esta impresora todavía no puede usarse para imprimir.») sin encolar nada, con «Configurar
     impresora» (lleva a Cobros › Políticas de cobro) y «Reintentar»; sin impresora de tickets, «No hay impresora
     configurada para precuentas.». Antes de la 9F salía «Requiere intervención. Se resuelve en Cocina → Cola de
     impresión».
   - Un cobro sin impresora válida se registra igual y su resultado explica por qué no salió el ticket, con los mismos
     botones; lo mismo los documentos de Caja.
   - Al elegir la impresora de tickets ya no se admiten impresoras de tipo agente ni inactivas.
7. **Cola de impresión** en WebView2, al 100 % y al 125 %: lista con altura limitada y desplazamiento propio; «Consultar
   trabajo e intentos» muestra el detalle a la vista junto a la lista; sin la referencia técnica suelta (el «.15150») ni
   el texto del agente; los repetidos agrupados con «×N»; un trabajo en error se reintenta con un toque y sin motivo, y
   reimprimir lo que ya salió pide motivo. Con una impresora USB apagada, el aviso debe decirlo en palabras sencillas.
8. **Corte de papel por la cola de Windows (RAW)**, con una impresora real (en la computadora de la 9E solo había
   «Microsoft Print to PDF», que se rechaza como virtual; los bytes por la cola de Windows solo se probaron con el
   administrador de impresión simulado):
   - Impresora de 80 mm **con cortador**: «Cortar el papel al terminar» con corte total y parcial y 0, 4 y 8 renglones
     de avance, con «Imprimir prueba de ancho» (también corta, con la configuración guardada); después un ticket, una
     precuenta, una comanda, un aviso de cancelación y un corte de caja: el corte no debe caer sobre el texto.
   - Impresora **sin cortador** (o de 58 mm): con el corte activado no debe imprimir caracteres extraños al final (la
     mayoría ignora `GS V`); si los imprime, anotar el modelo y desactivar el corte.
   - Una impresora nueva de 80 mm empieza con el corte activado y 4 renglones; una de 58 mm, sin corte.
   - Lo mismo por ESC/POS en red, si hay una impresora de red.
9. **Estaciones por terminal y por usuario** (Cocina › Estaciones › Terminales de cocina y Personal › Usuarios ›
   Estaciones de cocina; guía en [TERMINAL_DE_COCINA.md](TERMINAL_DE_COCINA.md)):
   - **Cambio de significado**: hasta la 9E, una terminal sin estaciones asignadas negaba Cocina a quien no tenía
     «Operar todas las estaciones del establecimiento» (`kitchen.stations.all`); desde la 9F, una terminal sin selección
     muestra todas, y quien tiene ese permiso queda limitado a las estaciones de su terminal en Preparación y en los
     cambios de estado (liberar a mano, programar, la cola, la reimpresión y el historial siguen con el permiso). Por eso
     conviene revisar los roles propios con «Actualizar preparación» antes de instalar (punto 4). Go rechaza una lista
     de estaciones vacía sin «todas».
   - Con la única terminal de Windows (varias terminales por red no están preparadas allí): con pantalla inicial
     «Cocina», abre en Preparación; con dos de tres estaciones en la terminal y una cocinera con una, ella ve solo la
     intersección; sin estaciones en común, el aviso en lugar de una pantalla vacía; su barra solo tiene Cocina y su
     menú.
   - Modo cocina en WebView2 a 1280 × 720, al 125 % y en pantalla completa: tarjetas grandes, botones de 64 px, ámbar
     con «Por vencer» y rojo con «Atrasada», sin «Actualizar».
   - Una cocinera sin `kitchen.stations.read` ve los cambios de su alcance con el sondeo de 5 s (no verificado en
     ningún sistema). Dos terminales de cocina a la vez no se pueden probar en Windows hoy.
10. **Sonido en WebView2**: activar «Aviso sonoro al llegar una comanda nueva» y, con Preparación abierta, hacer que
    llegue una comanda nueva (por ejemplo, un pedido programado que se libera solo): dos tonos cortos (Web Audio, sin
    archivos). WebView2, como WebKit, no reproduce sonido antes de que el usuario toque la pantalla: el sonido se activa
    con el primer toque y la pantalla avisa si no puede sonar; comprobar las dos cosas y que no suena al abrir la
    pantalla ni al cambiar de filtro. No se verificó en ningún equipo real.
11. **Barra compacta al 125 % con WebView2**: ventana de unos 1500 px al 125 % (≈ 1200 px de página), donde se observó
    el problema:
    - Isotipo «AE» nítido (40 px a partir de un PNG de 128 px) en lugar de la «A»; Caja con «Abierto», «En arqueo» o
      «Cerrado»; «En línea» con su texto; solo el nombre del usuario se acorta, con puntos suspensivos; sin
      desplazamiento horizontal; ayudas emergentes de Caja y del estado del servidor.
    - **Foco del logotipo**: tocar o hacer clic en el logotipo y después pulsar Escape, una flecha o Ctrl+2, o abrir y
      cerrar la paleta (Ctrl+K): no debe aparecer el contorno de foco; con Tab sí, e Intro vuelve al inicio. La
      reproducción se hizo con el Chromium de Playwright (el motor de WebView2), no con WebView2.
    - Leyenda del mapa de Mesas en dos renglones, sin barra horizontal.
    - Repetir a 1280, 1440 y 1920 px de página con Segoe UI (las medidas de la Mac usaron sus fuentes).
12. **Nombre de la precuenta**: en Administración › Documentos, elegir «Ticket de venta» y después «Cuenta»: botones del
    panel de mesa, la captura y el cobro, los avisos, la paleta (Ctrl+K) y el encabezado impreso. En Windows no hay menú
    nativo, así que el texto de «Imprimir precuenta» del menú no aplica.
13. **Cancelar unidades con supervisor y ruta `void` retirada**: no dependen del sistema; basta con la corrida de
    `test-browser` del punto 1.
14. **Impresora de tickets que no existe en Windows**: elegir en Cobros una impresora de la cola del equipo cuyo nombre
    no esté instalado en Windows y pedir una precuenta: Go debe detectar `ERROR_INVALID_PRINTER_NAME` (solo compilado en
    la 9F) y el aviso debe decir «No se encontró la impresora …» con «Configurar impresora» y «Reintentar» (no «no
    responde»). Tras corregirla en Cobros, «Reintentar» debe imprimir en la impresora vigente.
15. **Estaciones por terminal desde un cliente anterior**: `PUT kitchen/access` con una lista vacía sin «todas» responde
    `422 KITCHEN_ACCESS_ALL_REQUIRED` (la aplicación 0.9.5 siempre envía la marca); comprobarlo con la corrida de
    `test-integration` del punto 1.

## 7. Etapa 9G: qué hacer en la Fase W

La parte Mac de la 9G (fases 0 a G; versión 0.10.0, esquema 23, migración `0023_network_terminals_and_android.sql`) se
desarrolló y se probó en la Mac ([ETAPA_9G_AVANCE.md](ETAPA_9G_AVANCE.md) y
[VERIFICATION_STAGE9G.md](VERIFICATION_STAGE9G.md)). **Nada de la 9G se ejecutó en Windows**: Go se compiló y se revisó
con `GOOS=windows` (también con `-tags=integration`) en cada fase, pero Rust no se compiló para Windows (en la Mac no
está el destino `x86_64-pc-windows-msvc`). Lo propio de cada sistema está detrás de `//go:build`, `#[cfg(...)]` o
`isWindows()`, con los puntos de integración marcados en el código. La Fase W se ejecuta en la computadora con Windows
11 con la especificación (§8 de [etapas/ETAPA_9G.md](etapas/ETAPA_9G.md)): `git pull`, leer el avance y seguir esta
lista. Al terminar, actualizar esta sección, [INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md) e
[INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).

**Paradas (§0 de la especificación)**: crear la regla del firewall, cambiar la configuración de red o del router,
instalar o registrar algo que pida permisos de administrador (avisar qué ventana aparecerá) y reiniciar Windows se
consultan antes con el propietario.

1. **Compilar y correr las suites** (`scripts\dev.ps1`): `test`, `test-integration` (la tupla ahora incluye
   `./internal/apicontract`, `./internal/backupschedule`, `./cmd/ad-el-pos-server` y `./internal/discovery`; las
   pruebas de integración de estos dos últimos llevan `!windows` y `darwin`, así que en Windows no se compilan y hay
   que escribir las del servicio de Windows) y `test-browser` con
   Chromium y WebKit (las especificaciones `workflow-stage9g-*` y `workflow-zz-test-data` al final). `cargo test`
   compila por primera vez en Windows `installation.rs`, `pairing.rs`, `discovery.rs`, `print_agent.rs` y
   `backup_folder.rs` (solo difieren por `cfg!`/`#[cfg]`).
2. **Servidor como servicio de Windows** con el mismo `host` (`cmd/ad-el-pos-server/host_windows.go`): arranque
   automático al encender, sin iniciar sesión, y reinicio si se cae. Falta: el control del administrador de servicios
   (`HostControl`, que en macOS dice `managed` y permite «Reiniciar» desde Mantenimiento), la vigilancia de la modalidad
   y del archivo de habilitación (`installation-mode.json` y `service-enabled`, `internal/standalone/mode.go`) y
   `pauseServerForRestore` (hoy `restore` pide cerrar la aplicación; en macOS pausa y reanuda el servicio con el archivo
   de habilitación).
3. **Datos en `%ProgramData%\AD-EL POS\`** (`internal/standalone/layout_windows.go`, hoy `%LOCALAPPDATA%\AD-EL POS`,
   que sigue siendo el de la instalación por usuario de la 9E con `server_host.rs`), con ACL privada (`fileacl`) y la
   comprobación de `ADEL_TEST_MODE` contra la carpeta conocida nueva.
4. **Instalador para todo el equipo** (`scripts\build-windows.ps1` y NSIS): en `Archivos de programa`, con permisos de
   administrador, que pregunta la **modalidad** (Servidor y terminal, Solo servidor, Solo terminal; «Solo terminal» no
   instala PostgreSQL ni el servicio) y registra el servicio. En Rust, `ServicePaths::system()` y `Context::detect` de
   `installation.rs` para Windows (hoy `None`: `chosen` siempre `true` y `can_change` `false`), para que la elección
   del primer arranque y «Cambiar la modalidad» de Mantenimiento funcionen igual que en macOS. `server_host.rs` solo
   lanza el servidor en `server-terminal` (con `server-only` no lo haría) y con el servicio de Windows deja de hacer
   falta lanzarlo desde la aplicación. Desinstalar conserva los datos.
5. **Regla del firewall** para el puerto 8443 **solo en redes privadas**, cuando la modalidad incluye servidor
   (**parada**). Comprobar también si el anuncio mDNS necesita algo más (UDP 5353 entrante en redes privadas).
6. **Escucha de red en el `host` de Windows**: hoy crea la identidad (`identity/` con ACL privada) pero no abre la
   escucha HTTPS `:8443` (`hostNetworkAddress` es de `host_unix.go`). Abrirla con la misma regla que en macOS (solo con
   una modalidad con servidor; en modo de pruebas, solo con el indicador).
7. **Pasar la instalación por usuario de la 9E** a la del equipo: el instalador nuevo detecta la instalación de
   `%LOCALAPPDATA%\AD-EL POS`, la respalda y pasa los datos con `ad-el-pos-server adopt-legacy --from-config
"%LOCALAPPDATA%\AD-EL POS\server.json"` (lee la base anterior con su propio rol, no la modifica, compara los conteos
   de todas las tablas y deja `legacy-adopted.json`), verifica y solo entonces desinstala la versión por usuario. Nunca
   borra los datos sin haber verificado la copia. La computadora de la 9E tiene el establecimiento de prueba «Fonda
   Doña Ñeli»: sirve para ensayar este paso.
8. **Anuncio en la red** (`internal/discovery`): `announce_windows.go` con `DnsServiceRegister` /
   `DnsServiceDeRegister` de dnsapi.dll (Windows 10+), con el mismo `Announcer` y sin abrir un socket propio en el
   puerto 5353 (`announce_other.go` tiene el punto de integración). Hasta entonces un servidor de Windows no se anuncia
   y las terminales escriben su dirección.
9. **Búsqueda desde una terminal de Windows** (`discovery.rs`, `std::net::UdpSocket` a `224.0.0.251:5353`): comprobar
   que lista la Mac; si no, revisar el firewall de Windows para UDP 5353 entrante en redes privadas.
10. **Emparejamiento y credencial en Windows**: `pairing.rs` en Windows y el Administrador de credenciales con la cuenta
    nueva `adel-server:{server_id}|{perfil}`; emparejar de verdad Mac ↔ Windows con el código, revocar desde la otra
    computadora y comprobar el filtro de orígenes con una terminal en la Wi-Fi (aceptada) y otra por una VPN de túnel
    (rechazada).
11. **Agente de impresión en Windows** (`print_agent.rs`, que hoy no arranca en Windows): `local_printers` con
    `EnumPrinters` y la regla de `kitchen/windows_queue.go` (USB, COM, LPT y TCP/IP estándar; nunca virtuales, de otro
    equipo ni WSD) y `deliver` con `StartDocPrinter` tipo `RAW` + `WritePrinter`, con los mismos resultados (no se pudo
    abrir → error confirmado; falló después de abrir → incierto); después quitar el `cfg!(target_os = "windows")` de
    `start`. Probar la USB de una terminal Windows con la Mac como servidor y al revés.
12. **Respaldos programados en el `host` de Windows** (ya usan `installationBackup`, sin probar): `fileacl.CreatePrivate`
    en Windows (ACL privada con el archivo abierto y `pg_dump` hacia la salida estándar en binario); una USB FAT32 o
    exFAT, donde la ACL no se puede poner y el respaldo quedaría `dump_failed` (decidir si en una carpeta elegida se
    acepta sin ACL o se usa la predeterminada); rutas con acentos; `Describe` muestra la unidad (`E:`) como disco; y el
    diálogo nativo de carpeta (`choose_backup_directory`).
13. **Pruebas cruzadas** (§8.2, con el propietario en la misma red y una lista clara de lo que hace en la Mac):
    - Mac como servidor (servicio con la escucha `:8443`; `dns-sd -B _adelpos._tcp` en la Mac confirma el anuncio) y
      Windows como terminal: búsqueda y emparejamiento con código; mesas, captura, envío a cocina y cobro desde
      Windows con los cambios vistos al momento en la Mac y al revés; dos terminales en la misma cuenta; apagar la red
      de la terminal (la capa «Sin conexión» en ≤ 5 s, lo capturado se conserva y se guarda al volver); revocar la
      terminal; impresión a una impresora de red y a su USB si la hay.
    - **Cambio de IP real** de la computadora principal (renovar la concesión DHCP o reservar otra IP en el módem:
      **parada**, es configuración de red): la terminal debe seguir sola (la primera operación tarda ≈ 2 s) y guardar
      la dirección nueva; otro servidor AD-EL POS con el mismo nombre en la red nunca debe aceptarse.
    - Windows como servidor y Mac como terminal: lo mismo en sentido contrario.
    - **Reinicio de la computadora principal** (avisar antes si es Windows): el servicio arranca solo y las terminales
      se reconectan solas.
    - **Prueba de carga moderada**: varias terminales y clientes tipo comandero por la API durante unos minutos;
      registrar tiempos de respuesta y comprobar que no hay duplicados ni inconsistencias. Tener en cuenta que todas
      las escrituras comparten el lock 7820148305, que cada flujo SSE consulta cada 750 ms y que cada agente de
      impresión consulta cada 3 s.
14. **Documentación**: el resultado de las pruebas cruzadas y de la carga, la migración de la instalación por usuario y
    las limitaciones van al avance; actualizar `CLAUDE.md` (§3, §5, §17 y §19), esta sección,
    [INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md) e [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md).
