# Etapa 9G · Avance

Servidor del restaurante, terminales en red y preparación para Android. Especificación del propietario:
[etapas/ETAPA_9G.md](etapas/ETAPA_9G.md) (de solo lectura). Rama `etapa-9g`, creada desde `main` (0.9.5, esquema 22,
con la 9F fusionada). Las fases 0 a G se trabajan en la Mac (Apple Silicon, macOS 26.5.2); la Fase W, en la computadora
con Windows 11, en otra sesión. Al terminar, la versión será 0.10.0.

**Forma de trabajo**: un coordinador y como máximo 2 agentes a la vez, sin subagentes de subagentes; la Fase 0 la hace un
solo agente. Las pruebas corren en una copia aislada en el directorio temporal de la sesión, con un clúster PostgreSQL
temporal (puerto 55499) y Go temporal (8183, vista previa 1421); nunca contra la instalación real (`adel_pos`, `.local`,
Keychain, servidor 8181). Regla de autonomía de `CLAUDE.md` §0 y, además (§0 de la especificación), detenerse y preguntar
si hay que instalar o registrar algo que pida la contraseña de administrador del sistema (avisando qué ventana aparecerá),
cambiar el firewall, el router o la configuración de red, o reiniciar la computadora.

## Fase 0 · Verificación contra el código

Un solo agente, de solo lectura (13/11/2026). Citas `archivo:línea` relativas a la raíz del repositorio. Lo que pide §1
de la especificación (puntos 1–8) y, más breve, lo necesario para planear (9–13).

### Estado de la instalación real al hacer la verificación

La Mac se reinició hacia las 16:50 (hora local; `uptime` 2:09 a las 19:00). **Ni PostgreSQL (55432) ni Go (8181) están
corriendo**: `lsof -iTCP -sTCP:LISTEN` solo muestra un clúster temporal de esta sesión en 55499; `.local/server.pid`
apunta a un proceso que ya no existe y `.local/postgres/postmaster.pid` es del 8/11. Nada los relanza después de
reiniciar (ver punto 3). No se tocó nada: arrancarlos (`db-start`, `server-start`) actúa sobre la instalación real.

**Al terminar la Fase 0** el coordinador volvió a arrancar la instalación real con el mismo código de `main` (la rama
aún no tenía cambios de código): `db-start` (PostgreSQL recuperó el cierre del reinicio) y `server-start` → readiness 200,
`api/v1/version` 0.9.5 / esquema 22, `test_mode: false`. La ventana de la aplicación no se abrió. Así la instalación
vuelve al estado previo al reinicio y los puertos reales quedan ocupados, de modo que ninguna prueba pueda usarlos por
error.

### 1. Conexión de la aplicación de escritorio con el servidor

- **Dirección configurable, por archivo local.** `ConnectionConfig` = `installation_mode`, `server_url`,
  `credential_namespace` y `ca_certificate` (`desktop/src-tauri/src/connection.rs:4-13`). Orden: `ADEL_TERMINAL_CONFIG`
  → `connection.json` en la carpeta de configuración de la aplicación (`~/Library/Application Support/local.adel.pos.dev/`,
  hoy no existe en la Mac) → `desktop/src/generated/connection.json` incrustado en el binario (`connection.rs:64-79`),
  que es `server-terminal` + `http://127.0.0.1:8181`. React no puede cambiar el destino. Reglas: solo `http(s)` sin
  usuario, ruta ni parámetros; host `localhost` o IP literal privada o de loopback; **HTTP solo a loopback** y la LAN
  exige HTTPS (`connection.rs:28-56`); el escritorio rechaza `server-only` (`connection.rs:22-27`).
- **Puerto**: 8181 (Go real); Go escucha en **una sola** `listen_address` IP:puerto (`backend/cmd/ad-el-pos-server/main.go:179`).
- **Autenticación**: sin cookies ni CSRF. Rust agrega `X-Device-ID`, `X-Device-Token` y `Authorization: Bearer <sesión>`
  (`desktop/src-tauri/src/api.rs:760-767`, `realtime.rs:70-77`); los tokens viven en Keychain y se quitan de toda
  respuesta antes de llegar a React (`api.rs:974-980`). Go valida en cada petición dispositivo
  (`backend/internal/administration/service.go:277-294`: SHA-256 de un token de 256 bits, `DEVICE_REQUIRED`,
  `DEVICE_REVOKED`, actualiza `devices.last_seen_at`) y sesión (`service.go:296-339`: `SESSION_*`, permisos efectivos
  leídos de PostgreSQL en cada petición). Sesión de 12 h y **una sola sesión activa por usuario** en todo el sistema:
  iniciar sesión en otra terminal revoca la anterior (`SESSION_REPLACED`; `administration/auth.go:96-117`, índice
  `sessions_one_active_user_idx`, `backend/migrations/0002_identity_and_administration.sql:96`). Login con límite de 5
  fallos/15 min por usuario y terminal (`auth.go:72-81`). CORS: lista exacta de orígenes y prevuelo solo `GET`
  (`backend/internal/httpapi/router.go:99-115`). Un 401 `SESSION_*`/`DEVICE_*` borra la sesión guardada y
  `DEVICE_REVOKED`/`DEVICE_REQUIRED` también la credencial del dispositivo (`api.rs:835-845`, `982-990`).
- **SSE**: `GET /api/v1/events` (`administration/events.go:55-139`). No hay notificación por base: cada flujo relee el
  outbox `app.realtime_events` cada 750 ms, revalidando sesión, dispositivo, permisos y estaciones en cada lectura
  (`events.go:111-137`), y se cierra a los 30 s (`events.go:113`) o al apagar. Máximo 2 flujos por terminal
  (`events.go:80-84`). **Reanudación**: Go acepta `?after=<cursor>` o, si falta, `Last-Event-ID` (`events.go:59-61`); Rust
  usa `after` con el último cursor recibido y reintenta con espera creciente de 0.5 a 15 s (`realtime.rs:53-123`); fuera
  de la ventana de 5000 eventos Go responde `reset: true` (`backend/internal/events/store.go:61-65`). Al conectar o
  reconectar React vuelve a consultar todo lo vivo (`desktop/src/hooks/useRealtime.ts:66`, `useLiveResource.ts:68-80`),
  y además todo se sondea cada 5 s (`useLiveResource.ts:14`) y la sesión cada 5 s (`useSession.ts:65-73`).
- **TLS existente**: `tls-init --lan-ip IP` crea en `.local/tls/` una CA ECDSA P-256 (5 años) y un certificado de
  servidor (1 año) con SAN `127.0.0.1`, `::1` y **la IP privada fija indicada** (`backend/internal/localsetup/tls.go:21-29,
57-82`); se niega a reemplazarlos. **En la Mac nunca se ejecutó** (no existe `.local/tls/`). Go sirve TLS si la
  configuración tiene `tls_certificate`/`tls_private_key` (`main.go:207-212`, TLS 1.2+); fuera de loopback exige
  `allow_lan`, IP privada **explícita**, certificado y **entorno `development`** («producción remota aún no habilitada»,
  `backend/internal/config/config.go:92-111`); toda petición de red sin TLS recibe `403 TLS_REQUIRED`
  (`administration/http.go:18-24`). En Rust, `ca_certificate` se **agrega** a las raíces públicas de `webpki-roots`
  (`main.rs:155-169`, `reqwest` con `rustls-tls`): verifica cadena y nombre, **no fija el certificado** (cualquier CA
  pública válida para esa IP también pasaría) y un cambio de IP invalida el nombre del certificado.

### 2. Terminales

- **Tabla `app.devices`** (`0002_identity_and_administration.sql:64-74`): `id`, `name` (1–80), `secret_hash`,
  `register_id` (caja), `keyboard_mode`, `created_at`, `last_seen_at`, `revoked_at`; la 0020 agregó `home_screen`
  (`tables|takeaway|kitchen|cash`) y la 0022 `kitchen_all_stations` y `kitchen_sound_alert`; las estaciones de la
  terminal están en `kitchen_device_stations` (`0005_kitchen_and_printing.sql:73`). **No hay tipo de dispositivo,
  sistema operativo, versión de la aplicación ni huella del certificado**; tampoco «impresoras de la terminal». La
  identidad de una terminal es el par `X-Device-ID` + token.
- **Registro (enrolamiento)**, dos caminos, ambos con código de 43 caracteres de un solo uso que vence en 10 min
  (`app.enrollments`, `0002…sql:76-83`): (a) un administrador con `devices.manage` genera una invitación en Sistema ›
  Terminales (`POST devices/invitations`, `administration/devices.go:35-48`) y en la terminal se escribe el código y el
  nombre (`desktop/src/pages/AccessPages.tsx:137-183`, operación `enroll`); (b) `dev.sh create-enrollment` escribe
  `.local/enrollment-code` y el escritorio lo lee con `ADEL_ENROLLMENT_FILE` (`main.rs:214-217`, `api.rs:774-786`,
  `administration/local_enrollment.go:10-30`). `POST devices/enroll` (`auth.go:151-183`) devuelve `device_id` y
  `device_token`, que Rust guarda en Keychain (`api.rs:816-833`). **No hay límite de intentos** (no hace falta con 256
  bits; sí con un código de 6 dígitos).
- **Credenciales** (`desktop/src-tauri/src/vault.rs:5-42`): servicio `local.adel.pos.dev`, cuenta
  `"{server_url}|{credential_namespace}"` con `device_id`, `device_token` y `session_token`. Si cambia la dirección del
  servidor, la credencial deja de encontrarse.
- **Administración** (Sistema › Terminales, `desktop/src/pages/DevicesPage.tsx`): listar (`devices.read`), renombrar,
  caja, teclado y pantalla inicial (`PATCH devices/{id}`, `devices.manage`, auditado `device.updated`; no se cambia de
  caja con un turno activo, `devices.go:66-119`), revocar (revoca también sus sesiones, `devices.go:57-65`). Sesiones:
  `GET sessions`, `POST sessions/{id}/revoke` (`administration/sessions_audit.go:11-38`). Las estaciones por terminal se
  configuran en Cocina › Estaciones (9F, `PUT kitchen/access`). `GET auth/me` informa la terminal (id, nombre, caja,
  teclado, inicio; `administration/routes.go:236-256`).
- **Impresoras**: no son de una terminal. Comandas por ruta → estación → impresora; precuentas y tickets a **una sola
  impresora de tickets global** (`payment_settings.receipt_printer_id`, `0006_payments_and_refunds.sql:11`); el cajón, a
  la impresora de cada **caja** (`cash_drawer_settings`, `0008_cash_shifts_and_reconciliation.sql:328-336`), y la
  terminal llega a su cajón por `devices.register_id`.
- **Tipos**: no existen; los «modos» (`server-terminal`, `terminal-only`, `server-only`) son de configuración. Los roles
  sembrados incluyen «Mesero» (`administration/permissions.go:86-94`).

### 3. Cómo arranca hoy el servidor en macOS

- **Proceso independiente de la ventana**: `dev.sh server-start` compila y lanza `bin/ad-el-pos-server serve` con
  `start_new_session=True`, entrada nula y bitácora en `.local/server.log`, y guarda el PID (`scripts/dev.sh:53-56`,
  `scripts/server-process.py:37-83`); se detiene con SIGTERM (`server-process.py:86-109`). PostgreSQL: `dev.sh
db-start` → `pg_ctl -D .local/postgres` en 127.0.0.1:55432 (`scripts/database.py:17-27, 63-73`). En macOS Tauri no
  inicia ni detiene nada (`server_host.rs:33`, solo Windows). **Sí sobrevive al cerrar la ventana** (y «Salir» del menú no
  detiene Go).
- **No arranca con la Mac ni sin sesión**: no hay launchd instalado (`ops/launchd/local.adel.pos.server.plist.example` es
  una plantilla con rutas `/ABSOLUTE/PATH`, `RunAtLoad`, `KeepAlive` y bitácora en `.local`, nunca instalada; no hay
  nada de AD-EL POS en `launchctl list`, `~/Library/LaunchAgents` ni `/Library/LaunchDaemons`; `brew services` muestra
  `postgresql@18 none`). `DEVELOPMENT_MACOS.md:7`: «Tras reiniciar macOS se deben iniciar PostgreSQL y Go
  explícitamente». Confirmado hoy: tras el reinicio no corren.
- **Instalación del `.app`**: no se copia a Aplicaciones ni hay `.pkg`: `dev.sh build-macos` lo deja en
  `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app` y `dev.sh open-app` ejecuta su binario con
  `ADEL_PROVISIONING_FILE`/`ADEL_ENROLLMENT_FILE` (`dev.sh:63-81`). Firma **ad hoc** (`codesign -dv`: `adhoc`, sin Team
  ID). Hay 2 identidades de firma válidas en el llavero del usuario (no se revisó cuáles).
- **Datos** dentro del repositorio, en **`~/Documents/ADEL POS/ad-el-pos/.local/`** (clúster `postgres/` de 74 MB con
  superusuario `adel_dev_admin`, socket en `.local/socket`, `server.json`, secretos, bitácoras) y respaldos en
  `ad-el-pos/backups/`. `~/Documents` está protegido por TCC: un servicio del sistema no puede leerlo sin «Acceso total
  al disco».
- **PostgreSQL 18 del clúster real**: Homebrew, `/opt/homebrew/opt/postgresql@18/bin` →
  `/opt/homebrew/Cellar/postgresql@18/18.6/bin` (carpeta del usuario `eliver`, grupo `admin`), enlazado con bibliotecas
  de Homebrew (`openssl@3`, `icu4c@78`, `gettext`, `zstd`, `lz4`, `krb5`; `otool -L`). Un `brew upgrade` puede cambiarlo.
- **Herramientas**: `/usr/bin/pkgbuild`, `/usr/bin/productbuild`, `/usr/bin/codesign`, `/bin/launchctl`,
  `/usr/bin/dns-sd`; `notarytool` en las Command Line Tools. Firewall de aplicaciones **desactivado** (`socketfilterfw
--getglobalstate`). IP privada en `en0`.
- **`internal/standalone` + `host` (Windows)**: todo con `//go:build windows` (`backend/internal/standalone/*.go`,
  `cmd/ad-el-pos-server/host_windows.go`; `host_other.go` lo rechaza fuera de Windows). Hace: carpeta de datos
  (`%LOCALAPPDATA%\AD-EL POS`, `layout.go:43-65`), bloqueo exclusivo `host.lock` (`process.go:153-172`, `LockFileEx`),
  clúster propio con marcador (`cluster.go:56-139`, superusuario `adel_admin`, 127.0.0.1:55432, SCRAM, UTC, locale C),
  arranque o adopción de PostgreSQL, base y `server.json` (`bootstrap.go:23-161`), respaldo antes de migrar
  (`host_windows.go:153-180`), `serve` y parada de PostgreSQL al terminar (`host_windows.go:82-141`); respaldos,
  `restore-check` y `restore` (`backup.go:39-210`). **Para macOS** haría falta: separar lo propio de Windows
  (`CREATE_NO_WINDOW`, rutas cortas 8.3, `LockFileEx`, `QueryFullProcessImageName`, ACL de `fileacl`) en archivos
  `_windows.go` y escribir el equivalente Unix (`flock`, permisos 0700/0600, señales); `NewLayout` con
  `/Library/Application Support/AD-EL POS`; `--pg-bin` configurable (en esta Mac, Homebrew); un `host_darwin.go`
  sin `--watch-stdin` (launchd envía SIGTERM); y que el servicio corra con un **usuario que no sea root** (PostgreSQL no
  arranca como root): `UserName` en el plist (el usuario del propietario o uno propio, que hay que crear con permisos de
  administrador).

### 4. Concurrencia

- **Un solo lock global** para escribir operación, cocina y cobros: `pg_advisory_xact_lock(7820148305)` exclusivo en
  toda escritura de `restaurant/*`, `payments/*` y `kitchen/*`, compartido en lecturas (`operations/handler.go:54-60`,
  `payments/handler.go:98-103`, `kitchen/handler.go:72-74`): las escrituras de todas las terminales se serializan.
- **Revisión optimista** por cuenta: cada escritura envía `revision`; `ensureOpen` y `touch` (`UPDATE … WHERE
revision=$2`) responden **`409 ACCOUNT_CONFLICT`** «La cuenta o distribución cambió en otra terminal. Recarga y compara
  los cambios antes de continuar.» (`operations/queries.go:54-64, 95-103`, `operations/context.go:34-36`). Confirmar un
  pedido exige además la huella del cálculo visto (`expected_fingerprint`; si cambió precio o borrador, **`409
QUOTE_CHANGED`**, `operations/confirmations.go:36-38, 80-82`). Cobrar exige `expected_balance` (**`409
BALANCE_CHANGED`** «El saldo de la cuenta cambió en otra terminal…», `payments/collect.go:468-469`). Abrir una cuenta en
  una mesa que otra terminal ya ocupó: **`409 TABLE_HAS_ACCOUNT`** con las cuentas (7.1). Catálogo, inventario,
  diseñador y configuración usan `REVISION_CONFLICT`.
- **Qué ve la segunda terminal**: en la captura, el motor del borrador marca el conflicto (`desktop/src/pages/restaurant/
capture/draftSync.ts:129-135, 614-618`) y la cuenta muestra «Otra terminal cambió esta cuenta mientras agregabas
  productos. Tus productos nuevos se conservan en esta terminal: revisa la cuenta actual y decide.» con «Conservar mis
  productos nuevos» y «Descartar productos nuevos» (`CaptureBill.tsx:280-300`), y bloquea «Enviar» mientras tanto
  (`BillFooter.tsx:313`). `QUOTE_CHANGED` vuelve a calcular (`useCaptureDraft.ts:173`). Los demás 409 llegan como aviso
  de error con el mensaje de Go. La vista se actualiza por SSE o por el sondeo de 5 s. Ninguna escritura sobrescribe en
  silencio.

### 5. Disponibilidad de productos («agotado»)

- **Existe a medias**: `products.temporary_unavailable` (`0003_catalog_and_pricing.sql:57`) se cambia en Catálogo con
  «Pausar»/«Disponible» (`desktop/src/pages/catalog/CatalogRecord.tsx:128-138`) por `PATCH catalog/products/{id}/
availability` con `revision`, permiso **`catalog.availability.manage`** («Modificar disponibilidad y horarios», el mismo
  de los horarios; `backend/internal/catalog/actions.go:237-283`, `context.go:60`), auditado
  `catalog.product.availability`. Go lo hace cumplir en el cálculo: **`409 PRODUCT_UNAVAILABLE`** «…: El producto no está
  disponible temporalmente.» (`catalog/actions.go:18-34`, `calculator.go:139-145`, `context.go:38-40`), y la captura
  muestra el producto marcado con el motivo (`availability_reason`, `ProductGrid.tsx:133`). Además hay horarios por
  producto, presentación y categoría (`catalog/availability.go`).
- **Falta**: marcar «Agotado» desde la captura con un toque; permiso propio; propagación en tiempo real (**el catálogo
  no publica eventos**: ningún `events.Append` en `catalog/`, y `realtime_events.scope` no admite `catalog`,
  `0018_document_templates.sql:314-317`; la captura recarga `catalog_lookups` cada 15 s,
  `useRestaurantWorkspace.ts:159-162`, o con cualquier otro evento); reinicio al cambiar de día con el corte y opción
  «agotado indefinido»; agotar complementos. Se completa en la Fase D.

### 6. Idempotencia

- **Ya la tienen todas las escrituras de pedidos, cuentas, cocina y cobros** (obligatoria): `operation_id` UUID en el
  cuerpo de toda escritura de `restaurant/*` (`operations/handler.go:64-93`), `payments/*` incluida caja
  (`payments/handler.go:114-142`) y `kitchen/*` (`kitchen/handler.go:72-130`); también inventario, diseñador, identidad
  documental, «Quitar datos de prueba» y estaciones por usuario.
- **Cómo**: con el lock exclusivo ya tomado se busca la clave en `app.restaurant_operations` (`0004_restaurant_operations.
sql:83-91`: `id`, `actor_id`, `device_id`, `route`, `fingerprint`, `response jsonb`, `created_at`); si existe y
  coinciden usuario, terminal y huella, devuelve la **respuesta guardada sin repetir**; si no coinciden, **`409
IDEMPOTENCY_CONFLICT`** (`operations/handler.go:299-319`, `payments/handler.go:201-218`, `kitchen/handler.go:92-104`).
  Huella = SHA-256 de método + ruta + JSON canónico del cuerpo, sin la contraseña de una autorización de supervisor
  (`operations/handler.go:259-297`). Dos reintentos simultáneos se serializan en el lock: el segundo encuentra la fila. Un
  error no guarda la clave (la transacción se revierte), así que el reintento vuelve a ejecutar.
- **Conservación**: **para siempre** (no hay ningún `DELETE` de `restaurant_operations`, ni de `realtime_events`,
  sesiones o enrolamientos; `NEXT_STEPS_FOR_SOL.md:109`). Cumple «al menos 7 días».
- **No la aceptan**: el catálogo (incluido el «Pausar», solo `revision`), administración (usuarios, roles, terminales,
  invitaciones, cajas, ajustes, sesiones, preferencias), `auth/login` (cada reintento crea otra sesión y revoca la
  anterior), `auth/logout`, `auth/password`, `devices/enroll` y las vistas previas de solo lectura (`draft-preview`,
  `edit-preview`, `payments/…/preview`, `catalog/preview`). Dentro de pedidos, cuentas y pagos no hay ninguna escritura
  sin clave; lo que falta para Android es el envío de un pedido capturado sin conexión (§5.5), que hoy son dos pasos
  (`PUT …/draft` y `POST …/confirm`, o `POST restaurant/accounts` con `items`) y cuyo rechazo `QUOTE_CHANGED` no dice
  qué cambió.

### 7. Impresión USB local y «agente de impresión aún no implementado»

- **USB en la computadora del servidor: funciona hoy.** Se instala la impresora en macOS (cola CUPS) o en Windows y se
  configura con el transporte `cups` («cola de impresión del equipo»); Go imprime con `lp` (argumentos separados; `-o raw`
  si «Esta impresora entiende ESC/POS») o, en Windows, ESC/POS RAW por `winspool.drv` (`kitchen/system_queue*.go`,
  `windows_queue.go`, `cups_raw.go`). El cajón admite TCP o una cola CUPS raw. Transportes: `cups`, `escpos_tcp` y
  `agent` (`0005_kitchen_and_printing.sql:21`, `kitchen/printers.go:50-76`).
- **«Agente»**: `agent` es un transporte **reservado y sin implementación** (impresora de otra terminal; `destination` es
  texto libre, «agente futuro» en `printers.go:75`). Un trabajo a una impresora `agent` entra a la cola en «Requiere
  intervención» con «Esta impresora todavía no puede usarse para imprimir.» (`kitchen/documents.go:205-221`,
  `print_problems.go:20-35`; antes de la 9F decía «El destino está inactivo o requiere un agente de impresión aún no
  implementado»); desde la 9F la impresora de tickets ya no admite `agent` y precuentas, tickets y documentos de caja
  avisan antes de encolar (D4). El cajón responde «El agente de impresión aún no existe…» (`payments/cash_drawer.go:158`)
  y el diseñador marca la muestra como no compatible (`designer/samples.go:175`). USB en una terminal que no es el
  servidor: no hay forma de imprimir.
- **Asignación**: ruta → estación → impresora (comandas), impresora de tickets global (precuentas, tickets y documentos
  de caja), impresora del cajón por caja. La cola es durable con `lease_until` de 60 s y estados `pending`, `processing`,
  `submitted`, `error`, `uncertain`, `cancelled`, `intervention` (`0005…sql:172-185`, `kitchen/worker.go:116-180`): sirve
  de base para que un agente reclame trabajos.

### 8. Respaldos

- **Manual en macOS**: `dev.sh backup` → `pg_dump --format=custom --no-owner --no-acl` en `ad-el-pos/backups/` (0600,
  nombre `adel_pos-AAAAMMDDTHHMMSSZ-<6 hex>.dump`) y comprobación con `pg_restore --list` (`scripts/database.py:164-199`);
  `python3 scripts/restore-check.py backups/X.dump` restaura en una base temporal y compara los conteos de todas las tablas
  (`scripts/restore-check.py`), escribiendo `.local/stageN-restore-verification.json`. Último respaldo real:
  `backups/adel_pos-20261113T211630Z-a3fceb.dump` (5.4 MB en total en `backups/`).
- **«Quitar datos de prueba» (9F)**: respaldo previo en Go con `maintenance.DumpBackup` (mismo nombre, 0600,
  `pg_restore --list`; binarios de `postgres_bin`, `ADEL_PG_BIN` o Homebrew; carpeta `backup_directory` o `backups/` junto
  a la configuración; `backend/internal/maintenance/backup.go:26-165`), detrás de la interfaz `maintenance.Backuper`
  (`Backup(ctx) (string, error)`), que en el `host` de Windows es `standalone.Backup` (`host_windows.go:143-151`).
- **Windows**: `ad-el-pos-server backup [--pause]` (acceso directo «Crear respaldo de AD-EL POS»), `restore-check
--file` y `restore --file` con la aplicación cerrada y respaldo de seguridad previo (`standalone/backup.go:39-210`);
  respaldo automático antes de migrar.
- **Restaurar**: en la aplicación **no existe** ninguna acción de restaurar ni de respaldar; en macOS solo hay
  `restore-check.py` (verificación) y en Windows la línea de comandos.
- **Planificadores reutilizables en Go**: `kitchen.Worker` (planificador de liberaciones cada 1 s y dos trabajadores de
  impresión cada 300 ms, `kitchen/worker.go:26-62`) e `inventory.Worker` (1 s, `inventory/worker.go:53-71`), arrancados
  en `serveHTTP` con un contexto que se cancela al apagar (`main.go:190-205`); `internal/businessday` calcula el día
  comercial con la «Hora de inicio del día» (`inventory_settings.operating_day_cutoff`, 05:00 por omisión,
  `businessday/day.go:39-70`). No hay ninguna tarea programada por hora del día.

### 9. Versión de API y compatibilidad

`GET /api/v1/version` devuelve `product`, `server_version`, `api_version` (`v1`), `schema_version` (22),
`supported_api_versions`, `capabilities`, `events_available` y `test_mode` (`httpapi/router.go:56-66`); **no informa
versión mínima de cliente**. `GET /api/v1/health/ready` → 200 `ready` o 503 `not_ready` con `checks.postgres` y
`checks.schema` (`router.go:43-54`); el esquema es «compatible» solo si están aplicadas exactamente las migraciones
incrustadas (`database/migrate.go:73-86`). El cliente compara producto, `api_version` y `schema_version` **exactos**
(`desktop/src/services/diagnostics.ts:46-50`) y **solo lo muestra** en el estado del sistema (`StatusPage.tsx:130-139`,
`systemStatus.ts:46-50`); nada se bloquea. El cliente no envía su versión. **No hay OpenAPI**: el contrato es prosa en
`API_CONTRACTS.md` (≈400 KB).

### 10. Catálogo

Sin versión global ni etag: cada registro tiene `revision` y `updated_at` (`0003…sql:21-22, 31-32, …`). La captura
descarga `GET catalog/lookups` completo (categorías, productos con `availability_reason`, grupos, porciones,
indicaciones…) cada 15 s y con cualquier evento SSE (`useRestaurantWorkspace.ts:159-162`, `useLiveResource.ts:68-80`). Las
imágenes se guardan en `app.catalog_images` (`content bytea`, `media_type`, `width`, `height`, `checksum`) y se sirven
por `GET catalog/images/{id}` como JSON con `data_uri` base64 (`catalog/images.go:19-35`), una por producto
(`ProductGrid.tsx:126-127`); todas las respuestas llevan `Cache-Control: no-store` (`router.go:89`).

### 11. Sesiones, auditoría y permisos

`app.sessions` (`0002…sql:85-97`: usuario, terminal, expiración, `last_seen_at`, revocación con motivo). La auditoría
**ya guarda la terminal** de cada movimiento (`audit_events.device_id`, `service.go:147-171`; filtro `device_id` en el
Registro de movimientos, `sessions_audit.go:62-90`). Sesiones activas = `revoked_at IS NULL AND expires_at > now()`; por
tipo de dispositivo no se puede contar porque no hay tipo. `last_seen_at` de la terminal y de la sesión se escribe en
**cada** petición, también en cada lectura de 750 ms del SSE. **Permiso nuevo**: clave y descripción en el registro de Go
del módulo (`administration/permissions.go:23-65`, o `Permissions` de cada paquete), que `ensureFoundations` inserta en
instalaciones nuevas (`permissions.go:75-82`), más una migración con `INSERT INTO app.permissions` y la concesión al rol
`administrator` (patrón de `0021_account_cancellation.sql:78-84`). Ya existe `devices.manage` («Enrolar, configurar y
revocar terminales»).

### 12. Dependencias disponibles sin red para mDNS/DNS-SD

- **Go**: `golang.org/x/net v0.60.0` está en `go.mod` como indirecta y en `go.sum` (hash del módulo y del `go.mod`), y
  descomprimido en el `GOMODCACHE` del proyecto (`.cache/go-mod/golang.org/x/net@v0.60.0`, con `dns/dnsmessage` e
  `ipv4`): pasarla a directa no descarga nada. `golang.org/x/sys v0.49.0` ya es directa.
- **Rust**: ningún crate de mDNS en `Cargo.lock` ni en el registro del proyecto (`.tools/cargo/registry`; no existe
  `~/.cargo`). Sí están `socket2 0.6.5`, `rustls 0.23.46`, `ring 0.17.14`, `sha2 0.10.9`, `webpki-roots`, `objc2` y
  `windows 0.62.2`/`windows-sys 0.61.2` (con `DnsServiceBrowse` en `Win32/NetworkManagement/Dns`, útil en la Fase W). Una
  consulta mDNS de una sola vez se puede escribir con `std::net::UdpSocket` sin crates nuevos.
- **macOS**: `/usr/bin/dns-sd` existe.

### 13. Infraestructura de pruebas

- `scripts/database.py:143`: `packages = ("./internal/database", "./internal/administration", "./internal/catalog",
"./internal/operations", "./internal/payments", "./internal/reports", "./internal/reports/operational",
"./internal/designer", "./internal/identity")`; un paquete nuevo con pruebas de integración se agrega ahí.
- `scripts/browser-tests.py:16-100`: por motor (Chromium y WebKit) copia `.local/server.json`, cambia la base a
  `adel_pos_test_browser_<hex>`, `listen_address` 127.0.0.1:8183, `allow_lan: false`, **`test_mode: true`** y
  `backup_directory` temporal; crea la base, compila, `migrate` + `prepare-setup`, arranca `serve`, espera `health/ready`
  y corre `npm run test:browser` con `ADEL_BROWSER_ENGINE`, `ADEL_BROWSER_PROVISIONING` y `PLAYWRIGHT_BROWSERS_PATH`;
  al final detiene Go y borra la base y la carpeta.
- `desktop/e2e/native-driver.ts`: exige `test_mode` (`:69-77`); descubre las operaciones de `api.rs` con dos expresiones
  regulares, `"op" => {? Some((Method::X, "ruta"))` y `"op" => {? (Method::X, format!("…{identifier}…"))`
  (`:105-122`); lee `MENU_ACTIONS` de `menu.rs` y `EXPORTABLE_REPORTS` de `export.rs`; y simula a mano cada comando de
  Tauri que no es `api_request` (`events_start/stop`, `menu_*`, `authentication_state`, `connection_config`,
  `server_snapshot`, `export_report`, ventana; `:395-466`); **cualquier comando nuevo de Tauri** (descubrimiento,
  emparejamiento, agente, servicio) hay que simularlo ahí o la prueba falla con «Comando de prueba no autorizado».
  Playwright: `ADEL_BROWSER_SERVER_URL` y `ADEL_BROWSER_PREVIEW_PORT` cambian los puertos (`native-driver.ts:6`,
  `playwright.config.ts:5`).

### Contradicciones y huecos entre la especificación y el código

1. **Escucha en la red y cambio de IP** (§4.1) frente a `CLAUDE.md` §7 («LAN exige IP privada explícita… No escuchar en
   `0.0.0.0`») y `config.go:92-111` (una sola dirección, IP fija, solo `development`). Si Go escucha en una IP fija, un
   cambio por DHCP deja al servidor sin escuchar; si escucha en todas las interfaces, cambia una regla permanente de
   seguridad. **Decisión necesaria** (ver plan, Fase B).
2. **El destino lo elige React** (§4.1: lista de servidores encontrados y dirección escrita a mano) frente a `CLAUDE.md`
   §3 («valida el destino solo desde archivo local, nunca desde React», `connection.rs`). Propuesta: React solo elige un
   servidor de la lista que entrega Rust o propone una dirección que Rust valida (IP privada o `.local`); Rust guarda el
   destino y la huella del certificado solo después del emparejamiento aprobado.
3. **«Solo servidor» con pantalla de estado** (§2.3) frente a `connection.rs:22-27`, que rechaza `server-only` en el
   escritorio.
4. **«Restaurar sigue siendo una acción del administrador, con confirmación, como hoy»** (§7): hoy no hay restauración en
   la aplicación (macOS: ninguna; Windows: línea de comandos con la aplicación cerrada).
5. **Permiso propio para «Agotado»** (§5.4) frente a `catalog.availability.manage`, que ya cubre «Pausar» y horarios.
6. **Una sola sesión por usuario** (`sessions_one_active_user_idx`): un mesero no puede tener sesión a la vez en una caja
   y en un teléfono. La especificación no pide cambiarlo; afecta a la etapa de Android y a las licencias por sesiones.
7. **Impresora de tickets global**: §6 habla de «la impresora de la Caja 2»; hoy precuentas y tickets de todas las cajas
   van a una sola impresora. Hace falta decidir si la impresora de tickets pasa a ser por caja (como el cajón).
8. **«Bloquea las operaciones» sin conexión** (§0-B, §4.2): hoy se muestra «Sin conexión» con «Reintentar»
   (`navigation/serverState.ts:12-29`, `CaptureTopBar.tsx:204`), pero las pantallas no se bloquean: cada operación falla
   al enviarse.
9. **Datos en la carpeta del usuario dentro de `~/Documents`** (§2.2 pide moverlos): un `LaunchDaemon` no puede leerlos
   por TCC, y la carpeta del sistema exige permisos de administrador.
10. **API_CONTRACTS.md** dice que el cliente «verifica CA y hostname» en LAN (y `ARCHITECTURE.md:63`); es cierto, pero la
    CA se suma a las públicas: no es fijación del certificado.

### Plan de las fases A a F

**Migración**: las versiones deben ser consecutivas (`database/migrate.go:31-34`, `version == index+1`), así que dos
agentes no pueden crear cada uno su «0023». Se propone, como la 0022 de la 9F, **una sola `0023_network_terminals_and_
android.sql` con una sección por fase** (B, D, E y F; la A no necesita esquema), que cada agente entrega como fragmento y
el coordinador une; `schema_version` 23 en `config/product.json` (y `sync`) la sube el coordinador al crear la 0023; la
versión 0.10.0, en la Fase G. Mientras no se aplique a la instalación real, la 0023 se puede editar.

**Fase A · Servicio en macOS y modalidades** (sin migración)

- Go: llevar `internal/standalone` a Unix: separar lo de Windows en `*_windows.go` y crear `*_unix.go` (`flock`, 0700/0600,
  sin 8.3), `NewLayout` con `/Library/Application Support/AD-EL POS` en macOS; `cmd/ad-el-pos-server/host_darwin.go` (o
  `host_unix.go`): `host` sin `--watch-stdin`, parada ordenada de PostgreSQL con SIGTERM, `--pg-bin` (en esta Mac,
  Homebrew); `backup`, `restore-check` y `restore` también en macOS. `GET/POST maintenance/service` (estado y reinicio:
  Go sale con código distinto de cero y launchd lo relanza; `settings.manage`), sin root desde la aplicación.
- `ops/launchd/local.adel.pos.server.plist` real (`RunAtLoad`, `KeepAlive`, `ThrottleInterval`, `UserName`, bitácora en
  `/Library/Application Support/AD-EL POS/logs`) y `scripts/build-macos-pkg.sh` (`pkgbuild` + `productbuild`: `.app` en
  `/Applications`, servidor y plist, `postinstall` con `launchctl bootstrap system`). Se recomienda `.pkg` con
  `LaunchDaemon` en lugar de `SMAppService`: el `.app` está firmado ad hoc, `SMAppService.daemon` pide aprobación en
  Ajustes y la liga a la firma, que cambia en cada compilación, y no hay crate de ServiceManagement en caché.
- Paso de datos (con el propietario): respaldo `pg_dump` del clúster actual → clúster nuevo en la carpeta del sistema →
  `pg_restore` → comparación de conteos de todas las tablas → arrancar el servicio; **`.local` no se borra**. La
  credencial de la terminal de la Mac (`http://127.0.0.1:8181|main`) se conserva si la dirección local no cambia. Por TCC,
  la copia la hace el usuario (aplicación o línea de comandos), no el `postinstall`.
- Rust: `connection.rs` admite `server-only`; modalidad elegida en el primer arranque y guardada por Rust en
  `connection.json` (lista cerrada). React: elección de modalidad, pantalla de estado de «Solo servidor» (servidor activo,
  terminales conectadas, último respaldo y acceso a Administración), en Mantenimiento «Servicio del servidor» y «Cambiar la
  modalidad» con confirmación (nunca borra la base).
- Pruebas: Go unitarias de layout, bloqueo y plist, e integración del `host` Unix con un clúster temporal (55499); Rust
  de modalidades; Vitest de pantallas; `GOOS=windows go build/vet`. Lo real (sobrevive al cerrar la ventana, arranca sin
  sesión) exige instalar y reiniciar: **paradas**.

**Fase B · Emparejamiento y cifrado** (sección B de la 0023)

- 0023-B: `devices.kind` (`desktop`, `android`, `kitchen`), `os`, `app_version`, `paired_at`; tabla
  `device_pairing_requests` (id, nombre propuesto, tipo, sistema, nonce de la terminal y del servidor, código derivado,
  intentos, vencimiento de 5 min, estado `pending|approved|rejected|expired|claimed`, quién aprobó, terminal creada).
  Sin permiso nuevo: `devices.manage`.
- Go (`administration/pairing.go` o paquete `internal/pairing`): `POST pairing/requests` (sin dispositivo, con límite por
  dirección de origen), `GET pairing/requests/{id}` (la terminal espera con un secreto de la solicitud y recibe la
  credencial una sola vez), `GET devices/pairing-requests`, `POST devices/pairing-requests/{id}/approve|reject` (el
  administrador escribe o compara el código; 5 intentos). **Código de 6 dígitos = derivado de SHA-256(huella SPKI del
  certificado ‖ nonce de la terminal ‖ nonce del servidor)**: un intermediario con otro certificado produce otro código.
  Propuesta: aprobar solo desde la computadora principal (`request.Local`, como `setup`, `setup.go:64`), que es lo que dice
  §0-B. Conteo `GET devices/sessions-summary` por tipo. Auditoría `device.pairing_*`.
- TLS: identidad del servidor generada al instalar (certificado propio de larga vigencia y su huella SPKI); dos escuchas:
  HTTP en `127.0.0.1:8181` y HTTPS en la red (decisión 1); `config.go` sin la restricción a `development`.
- Rust: verificador `rustls` que compara la huella fijada e ignora el nombre (sirve aunque cambie la IP), con
  `reqwest::ClientBuilder::use_preconfigured_tls` y `rustls 0.23.46` como dependencia directa (ya en `Cargo.lock` y en el
  registro: sin descarga); `connection.json` con `server_id`, huella y última dirección; llave del Keychain por
  `server_id` para terminales emparejadas, conservando la llave actual para `127.0.0.1`. Operaciones nuevas en `api.rs`
  (`pairing_*`, `devices_pairing_*`, `devices_sessions_summary`) y comandos Tauri de emparejamiento (simularlos en
  `native-driver.ts`).
- React: pantalla de emparejamiento (código grande y nombre editable), Sistema › Terminales con solicitudes pendientes,
  tipo, sistema, última conexión, usuario actual, en línea (`last_seen_at` reciente), renombrar, revocar y ver
  configuración.
- Pruebas (§9): código correcto e incorrecto, vencido, límite de intentos, revocación inmediata (el SSE se corta en la
  siguiente lectura), certificado cambiado (la terminal no se conecta), conteo de sesiones. **Revisor independiente.**

**Fase C · Descubrimiento y red** (sin migración)

- Go: `internal/discovery` (respondedor mDNS con `x/net/dns/dnsmessage` e `ipv4`, que pasa a directa; `_adelpos._tcp`
  con nombre del establecimiento y TXT `server_id`, puerto, huella corta y versión de API); se apaga en modo de pruebas
  salvo en su prueba.
- Rust: `discovery.rs` (consulta mDNS de una vez por `UdpSocket` a 224.0.0.251:5353 con respuesta unicast, analizador
  mínimo propio); dirección manual validada; si falla la conexión, vuelve a buscar por `server_id` + huella (cambio de IP).
  `Info.plist` con `NSLocalNetworkUsageDescription` y `NSBonjourServices`.
- React: lista de servidores y dirección manual; **capa «Sin conexión con el servidor»** que bloquea operaciones sin
  perder lo capturado, reconexión con espera creciente y consulta completa al reconectar (ya existe la recarga).
- Pruebas: Go unitarias de mensajes; Rust de análisis; Playwright con dos contextos del navegador contra el mismo Go
  temporal (misma cuenta, propagación de mesas, cocina, cobros y turnos); simulación de pérdida de conexión.

**Fase D · API v1 y Android** (sección D de la 0023)

- Versión: `/version` con `min_client_version` y `server_version`; Rust envía `X-Client-Version` y `X-Client-Kind`
  (`api.rs`, `realtime.rs`); Go responde `426 CLIENT_UPDATE_REQUIRED` («Actualiza esta terminal») y el cliente avisa si el
  servidor es más viejo. **OpenAPI 3.1 en JSON** (no hay biblioteca YAML ni de OpenAPI en caché) incrustado en Go y copiado
  en `docs/API_V1.md`, limitado a la API de clientes (versión, emparejamiento, sesión, catálogo, cuentas y pedidos,
  estados de cocina, recuperación); prueba de contrato en Go que compara el documento con las rutas reales y con
  respuestas de integración.
- Catálogo: tabla `catalog_changes` (secuencia) llenada por disparadores de las tablas del catálogo (o por `saveRecord`),
  `GET catalog/sync` completo y `GET catalog/sync?since=N` con bajas; imágenes aparte con `checksum`/etag.
- «Agotado»: columnas en `products` (agotado hasta el fin del día comercial o indefinido, quién y cuándo), permiso
  `catalog.sold_out.manage` (solo administrador), `POST catalog/products/{id}/sold-out` con `operation_id`, ámbito
  `catalog` en `realtime_events_scope_check` y evento; botón en la captura; el reinicio con el corte se calcula al leer.
- Pedido capturado sin conexión: `POST restaurant/offline-orders` (o una variante de `POST restaurant/accounts` /
  `…/orders`) con `operation_id`, versión de catálogo, hora de captura y precios vistos; si algo cambió, `409
ORDER_REQUIRES_CONFIRMATION` con la lista (producto, antes, ahora, motivo) y se acepta solo con la confirmación del
  mesero y el catálogo vigente; auditoría de captura sin conexión. Recuperación: `GET restaurant/state` (mesas y cuentas
  con revisión, preparación, permisos y estado de la terminal).
- Pruebas: reintentos simultáneos, respuesta perdida, misma clave con otro contenido, versión mínima, catálogo completo y
  por cambios, agotado y los cinco casos de §5.5. **Revisor independiente.**

**Fase E · Agente de impresión** (sección E de la 0023)

- 0023-E: `kitchen_printers.agent_device_id` (con `CHECK` transporte `agent` ⇔ terminal) e impresoras locales que informa
  cada terminal; decidir impresora de tickets por caja (contradicción 7).
- Go: `GET kitchen/agent/jobs` (reclama con `lease_until` los trabajos de las impresoras de esa terminal; bytes ESC/POS ya
  generados por Go), `POST kitchen/agent/jobs/{id}/result`, `PUT kitchen/agent/printers`; problema nuevo
  `agent_offline` («La Caja 2 está desconectada») por `last_seen_at`; las impresoras `agent` vuelven a ser válidas para
  tickets y cajón.
- Rust: `print_agent.rs` mientras la aplicación está abierta (SSE para despertar, sondeo de respaldo): `lp -d cola -o raw`
  y `lpstat` en macOS con argumentos separados; Windows RAW en la Fase W.
- React: impresoras locales en Terminales y nombre de la terminal al asignar impresoras.
- Pruebas: trabajo para una terminal conectada y desconectada, lease vencido, resultado incierto, cajón.

**Fase F · Respaldos programados** (sección F de la 0023)

- 0023-F: `backup_settings` (activo, hora —por omisión la del corte—, cuántos conservar —30—, carpeta) y `backup_runs`
  (inicio, fin, archivo, tamaño, carpeta usada, verificado, resultado, motivo).
- Go: paquete `internal/backups` con un trabajador como los de cocina e inventario que usa `maintenance.Backuper`
  (`DumpBackup` en macOS, `standalone.Backup` en Windows), verifica con `pg_restore --list`, borra solo archivos con su
  nombre en la carpeta, usa la carpeta predeterminada si la configurada no está y lo avisa; un lock consultivo nuevo
  (7820148309, a la tabla de `CLAUDE.md` §3) con `pg_try_advisory_lock` para no duplicar. Rutas
  `maintenance/backups*` (`settings.manage`), respaldo manual con `operation_id`.
- React: Mantenimiento › Respaldos, aviso en la pantalla de inicio al administrador si falla, último respaldo en el
  estado del sistema y en la pantalla de «Solo servidor».
- Pruebas: cálculo de la próxima hora (corte, cambio de horario), retención, carpeta no disponible, fallo de
  verificación, aviso. **Revisor independiente.**

**Paralelo con 2 agentes**: A ∥ D, después B ∥ F y por último C ∥ E (C depende de B; A y B tocan `main.go`, `config.go`
y `connection.rs`). Archivos compartidos que hay que coordinar: `desktop/src-tauri/src/api.rs` (todas), `main.rs` (A,
B, C, E), `desktop/e2e/native-driver.ts` (B, C, E), `administration/routes.go` (A, B, D, E, F), la 0023 (B, D, E, F, por
fragmentos), `config/product.json` (solo el coordinador), `navigation/sections.ts` y `MaintenancePage.tsx` (A, F),
`DevicesPage.tsx` (B, E), `API_CONTRACTS.md` y `CLAUDE.md` (fragmentos que une el coordinador). Cada agente trabaja en su
copia y entrega un parche desde `HEAD`, como en la 9D y la 9F.

**Riesgos y paradas para preguntar al propietario**

- **Contraseña de administrador**: instalar el `.pkg` (ventana del Instalador de macOS), registrar el `LaunchDaemon` y crear
  `/Library/Application Support/AD-EL POS` (y un usuario propio del servicio, si se elige). macOS mostrará además el aviso
  «Elementos en segundo plano añadidos».
- **Usuario del servicio**: el del propietario (sencillo; los secretos siguen siendo suyos) o uno propio (`_adelpos`,
  más aislado, requiere administrador): decisión.
- **Mover los datos reales** de `~/Documents/ADEL POS/ad-el-pos/.local` a la carpeta del sistema (respaldo, restauración
  verificada, `.local` sin borrar): acción sobre la instalación real; conviene confirmarla con el propietario.
- **Reiniciar la Mac** para comprobar que el servicio arranca sin iniciar sesión.
- **Red**: decidir la escucha en la red (contradicción 1). El firewall de macOS está desactivado (sin aviso hoy); en macOS
  15+ aparecerá el permiso de **«Red local»** para AD-EL POS al buscar o conectarse a un servidor, y con firma ad hoc puede
  volver a pedirse en cada compilación. En Windows (Fase W) la regla del firewall sí es una parada.
- **PostgreSQL para un `.pkg` distribuible**: el de Homebrew depende de `/opt/homebrew`; incluir PostgreSQL en el paquete
  (como en Windows) requiere **descargar** binarios: parada. Para esta Mac basta `postgres_bin` con Homebrew.
- **Dependencias**: `x/net` (Go) y `rustls` (Rust) ya están en caché y en los lockfiles; si alguna función pidiera un crate
  o módulo nuevo, parada. No hay crates de mDNS, ServiceManagement, OpenAPI ni YAML.
- **Keychain**: cambiar la llave de la credencial puede mostrar el diálogo de acceso de macOS; la credencial actual de la
  Mac no debe perderse.
- **Carga**: el lock 7820148305 serializa todas las escrituras y cada flujo SSE hace dos `UPDATE` cada 750 ms; medir con
  varias terminales (Fase W, prueba de carga).
- **Dos computadoras reales**: en la Mac solo se pueden simular dos terminales (contextos de navegador o dos perfiles con
  `credential_namespace`); las pruebas cruzadas son de la Fase W.
- **Instalación real detenida** tras el reinicio de hoy (ver arriba): decidir con el propietario cuándo arrancarla.

### Decisiones del coordinador (ninguna cambia una decisión de la especificación)

- **D1 · Escucha en la red** (contradicción 1). Go conserva HTTP en `127.0.0.1:8181` para la ventana de la propia
  computadora y, cuando la modalidad incluye servidor, agrega una escucha **HTTPS en todas las interfaces, puerto 8443**,
  que solo acepta conexiones cuyo origen sea loopback, privado (RFC 1918), enlace local o ULA de IPv6 (las demás se
  cierran al aceptarlas), con TLS 1.2 o superior y el certificado propio del servidor. Por la escucha de red nunca se
  aceptan las rutas locales (asistente inicial, aprovisionamiento, aprobación de emparejamiento). Así un cambio de IP por
  DHCP no deja al servidor sin escuchar. Sustituye la regla «no escuchar en `0.0.0.0`» de `CLAUDE.md` §7, que se
  actualiza en la Fase G. El firewall de macOS está desactivado en esta Mac: no habrá aviso; en Windows (Fase W) la regla
  del firewall sí es una parada.
- **D2 · Destino elegido en la interfaz** (contradicción 2). React solo elige un servidor de la lista que entrega Rust o
  propone una dirección escrita a mano; Rust la valida (IP privada o nombre `.local`), obtiene el certificado, hace el
  emparejamiento y guarda destino, `server_id` y huella fijada solo cuando el emparejamiento se aprueba. React sigue sin
  poder elegir un destino arbitrario.
- **D3 · Servicio en macOS** (contradicción 9). Instalador `.pkg` con `LaunchDaemon` (no `SMAppService`: el `.app` está
  firmado ad hoc). El servicio corre con la cuenta del usuario que instala (PostgreSQL no arranca como root) y sus datos
  quedan en `/Library/Application Support/AD-EL POS/`, de esa cuenta y con permisos 0700. PostgreSQL es el instalado en la
  computadora (`postgres_bin`; en esta Mac, Homebrew): incluirlo en el paquete exige descargar binarios y queda para la
  distribución de la etapa 10. El servicio se mantiene vivo mientras existe un archivo de habilitación en la carpeta de
  datos (`KeepAlive` → `PathState`), de modo que reiniciar el servicio o cambiar de modalidad desde la aplicación no pide
  la contraseña de administrador; si el proceso se cae, launchd lo relanza.
- **D4 · Paso de los datos de la instalación real** a la carpeta del sistema: respaldo con `pg_dump`, ensayo de
  restauración, restauración en el clúster nuevo y comparación de los conteos de todas las tablas; `.local` no se borra.
  Se hace en la Fase G junto con la instalación del `.pkg`, que pide la contraseña de administrador: **parada**.
- **D5 · Emparejamiento**. La aprobación solo se acepta desde la computadora principal (conexión local), como dice §0-B.
  En Sistema › Terminales aparece la solicitud pendiente (nombre propuesto, sistema y hora) y, para aprobarla, el
  administrador **escribe el código de 6 dígitos que muestra la terminal**: así compara de verdad el código (no basta un
  clic) y tienen sentido las pruebas de código correcto e incorrecto y el límite de intentos. El código vence a los 5
  minutos y 5 intentos fallidos rechazan la solicitud. Se deriva del certificado y de dos números aleatorios con
  compromiso previo de la terminal (`SHA-256(huella SPKI ‖ nonce de la terminal ‖ nonce del servidor ‖ solicitud)`), de
  modo que un intermediario con otro certificado produce otro código.
- **D6 · Certificado e identidad del servidor**. Al instalarse, el servidor genera su `server_id` y su propio certificado
  (ECDSA P-256, larga vigencia); cada terminal fija la huella SPKI durante el emparejamiento e ignora el nombre y la IP
  del certificado (sirve aunque cambie la dirección). Un certificado distinto impide la conexión y se avisa; volver a
  conectarse exige emparejarse de nuevo.
- **D7 · Restaurar** (contradicción 4) sigue «como hoy»: una acción del administrador por línea de comandos con el
  servicio detenido (`restore-check` y `restore` del servidor, que pasan a funcionar también en macOS), documentada en
  `docs/INSTALAR_EN_RED.md`. No se agrega restauración en la aplicación.
- **D8 · «Agotado»** (contradicción 5): permiso nuevo `catalog.sold_out.manage` (solo al administrador, como todo permiso
  nuevo; se asigna a los roles desde Personal), distinto de «Pausar» (`catalog.availability.manage`), que no cambia. Un
  producto agotado no se puede capturar; el agotado vuelve a «disponible» con el corte del día salvo que sea indefinido,
  y el catálogo empieza a publicar eventos en tiempo real.
- **D9 · Una sesión activa por usuario** (contradicción 6): no cambia en esta etapa; se cuentan las sesiones activas por
  tipo de dispositivo y se anota para la etapa de Android y las licencias.
- **D10 · Impresora de tickets por caja** (contradicción 7): cada caja puede tener su propia impresora de tickets (por
  ejemplo, la USB de la Caja 2 por medio de su agente); si no la tiene, se usa la general como hoy.
- **D11 · Sin conexión** (contradicción 8): la terminal de escritorio muestra una capa «Sin conexión con el servidor» que
  bloquea las operaciones sin desmontar las pantallas (lo capturado y no guardado se conserva) y se reconecta sola.
- **D12 · Una sola migración `0023` con una sección por fase** (B, D, E y F) y esquema 23; la versión 0.10.0 se fija en
  la Fase G.
- **D13 · El agente de impresión** funciona mientras la aplicación de la terminal está abierta; si está cerrada o sin
  red, sus trabajos esperan y la cola lo dice («La Caja 2 está desconectada»).
- **D14 · Lo que exige la computadora real** (instalar el `.pkg`, mover los datos, comprobar que el servicio sobrevive al
  cerrar la ventana y que arranca sin iniciar sesión tras reiniciar) se hace en la Fase G con el propietario, avisándole
  de cada ventana; mientras tanto todo se prueba en copias aisladas.
- **Orden**: A ∥ D, después B ∥ F y al final C ∥ E (2 agentes a la vez, cada uno en su propio árbol de trabajo `git
worktree` en el directorio temporal); revisión independiente de B, D y F.
