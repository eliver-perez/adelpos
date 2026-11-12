# Notas para Windows · AD-EL POS

Punto de partida para preparar el instalador de Windows 11 (etapa posterior, con un prompt aparte). Recoge cómo se
empaqueta y se ejecuta hoy AD-EL POS en macOS y qué partes dependen de macOS, con el archivo y la línea de cada una.
Se escribió en la Fase 0 de la etapa 9D (versión 0.9.2, esquema 20) leyendo el código; **nada se compiló ni se probó en
Windows**. Lo marcado «por comprobar» es una expectativa razonable, no un hecho verificado.

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
(`main.go:130-145`) y corren aunque nadie abra la aplicación.

### 2.12 Red local y TLS

`tls-init --lan-ip` crea la CA y el certificado en `.local/tls/` (`localsetup/tls.go`); la terminal remota confía solo
en el `ca_certificate` de su `connection.json` (`main.rs:115-129`). En Windows falta: regla del Firewall para el puerto
de Go en la red privada y ubicación de `ca.pem` en la terminal. Go rechaza escuchar en LAN fuera de desarrollo
(`config.go:77`).

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
