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

## Fase A · Modalidades de instalación y servidor como servicio (macOS)

Un agente, en su propio árbol de trabajo (`etapa-9g-a`, desde `etapa-9g` en `ba83e55`), en paralelo con la Fase D.
13–14/11/2026. Sin migración (la 0023 no tiene sección A). Pruebas en copias aisladas: clúster temporal 55499, clústeres
propios de la prueba del `host` en 55511–55513, Go temporal 8184 (Playwright) y 8192–8194 (prueba del `host`), vista
previa 1422, Rust en `cache/cargo-A` y carpetas «del sistema» dentro del directorio temporal. **No se instaló nada, no se
usó `sudo` ni `launchctl` (salvo el simulado de las pruebas de los scripts) y no se tocó la instalación real.**

Commits: `dcce2c9` (código, pruebas y contrato) y `aa748b7` (instalador compilado y examinado: firma ad hoc de la copia
empaquetada y arquitectura del instalador). El fragmento vive fuera del repositorio (`frag/A.md`).

### Qué cambió

**Servidor Go (`backend/internal/standalone`, `backend/cmd/ad-el-pos-server`)**

- `internal/standalone` deja de ser solo de Windows. Lo común queda sin etiqueta de compilación (`layout.go`,
  `cluster.go`, `bootstrap.go`, `backup.go`, `testmode.go`, `mode.go`, `adopt.go`) y lo propio de cada sistema en
  `*_windows.go` (lo de la 9E, movido sin cambios: `CREATE_NO_WINDOW`, rutas cortas 8.3, `LockFileEx`,
  `QueryFullProcessImageName`, ACL de `fileacl`, carpeta conocida `LocalAppData`) y `*_unix.go` (nuevo):
  - **Carpeta de datos** `--data-dir` → `ADEL_DATA_DIR` → `/Library/Application Support/AD-EL POS` (`SystemDataDir`).
    **PostgreSQL** `--pg-bin` → `postgres_bin` del `server.json` existente → `ADEL_PG_BIN` → Homebrew
    (`/opt/homebrew/opt/postgresql@18/bin`; el enlace `opt` no se resuelve, así sobrevive a `brew upgrade`).
  - **Bloqueo** `host.lock` con `flock(LOCK_EX|LOCK_NB)` (pertenece al archivo abierto: también lo respeta otro intento
    del mismo proceso). **Permisos**: carpeta 0700 (también si ya existía), secretos y configuración 0600 y respaldos
    `pg_dump` 0600 (`privateFile`; antes de esto quedaban 0644 con la máscara de la consola).
  - **PostgreSQL vivo**: `postmasterAlive` = el PID de `postmaster.pid` existe (`kill 0`) y se llama `postgres`
    (`sysctl kern.proc.pid`). Un `postmaster.pid` solo se borra si su proceso ya no existe o es otro programa; con un
    PostgreSQL vivo nunca se borra (aunque fuera de otra versión de Homebrew), porque otro postmaster sobre la misma
    carpeta podría dañar los datos. `pg_ctl` separa a PostgreSQL en su propia sesión, así que launchd no lo mata con el
    grupo de procesos y, si Go muere con SIGKILL, el siguiente arranque lo adopta.
  - Los programas de PostgreSQL corren sin las variables `PG*` heredadas (como el respaldo de `internal/maintenance`).
  - `ADEL_TEST_MODE=1` rechaza la carpeta real del servicio (`RealDataDir` = `SystemDataDir`, nunca la variable ni la
    bandera; comparación sin mayúsculas y con enlaces resueltos) **antes de crear o abrir nada**.
- **Modalidad y habilitación** (`mode.go`): `installation-mode.json` (`{"installation_mode": …}`, lista cerrada de tres)
  y el archivo de habilitación `service-enabled` en la carpeta de datos, escritos 0600 con reemplazo atómico. Sin archivo
  de modalidad cuenta como «Servidor y terminal»; un archivo ilegible no se adivina (error).
- **`host` en macOS** (`host_unix.go`): si no hay archivo de habilitación o la modalidad es «Solo terminal», termina con
  código 0 **sin iniciar PostgreSQL ni Go** (launchd lo lanza al arrancar por `RunAtLoad`, pero sin el archivo no lo
  mantiene vivo). Si no, lo mismo que el de Windows: bloqueo, clúster propio con marcador, arranque o adopción,
  respaldo antes de migrar, migración y `serve`. Además:
  - antes de crear el clúster la primera vez exige el puerto de PostgreSQL libre (si lo ocupa la instalación anterior no
    crea nada y la carpeta sigue lista para `adopt-legacy`);
  - vigila cada 2 s el archivo de habilitación y la modalidad: si dejan de pedir servidor, deja de servir, detiene
    PostgreSQL, **conserva los datos** y termina con 0 (launchd no lo relanza);
  - `SIGTERM`/`SIGINT` (launchd al detenerlo o al apagar): cierre ordenado de Go y después `pg_ctl stop -m fast`.
- **Comandos compartidos** (`host.go`): `backup`, `restore-check` y `restore` pasan a macOS con la lógica de Windows. En
  macOS `restore` con el servicio en marcha lo **pausa** (quita el archivo de habilitación, espera hasta 90 s a tomar el
  bloqueo), crea el respaldo de seguridad, restaura y lo **reanuda** (vuelve a crear el archivo; launchd lo relanza);
  en Windows sigue diciendo «cierra AD-EL POS antes de restaurar un respaldo». `--pg-port` y `--listen` (solo pruebas, al
  crear una carpeta nueva) y `--from-config`.
- **`adopt-legacy --from-config <server.json anterior>`** (`adopt.go`, decisión D4): se niega si la carpeta del servicio
  ya tiene datos (clúster, configuración o marcadores) o si el servidor anterior responde en su `listen_address`; cuenta
  todas las tablas de `app` de la base anterior con su propio rol, la respalda (`pg_dump`, en `backups/` del servicio,
  `…-instalacion-anterior.dump`, 0600), comprueba el respaldo con `pg_restore --list`, vuelve a contar (si algo cambió,
  no sigue), crea el clúster nuevo y lo arranca **en un puerto temporal libre** (`pg_ctl -o "-p N"`: el anterior sigue
  ocupando 55432; la configuración y el marcador conservan el definitivo), restaura, compara los conteos de todas las
  tablas, lo detiene y deja `legacy-adopted.json` (sin secretos). Si algo falla quita solo lo que creó (clúster,
  `server.json`, marcador del clúster) y conserva el respaldo. **Nunca modifica la instalación anterior.**
- **Rutas** (`administration/service_host.go`, bloque en `routes.go` y caso en `completeDeferred`):
  `GET maintenance/service` y `POST maintenance/service/restart` con `settings.manage` (contrato en `API_CONTRACTS.md`,
  bloque «Etapa 9G · servidor como servicio de macOS y modalidades»). `Service.Host` (`HostControl`) lo implementa el
  `host` de macOS: `managed` = proceso padre 1 (launchd). El reinicio se audita (`maintenance.service_restarted`,
  «Servidor reiniciado» en el Registro de movimientos) y, ya confirmado, el servidor se cierra en orden a los 300 ms;
  sin servicio administrado o sin habilitación, `409 SERVICE_NOT_MANAGED` sin auditar. Las terminales conectadas se
  cuentan sin la que pregunta y sin las revocadas, con `last_seen_at` de los últimos 2 minutos. `last_backup` = `null`
  (hueco para la Fase F).

**Servicio e instalador (`ops/launchd`, `scripts/macos-pkg`, `scripts/build-macos-pkg.sh`)**

- `ops/launchd/local.adel.pos.server.plist` (sustituye al `.example`): plantilla con `__ADEL_USER__`,
  `__ADEL_SERVER__` y `__ADEL_DATA_DIR__`; `UserName`, `ProgramArguments` = servidor de dentro de la aplicación `host
--data-dir …`, `WorkingDirectory`, `RunAtLoad`, `KeepAlive` → `PathState` del archivo de habilitación,
  `ThrottleInterval` 10, `ExitTimeOut` 90, `Umask` 077 y bitácoras `logs/service.log` y `logs/service-error.log`.
- `scripts/build-macos-pkg.sh` (`dev.sh build-macos-pkg [--reuse-app]`): compila el servidor como binario externo de
  Tauri (`desktop/src-tauri/tauri.macos-pkg.conf.json`: `Contents/MacOS/ad-el-pos-server`, firmado con la aplicación),
  comprueba la firma (`codesign --verify --deep --strict`) y arma tres paquetes con `pkgbuild` y la distribución
  (`scripts/macos-pkg/Distribution.xml`, `customize="always"`, bienvenida en español) con `productbuild`:
  - `local.adel.pos.app` (siempre): `AD-EL POS.app` a `/Applications`, **no reubicable**; `preinstall` descarga el
    servicio anterior (`launchctl bootout`, espera a que termine).
  - `local.adel.pos.server` = opción **«Servidor del restaurante» (marcada por omisión; desmarcarla es «Solo
    terminal»)**: `preinstall` exige PostgreSQL 18 (si falta, el instalador falla con el aviso de instalarlo o
    desmarcar la opción); `postinstall` crea la carpeta de datos de la cuenta que instala (0700), guarda «Servidor y
    terminal» (conserva «Solo servidor» si ya estaba) y crea el archivo de habilitación.
  - `local.adel.pos.service` (siempre, invisible, al final): crea la carpeta si falta (con «Solo terminal» si no hay
    modalidad), escribe el plist con la cuenta de la consola (`stat -f %Su /dev/console`), lo valida con `plutil`, lo
    deja `root:wheel 0644` y hace `launchctl enable` + `launchctl bootstrap system`.
  - Seguridad de los scripts: si la carpeta de datos existe y es de otra cuenta, **no la toca** y falla; la cuenta se
    valida (no root, no `_mbsetupuser`) antes de ponerla en el plist; solo el disco de arranque. Las bitácoras de launchd
    se crean de la cuenta que instala.
  - **Parametrizables para pruebas**: sin root solo corren con `ADEL_PKG_TEST_ROOT` (prefijo de destino) y
    `ADEL_PKG_TEST_LAUNCHCTL` (launchctl simulado); como root esas variables se ignoran. `scripts/macos-pkg/
test-scripts.sh` los ejecuta así (servidor nuevo, actualización con «Solo servidor», «Solo terminal» y rechazos).

**Escritorio (Rust)**

- `connection.rs`: admite `server-only`; `server_url` vacío solo en `terminal-only` sin certificado (terminal sin
  servidor: no se abre el Keychain ni se hace ninguna petición; `AWAITING_SERVER_MESSAGE` en `main.rs`).
- `installation.rs` (nuevo): lista cerrada de modalidades; `installation_state`, `set_installation_mode {mode}` y
  `restart_application` (`AppHandle::request_restart`), registrados en `build.rs`, `permissions/diagnostics.toml` y
  `main.rs`. Cambiar la modalidad escribe primero el lado del servidor (modalidad y archivo de habilitación, si el
  servicio está instalado: plist en `/Library/LaunchDaemons` y carpeta de datos) y después `connection.json` (0600,
  reemplazo atómico): con servidor `http://127.0.0.1:8181`; «Solo terminal» conserva un servidor emparejado o queda vacía.
  Sin permiso en la carpeta del servicio («Solo la cuenta de la computadora que instaló AD-EL POS puede cambiar su
  modalidad.») no cambia nada. Nunca borra la base ni otros archivos. `chosen` = existe `connection.json` (o
  `ADEL_TERMINAL_CONFIG`); **en Windows siempre `true` y `can_change` `false`** (la modalidad de Windows es de la Fase W).
- El token del asistente inicial también sale de la carpeta del servicio (`provisioning-token`) en una modalidad con
  servidor, para preparar el establecimiento desde la propia Mac en una instalación nueva.
- `api.rs`: `maintenance_service` y `maintenance_service_restart`.

**Escritorio (React)**

- `App.tsx`: `InstallationGate` antes de la sesión: sin modalidad elegida → `pages/setup/ModalityChoicePage.tsx`
  (sugiere la del instalador: con servicio instalado y sin habilitar, «Solo terminal»; si no, «Servidor y terminal»);
  «Solo terminal» sin servidor → `pages/setup/FindServerPage.tsx` («Buscar el servidor»: «Esta terminal todavía no está
  conectada al servidor del restaurante», aviso de datos conservados y «Usar esta computadora como servidor» con
  confirmación). **Punto de integración de las fases B y C** marcado en `FindServerPage.tsx`: cuando Rust guarde el
  servidor emparejado, `server_configured` pasa a `true` y se muestra el acceso normal.
- «Solo servidor» (`pages/server/ServerOnlyPage.tsx`): antes de iniciar sesión, «Servidor del restaurante» con el
  estado (consultas públicas de salud) y «Entrar a Administración»; después, barra reducida («Estado del servidor»,
  «Administración» si la puede abrir, estado de la conexión, usuario y pantalla completa) y la pantalla de estado
  (servidor activo y desde cuándo, terminales conectadas —solo con `settings.manage`—, «Último respaldo: Sin registro»
  como hueco de la Fase F y «Abrir Administración»). Sin interfaz de venta: `requestScreen` rechaza las secciones de
  operación, no se consulta Caja y los atajos y la paleta no se ofrecen. Pantalla nueva `server-status` en
  `navigation/sections.ts` (`serverOnly`, la paleta no la ofrece en otras modalidades).
- Mantenimiento (`pages/maintenance/ServerSections.tsx`, insertado en `MaintenancePage.tsx`): «Servicio del servidor»
  (estado en lenguaje sencillo, desde cuándo y terminales; «Reiniciar» con confirmación, solo si Go lo ofrece) y
  «Modalidad de esta computadora» (opciones con «Actual» y «Cambiar la modalidad» con confirmación que explica lo que
  cambia; al dejar de ser servidor: «La base de datos se conserva en esta computadora y no se borra nada»; luego la
  aplicación se reinicia). Opciones grandes con patrón de radio (`pages/setup/ModalityOptions.tsx`). Tokens de color en
  un bloque nuevo al final de `styles.css`. «Instalación» del estado del sistema muestra las tres modalidades.
- `services/installation.ts`, `services/serverService.ts`, etiqueta de auditoría en `AuditPage.tsx`.

**Pruebas y documentación**

- Go: `standalone/standalone_unix_test.go` (rutas y precedencias, rechazo de la carpeta real, `flock`, modalidad y
  habilitación incluido el formato que escriben Rust y el instalador, configuración de prueba, respaldos fuera de
  `backups`, `postmasterAlive`, `HasData`, puerto temporal), `standalone/plist_test.go` (el plist real con los valores
  del `postinstall`), `cmd/…/host_unix_test.go` (vigilancia y reinicio) y **`cmd/…/host_unix_integration_test.go`**
  (`integration`, macOS: el binario real como proceso, ver «Pruebas»). `administration/service_host_acceptance_test.go`
  (llamado desde `TestStageTwoAcceptance`). `./cmd/ad-el-pos-server` se agregó a la tupla de `scripts/database.py`.
- Rust: `connection.rs` (tres modalidades, terminal sin servidor), `installation.rs` (7 pruebas), `api.rs`.
- Vitest `pages/stage9gModality.test.tsx` (14) y ajustes de `maintenance.test.tsx`, `App.test.tsx`,
  `inventory.test.tsx` (el puente simulado responde `installation_state`).
- Playwright `workflow-stage9g-modality.spec.ts` y `native-driver.ts` (simula `installation_state`,
  `set_installation_mode` con la misma lista cerrada y `restart_application` como recarga; `setInstallation` para la
  prueba; `connection_config` devuelve la modalidad simulada).
- `API_CONTRACTS.md`: bloque nuevo al final.

### Decisiones tomadas en la fase

1. **Semántica de `PathState`** (de `man launchd.plist`, macOS 26.5): «If the value of the key is true, then the job will
   be kept alive as long as the path exists»; con `KeepAlive` como diccionario launchd hace un lanzamiento especulativo
   al cargar (`RunAtLoad` implícito) y, si no hay razón para relanzarlo, vuelve a la invocación por demanda. **Supuestos
   documentados** (no verificables sin instalar): (a) quitar el archivo **no detiene** un trabajo en marcha, por eso el
   propio `host` vigila el archivo y termina; (b) crear el archivo hace que launchd lance el trabajo (vigilancia del
   sistema de archivos, que el manual describe como «race-prone and lossy»): si en la Fase G no arrancara solo, basta
   abrir la aplicación o reiniciar la Mac (`RunAtLoad`), y queda anotado; (c) el relanzamiento respeta `ThrottleInterval`.
2. El servicio siempre queda registrado (también en «Solo terminal», inactivo): así cambiar a una modalidad con servidor
   desde la aplicación no pide la contraseña de administrador (D3). «Solo terminal» no inicia PostgreSQL ni Go.
3. El reinicio solo se ofrece si launchd lo va a relanzar (proceso padre 1 y archivo de habilitación); con `serve`
   (desarrollo, Playwright) la sección dice que el servidor no es un servicio de la computadora.
4. Restaurar en macOS pausa y reanuda el servicio con el archivo de habilitación (sin `sudo` ni `launchctl`).
5. El paso de datos usa un puerto temporal (el anterior sigue en 55432) y se niega con el servidor anterior en marcha.
   Además el `host` no crea el clúster si el puerto de PostgreSQL está ocupado: si en la Fase G se dejara marcada la
   opción de servidor por error, no se crea nada que impida el paso de datos.
6. Modalidad elegida en el primer arranque **solo en macOS**; Windows no cambia hasta la Fase W.
7. Tras cambiar la modalidad la aplicación se reinicia (en lugar de recargar la configuración en caliente, que tocaría
   cada uso de `state.config` en `api.rs`, `realtime.rs` y `main.rs`, compartidos con otras fases).
8. Los cambios de modalidad no se auditan en Go: los hace Rust en la propia computadora y, al dejar de ser servidor, el
   servidor se detiene. El reinicio sí se audita.
9. PostgreSQL no viaja en el `.pkg` (D3): la opción de servidor exige el de Homebrew y falla con un aviso si falta.
10. Tauri deja la aplicación sin firma de paquete (solo la firma ad hoc del enlazador en cada ejecutable; por eso
    `codesign --verify --deep` falla en el `.app` de `build-macos`): la copia que va en el `.pkg` se firma ad hoc como
    paquete completo (`codesign --force --deep --sign -`) y se verifica. Sin firma de desarrollador (etapa 10).
11. `hostArchitectures` del instalador = la arquitectura compilada (`lipo -archs`; arm64 en esta Mac): la aplicación y
    el servidor no son universales.

### Pruebas ejecutadas (aisladas)

| Suite                                                                                                                   | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `format` (Prettier, gofmt, cargo fmt; `docs/etapas` intacto por md5)                                                    | aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `sync`                                                                                                                  | aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `vet` (normal e `integration`)                                                                                          | aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `win` (`GOOS=windows` build, vet y vet `integration`)                                                                   | aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Go unitarias con `-race` (`go test ./...`)                                                                              | aprobado (14 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS`, `-race`)                                                | aprobado, 41–43 s                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Integración `./internal/administration` (`TestStageTwoAcceptance` con `testServiceStatus`)                              | aprobado, 82 s                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `cargo test --locked --offline`                                                                                         | 54 aprobadas, 1 ignorada                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `tsc --noEmit`                                                                                                          | sin errores                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Vitest (`npm test`)                                                                                                     | 87 archivos, 835 pruebas aprobadas (14 nuevas en `stage9gModality.test.tsx`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Playwright Chromium (`restaurant`, `workflow-stage9c-navigation`, `workflow-stage9g-modality`, `workflow-zz-test-data`) | 4 de 4 (1.7 min)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Playwright WebKit (las mismas, pantalla desbloqueada)                                                                   | 4 de 4 (1.7 min)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Scripts del instalador con destino falso (`scripts/macos-pkg/test-scripts.sh`)                                          | 31 comprobaciones aprobadas (servidor nuevo, actualización que conserva «Solo servidor», «Solo terminal», sin PostgreSQL, carpeta de otra cuenta y sin destino de prueba)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `.pkg` compilado y examinado                                                                                            | `dev.sh build-macos-pkg`: aplicación con el servidor dentro (Tauri 1 min 23 s en `release`), `AD-EL POS_0.9.5.pkg` de 20 MB (la versión será la de `config/product.json`); `pkgutil --expand`: `Distribution` con las tres opciones y `hostArchitectures="arm64"`, `app.pkg` no reubicable con `AD-EL POS.app` (`Contents/MacOS/ad-el-pos-desktop` y `ad-el-pos-server`, firma ad hoc de paquete verificada con `codesign --verify --deep --strict`), `server.pkg` y `service.pkg` sin contenido con sus scripts idénticos a los del repositorio (`bash -n` correcto) y la plantilla del plist (`plutil -lint` correcto). El servidor del paquete, con `ADEL_TEST_MODE=1`, termina con 0 sin habilitación en una carpeta temporal y rechaza la carpeta real sin crearla. **No se instaló** |

`TestServiceHostOnMacOS` recorre con el binario real (la prueba se vuelve a ejecutar a sí misma como el servidor):
sin habilitación y en «Solo terminal» termina con 0 sin crear nada; con el puerto de PostgreSQL ocupado no crea el
clúster; habilitado crea el clúster (carpeta 0700, `server.json`/`admin-password` 0600, marcador), migra y sirve
(`test_mode` informado); un segundo `host` termina con error «ya está en uso»; `backup` (0600) y `restore-check` con el
servicio en marcha; quitar el archivo de habilitación lo detiene en orden y conserva los datos; `restore` con el
servicio en marcha lo pausa, restaura (con respaldo de seguridad) y lo vuelve a habilitar; `SIGTERM` cierra Go y
PostgreSQL (`server_shutdown_complete`, `cluster_stopped`); `adopt-legacy` desde una base del clúster 55499 con todas
las migraciones (se niega con el servidor anterior escuchando, pasa los datos con conteos idénticos de todas las tablas,
deja el marcador sin secretos y 0600, no cambia la base anterior, se niega a repetirse) y el servicio arranca después
con esos datos.

### No verificado (exige la computadora real; Fase G)

- Instalar el `.pkg`, que launchd lance el servicio al cargar y al encender sin iniciar sesión, que lo relance al
  reiniciar desde Mantenimiento y al crear el archivo de habilitación (supuesto 1.b), el aviso «Elementos en segundo
  plano añadidos» y el acceso al Keychain desde la aplicación instalada en `/Applications`.
- El `postinstall` como root (`chown`, `root:wheel`, `launchctl bootstrap system`) solo se probó sin root con destino
  falso y `launchctl` simulado.
- No hay `shellcheck` en el equipo: los scripts se comprobaron con `bash -n` y su ejecución con destino falso.

### Procedimiento para la Fase G (instalación real, con el propietario)

Parada del §0 de la especificación: instalar pide la contraseña de administrador y conviene reiniciar al final. Se
hace después de fusionar y aplicar todo en `REAL` (versión 0.10.0, esquema 23) y con la regresión aprobada.

1. **Compilar** (sin efectos sobre datos): `./scripts/dev.sh build-macos-pkg` →
   `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.10.0.pkg` (anotar SHA-256).
2. **Respaldo y ensayo** de la instalación actual: `./scripts/dev.sh backup` y
   `python3 scripts/restore-check.py backups/<archivo>` (conteos idénticos). Comprobación de solo lectura de conteos
   como en la 9F, sin turnos en arqueo ni trabajos de impresión pendientes.
3. **Detener solo Go** de la instalación actual: `./scripts/dev.sh server-stop` (PostgreSQL de `.local` **sigue en
   marcha**: `adopt-legacy` lo lee). Cerrar la aplicación 0.9.5 si está abierta.
4. **Instalar el `.pkg` (propietario)**: doble clic en el `.pkg` → Instalador de macOS → Introducción → **Tipo de
   instalación: desmarcar «Servidor del restaurante»** (solo esta primera vez: el servicio aún no debe crear una base
   vacía) → Instalar → **ventana de contraseña de administrador** («Instalador está intentando instalar software
   nuevo», contraseña o Touch ID) → Cerrar. macOS mostrará la notificación **«Elementos en segundo plano añadidos»**
   (permitir; en Ajustes › General › Ítems de inicio y extensiones › «Permitir en segundo plano» debe quedar activado).
   Queda: `/Applications/AD-EL POS.app`, `/Library/Application Support/AD-EL POS` (de `eliver`, 0700, «Solo terminal»)
   y el servicio cargado e inactivo. Comprobar (solo lectura): `launchctl print system/local.adel.pos.server` (estado
   `not running`), `ls -le "/Library/Application Support/AD-EL POS"`, `cat ".../logs/server.log"` (`host_disabled`).
   Si por error se dejó marcada la opción: el servicio no crea nada mientras 55432 esté ocupado (lo dice
   `logs/server.log`); quitar `".../service-enabled"` (sin `sudo`) y seguir.
5. **Pasar los datos** (sin `sudo`, como `eliver`):
   `"/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" adopt-legacy --from-config "/Users/eliver/Documents/ADEL POS/ad-el-pos/.local/server.json"`
   → «Datos pasados y comprobados: N tablas con conteos idénticos.» y el nombre del respaldo; revisar
   `legacy-adopted.json` (`counts_equal: true`, 134+ tablas). `.local` y `backups/` del proyecto no se tocan.
6. **Detener PostgreSQL anterior**: `./scripts/dev.sh db-stop` (55432 queda libre).
7. **Habilitar el servidor**: abrir `/Applications/AD-EL POS.app` → «¿Cómo se usará esta computadora?» (sugiere «Solo
   terminal») → elegir **«Servidor y terminal»** → Continuar. La aplicación escribe la modalidad y `service-enabled`;
   launchd lanza el servicio (arranca PostgreSQL de la carpeta nueva en 55432, respalda `…-antes-de-migrar` si hay
   migraciones pendientes, migra y sirve en 8181) y la aplicación se reinicia. Puede aparecer el diálogo del
   **Llavero** («AD-EL POS quiere usar información confidencial guardada en "local.adel.pos.dev"»): contraseña de la
   sesión y **«Permitir siempre»** (la aplicación instalada es otra firma ad hoc). La credencial
   `http://127.0.0.1:8181|main` no cambia, así que la terminal sigue enrolada. Alternativa sin la ventana (si el
   lanzamiento por `PathState` no ocurriera, supuesto 1.b): reiniciar la Mac.
8. **Verificar**: `curl -s http://127.0.0.1:8181/api/v1/health/ready` (200, esquema compatible) y `…/version` (0.10.0,
   `test_mode` ausente); `lsof -nP -iTCP:55432 -sTCP:LISTEN` y `-iTCP:8181` (procesos de `eliver`, `postgres` con datos
   en `/Library/Application Support/AD-EL POS/postgres`); `logs/server.log` (`host_ready`, `server_started`);
   Mantenimiento › «Servicio del servidor» dice que funciona como servicio y ofrece «Reiniciar»; conteos de solo lectura
   iguales a los del paso 2 (más lo que agregue la 0023). **Cerrar la ventana** y repetir el `curl` (sigue activo).
9. **Reiniciar desde Mantenimiento** → «Reiniciar» → en ≈ 10–15 s vuelve (`logs/server.log`:
   `service_restart_requested` y un `host_starting` nuevo).
10. **Reiniciar la Mac (parada: avisar al propietario)** y, sin abrir la aplicación, comprobar al iniciar sesión que el
    `host_starting` de `logs/server.log` es anterior al inicio de sesión (`ps -o lstart= -p <pid de ad-el-pos-server>`).
11. Desde entonces **no usar `dev.sh db-start`, `server-start`, `migrate`, `backup` ni `restore-check.py` con la
    instalación real**: actúan sobre la base anterior de `.local` (que queda como copia histórica) y ocuparían los
    mismos puertos. Respaldos de la instalación: `"/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server" backup`
    y `restore-check` (en `/Library/Application Support/AD-EL POS/backups`); restaurar: `… restore --file <respaldo>`
    (pausa y reanuda el servicio). Desinstalar el servicio, si hiciera falta (pide contraseña):
    `sudo launchctl bootout system/local.adel.pos.server` y borrar `/Library/LaunchDaemons/local.adel.pos.server.plist`;
    los datos se conservan.

### Para la Fase W (Windows) y otras fases

- **Fase W**: servicio de Windows con el mismo `host` (falta `HostControl` para el administrador de servicios, la
  vigilancia de la modalidad y el archivo de habilitación en `host_windows.go`, y `pauseServerForRestore`, que hoy pide
  cerrar la aplicación); carpeta `%ProgramData%\AD-EL POS` en `layout_windows.go` (hoy `%LOCALAPPDATA%`, que sigue para
  la instalación por usuario de la 9E con `server_host.rs`, sin cambios); `ServicePaths::system()` y `Context::detect`
  de `installation.rs` para Windows (hoy `None`, `chosen` siempre `true` y `can_change` `false`); modalidad en el
  instalador NSIS; paso de la instalación por usuario: `AdoptLegacy` sirve tal cual con el `server.json` de
  `%LOCALAPPDATA%\AD-EL POS` (lee con su rol, no lo modifica). `server_host.rs` solo lanza el servidor en
  `server-terminal`: con `server-only` no lo haría.
- **Fases B y C**: `FindServerPage.tsx` es el punto de integración (búsqueda, dirección manual y emparejamiento);
  Rust debe escribir el servidor emparejado en `connection.json` (`terminal-only` con `server_url`, que
  `connection_for` ya conserva) y `server_configured` pasa a `true`. La escucha HTTPS 8443 (D1) solo con una modalidad
  con servidor: `standalone.ServiceWanted` y el `installation_mode` de `HostStatus` dicen cuál.
- **Fase F**: llenar `last_backup` de `GET maintenance/service` (hoy `null`) y la tarjeta «Último respaldo» de
  `ServerOnlyPage.tsx` («Sin registro»); en el `host` de macOS el respaldo es `standalone.Backup`
  (`installationBackup`), con la carpeta `backups` del servicio.

### Para CLAUDE.md

- §3: en macOS, desde la 9G, el servidor es un **servicio del sistema** (LaunchDaemon `local.adel.pos.server`,
  `ops/launchd/local.adel.pos.server.plist`) que corre con la cuenta del usuario que instaló, con datos en
  `/Library/Application Support/AD-EL POS` (0700) y PostgreSQL de Homebrew; `internal/standalone` y `host` ya no son
  solo de Windows (`*_windows.go` / `*_unix.go`). La aplicación sigue sin iniciar ni detener Go: solo crea o quita el
  archivo de habilitación `service-enabled` al cambiar la modalidad.
- Modalidades (§2.3): `server-terminal`, `server-only`, `terminal-only`; lado servidor `installation-mode.json` +
  `service-enabled` (KeepAlive → PathState); lado terminal `connection.json` (solo Rust, lista cerrada, se reinicia la
  aplicación al cambiar). Primer arranque en macOS sin `connection.json` → elección. «Solo servidor» sin interfaz de
  venta. Cambiar de modalidad nunca borra la base.
- §5: `dev.sh build-macos-pkg [--reuse-app]` (instalador `.pkg`, no instala nada); comandos del servidor `host`,
  `backup`, `restore-check`, `restore` y `adopt-legacy` en macOS; `ADEL_DATA_DIR`; `scripts/macos-pkg/test-scripts.sh`.
  Tras la Fase G, los comandos de `dev.sh` que actúan sobre la base (`db-*`, `server*`, `migrate`, `backup`,
  `restore-check.py`) **ya no son la instalación real** (lo es el servicio).
- §6: `ADEL_TEST_MODE=1` también rechaza la carpeta del servicio en macOS. `./cmd/ad-el-pos-server` tiene pruebas de
  integración (`TestServiceHostOnMacOS`, clústeres propios en 55511–55513 y Go en 8192–8194; necesita PostgreSQL 18 de
  Homebrew o `ADEL_PG_BIN`); `iso.sh` solo toma por omisión los paquetes `./internal/…`: pasarlo con `ISO_PKGS`.
- §7: ninguna regla nueva de red en esta fase (la escucha en la red es de la Fase B, D1).
- §15: permiso sin cambios (`settings.manage` para el servicio); auditoría nueva `maintenance.service_restarted`.
- §19: supuestos de `PathState` (1.a–c) hasta verificarlos en la Fase G; PostgreSQL fuera del paquete (etapa 10); el
  `.pkg` y la aplicación con firma ad hoc (el Llavero vuelve a pedir permiso con cada compilación instalada).

**Verificación del coordinador** (13/11/2026): copia exacta de `aa748b7` (`git archive`) en el entorno aislado: formato,
`sync --check`, `go vet` (normal e `integration`), `GOOS=windows` build y vet, Go unitarias con `-race`, integración
`./cmd/ad-el-pos-server` (43 s) y `./internal/administration` (92 s), `tsc`, Vitest y `cargo test`: todo aprobado. Fusión
en `etapa-9g` por avance rápido.

## Fase D · API con versión y preparación para Android

Un agente (13/11/2026), en su árbol de trabajo `etapa-9g-d` (desde `etapa-9g`), en paralelo con la Fase A. Decisiones del
coordinador D8 («Agotado»), D9 (una sesión por usuario, sin cambio) y D12 (una sola 0023 con una sección por fase).
Pruebas en el clúster temporal 55499, Go temporal en 8185, vista previa en 1423 y Rust en `cache/cargo-D`; nunca contra la
instalación real. Commits: `f670382` (servidor), `7842d60` (contrato OpenAPI y pruebas de contrato), `dbf6c19` (escritorio) y
`545683b` (pruebas de actualización anteriores adaptadas a la 0023).

### Qué cambió

**Migración `0023_network_terminals_and_android.sql`** (creada en esta fase con el encabezado general y la sección
`-- Fase D`; esquema 23 en `config/product.json` y `sync`; la versión sigue en 0.9.5):

1. `catalog_products.sold_out_at`, `sold_out_until` (NULL = hasta nuevo aviso) y `sold_out_by` con
   `catalog_products_sold_out_check` (marca completa o vacía, fin posterior al inicio) e índice parcial.
2. Permiso `catalog.sold_out.manage` («Marcar productos como agotados y disponibles otra vez desde la captura»), solo al
   administrador; también en `catalog.Permissions` con la misma descripción.
3. Ámbito `catalog` en `realtime_events_scope_check`. **Si otra fase redefine esta restricción después, debe conservar
   `catalog`.**
4. Versión del catálogo: secuencia `catalog_version_seq`, tabla `catalog_change_log` (una fila por registro: entidad,
   id, último número, borrado) y disparadores en las 19 tablas del catálogo y en `payment_settings.tax_rate`. Un número
   por transacción (variable local `adel.catalog_version`); el disparador toma el lock 7820148304 (reentrante: toda
   escritura del catálogo ya lo tiene exclusivo; el IVA general lo toma después de 7820148305, en el orden de siempre).
5. Comentario de conservación en `restaurant_operations` (las claves se guardan para siempre).

**Go**

- `internal/httpapi/clientversion.go` + `router.go`: cabeceras `X-Client-Version` y `X-Client-Kind`; las rutas de la
  aplicación responden `426 CLIENT_UPDATE_REQUIRED` («Actualiza esta terminal», con `details`) a una versión anterior a
  `min_client_version`; versión o tipo ilegibles → 400. **Sin cabecera se acepta** (herramientas, pruebas, aplicaciones
  anteriores a la 9G que solo existen en la propia computadora principal). `/version`, `/health/*` y `/openapi.json`
  siempre responden. `/version` agrega `min_client_version`, `client_kinds`, `openapi_path` y funciones nuevas.
  `GET /api/v1/openapi.json` sirve el contrato.
- `internal/product`: `MinClientVersion` y `MinServerVersion` (de `config/product.json`, hoy 0.9.5 los dos).
- `internal/apicontract` (nuevo, en la tupla de `scripts/database.py`): `openapi.json` (OpenAPI 3.1, 20 operaciones),
  validador de un subconjunto de JSON Schema sin dependencias (una palabra clave fuera del subconjunto hace fallar),
  pruebas unitarias del documento y prueba de integración `TestContractV1`.
- `internal/idempotency` (nuevo, neutral): huella, `Replay` y `Remember` sobre `restaurant_operations` con las reglas de
  pedidos y cobros; `Remember` devuelve lo guardado para que la primera respuesta y la repetida sean idénticas. Lo usa
  «Agotado»; los módulos anteriores no se tocaron.
- `internal/catalog`: `soldout.go` (ruta `POST catalog/products/{id}/sold-out`, fin con `businessday` o hasta nuevo
  aviso, auditoría, evento, `ResetExpiredSoldOut` y `SoldOutWorker` cada 30 s); `sync.go` (`GET catalog/sync[?since=X]`,
  `Version`); `images.go` (`GET catalog/images/{id}/content` binaria con `ETag`/304, tipo `Binary`); `actions.go`,
  `calculator.go` y `group_calculator.go` (agotado = no disponible, sin excepción; `409 PRODUCT_SOLD_OUT` para el
  producto, un complemento o un componente; `Error.ProductID` interno); `handler.go` (`route` separado de `Handle`,
  evento `catalog.changed` en toda escritura correcta, `sold_out*` en el listado); `context.go` (permiso, `Actor` con
  usuario y terminal, `IfNoneMatch`); `models.go` (`SoldOut*` en `Product`, omitidos si vacíos).
- `internal/operations`: `offline_orders.go` (`POST restaurant/offline-orders`), `recovery.go` (`Recovery`),
  `confirmations.go` (`confirmLines` compartido por la confirmación y el pedido sin conexión, sin cambio de
  comportamiento), `handler.go` (permiso de la ruta y sin evento genérico duplicado), `context.go` (`Error.Rejection`).
- `internal/administration`: `recovery.go` (`GET restaurant/state`, antes del prefijo `restaurant/`), `binary.go`
  (respuesta binaria y 304), `routes.go` (`operationFault` con auditoría de rechazos, actor e `IfNoneMatch` del
  catálogo), `http.go` y `service.go` (`If-None-Match`).
- `internal/events/store.go`: ámbito `catalog` → `catalog.read`.
- `cmd/ad-el-pos-server/main.go`: arranca `catalog.SoldOutWorker` junto a los de cocina e inventario (bloque acotado).
- `internal/database`: `stage9g_android_upgrade_test.go` (actualización desde el esquema 22, para que las fases B, E y
  F agreguen sus comprobaciones) y las dos pruebas 9F que exigían «la última migración es la 22» ahora migran desde la
  21 hasta la última.

**Escritorio**

- Rust: `api.rs` (`with_client_headers`, `CLIENT_VERSION` = `CARGO_PKG_VERSION`, `CLIENT_KIND` = `desktop`;
  operaciones `version` y `catalog_product_sold_out`; pruebas), `realtime.rs` y `export.rs` (mismas cabeceras).
- React: `services/compatibility.ts` (comparación de versiones y avisos), `hooks/useSession.ts` (fases
  `update-terminal` y `update-server`; la versión del servidor se comprueba al conectar y después de perder la
  conexión), `services/api.ts` (evento `adel-client-update-required` con un 426), `App.tsx` («Actualiza esta terminal»
  y «Actualiza el servidor» con «Volver a comprobar» y «Ver estado del sistema»), `services/soldOut.ts`,
  `ProductGrid.tsx` («Marcar agotados» solo con el permiso: barra con «Solo por hoy» / «Hasta nuevo aviso»; un toque
  marca o devuelve; etiqueta «Agotado» o «Agotado hasta nuevo aviso» y producto no seleccionable), `CaptureScreen.tsx`,
  `captureRules.ts`, `AuditPage.tsx` (nombres de las acciones nuevas) y `styles.css` (solo tokens). La recarga por el
  evento nuevo no necesitó código: todo evento invalida las consultas vivas.
- `desktop/e2e/native-driver.ts`: el puente envía las cabeceras de cliente como Rust (sin comandos Tauri nuevos).
- Documentación: `docs/API_V1.md` (nuevo) y sección 9G en `API_CONTRACTS.md`.

### Decisiones de la fase

- **Peticiones sin `X-Client-Version`**: se aceptan como «cliente sin identificar» (ver arriba). Toda aplicación desde
  la 9G envía las dos cabeceras; la fase B puede exigirlas en la escucha de red si lo considera necesario.
- **Versiones mínimas en `config/product.json`**: `min_client_version` y `min_server_version`, hoy 0.9.5. **En la Fase G,
  al fijar 0.10.0, subir las dos a 0.10.0** (el escritorio 0.10.0 usa rutas que un servidor 0.9.5 no tiene; un cliente
  0.10.0 es el primero que se identifica). `services/compatibility.test.ts` comprueba que ninguna mínima supere la
  versión.
- **«Agotado» aplica a productos**, también cuando son complemento (`complements.product_id`) o componente de combo. Los
  complementos que son ingredientes y las porciones no se agotan desde la captura (se desactivan en Catálogo). Es un
  hecho físico: ninguna excepción de disponibilidad (`catalog.availability.override` de la vista previa) lo pasa por
  alto. No cambia la revisión del producto. El fin se calcula al marcarlo (cambiar después la «Hora de inicio del día»
  no lo mueve); la lectura ya ignora una marca vencida y el trabajador la limpia (sube la versión, audita como proceso y
  avisa).
- **El catálogo publica `catalog.changed` en toda escritura** (no solo «Agotado»), como pide D8; la nota de `CLAUDE.md` §6
  («Los cambios de catálogo no publican eventos…») deja de ser cierta.
- **Catálogo descargable**: requiere `catalog.read` y `catalog.prices.read`; precio por producto y presentación con
  `effectivePrice` de la lista predeterminada (la misma función de `catalog.Calculate`); disponibilidad calculada al
  descargar; archivados fuera de la descarga completa y con `archived` en la de cambios; un cambio de categoría trae sus
  productos y un cambio de lista predeterminada o de IVA general, todos. Las promociones no se descargan (el servidor las
  aplica al confirmar).
- **Pedido sin conexión**: destino = cuenta abierta o mesa (cuenta nueva). Cambios aceptables con la confirmación
  (`price_changed`, `table_has_account`) y cambios que exigen corregir el pedido (agotado, eliminado, no disponible,
  complemento o parte no disponible, partida inválida, cuenta cerrada, mesa fuera de servicio). La confirmación debe
  traer la versión vigente del catálogo **y** cubrir cada cambio vigente (tipo, partida, producto y valor «ahora»), así
  que una confirmación con catálogo viejo o de otro precio se rechaza de nuevo. Se compara el precio por unidad antes de
  promociones. El rechazo no guarda la clave: el reenvío confirmado usa la misma `operation_id` (así un envío tardío del
  original es `IDEMPOTENCY_CONFLICT` y nunca un pedido doble). Confirma sus propias partidas sin tocar el borrador de la
  cuenta (otra terminal puede estar capturando en ella) y sube la revisión de la cuenta. Auditoría
  `restaurant.offline_order.accepted` y, por rechazo, `restaurant.offline_order.requires_confirmation` (`success=false`,
  conservada aunque se revierta, como los rechazos de cobro); `order.confirmed` lleva `offline`.
- **`GET restaurant/state`** responde a cualquier sesión válida (en `administration`, antes de `operations`), con listas
  `null` sin el permiso; una terminal revocada recibe 401 `DEVICE_REVOKED` como en cualquier ruta.
- **OpenAPI en JSON** (no hay biblioteca YAML) y contrato acotado a la API de clientes. Ampliarlo: documentar la operación
  en `openapi.json`, explicarla en `docs/API_V1.md` con su `operationId` y su ruta (lo comprueba una prueba) y agregar su
  paso en `contractScenario`; la prueba falla si una operación documentada no se recorre.

### Pruebas

13/11/2026, en el árbol `etapa-9g-d` con el arnés `iso.sh` (clúster 55499, Go 8185, vista previa 1423, Rust en
`cache/cargo-D`). La pantalla del equipo no estaba bloqueada (`CGSSessionScreenIsLocked` ausente).

| Suite                                                                                                                                                     | Resultado                                                                                                                                                                                    |
| --------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`), `sync --check`                                                                                                  | aprobado; `docs/etapas/*.md` con el mismo md5 del inicio                                                                                                                                     |
| `go vet` (normal e `integration`) y `GOOS=windows go build`/`vet`                                                                                         | aprobado                                                                                                                                                                                     |
| Go unitarias con `-race` (`go test ./...`)                                                                                                                | aprobado (17 s); nuevas: `httpapi` (versiones y 426), `apicontract` (documento, `docs/API_V1.md`, validador)                                                                                 |
| Integración con `-race`, las 10 de la tupla                                                                                                               | `database` 11 s, `administration` 94 s, `catalog` 22 s, `operations` 306 s, `apicontract` 9 s, `payments` 4 s, `reports` 126 s, `reports/operational` 45 s, `designer` 25 s, `identity` 21 s |
| `tsc --noEmit` / Vitest                                                                                                                                   | sin errores / 89 archivos, 830 pruebas                                                                                                                                                       |
| `cargo test --locked --offline`                                                                                                                           | 47 aprobadas, 1 ignorada (nuevas: rutas `version` y `catalog_product_sold_out`, cabeceras de cliente)                                                                                        |
| Playwright `restaurant.spec.ts` + `workflow-stage9g-sold-out.spec.ts`                                                                                     | Chromium 2/2 (1.5 min), WebKit 2/2 (1.6 min)                                                                                                                                                 |
| Playwright `restaurant`, `table-price-decrease`, `takeaway-quick-notes`, `workflow-stage9c-navigation`, `workflow-stage9g` (por los eventos del catálogo) | Chromium 5/5 (1.9 min), WebKit 5/5 (1.9 min)                                                                                                                                                 |

Pruebas nuevas de integración:

- `catalog/stage9g_test.go` (`TestStage9GCatalog`): «Agotado» sin permiso (403), sin clave (400), **4 reintentos
  simultáneos con la misma clave** (misma respuesta byte a byte, una auditoría, un evento), fin = inicio del día comercial
  siguiente, reintento tras respuesta perdida, misma clave con otro contenido (409), `PRODUCT_SOLD_OUT` del producto y de
  su presentación, listado de la captura, revisión intacta, «Disponible otra vez» y sin auditoría si no cambió nada;
  agotado hasta nuevo aviso, complemento agotado, marca vencida ignorada al leer, `ResetExpiredSoldOut` (una vez,
  auditoría de proceso, evento, versión); descarga completa, sin cambios, cambio de precio, agotado, archivado, borrado,
  categoría, versión futura (`reset`) y `since` inválido; imagen binaria 200/304/404 y referencia en la descarga.
- `operations/stage9g_offline_test.go` (`TestStage9GOfflineOrdersAndIdempotency`): **5 aperturas simultáneas con la
  misma clave** (una cuenta, una clave, misma respuesta), reintento tras respuesta perdida, misma clave con otro
  contenido y desde otra terminal y usuario, **5 confirmaciones simultáneas** (un pedido); pedido sin conexión sin cambios
  con **3 envíos simultáneos** (un pedido) y reenvío tardío, auditoría e historia; **cambio de precio** (409 con antes y
  ahora, rechazo auditado, nada guardado), **confirmación con catálogo viejo** (rechazada de nuevo), confirmación de otro
  precio (rechazada), confirmación vigente (aceptada con el precio del servidor) y envío tardío del original
  (`IDEMPOTENCY_CONFLICT`); **producto agotado** y eliminado (no se aceptan ni con confirmación; sin ellos, sí); **cuenta
  cerrada mientras tanto**; mesa ocupada por otra terminal (confirmada: otra cuenta); 7 cuerpos inválidos (400) y sin
  `orders.confirm` (403); `GET restaurant/state` con mesas, cuentas y permisos, con `accounts.read` revocado (cuentas
  `null`) y con la terminal revocada (401 `DEVICE_REVOKED`).
- `apicontract/contract_integration_test.go` (`TestContractV1`): recorre las 20 operaciones con el servidor real y valida
  cuerpos y respuestas (incluidos 304, 409 de confirmación, 426 y el primer mensaje SSE). Comprobado que detecta una
  ruptura: renombrar un campo de la respuesta de «Agotado» la hizo fallar (cambio revertido).
- `database/stage9g_android_upgrade_test.go`: actualización desde el esquema 22 sin cambiar productos, cuentas, claves ni
  cobros; CHECK de «Agotado»; permiso solo al administrador; ámbito `catalog`; bitácora (un número por transacción,
  presentación → producto, borrado = baja, IVA general sí y otros cambios de cobro no).
- Se adaptaron 4 pruebas de actualización anteriores que suponían que la 0022 era la última o comparaban todas las
  columnas de productos y todos los permisos (`stage9f_printing`, `stage9f_kitchen`, `existing_install` de la etapa 8 y
  `stage9a_catalog_precheck`): migran hasta la última y excluyen lo de la 0023, que se comprueba en su propia prueba.

### Para el revisor independiente (idempotencia y validación de pedidos)

Archivos: `backend/internal/operations/offline_orders.go`, `operations/handler.go` (permiso, clave, sin evento
genérico), `operations/confirmations.go` (`confirmLines`), `operations/recovery.go`,
`administration/recovery.go`, `administration/routes.go` (`operationFault`), `internal/idempotency/idempotency.go`,
`catalog/soldout.go`, `catalog/sync.go`, `catalog/calculator.go` y `group_calculator.go` (agotado), la sección D de la
0023 (disparadores y lock) y las pruebas `operations/stage9g_offline_test.go`, `catalog/stage9g_test.go`,
`apicontract/contract_integration_test.go`, `database/stage9g_android_upgrade_test.go`.

Reglas que conviene comprobar:

1. Una clave de operación produce un solo efecto con reintentos simultáneos (lock exclusivo antes de buscar la clave:
   7820148305 en pedidos, 7820148304 en «Agotado»), devuelve la misma respuesta (byte a byte en «Agotado»; mismo JSON en
   pedidos) y rechaza otro contenido, otro usuario u otra terminal sin efecto.
2. Un rechazo (409 de cambios, errores) nunca guarda la clave; la auditoría del rechazo se confirma sola.
3. Un pedido sin conexión nunca entra con un precio distinto al del servidor sin una confirmación que cubra ese precio y
   la versión vigente; nunca entra un producto agotado, eliminado o no disponible; nunca entra en una cuenta cerrada.
4. El pedido aceptado no toca el borrador de la cuenta y respeta la revisión (otra terminal recibe `ACCOUNT_CONFLICT`).
5. La versión del catálogo nunca se lee mayor mientras una transacción con un número menor sigue abierta (lock 04).
6. Ninguna lectura de catálogo, estado o contrato expone precios sin `catalog.prices.read` ni secretos.

### Lo no verificado y lo que queda para otras fases

- Android no existe: el contrato se probó con un cliente simulado en Go (integración) y desde Node (Playwright).
- Fase B: agregar al contrato las rutas de emparejamiento (y `devices.kind` si cambia `/auth/me` o el estado), con su paso
  en `contractScenario`; decidir si la escucha de red exige `X-Client-Version`.
- Fase G: subir `min_client_version` y `min_server_version` a 0.10.0 junto con la versión; actualizar CLAUDE.md.
- Windows (Fase W): nada propio de Windows; `GOOS=windows go build/vet` aprobado.

### Para CLAUDE.md

- Esquema 23 (0023, sección D): «Agotado», `catalog.sold_out.manage`, ámbito `catalog`, `catalog_change_log`.
- §3 Composición: `restaurant/state` → `administration/recovery.go` (antes del prefijo `restaurant/`);
  `catalog/sync`, `catalog/images/{id}/content` y `catalog/products/{id}/sold-out` en `catalog`;
  `restaurant/offline-orders` en `operations`; paquetes nuevos `internal/apicontract` (contrato OpenAPI y pruebas de
  contrato) e `internal/idempotency`; trabajador `catalog.SoldOutWorker` en `main.go`.
- §3 Locks: el disparador de `catalog_change_log` toma 7820148304 (reentrante); sin números nuevos.
- §6 Pruebas: `./internal/apicontract` en la tupla; el puente envía `X-Client-Version`/`X-Client-Kind`; **los cambios de
  catálogo sí publican eventos desde la 9G** (corregir la nota de Playwright WebKit en Windows); la especificación nueva
  `workflow-stage9g-sold-out.spec.ts` usa una segunda terminal enrolada que habla con Go desde Node.
- §15: reglas de «Agotado», del pedido sin conexión, de la versión mínima de cliente/servidor (y subirlas en cada versión
  que rompa la compatibilidad), de la conservación de claves (para siempre) y del contrato (`docs/API_V1.md`: ampliar el
  documento, la guía y el recorrido juntos).
- Reglas permanentes propuestas: «Toda ruta nueva para clientes se documenta en `openapi.json` y `docs/API_V1.md` y se
  recorre en `TestContractV1`»; «Una escritura nueva de cuentas, pedidos, pagos o catálogo operativo lleva
  `operation_id` (paquete `idempotency` si el módulo no tiene la suya)».

**Fusión con la Fase A y verificación del coordinador** (13/11/2026): la rama `etapa-9g-d` salió antes de la Fase A; se
fusionó en `72b81dc` (conflictos solo en `API_CONTRACTS.md` —se conservan ambos bloques— y en la tupla de integración de
`scripts/database.py` —se unen `./internal/apicontract` y `./cmd/ad-el-pos-server`—). Copia exacta de la fusión
(`git archive`) en el entorno aislado: Go unitarias con `-race`; integración de los 11 paquetes de la tupla (`operations`
310 s, `reports` 130 s, `administration` 92 s, `reports/operational` 45 s, `cmd/ad-el-pos-server` 43 s, `catalog`,
`designer`, `identity`, `database`, `apicontract` y `payments`); `cargo test`; Playwright `restaurant`,
`table-price-decrease`, `takeaway-quick-notes`, `workflow-stage9c-navigation`, `workflow-stage9g-modality`,
`workflow-stage9g-sold-out` y `workflow-zz-test-data`: 7 de 7 en Chromium y 7 de 7 en WebKit; todo aprobado salvo
Vitest: `compatibility.test.tsx` (de D) no simulaba `installation_state` (de A) y la aplicación se quedaba en la elección
de modalidad. Corregido en `67741a2` (4 renglones en la prueba, sin cambios de producto) y repetido: formato, `tsc` y
Vitest aprobados.

## Fase B · Emparejamiento de terminales y conexión cifrada

Un agente, en su propio árbol de trabajo (`etapa-9g-b`, creada desde `etapa-9g-a` en `aa748b7`), en paralelo con la
Fase D. 13/11/2026. Sección B de la `0023` y `schema_version` 23 (D pone lo mismo). Pruebas en copias aisladas: clúster
temporal 55499, Go temporal 8186 (Playwright y la prueba en vivo), vista previa 1424, Rust en `cache/cargo-B`, escucha
HTTPS de prueba en `127.0.0.1:8196` (prueba del `host`), `127.0.0.1:8197` (prueba en vivo de Rust contra Go) y puertos
efímeros `127.0.0.1:0` (pruebas de Go). **Nunca se abrió el 8443 real, no se usó el Keychain real ni la instalación real.**

Commits: `7cb3231` (Go y Rust: identidad, escucha de red, emparejamiento, verificador), `e487660` (interfaz, puente,
Vitest, Playwright, contrato en `API_CONTRACTS.md` y escucha de red en la prueba del `host`) y `5b9cb88` (ajuste del
ayudante de Playwright).

### Qué cambió

**Servidor Go**

- `internal/localsetup/identity.go` (nuevo): identidad del servidor en la carpeta `identity/` junto a `server.json`
  (servicio: `/Library/Application Support/AD-EL POS/identity`; `serve`: `.local/identity`; Windows:
  `%LOCALAPPDATA%\AD-EL POS\identity`): `identity.json` (`server_id` UUID v4 y fecha), `server-cert.pem` (ECDSA P-256
  autofirmado, 30 años, CN «AD-EL POS <server_id>», sin nombres ni IPs: no se usan para confiar) y `server-key.pem`
  (0600; carpeta 0700 y ACL privada en Windows con `fileacl`). Se crea al primer arranque con `O_EXCL`; **nunca se
  reemplaza sola**: incompleta, ilegible o con una llave que no corresponde, el servidor no arranca («revisión manual
  necesaria»), porque regenerarla obligaría a emparejar de nuevo todas las terminales. `SPKIFingerprint` = SHA-256 de
  `RawSubjectPublicKeyInfo`.
- `internal/lanlistener` (nuevo): escucha HTTPS de la red local. `AllowedPeer`: loopback, `IsPrivate` (RFC 1918 y ULA
  fc00::/7) y enlace local (169.254/16, fe80::/10), IPv4 mapeada evaluada como IPv4; todo lo demás (Internet, CGNAT
  100.64/10, multidifusión, sin especificar) se **cierra al aceptarlo, antes del intercambio TLS**. TLS ≥ 1.2 con el
  certificado de la identidad, mismos plazos que la escucha local y bitácora de errores TLS descartada (exploradores de
  puertos). **Toda petición que llega por ella queda marcada en su contexto** (`lanlistener.FromNetwork`): el cliente no
  puede falsificarlo.
- `internal/config`: `network_listen_address` opcional (`omitempty`; vacía = sin escucha de red, los archivos existentes
  no cambian): `:puerto`, `0.0.0.0`/`[::]`, loopback o IP privada; nunca IP pública, nombre ni el puerto de
  `listen_address` en todas las interfaces (`CheckNetworkAddress`). Lo de `tls-init`, `tls_certificate` y la escucha LAN
  de `listen_address` (IP fija, `allow_lan`, `development`) **se conserva sin cambios** como camino anterior.
- `cmd/ad-el-pos-server`: `serveHTTP` recibe `networkOptions` (carpeta de la identidad y dirección de red), carga o crea
  la identidad (`server_identity_ready` con `server_id` y huella en la bitácora), abre las dos escuchas y las cierra en
  orden juntas (si una falla, la otra no queda sola). `serve`: identidad junto a la configuración y escucha solo con
  `network_listen_address`. `host` (macOS): **`:8443` siempre** (el servicio solo sirve con una modalidad con servidor);
  `--network-listen D|off` para pruebas; con `ADEL_TEST_MODE=1` sin el indicador **no se abre** (ninguna prueba ocupa el
  8443 real por omisión). `host` de Windows: identidad sí, escucha de red no (Fase W). `Service.NetworkPort` se fija
  antes de atender.
- `internal/administration`:
  - `http.go`: **`local = origen loopback && !FromNetwork`** (antes solo la IP). `SourceAddress` (IP de origen) y
    `PairingSecret` (`X-Pairing-Secret`) en `Request`.
  - `service.go`: `Identity` y `NetworkPort` en `Service`; `pairing/*` público antes de autenticar el dispositivo;
    **`devices/enroll` solo local** (`403 ENROLLMENT_LOCAL_ONLY`; ver decisión 1).
  - `pairing.go` (nuevo): protocolo, derivación del código, límites, auditoría, solicitudes en Terminales con las
    direcciones de la computadora principal (`localAddresses`: IPv4 privadas y nombre `.local`).
  - `devices.go`: `devicesQuery` (auditoría) con `kind`, `os`, `app_version`, `paired_at`; `deviceListQuery` además
    con `online` (petición en los últimos 2 min, como el conteo de la fase A) y `"current_user"` (sesión vigente más
    reciente, nunca `sa`); `GET devices/{id}/configuration`; `sessionsSummary`.
  - `routes.go`: un bloque antes de la comprobación de identificadores para `devices/pairing-requests*` y
    `devices/sessions-summary`.
- `migrations/0023_network_terminals_and_android.sql`: encabezado común (el mismo texto que el de D) y **solo la
  sección `-- Fase B`**: columnas de `devices` (`kind` con CHECK desktop/android/kitchen, `os`, `app_version`,
  `paired_at`, `server_fingerprint`) y tabla `device_pairing_requests` con restricciones con nombre (estados, intentos
  0–5, motivo de rechazo solo al rechazar, revelación coherente, reclamo con terminal, aprobación con nombre) e índices
  por origen y abiertas. `config/product.json` → `schema_version: 23`; `stage9f_kitchen_upgrade_test.go` y
  `stage9f_printing_upgrade_test.go` con **el mismo cambio que hizo D** (fusión limpia).

**Escritorio (Rust)**

- `Cargo.toml`: `rustls = "=0.23.46"` (sin las opciones por omisión, que traerían `aws-lc-rs`; con `ring`, `std` y
  `tls12`, como lo usa reqwest) y `sha2 = "=0.10.9"`. **Ambas ya estaban en `Cargo.lock` y en el registro local**: el
  único cambio del `Cargo.lock` es la lista de dependencias del paquete raíz (`cargo metadata --offline`, sin descargas);
  todo compila y prueba con `--locked --offline`.
- `pairing.rs` (nuevo): `PinnedVerifier` (`ServerCertVerifier`): compara la huella SPKI del certificado e **ignora
  nombre, IP, cadena y vigencia**, y verifica la firma del intercambio TLS 1.2/1.3 con esa llave
  (`rustls::crypto::verify_tls1x_signature`); en una sesión de emparejamiento fija el primer certificado de la sesión y
  exige el mismo en las siguientes conexiones. Cliente TLS sin reanudación de sesiones (cada conexión vuelve a
  comprobar la llave). `normalize_address` (D2): HTTPS a IPv4 privada o de enlace local, IPv6 ULA o de enlace local o
  nombre `.local`; nunca loopback, IP pública ni otro nombre; puerto 8443 si no se indica. Comandos `pairing_start`,
  `pairing_status` (reclama la credencial, la guarda en el almacén seguro y después escribe `connection.json`),
  `pairing_rename`, `pairing_cancel` y `pairing_forget`. El secreto de la solicitud y los números nunca llegan a React.
- `connection.rs`: `server_id`, `server_fingerprint`, `server_name` (solo Rust; lista cerrada `check_paired`:
  «Solo terminal», HTTPS a la red local, UUID y huella válidos, sin CA); `credential_server()` =
  `adel-server:{server_id}` para una terminal emparejada y la dirección como siempre si no (la credencial
  `http://127.0.0.1:8181|main` **no cambia**).
- `main.rs`: cliente con la huella fijada si la conexión la tiene; `connection_error()` devuelve «El servidor cambió su
  identidad y esta terminal ya no puede comprobar que es el del restaurante: vuelve a emparejar esta terminal.» cuando
  el verificador vio otro certificado. `installation.rs`: `paired`/`server_name` en el estado y una modalidad con
  servidor olvida el servidor emparejado. `api.rs`: operaciones nuevas y `paired` en `authentication_state`. `build.rs` y
  `permissions/diagnostics.toml`: los cinco comandos. `src/testdata/`: dos certificados ECDSA P-256 de prueba con su
  llave (hexadecimal; solo pruebas).

**Escritorio (React)**

- `pages/setup/FindServerPage.tsx` (punto de integración de la fase A): dirección escrita a mano y nombre («Caja 2»,
  editable) → «Pedir código» → **código grande en dos grupos** (`481 926`), restaurante, nombre con «Cambiar nombre»,
  vencimiento en cuenta regresiva y estado (esperando, emparejada → la aplicación se reinicia, rechazada, vencida,
  cancelada) con «Pedir otro código» y «Cancelar». Marca el punto de integración de la fase C (lista de servidores
  encontrados arriba de la dirección).
- `pages/DevicesPage.tsx`: «Terminales nuevas» (consulta cada 3 s; nombre, tipo, sistema, versión y hora; en la
  computadora principal el campo «Código que muestra …» con «Aprobar» y «Rechazar» con confirmación, intentos que
  quedan y la dirección de esta computadora para escribirla en la terminal; en otra terminal «se aprueban en la
  computadora principal»); «Sesiones activas por tipo de terminal» («Solo se cuentan; todavía no se limita ninguna»);
  tabla con tipo y sistema, usuario actual y «En línea»/«Desconectada»/«Revocada» (se refresca cada 5 s); «Ver
  configuración» (tipo, sistema, emparejada, pantalla inicial, caja, teclado, estaciones de Cocina, aviso sonoro e
  «Impresoras de esta terminal: Sin impresoras propias», hueco de la fase E); revocar dice que habrá que emparejarla de
  nuevo. «Autorizar otra terminal» pasa a «Código para esta computadora» (decisión 1).
- `pages/setup/RepairTerminal.tsx` + `App.tsx`: una terminal emparejada revocada ve «Esta terminal necesita
  emparejarse de nuevo»; sin conexión (p. ej. el servidor cambió de identidad) ofrece «Emparejar de nuevo»; ambos con
  confirmación, `pairing_forget` y reinicio a «Buscar el servidor».
- `services/pairing.ts`, `models.ts`, `api.ts`, `installation.ts`; etiquetas del Registro de movimientos
  (`AuditPage.tsx`); estilos al final de `styles.css`, solo tokens.

**Puente y pruebas de navegador**

- `e2e/pairing.ts` (nuevo): la terminal nueva simulada por la API de Go (compromiso, número, revelación y código como
  Rust; la huella sale de `identity/server-cert.pem` junto a la configuración del Go temporal porque las pruebas no
  abren la escucha de red) y la misma lista cerrada de direcciones.
- `e2e/native-driver.ts`: simula los cinco comandos `pairing_*` contra Go (aprobada, la credencial reclamada sustituye
  a la del puente y la terminal queda emparejada), `paired`/`server_name` en `installation_state`, `paired` en
  `authentication_state` y, como Rust, `401 DEVICE_REVOKED|DEVICE_REQUIRED` borra la credencial del puente.
- `e2e/workflow-stage9g-terminals.spec.ts` (nuevo, 2 pruebas).

### Protocolo exacto (para el revisor independiente)

Terminal T (Rust), servidor S (Go). F = SHA-256 del SPKI del certificado de S. Números de 32 bytes del generador seguro
(`ring` en Rust, `crypto/rand` en Go), en hexadecimal minúsculo en el JSON.

1. T valida la dirección propuesta (D2) y abre una sesión TLS con `PinnedVerifier::first_seen()`: acepta el primer
   certificado, guarda su huella F_T y exige esa misma llave en toda la sesión (verificando siempre la firma del
   intercambio).
2. T → S `POST /api/v1/pairing/requests` `{name, device_kind:"desktop", os, app_version, commitment: hex(SHA-256(nT))}`.
   S (con la tabla bloqueada en `SHARE ROW EXCLUSIVE`: límites sin carreras) valida, aplica 5 por origen/10 min y 20
   abiertas, genera nS y un secreto de 256 bits (guarda solo su SHA-256, el compromiso, nS y su huella F_S) y responde
   `{request_id, server_nonce: nS, pairing_secret, expires_at (+5 min), server_id, server_name, name}`.
3. T → S `POST …/{id}/reveal` (`X-Pairing-Secret`) `{terminal_nonce: nT}`. S exige `SHA-256(nT) = commitment` (si no,
   la rechaza con motivo `commitment`) y guarda nT.
4. Ambos: `código = SHA-256(F ‖ nT ‖ nS ‖ id[16 bytes]) → u64 big-endian de los primeros 8 bytes mod 10⁶`, 6 dígitos.
   T usa F_T; S usa la F_S guardada en la solicitud **y exige que siga siendo la huella vigente**.
5. En la computadora principal, un usuario con `devices.manage` escribe el código: `POST
devices/pairing-requests/{id}/approve {code}` **solo por la escucha local** (`request.Local`; por la red, aunque
   venga de 127.0.0.1: `403 PAIRING_LOCAL_ONLY` + `access.denied`). Comparación en tiempo constante; incorrecto: +1
   intento (se confirma aunque la petición falle) y al quinto la solicitud queda rechazada. Correcto: `approved`, y la
   terminal tiene 5 minutos más para reclamar.
6. T consulta `GET …/{id}` cada 2 s con su secreto y, aprobada, `POST …/{id}/claim`: con la fila bloqueada, S crea la
   terminal con un token nuevo de 256 bits (solo su SHA-256 en la base), marca la solicitud `claimed` y responde **una
   sola vez** `{device_id, device_token, server_id, server_name, name}`; cualquier otro reclamo (también simultáneo):
   `409 PAIRING_ALREADY_CLAIMED` sin token.
7. T guarda la credencial en el almacén seguro (`adel-server:{server_id}|{perfil}`) y **después** `connection.json`
   (`terminal-only`, `server_url` https, `server_id`, `server_fingerprint` = F_T, `server_name`). Desde entonces el
   cliente usa `PinnedVerifier::pinned(F_T)` (también el SSE y el estado del sistema).

**Qué protege el compromiso y qué no (corregido tras la revisión independiente, hallazgo 1).** M está entre T y S y
debe lograr que coincidan dos códigos: el que T calcula con la huella de M, `H(F_M ‖ nT ‖ nS' ‖ id')`, y el que S
calcula con la suya, `H(F_S ‖ nT' ‖ nS ‖ id)`. El compromiso impide ajustar los números **dentro de una misma
solicitud**: T no revela nT hasta haber recibido nS', de modo que M fija nS' sin conocer nT; y S no entrega nS hasta
haber recibido el compromiso de nT'. Pero en cuanto T revela nT, el código de T queda fijo y M lo conoce, y M **sí puede
abrir muchas solicitudes en S**: cada una le da un nS nuevo, calcula sin conexión el código que esperaría S y, si no
coincide, la abandona (la cancela o la deja vencer) y prueba otra; solo revela la que coincide. La afirmación anterior
(«1 en 10⁶ por solicitud») era engañosa: sin tope, la revisión logró 3 000 pruebas en 3,3 s con orígenes `fe80::`
distintos (≈ 1–5 % de éxito por emparejamiento). La probabilidad real queda acotada por cuántas solicitudes acepta S:
**tope global de 30 solicitudes creadas cada 10 minutos**, sin importar estado ni origen (las canceladas y rechazadas
siguen contando), y 5 por grupo de origen (la IPv4, el /64 de una IPv6 y todo el enlace local como un solo grupo). Así M
tiene como máximo 30 intentos por cada 10 minutos que el administrador tarde en escribir el código: ≤ 3×10⁻⁵ por
ventana. Además el intento queda a la vista: Terminales muestra en la computadora principal cuántas solicitudes se
cancelaron, rechazaron o vencieron sin aprobarse en la última ventana y si se alcanzó el tope (y el Registro de
movimientos lo anota una vez por ventana).

### Amenazas consideradas

| Amenaza                                                                        | Defensa                                                                                                                                                                                                                                                                                                                                         |
| ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Intermediario durante el emparejamiento (otro certificado)                     | Código ligado a F y a los dos números con compromiso: códigos distintos; el administrador escribe el código que ve en la terminal.                                                                                                                                                                                                              |
| Intermediario que cambia de certificado a mitad de la sesión de emparejamiento | La sesión fija el primer certificado y exige el mismo después (prueba `a_pairing_session_pins_the_first_certificate`).                                                                                                                                                                                                                          |
| Servidor falso o certificado cambiado después de emparejar                     | Verificador por huella: no se conecta y avisa «vuelve a emparejar»; nunca acepta cualquier certificado. Copiar el certificado sin la llave privada no basta: se verifica la firma del intercambio.                                                                                                                                              |
| Cambio de IP por DHCP                                                          | La confianza es la llave, no el nombre ni la IP (prueba con nombres/IP distintos). La re-búsqueda por `server_id` + huella es de la fase C.                                                                                                                                                                                                     |
| Aprobar desde otra computadora (o por la IP de la propia)                      | `local` depende de la escucha de loopback; por la escucha de red nunca, aunque el origen sea 127.0.0.1 (prueba real con las dos escuchas).                                                                                                                                                                                                      |
| Terminal intrusa en la red                                                     | Sin acceso a la computadora principal no hay aprobación; el código de 43 caracteres tampoco sirve por la red (`ENROLLMENT_LOCAL_ONLY`).                                                                                                                                                                                                         |
| Adivinar el código                                                             | 5 intentos por solicitud; 10⁶ combinaciones; vence a los 5 minutos; tope global de 30 solicitudes en 10 minutos (cada solicitud abandonada es un intento visible en Terminales).                                                                                                                                                                |
| Inundar de solicitudes                                                         | 5 por grupo de origen (IPv4, /64 de IPv6, todo el enlace local) y 30 en total cada 10 min, contando canceladas y rechazadas (`429`); conteos por índice sobre la ventana; limpieza por lotes en un trabajador. Riesgo aceptado: un atacante en la LAN puede agotar el tope global durante 10 minutos (se ve en Terminales).                     |
| Robar o repetir la credencial                                                  | Se entrega una sola vez (fila bloqueada), por la conexión fijada; en la base solo SHA-256; nunca llega a React (la recibe Rust).                                                                                                                                                                                                                |
| Consultar o cancelar solicitudes ajenas                                        | Secreto de 256 bits por solicitud (comparación de su SHA-256 en tiempo constante); id o secreto incorrectos responden igual (404).                                                                                                                                                                                                              |
| Orígenes fuera de la red local                                                 | Solo loopback u orígenes privados/enlace local/ULA dentro de la subred de una interfaz local no punto a punto (las VPN como Tailscale o ZeroTier quedan fuera salvo que se presenten como interfaz normal); se cierran al aceptarlos, antes de TLS. HTTP por la escucha de red: 400 del servidor TLS; la escucha local sigue solo en 127.0.0.1. |
| Degradar TLS                                                                   | Mínimo TLS 1.2 en Go (TLS 1.1 rechazado, comprobado con `curl --tls-max 1.1`); Rust solo 1.2/1.3 y sin reanudación.                                                                                                                                                                                                                             |
| Llave del servidor                                                             | 0600 en carpeta 0700 de la cuenta del servicio (ACL en Windows); nunca se registra ni se muestra; la huella sí (pública).                                                                                                                                                                                                                       |
| Revocación                                                                     | Cada petición y cada lectura del SSE (750 ms) revalidan el dispositivo: el flujo se corta con `DEVICE_REVOKED` en menos de 3 s (prueba de integración por HTTPS real y Playwright < 8 s).                                                                                                                                                       |
| Página web maliciosa en la LAN                                                 | Sin credencial no hay nada que hacer salvo pedir un código; el prevuelo CORS solo admite `GET` de orígenes exactos; el navegador rechaza el certificado autofirmado. Sin cambio para la escucha local (riesgo previo de reasociación DNS a 127.0.0.1 sin variación).                                                                            |

Archivos para el revisor: `backend/internal/administration/pairing.go` (+ `http.go`, `service.go`, `devices.go`,
`routes.go`), `backend/internal/lanlistener/lanlistener.go`, `backend/internal/localsetup/identity.go`,
`backend/cmd/ad-el-pos-server/main.go` (`serveHTTP`, `loadIdentity`) y `host_unix.go` (`hostNetworkAddress`),
`backend/migrations/0023_…sql` (sección B), `desktop/src-tauri/src/pairing.rs`, `connection.rs` (`check_paired`),
`main.rs` (`client`, `connection_error`), y las pruebas `administration/pairing_test.go`,
`pairing_acceptance_test.go`, `lanlistener_test.go`, `identity_test.go`, `cmd/…/host_unix_network_integration_test.go`
y las de `pairing.rs`.

### Decisiones tomadas en la fase

1. **`devices/enroll` solo en la propia computadora** (§0-B: «ninguna terminal se conecta sin emparejarse»): el código
   de 43 caracteres solo sirve para volver a autorizar la ventana de la computadora principal (u otro perfil local);
   el botón «Autorizar otra terminal» pasa a «Código para esta computadora». Las pruebas de integración que enrolan lo
   hacen por loopback y no cambian.
2. La identidad vive junto a `server.json` (la carpeta del servicio en macOS), dura 30 años y **nunca se regenera
   sola**. Los respaldos `pg_dump` no la incluyen (ver Fase F/G).
3. La escucha de red del servicio es siempre `:8443`; en `serve` es opcional por configuración; en modo de pruebas el
   `host` no la abre sin indicarla.
4. Derivación exacta del código (8 bytes big-endian mod 10⁶; sesgo despreciable) con un vector común en Go, Rust y el
   puente (`166101`).
5. La terminal se crea **al reclamar**, no al aprobar (así ningún token queda guardado en claro esperando); aprobada,
   hay 5 minutos para reclamar.
6. La aprobación admite cambiar el nombre (`name` opcional), pero la interfaz aprueba con el propuesto (se renombra
   después con «Editar»); la terminal puede cambiarlo mientras espera (`POST …/name`).
7. Auditoría de la solicitud con autor `pairing` («Terminal por emparejar»); la aprobación, los códigos incorrectos y
   el rechazo con el usuario y la terminal del administrador; el emparejamiento con la terminal nueva como autor.
8. Tipo de dispositivo: hoy toda terminal emparejada es `desktop`; una terminal de escritorio con inicio en Cocina
   cuenta como escritorio en el conteo de sesiones (el tipo `kitchen` queda para un dispositivo de cocina dedicado). D
   usa `X-Client-Kind` ∈ {desktop, android} para la versión de la aplicación, que es otra cosa.
9. La computadora principal muestra en Terminales su dirección (IPv4 privadas y nombre `.local`) **solo por la escucha
   local** y solo para escribirla en una terminal nueva: es la excepción necesaria al «sin direcciones en la interfaz»
   mientras no exista la búsqueda automática (fase C).
10. Volver a emparejar (`pairing_forget`) solo quita el servidor de `connection.json`; la credencial anterior queda sin
    uso en el almacén seguro (no se borra: evita otra autorización del Llavero y no se pierde nada si fue un error).
11. Una dirección con `:443` explícito se normaliza al 8443 (el analizador de URL omite el puerto por omisión de HTTPS);
    sin importancia práctica.

### Pruebas ejecutadas (aisladas)

| Suite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Resultado                                                                                                                                         |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `format` (Prettier, gofmt, cargo fmt; `docs/etapas` intacto por md5)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | aprobado                                                                                                                                          |
| `sync`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | aprobado                                                                                                                                          |
| `vet` (normal e `integration`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | aprobado                                                                                                                                          |
| `win` (`GOOS=windows` build, vet y vet `integration`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | aprobado                                                                                                                                          |
| Go unitarias con `-race` (`go test ./...`): derivación y vector del código, compromiso, etiquetas; filtro de orígenes, escucha TLS 1.2 marcada; identidad (creación, recarga, nunca reemplazada); `network_listen_address`; `hostNetworkAddress`                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | aprobado (15 s)                                                                                                                                   |
| Integración `./internal/administration` (`TestStageTwoAcceptance` → `testPairingAndTerminals`, escuchas reales HTTP local y HTTPS de red): código correcto, sin revelar, incorrecto ×4 con intentos que quedan, quinto rechaza, vencido (pendiente y aprobado sin reclamar), aprobación/rechazo/asistente/`enroll` rechazados por la escucha de red desde 127.0.0.1 con `access.denied`, compromiso falso, secreto ajeno, 4 reclamos simultáneos → 1 credencial, límite por origen, lista con tipo/sistema/en línea/usuario actual, configuración, auditoría con usuario y terminal, conteo por tipo (Android aparte, sin permiso 403), revocación con el SSE por HTTPS cortado < 3 s y `DEVICE_REVOKED` después | aprobado (108 s)                                                                                                                                  |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS` + `checkServiceNetwork`): el `host` real con `--network-listen 127.0.0.1:8196` crea la identidad en la carpeta del servicio (0700/0600), presenta ese certificado por HTTPS, no atiende HTTP sin cifrar y lo informa en la bitácora; en modo de pruebas sin el indicador no abre escucha de red                                                                                                                                                                                                                                                                                                                                                   | aprobado (46 s)                                                                                                                                   |
| `cargo test --locked --offline` (verificador con intercambio TLS completo en memoria: huella correcta con nombres/IP distintos, certificado cambiado rechazado y avisado, sesión que fija el primero, llave ajena; vector del código; direcciones; conexión emparejada; rutas)                                                                                                                                                                                                                                                                                                                                                                                                                                   | 65 aprobadas, 2 ignoradas (Keychain real y la prueba en vivo)                                                                                     |
| Prueba en vivo Rust → Go (`dtools/live-pairing-B.sh`, ignorada en la suite): Go temporal con `network_listen_address 127.0.0.1:8197`; huella de la bitácora = huella de `openssl`; HTTP por la escucha de red → 400; TLS 1.1 rechazado; `request_pairing` real (reqwest + rustls + verificador) fija la huella del servidor, la otra huella no conecta y el código de Rust coincide con el que se deriva de los datos guardados por Go                                                                                                                                                                                                                                                                           | aprobado                                                                                                                                          |
| `tsc --noEmit`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | sin errores                                                                                                                                       |
| Vitest (`npm test`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | 88 archivos, 847 pruebas (12 nuevas en `pages/stage9gPairing.test.tsx`; `stage9gModality.test.tsx` ajustada: la pantalla ya tiene «Pedir código») |
| Playwright Chromium (`restaurant`, `workflow-stage9c-navigation`, `workflow-stage9f-kitchen-stations`, `workflow-stage9g-modality`, `workflow-stage9g-terminals` ×2, `workflow-zz-test-data`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | 7 de 7 (2.2 min)                                                                                                                                  |
| Playwright WebKit (las mismas, pantalla desbloqueada)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | 7 de 7 (2.2 min)                                                                                                                                  |
| `workflow-stage9g-terminals` repetida tras el último ajuste                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Chromium 2/2, WebKit 2/2                                                                                                                          |

No se ejecutó la suite `app` (compilación `release` de Tauri): `cargo test` ya compila los comandos, `build.rs` y los
permisos.

### No verificado y lo que queda para otras fases

- **Fase W (otra computadora real)**: emparejar de verdad Mac ↔ Windows por la red (escucha 8443 de la Mac con su
  certificado, código, revocación con la otra computadora); la ventana de permiso **«Red local»** de macOS 15+ al
  conectarse la terminal a una IP de la LAN (falta `NSLocalNetworkUsageDescription`, previsto en la fase C; con firma
  ad hoc puede volver a pedirse en cada compilación); el firewall de macOS está desactivado en esta Mac (si se activa, el
  servicio necesitaría permiso de conexiones entrantes); Windows: escucha 8443 en el `host` de Windows + regla del
  firewall solo redes privadas (**parada**), compilar `pairing.rs` para Windows (solo difiere por `cfg!`) y el
  Administrador de credenciales con la cuenta `adel-server:…`.
- **Keychain real** con la cuenta nueva `adel-server:{server_id}|main` (las pruebas de Rust no escriben en Keychain): en la
  Fase G/W la primera terminal emparejada mostrará el diálogo del Llavero.
- **Fase C**: la lista de servidores encontrados va arriba de la dirección en `FindServerPage.tsx` (punto marcado) y
  entra a `startPairing`; para el cambio de IP basta reescribir `server_url` en `connection.json` (Rust) al encontrar el
  mismo `server_id` con la misma huella (`PinnedVerifier` no depende de la dirección); el TXT de mDNS puede llevar
  `server_id` (`Service.Identity`) y la huella corta; `NetworkPort` dice si hay escucha de red.
- **Fase E**: `printers` de `GET devices/{id}/configuration` y la línea «Impresoras de esta terminal».
- **Fase F/G**: la carpeta `identity/` no va en los respaldos `pg_dump`; restaurar en otra computadora crea otra
  identidad y obliga a emparejar de nuevo las terminales (documentarlo en `INSTALAR_EN_RED.md` o respaldar también la
  carpeta: decisión de F).
- **Fase G**: revisor independiente; actualizar `CLAUDE.md` §7 (D1); `docs/INSTALAR_EN_RED.md` (dónde ver la dirección:
  Sistema › Terminales en la computadora principal; cómo revocar). Para probar la red en desarrollo: agregar
  `"network_listen_address": ":8443"` a `.local/server.json` (no al ejemplo: las pruebas lo copian).

### Rutas nuevas para el contrato OpenAPI (Fase D)

| Método | Ruta (`/api/v1/…`)                      | Autenticación / permiso                              | Cuerpo                                             | Respuesta 200                                                                                                                                                |
| ------ | --------------------------------------- | ---------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| POST   | `pairing/requests`                      | ninguna (terminal sin credencial; límite por origen) | `{name, device_kind, os, app_version, commitment}` | `{request_id, server_nonce, pairing_secret, expires_at, server_id, server_name, name}`                                                                       |
| POST   | `pairing/requests/{id}/reveal`          | `X-Pairing-Secret`                                   | `{terminal_nonce}`                                 | `{state, expires_at}`                                                                                                                                        |
| GET    | `pairing/requests/{id}`                 | `X-Pairing-Secret`                                   | —                                                  | `{state, expires_at, name}`                                                                                                                                  |
| POST   | `pairing/requests/{id}/claim`           | `X-Pairing-Secret`                                   | `{}`                                               | `{device_id, device_token, server_id, server_name, name}` (una sola vez)                                                                                     |
| POST   | `pairing/requests/{id}/name`            | `X-Pairing-Secret`                                   | `{name}`                                           | `{state, name}`                                                                                                                                              |
| POST   | `pairing/requests/{id}/cancel`          | `X-Pairing-Secret`                                   | `{}`                                               | `{state}`                                                                                                                                                    |
| GET    | `devices/pairing-requests`              | sesión + `devices.read`                              | —                                                  | `{requests: [{id, name, device_kind, os, app_version, created_at, expires_at, ready, attempts_left}], approve_here, network?: {available, port, addresses}}` |
| POST   | `devices/pairing-requests/{id}/approve` | sesión + `devices.manage` + escucha local            | `{code, name?}`                                    | `{state: "approved", name}`                                                                                                                                  |
| POST   | `devices/pairing-requests/{id}/reject`  | sesión + `devices.manage` + escucha local            | `{}`                                               | `{state: "rejected"}`                                                                                                                                        |
| GET    | `devices/sessions-summary`              | sesión + `devices.read`                              | —                                                  | `{by_kind: {desktop, android, kitchen}, total, limited}`                                                                                                     |
| GET    | `devices/{id}/configuration`            | sesión + `devices.read`                              | —                                                  | `{device, kitchen_all_stations, kitchen_sound_alert, kitchen_stations, printers}`                                                                            |

Errores (formato `{error: {code, message, request_id, details?}}`): `PAIRING_UNAVAILABLE` 503, `PAIRING_NOT_FOUND` 404,
`SETUP_REQUIRED` 409, `PAIRING_RATE_LIMITED`/`PAIRING_LIMIT_REACHED` 429, `PAIRING_SOURCE_UNKNOWN` 400,
`PAIRING_COMMITMENT_MISMATCH` 400, `PAIRING_ALREADY_REVEALED`/`PAIRING_NOT_READY`/`PAIRING_NOT_APPROVED`/
`PAIRING_ALREADY_APPROVED`/`PAIRING_ALREADY_CLAIMED`/`PAIRING_EXPIRED`/`PAIRING_REJECTED`/`PAIRING_CANCELLED`/
`PAIRING_NOT_PENDING`/`PAIRING_IDENTITY_CHANGED`/`PAIRING_ATTEMPTS_EXCEEDED` 409, `PAIRING_CODE_INVALID` 400 (con
`details.attempts_left`), `PAIRING_LOCAL_ONLY` 403, `ENROLLMENT_LOCAL_ONLY` 403. `GET devices` agrega `kind`, `os`,
`app_version`, `paired_at`, `online` y `current_user`. Detalle completo en `API_CONTRACTS.md`, bloque «Etapa 9G ·
emparejamiento de terminales y conexión cifrada (fase B)».

### Para la fusión (coordinador)

- `0023`: ambos ramas crean el archivo con el mismo encabezado; unir encabezado + sección D + sección B (B no toca
  `realtime_events_scope_check`).
- `config/product.json` y los dos `product.json`/`metadata.json` generados: tomar los de D (schema 23 + versiones
  mínimas). `stage9f_*_upgrade_test.go`: cambio idéntico en ambas ramas.
- Conflictos pequeños esperables (mismas zonas): `administration/http.go` (literal `Request`), `service.go` (campos),
  `routes.go` (bloque antes de `parts :=`), `api.rs` (operaciones), `native-driver.ts` (`request()`: cabeceras de D y
  borrado de credencial de B), `App.tsx`, `AuditPage.tsx` (etiquetas). Tras unir, `pairing.rs` envía ya
  `X-Client-Version`/`X-Client-Kind` por su cuenta (podría usar `with_client_headers` de D).
- Agregar las rutas de la tabla anterior a `openapi.json` y `docs/API_V1.md` (pruebas de contrato de D).

### Para CLAUDE.md

- §3 Arquitectura: dos escuchas: HTTP `127.0.0.1:8181` (la ventana de la propia computadora) y, en el servicio, HTTPS
  `:8443` en todas las interfaces solo para loopback y orígenes privados/enlace local/ULA dentro de la subred de una
  interfaz local que no sea punto a punto (`internal/lanlistener`;
  `serve` con `network_listen_address`). «Local» = escucha de loopback, nunca la de red aunque venga de 127.0.0.1.
  Identidad propia del servidor en `identity/` junto a `server.json` (ECDSA P-256, 30 años, `server_id`), que nunca se
  regenera sola. Terminales emparejadas: `connection.json` con `server_id`, `server_fingerprint`, `server_name`; TLS con
  huella SPKI fijada (`pairing.rs`, `rustls` sin `aws-lc-rs`), credencial `adel-server:{server_id}|{perfil}`.
- §6 Pruebas: `testPairingAndTerminals` (desde `TestStageTwoAcceptance`) usa las dos escuchas reales en puertos
  efímeros; la prueba del `host` abre la escucha de red en `127.0.0.1:8196`; `e2e/pairing.ts` simula la terminal nueva
  (huella del certificado del Go temporal); el puente simula `pairing_*` y borra su credencial con `DEVICE_REVOKED`;
  prueba en vivo de Rust contra Go (`live_pairing_against_a_temporary_go_server`, ignorada) con
  `ADEL_PAIRING_TEST_BASE`/`ADEL_PAIRING_TEST_FINGERPRINT`.
- §7 Seguridad: sustituir «No escuchar en `0.0.0.0`» por la regla de D1 (escucha HTTPS de red con filtro de orígenes);
  ninguna terminal se registra sin emparejarse (código de 6 dígitos ligado a la huella y a dos números con compromiso,
  aprobado solo en la computadora principal, 5 intentos, 5 minutos, límite por origen); `devices/enroll` solo local;
  nunca aceptar otro certificado en silencio.
- §15: Sistema › Terminales con «Terminales nuevas», tipo y sistema, usuario actual, en línea, «Ver configuración» y
  sesiones activas por tipo (solo se cuentan, D9); auditoría `device.pairing_*` y `device.paired`; sin permisos nuevos.
- Reglas permanentes propuestas: toda petición que dependa de «la propia computadora» usa `Request.Local` (nunca la IP
  sola); el secreto de una solicitud de emparejamiento y la credencial de dispositivo solo los ve Rust; la identidad del
  servidor nunca se reemplaza automáticamente.
- §19 Riesgos: la identidad no viaja en los respaldos (restaurar en otra computadora = emparejar de nuevo); permiso
  «Red local» de macOS y firewall pendientes de verificar en la computadora real; el cupo de 20 solicitudes abiertas
  puede llenarse desde la LAN durante 5 minutos.

**Fusión con A y D y verificación del coordinador** (13/11/2026): la rama `etapa-9g-b` salió de la Fase A, antes de la D.
Se fusionó en `bf6fa95` con conflictos en la 0023 (archivo creado por las dos ramas: queda el encabezado, la sección D y
después la sección B; la B no toca `realtime_events_scope_check`), `config/product.json` y sus copias generadas (se
conservan `min_client_version` y `min_server_version` de D), `main.go` (trabajador de «Agotado» y canal para las dos
escuchas), `http.go` y `service.go` (campos de la petición de ambas fases) y `API_CONTRACTS.md` (ambos bloques). Copia
exacta de la fusión: Go unitarias con `-race`, integración de los 11 paquetes de la tupla (`administration` 106 s con el
emparejamiento) y `cargo test` aprobados; Playwright falló solo en `workflow-stage9g-terminals` (y en cascada
`workflow-zz-test-data`) porque la prueba de «Agotado» de D enrolaba una terminal «Caja 2 9G» que el filtro
`hasText: 'Caja 2'` también encontraba. Corregido en `c502706` (la terminal de esa prueba se llama «Barra de agotados
9G»; sin cambios de producto) y repetido: `restaurant`, `workflow-stage9g-modality`, `workflow-stage9g-sold-out`,
`workflow-stage9g-terminals` (2) y `workflow-zz-test-data`, 6 de 6 en Chromium y 6 de 6 en WebKit. Formato, `sync`,
`vet`, `GOOS=windows`, `tsc` y Vitest también aprobados sobre la fusión.

**Decisión del coordinador D15 · Identidad del servidor y respaldos**: la carpeta `identity/` (certificado, llave privada y
`server_id`) no viaja en los respaldos de la base (que pueden ir a una USB): restaurar en otra computadora genera otra
identidad y obliga a emparejar de nuevo las terminales, como pide §3.1 ante un certificado distinto. Se documenta en
`docs/INSTALAR_EN_RED.md`.

## Fase F · Respaldos programados

Un agente (13–14/11/2026), en su árbol de trabajo `etapa-9g-f` (desde la fusión de A y D, `72b81dc`, más la corrección
del coordinador `67741a2` de `compatibility.test.tsx`), en paralelo con la Fase B. Decisiones del coordinador D7
(restaurar sigue por línea de comandos) y D12 (una sola 0023 con una sección por fase). Pruebas en el clúster temporal
55499, Go temporal 8187, vista previa 1425, Rust en `cache/cargo-F` y todas las carpetas «del sistema» y de respaldos
dentro del directorio temporal (`TMPDIR=$SCR/tmp-F` para las pruebas de Go); nunca contra la instalación real.

Commits: `7bf10ba` (servidor: migración, paquete, rutas, trabajador y pruebas de Go; el mensaje dice «1/3» por error,
son dos) y `39a0390` (escritorio, puente nativo, pruebas de interfaz y de Playwright, contrato y formato).

### Qué cambió

**Migración `0023`, sección `-- Fase F`** (al final, sin tocar la D; transaccional, sin modificar filas existentes):

1. `app.backup_settings` (una fila): `enabled` (true), `scheduled_time` (NULL = la «Hora de inicio del día»; sin
   segundos), `keep_count` (30, de 1 a 365), `directory` (NULL = la predeterminada; 2–1024 caracteres sin controles),
   `updated_by`, `updated_at`. Se inserta la fila con los valores por omisión.
2. `app.backup_runs`: `kind` (`automatic` con `scheduled_for`; `manual` con `operation_id` único, `requested_by` y
   `requested_device`), `status` (`running`/`succeeded`/`failed`), `started_at`, `finished_at`, `requested_directory`
   (la carpeta configurada al empezar), `directory` (la usada), `used_default_directory` y `fallback_reason`
   (`missing`/`not_directory`/`not_writable`, solo si había una carpeta elegida), `file_name` (sin barras), `size_bytes`
   (> 0), `verified`, `failure_reason` (`directory_unavailable`/`dump_failed`/`verify_failed`/`interrupted`) y
   `removed_at` (la retención borró el archivo). Restricciones con nombre que impiden registros incoherentes (correcto ⇒
   verificado con archivo, tamaño y carpeta; fallido ⇒ sin archivo y con motivo; en curso ⇒ sin fin ni resultado; fin
   ≥ inicio). Índices por `started_at` y parcial de las que siguen en curso.

**Go**

- `internal/backupschedule` (paquete nuevo, en la tupla de `scripts/database.py`; se llama así porque el `.gitignore`
  ignora toda carpeta `backups/`):
  - `schedule.go`: `LastSlot`/`NextSlot` (la hora configurada del día local en la zona del establecimiento con
    `businessday.CutoffInstant`, la misma regla de cambio de horario que la «Hora de inicio del día»), `Due` (no hay un
    automático correcto ni uno en curso desde el último instante programado, quedan intentos y pasó la espera) y la
    explicación de toda la programación en el comentario del paquete.
  - `files.go`: nombres `<base>-<AAAAMMDDTHHMMSSZ>-<6 hex>-automatico.dump` / `-manual.dump` (`.partial` mientras se
    crea), `Retain` (solo archivos regulares con el nombre exacto de un automático de esta base), `CleanPartials`,
    `CheckDirectory` (existe, es carpeta y acepta un archivo de prueba; nunca la crea), `CheckExists` (sin escribir,
    para la pantalla), `ValidateDirectory` (ruta absoluta sin controles, existente, escribible y que no sea una carpeta
    de datos de PostgreSQL), `Describe` (nombre visible de la carpeta y del disco: `/Volumes/<disco>` en macOS, la
    unidad en Windows) y `SameDirectory`.
  - `settings.go` (lectura y guardado de la configuración), `runs.go` (ejecuciones, `RunView`, `LastBackup`,
    `FindOperation`, historial del instante programado y el aviso: `ComputeAlert` + `alertFor` puro), `state.go`
    (sección de Mantenimiento y resumen), `runner.go` (el respaldo en sí, ver «Decisiones») y `worker.go` (trabajador y
    `Plan`).
- `internal/maintenance/backup.go`: interfaz `ScheduledBackuper` (extiende `Backuper` con `DefaultDirectory`,
  `DatabaseName`, `Dump(ctx, archivo)` y `Verify(ctx, archivo)`), `CheckListing` (el catálogo de `pg_restore --list`
  debe traer `TABLE DATA public schema_migrations`) y `DumpBackup` la implementa; `DumpBackup.Backup` reutiliza `Dump`
  sin cambiar su comportamiento ni su comprobación.
- `internal/standalone/backup.go`: `DumpFile` y `VerifyFile` con el `pg_dump`/`pg_restore` de la instalación;
  `Backup` reutiliza `DumpFile` (el archivo a medias ahora también se borra si no se pueden ajustar sus permisos).
- `cmd/ad-el-pos-server`: `installationBackup` (host de macOS y de Windows) implementa `ScheduledBackuper` con la
  carpeta `backups` del servicio; `newApplication` crea `Service.ScheduledBackups` cuando el respaldo lo admite (con
  `serve`, `DumpBackup` y `backup_directory` o `backups/` junto a la configuración) y `serveHTTP` arranca
  `backupschedule.Worker` junto a los de cocina, inventario y «Agotado» (bloque acotado; espera su fin al apagar).
- `internal/administration/backups.go` (rutas `maintenance/backups*`, bloque en `routes.go` antes del prefijo
  `maintenance/`, caso `*backupNow` en `completeDeferred`, campo `Service.ScheduledBackups`) y `service_host.go`
  (`last_backup` = `backupschedule.LastBackup`, `null` sin respaldos).

**Escritorio**

- Rust: `src/backup_folder.rs` (comando cerrado `choose_backup_directory`, registrado en `main.rs`, `build.rs` y
  `permissions/diagnostics.toml`), cinco operaciones fijas en `api.rs` (`maintenance_backups`,
  `maintenance_backups_status`, `maintenance_save_backup_settings`, `maintenance_backup_default_directory`,
  `maintenance_backup_now` con 130 s de espera). La ruta que guarda una carpeta **no** está en la lista cerrada.
- React: `services/backups.ts` (tipos, llamadas y todos los textos en palabras del restaurante),
  `pages/maintenance/BackupSection.tsx` («Respaldos» en Mantenimiento, entre «Servicio del servidor» y «Modalidad»),
  `pages/maintenance/BackupAlert.tsx` (aviso persistente), `App.tsx` (aviso en la pantalla de inicio, `onMaintenance`
  de «Solo servidor»), `ServerOnlyPage.tsx` (tarjeta «Último respaldo» real y aviso), `StatusPage.tsx` (tarjeta «Último
  respaldo» con `settings.manage`), `AuditPage.tsx` (tres acciones nuevas), `services/serverService.ts` (tipo de
  `last_backup`), `navigation/sections.ts` (descripción de Mantenimiento), `styles.css` (bloque al final, solo tokens) y
  `testing/backups.ts` (datos de prueba).
- `e2e/native-driver.ts`: simula `choose_backup_directory` (solo con el servidor local; `nextBackupFolder(ruta |
'cancel')`, sin respuesta se cancela; envía la ruta con `PUT maintenance/backups/directory`).
- `API_CONTRACTS.md`: bloque «Etapa 9G · respaldos programados» al final y una frase del bloque de la fase A
  (`last_backup`).

### Decisiones tomadas en la fase

1. **Hora y días**: el instante programado de cada día es la hora configurada (por omisión, la «Hora de inicio del día»,
   que si cambia mueve también el respaldo) en la zona horaria del establecimiento. Cambio de horario como la «Hora de
   inicio del día»: una hora inexistente (adelanto) pasa al primer instante después del salto; una que ocurre dos veces
   (atraso) es la primera, y la segunda no repite el respaldo del día.
2. **Computadora apagada a esa hora**: toca respaldar cuando no hay un respaldo **automático correcto que haya empezado
   en o después del último instante programado**. Al arrancar, el trabajador espera **2 minutos** (que termine de
   arrancar la computadora, se monte una USB y se reconecten las terminales) y hace el pendiente; después revisa cada 30
   s. Cambiar la hora a media jornada no repite el respaldo del día (el de la mañana ya empezó después del instante
   anterior). Activar los respaldos desactivados o terminar el asistente inicial hace el del día en la siguiente
   revisión (una instalación nueva tiene su primer respaldo en cuanto se configura). Antes del asistente inicial no se
   respalda.
3. **Reintentos**: un automático fallido se reintenta a la hora, como máximo **3 intentos por instante programado**;
   el aviso sigue hasta un respaldo correcto posterior. Una ejecución que quedó «en curso» porque el servidor se detuvo
   se cierra como `interrupted` (al arrancar el trabajador o antes de cada respaldo, siempre con el lock tomado).
4. **Lock 7820148309** (`pg_try_advisory_lock`, de sesión, en una conexión propia del pool que se conserva todo el
   respaldo): un solo respaldo a la vez aunque dos servidores compartan la base; si está ocupado, el automático espera a
   la siguiente revisión y «Respaldar ahora» responde `409 BACKUP_IN_PROGRESS`. **Orden**: se toma solo, sin ninguna
   transacción abierta y nunca dentro de una que tenga otro lock; mientras se tiene únicamente se escriben
   `backup_runs` y la auditoría de «Respaldar ahora» en transacciones cortas sin locks consultivos (`pg_dump` no toma
   locks consultivos; sus `ACCESS SHARE` solo esperarían a un DDL). Si no se puede soltar, se cierra la conexión.
5. **Cada respaldo se verifica antes de conservarlo**: se crea como `….dump.partial`, se verifica con `pg_restore
--list` (y su catálogo debe traer los datos de `public.schema_migrations`: un archivo legible de otra base o vacía no
   cuenta) y solo entonces recibe su nombre definitivo. Si falla, se borra (no sirve para restaurar) y queda
   `verify_failed`. Así la retención nunca cuenta un respaldo a medias o sin verificar, y siempre queda al menos un
   respaldo verificado (la retención solo corre después de uno correcto).
6. **Retención**: después de cada automático correcto, en la carpeta donde quedó, se conservan los `keep` automáticos
   más recientes (por el nombre, que lleva la fecha UTC) y se borran solo los archivos regulares con el nombre exacto de
   un automático de esta base; nunca los de «Respaldar ahora», los de la línea de comandos, los de antes de migrar o de
   «Quitar datos de prueba», los `.partial`, los enlaces, las carpetas ni otros archivos. Se anota `removed_at`.
7. **Carpeta**: se elige solo en la computadora principal (Go exige la conexión local; Rust solo abre el diálogo con el
   servidor local) y solo con el diálogo nativo; Go la vuelve a validar. **Nunca se crea**: si la USB no está, no se
   crea una carpeta con su nombre en el disco. Si al respaldar no existe, no es carpeta o no acepta archivos, se usa la
   predeterminada (que sí se crea, 0700) con el motivo, y se avisa. Elegir la predeterminada equivale a volver a ella.
   La pantalla muestra el nombre de la carpeta y del disco; la ruta completa solo en «Información para soporte».
8. **«Respaldar ahora»** usa el mismo camino (carpeta, verificación, registro) con la etiqueta `manual`; **no cuenta
   para la retención** ni sustituye al automático del día. Idempotencia con `backup_runs.operation_id` (misma clave del
   mismo usuario y terminal → la misma ejecución, en curso o terminada; de otro → `409 IDEMPOTENCY_CONFLICT`), sin
   `restaurant_operations`. El respaldo corre en segundo plano con el contexto del trabajador (si la terminal se
   desconecta, termina igual); la respuesta espera hasta 5 s antes del plazo de la petición y si no, devuelve «en
   curso» (la pantalla consulta cada 3 s mientras hay uno). Se audita al terminar, correcto o no
   (`maintenance.backup_created`).
9. **Aviso al administrador**: con `settings.manage`, el permiso de Mantenimiento: quien puede cambiar la configuración
   es quien puede corregir la carpeta o la hora, y el aviso le dice qué hacer. Dos tipos que calcula Go: `failed` (el
   último respaldo terminado falló; dice desde cuándo y cuántos intentos) y `fallback` (el último fue correcto pero en
   la predeterminada porque la carpeta elegida, la que sigue configurada, no estaba). No se puede cerrar: desaparece con
   un respaldo correcto posterior (o, el de carpeta, al volver a la predeterminada o elegir otra). Se muestra en la
   **pantalla de inicio de la terminal** (sea cual sea, solo cuando está en ella y fuera de la captura), dentro de
   «Respaldos» y en la pantalla de «Solo servidor». Se consulta cada 60 s (y tras cada cambio de respaldos) en lugar
   de un evento en tiempo real: un ámbito nuevo obligaba a redefinir `realtime_events_scope_check` en paralelo con la
   Fase B; para un respaldo diario basta.
10. **Respaldos de la línea de comandos** (`dev.sh backup`, `ad-el-pos-server backup`, antes de migrar) no cambian y no
    se registran en `backup_runs` (pueden correr sin el servidor). La lista y el aviso son de los que hace el servidor.
11. **Restaurar** no cambia (D7): la sección lo dice («pide ayuda a soporte: se hace desde la computadora principal con
    el servidor detenido») y no hay restauración en la aplicación.
12. **Contrato de la API (`internal/apicontract`)**: sus pruebas solo exigen que cada operación **documentada** esté en
    `docs/API_V1.md` y se recorra; no obligan a documentar rutas nuevas. La regla propuesta en la D es para rutas de
    **clientes** (Android); estas son de administración en la computadora principal (y la de la carpeta, solo local),
    así que no se agregan al contrato y se documentan en `API_CONTRACTS.md`.
13. Con `serve` (desarrollo y, hasta la Fase G, la instalación actual de la Mac si se arranca con este código) el
    respaldo automático también corre y deja sus archivos en `backups/` junto a la carpeta de la configuración
    (`backup_directory` en las pruebas): solo borra sus propios automáticos, nunca los respaldos que ya hay ahí.

### Pruebas ejecutadas

14/11/2026, en el árbol `etapa-9g-f` con el arnés `iso.sh` (clúster 55499, Go 8187, vista previa 1425, Rust en
`cache/cargo-F`, `TMPDIR=$SCR/tmp-F`). Pantalla desbloqueada (`CGSSessionScreenIsLocked` ausente).

| Suite                                                                                                                                                                                     | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `format` (Prettier, `gofmt`, `cargo fmt`), `sync --check`                                                                                                                                 | aprobado; `docs/etapas/*.md` con el mismo md5 antes y después de `dev.sh format`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `vet` (normal e `integration`) y `win` (`GOOS=windows` build, vet y vet `integration`)                                                                                                    | aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Go unitarias con `-race` (`go test ./...`)                                                                                                                                                | aprobado (15 s); nuevas: `backupschedule` (instante programado con la hora de inicio del día, fin de mes y año, zona del establecimiento, adelanto y atraso de horario en `America/Tijuana`, `Due`, nombres, retención que solo borra sus archivos —manuales, de la línea de comandos, de antes de migrar, de otra base, `.partial`, mayúsculas, enlaces y carpetas intactos—, restos a medias, carpeta disponible/inexistente/archivo/sin permiso sin crearla nunca, validación de la carpeta elegida, nombres visibles, carpeta no disponible → predeterminada, verificación antes del nombre definitivo, fallo de verificación/pg_dump/interrumpido sin restos y el aviso) y `maintenance` (`Dump`/`Verify` privados, archivo legible de otra base rechazado, `CheckListing`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Integración `./internal/backupschedule` (`-race`)                                                                                                                                         | aprobado, 7 s: trabajador con respaldo simulado (sin establecimiento no respalda; pendiente al arrancar; no repite al cambiar la hora; retención con archivos ajenos; USB desconectada → predeterminada con aviso y la carpeta sin crear, luego la USB vuelve y el aviso se va; fallo de verificación → aviso con reintento a la hora y desde cuándo, hasta un respaldo correcto; máximo 3 intentos; interrumpido; desactivados), trabajador real con esperas cortas (respalda lo pendiente una vez y, al apagarse a media copia, el manual queda `interrupted` y se espera), **lock 7820148309** (un segundo servidor con otro pool recibe ocupado para el automático y para «Respaldar ahora»; el lock se ve tomado y se suelta; 8 revisiones simultáneas desde dos servidores dejan un solo respaldo por instante), **«Respaldar ahora» idempotente** (6 envíos simultáneos con la misma clave: una ejecución, una copia, una auditoría; otro usuario u otra terminal → conflicto; no cuenta para el automático del día) y **respaldo real** con `pg_dump`/`pg_restore` de PostgreSQL 18 contra el clúster 55499 (archivo 0600 con `backup_runs` en su catálogo, automáticos en una carpeta elegida con retención de 2 que no toca el manual, carpeta que desaparece → predeterminada, verificación real) |
| Integración `./internal/administration` (`-race`, toda la tupla del paquete)                                                                                                              | aprobado, 105 s: `TestStageTwoAcceptance` (con `last_backup` en `null` de la fase A) y `TestStage9GBackupRoutes` (403 sin `settings.manage`; estado inicial; 8 configuraciones inválidas; guardado auditado con antes y después y sin auditoría si no cambia; carpeta por la red → `403 LOCAL_REQUIRED` y `directory_change_available` falso; 5 carpetas inválidas sin crear ninguna; carpeta elegida auditada; elegir la predeterminada = volver a ella; «Respaldar ahora» con clave inválida, correcto y auditado, reenvío sin otra copia, otro usuario → 409; fallo de verificación → `failed`, auditoría fallida, aviso en `maintenance/backups/status` y `last_backup` fallido en `maintenance/service`; respaldo en curso → otro `409 BACKUP_IN_PROGRESS`, reenvío «en curso», respuesta «en curso» al vencer el plazo y auditoría al terminar después; un correcto posterior quita el aviso)                                                                                                                                                                                                                                                                                                                                                                                                          |
| Integración `./internal/database`                                                                                                                                                         | aprobado, 11 s (`TestStage9GAndroidUpgrade` con `checkStage9GBackups`: fila de configuración por omisión, registro vacío, 11 registros incoherentes rechazados y uno manual con predeterminada por falta de la elegida aceptado)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS`, con el trabajador de respaldos en el `host`)                                                                              | aprobado, 49 s                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `cargo test --locked --offline`                                                                                                                                                           | 61 aprobadas, 1 ignorada (nuevas: `backup_folder` —solo servidor local, solo rutas absolutas en texto, un diálogo a la vez, la ventana sigue sin permisos `dialog:*`/`fs:*`— y `api` —rutas fijas de respaldos y ninguna operación que guarde una carpeta—)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `tsc --noEmit` / Vitest (`npm test`)                                                                                                                                                      | sin errores / 91 archivos, 856 pruebas (12 nuevas en `stage9gBackups.test.tsx`: textos sin rutas, sección completa, guardado con `time: null` u otra hora, clave conservada ante una respuesta sin conexión y nueva después, carpeta con la operación nativa sin argumentos ni rutas, sin carpeta desde otra terminal, aviso con «Ver respaldos», «Solo servidor» y el aviso en la aplicación solo con `settings.manage` y solo en la pantalla de inicio; `maintenance.test.tsx` responde la sección nueva)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Playwright Chromium y WebKit (`restaurant`, `workflow-stage9c-navigation`, `workflow-stage9g-backups`, `workflow-stage9g-modality`, `workflow-stage9g-sold-out`, `workflow-zz-test-data`) | Chromium 6/6 (2.3 min), WebKit 6/6 (1.9 min)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Playwright final sobre el último commit (`workflow-stage9g-backups`, `workflow-stage9g-modality`, `workflow-zz-test-data`)                                                                | Chromium 3/3 (27 s), WebKit 3/3 (28 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |

`workflow-stage9g-backups.spec.ts`: desactiva el respaldo automático durante la prueba (y lo deja activo con 30 al
final), cambia los conservados (auditado), cancela el diálogo, elige una carpeta, rechaza una inexistente, «Respaldar
ahora» real queda en la carpeta elegida y revisado, la carpeta desaparece → respaldo en la predeterminada con aviso en
Mantenimiento y en la pantalla de inicio, «Ver respaldos» lleva a Mantenimiento, la carpeta predeterminada se sustituye
por un archivo → respaldo fallido con aviso en la pantalla de inicio (y no en otras pantallas) y, al devolverla, un
respaldo correcto quita el aviso. `workflow-stage9g-modality.spec.ts` (fase A) ya no supone «Sin registro» en «Último
respaldo» (puede haber respaldos de esta especificación o del automático del servidor temporal).

### No verificado y lo que queda

- El diálogo nativo de carpeta real (`blocking_pick_folder` sobre la ventana) solo se probó compilado y con la operación
  simulada en Playwright; una USB real conectada y desconectada se simuló con carpetas que aparecen y desaparecen.
- **Fase W (Windows)**: el host de Windows ya tiene los respaldos programados (usa el mismo `installationBackup`), pero
  sin probar: (a) en una USB con FAT32/exFAT `fileacl.Protect` (ACL privada) fallará y el respaldo quedará `dump_failed`
  en esa carpeta (decidir si en Windows se acepta sin ACL en una carpeta elegida o se usa la predeterminada); (b)
  `pg_dump --file` con rutas no ASCII (la instalación usa rutas cortas 8.3 para los programas, no para el archivo); (c)
  `Describe` muestra la unidad (`E:`) como disco; (d) el diálogo nativo de carpeta en Windows.
- Un respaldo en una USB lo puede leer cualquiera que tenga la USB (FAT/exFAT no guarda permisos): decirlo en la guía.
- `scripts/restore-check.py` compara conteos de todas las tablas: si un respaldo automático empieza entre `dev.sh
backup` y la comprobación, `backup_runs` tendrá una fila más (falso «difieren»; repetir).

### Para `docs/INSTALAR_EN_RED.md` (Fase G) · «Dónde están los respaldos y cómo cambiar su carpeta»

> La computadora principal respalda sola la base de datos del restaurante **todos los días** a la «Hora de inicio del
> día» (05:00 si no la cambiaste) y conserva los **últimos 30** respaldos automáticos; los más viejos se borran solos.
> Si la computadora estaba apagada a esa hora, el respaldo se hace unos minutos después de encenderla. Cada respaldo se
> revisa al crearse.
>
> **Dónde están**: en la carpeta predeterminada de la computadora principal (en la Mac,
> `/Library/Application Support/AD-EL POS/backups`; en Windows, la carpeta `backups` de los datos de AD-EL POS), o en la
> carpeta que elijas. En **Administración › Sistema › Mantenimiento › Respaldos** ves el último respaldo, si fue
> correcto, dónde está y la lista de los más recientes (la ruta completa está en «Información para soporte»).
>
> **Cambiar la carpeta** (por ejemplo, a otro disco o a una USB): en la **computadora principal**, en
> Mantenimiento › Respaldos, toca **«Elegir carpeta»** y elige la carpeta en la ventana que se abre. Desde otra terminal
> no se puede. Para volver a la de siempre, **«Usar la carpeta predeterminada»**. Ahí mismo cambias la hora, cuántos
> conservar o desactivas el respaldo automático.
>
> **Si la USB no está conectada** a la hora del respaldo, se guarda en la carpeta predeterminada y aparece un aviso en
> la pantalla de inicio del administrador. Si un respaldo **falla**, el aviso dice qué pasó y sigue hasta que haya un
> respaldo correcto; puedes tocar **«Respaldar ahora»** después de corregirlo.
>
> **Cuida la USB**: quien la tenga puede leer los datos del restaurante. Guárdala en un lugar seguro.
>
> **Restaurar un respaldo** no se hace desde la aplicación: pide ayuda a soporte. En la computadora principal, con una
> ventana de Terminal, primero se ensaya (`".../ad-el-pos-server" restore-check --file <respaldo>`) y después se
> restaura (`restore --file <respaldo>`), que pausa el servidor, guarda antes un respaldo del estado actual y lo vuelve a
> arrancar. (En la Mac el programa está en `/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server`; el respaldo
> debe estar en la carpeta de respaldos de AD-EL POS: copia ahí el de la USB si hace falta.)

### Para el revisor independiente (respaldos)

Archivos: `backend/internal/backupschedule/*.go` (sobre todo `runner.go`, `schedule.go`, `files.go`, `runs.go`),
`backend/internal/administration/backups.go` (y el caso en `reports.go` → `completeDeferred`, el bloque de
`routes.go`, `service_host.go`), `backend/internal/maintenance/backup.go` (`Dump`, `Verify`, `CheckListing`),
`backend/internal/standalone/backup.go` (`DumpFile`, `VerifyFile`), `backend/cmd/ad-el-pos-server/host.go` y
`main.go` (trabajador), la sección F de la 0023, `desktop/src-tauri/src/backup_folder.rs`, las operaciones de `api.rs`,
`desktop/src/pages/maintenance/BackupSection.tsx`, `BackupAlert.tsx`, `services/backups.ts`, la simulación de
`native-driver.ts` y las pruebas (`backupschedule/*_test.go`, `administration/backups_acceptance_test.go`,
`database/stage9g_backups_upgrade_test.go`, `maintenance/backup_test.go`, `pages/stage9gBackups.test.tsx`,
`e2e/workflow-stage9g-backups.spec.ts`).

Reglas que conviene comprobar:

1. La retención nunca borra un archivo que no sea un respaldo automático de esta base con su nombre exacto, ni un
   enlace o carpeta, ni en otra carpeta que la del respaldo recién hecho; nunca corre después de un fallo; siempre queda
   al menos un respaldo verificado.
2. Ningún archivo sin verificar recibe el nombre definitivo; un fallo de creación o de verificación no deja archivos.
3. Nunca se crea la carpeta elegida; sin ella se usa la predeterminada y se registra el motivo; la ruta que guarda Go
   solo llega del diálogo nativo (Rust) por una conexión local y Go la valida.
4. Nunca hay dos respaldos a la vez (lock 7820148309 de sesión con `pg_try_advisory_lock`, soltado siempre) y el lock no
   se combina con otros.
5. El instante programado y «toca respaldar» son correctos con la zona del establecimiento, el cambio de horario, la
   computadora apagada, el cambio de hora y los reintentos (máximo 3, cada hora).
6. «Respaldar ahora» es idempotente (misma clave → misma ejecución; otro usuario o terminal → 409) y se audita una vez,
   también si la petición venció antes de terminar.
7. El aviso aparece a quien tiene `settings.manage` (y solo a quien lo tiene: Go exige el permiso) y desaparece solo
   con un respaldo correcto posterior; un registro «en curso» de un servidor detenido termina como `interrupted`.
8. Ningún texto visible tiene rutas, nombres de programas ni términos técnicos fuera de «Información para soporte».

### Para CLAUDE.md

- Esquema 23 (0023, sección F): `backup_settings` y `backup_runs`.
- §3 Composición: `maintenance/backups*` → `administration/backups.go` con el paquete `internal/backupschedule`
  (trabajador `backupschedule.Worker` en `main.go`); `maintenance.ScheduledBackuper` (`DumpBackup` con `serve`,
  `installationBackup` con `standalone.DumpFile`/`VerifyFile` en el host); comando Tauri `choose_backup_directory`
  (`backup_folder.rs`).
- §3 **Locks, fila nueva**: `7820148309 | Respaldos programados y «Respaldar ahora» (etapa 9G, internal/backupschedule):
pg_try_advisory_lock de sesión en una conexión propia mientras dura el respaldo (uno a la vez, sin esperar); se toma
solo, sin transacción abierta ni otros locks, y mientras se tiene solo se escriben backup_runs y la auditoría en
transacciones cortas sin locks consultivos.`
- §5: `dev.sh backup` y los comandos `backup`/`restore-check`/`restore` siguen igual; el servidor hace además un respaldo
  automático diario (también con `serve`, en `backup_directory` o `backups/`).
- §6: `./internal/backupschedule` en la tupla de integración (sus pruebas usan carpetas de `t.TempDir()`: correr con
  `TMPDIR` dentro del directorio temporal de la sesión); el Go temporal de Playwright también hace su respaldo automático
  diario (2 min después de arrancar, en su `backup_directory`); `workflow-stage9g-backups.spec.ts` aparta y devuelve la
  carpeta predeterminada del servidor temporal para provocar un fallo.
- §11–12 (regla permanente propuesta): «Un aviso que exige que el administrador corrija algo (como un respaldo
  fallido) se muestra en la pantalla de inicio de la terminal mientras siga vigente y lo calcula Go».
- §15: reglas de los respaldos programados (decisiones 1–13); auditoría nueva `maintenance.backup_settings_changed`,
  `maintenance.backup_directory_changed` y `maintenance.backup_created`; sin permisos nuevos (`settings.manage`).
- §19: diálogo de carpeta real, USB real y Windows sin verificar (ver «No verificado»).

**Fusión con A, D y B y verificación del coordinador** (13/11/2026): la rama `etapa-9g-f` salió de la fusión de A y D,
antes de la B. Se fusionó en `a7989e9` con conflictos solo de bloques agregados por ambas ramas (0023 —queda el
encabezado y las secciones D, B y F, en ese orden—, trabajadores de `main.go`, campos de `Application`, comandos de
`build.rs`, `main.rs` y `diagnostics.toml`, nombres del Registro de movimientos, estilos y `API_CONTRACTS.md`), más el
formato de la prueba de «Agotado» renombrada en la fusión anterior (`b84415c`). Copia exacta de `b84415c`: formato,
`sync`, `vet`, `GOOS=windows`, `tsc`, Go unitarias con `-race`, integración de los 12 paquetes de la tupla
(`operations` 308 s, `reports` 130 s, `administration` 117 s, `reports/operational` 45 s, `cmd/ad-el-pos-server` 43 s,
`designer`, `catalog`, `identity`, `database`, `apicontract`, `backupschedule` y `payments`), Vitest, `cargo test` y
Playwright (`restaurant`, `table-price-decrease`, `takeaway-quick-notes`, `workflow-stage9c-navigation`, las cinco
`workflow-stage9g-*` y `workflow-zz-test-data`: 10 de 10 en Chromium y 10 de 10 en WebKit): todo aprobado.

## Fase C · Descubrimiento del servidor y comportamiento en red

Un agente, en su propio árbol de trabajo (`etapa-9g-c`, creada desde la fusión de A, D y B en `bf6fa95`, más
`5d4fa94`: el arreglo del coordinador de la prueba de «Agotado»), en paralelo con la Fase F. 13/11/2026. **Sin
migración** (la 0023 no tiene sección C). Decisiones del coordinador D1, D2, D6 y D11. Pruebas en copias aisladas:
clúster temporal 55499, Go temporal 8188 (Playwright), vista previa 1426, Rust en `cache/cargo-C`; las pruebas TLS
de Rust en puertos efímeros de 127.0.0.1. Los anuncios de prueba en la red local duran unos segundos, con nombres
únicos «AD-EL POS prueba <hex>» y puertos 8201/8202 sin escucha. **Nunca se abrió el 8443 real, no se usó el
Keychain real ni la instalación real.**

Commits: `62b6400` (Go, contrato, Rust e `Info.plist`) y `43e714f` (interfaz, puente y Playwright). La
`workflow-stage9g-sold-out.spec.ts` del arreglo del coordinador (`5d4fa94`) quedó sin formato Prettier (una línea
larga); `scripts/dev.sh format` la partió en el segundo commit (solo formato, el contenido no cambió).

### Qué cambió

**Servidor Go**

- `internal/discovery` (nuevo): anuncio del servidor en la red local por mDNS/DNS-SD como `_adelpos._tcp`.
  - `discovery.go`: `InstanceName` («Fonda Doña Ñeli — Mac de caja»: establecimiento y computadora; quita
    controles y puntos, junta espacios, cabe en una etiqueta DNS de 63 bytes UTF-8 sin partir caracteres: si no
    cabe, la computadora se acorta a 24 bytes y el establecimiento ocupa el resto), `TXTRecords` (`txtvers=1`,
    `server_id`, `api=v1`, `port`, `fp` = primeros 8 bytes de la huella SPKI en hexadecimal) y `Announcer.Run`, que
    mantiene el anuncio: lo vuelve a publicar si cambia el nombre (revisión cada minuto) o si la publicación del
    sistema termina sola (espera creciente de 5 s a 1 min), lo retira al terminar el contexto y, en un sistema sin
    implementación, lo informa una vez (`discovery_unavailable`) y termina sin error.
  - `announce_darwin.go`: Bonjour del sistema con `/usr/bin/dns-sd -R <instancia> _adelpos._tcp local <puerto>
<TXT…>` como proceso hijo (argumentos separados, sin intérprete de órdenes); se detiene con SIGTERM (3 s y
    después SIGKILL) para que mDNSResponder despida el registro. Lee la salida solo para la bitácora
    (`discovery_registered`, `discovery_name_conflict`). `announce_other.go`: «no disponible» fuera de macOS, con
    el punto de integración de Windows anotado.
  - `computer*.go`: nombre de la computadora (macOS: `scutil --get ComputerName`, con acentos; si no, el nombre de
    red sin `.local`).
- `cmd/ad-el-pos-server/main.go`: en `serveHTTP`, **solo con la escucha de red abierta y fuera del modo de
  pruebas**, arranca el anuncio con `discoverySource` (nombre del establecimiento de la base, computadora, puerto de
  la escucha HTTPS y TXT con la identidad de B) y lo espera al apagar junto a los trabajadores.
  `administration/discovery.go`: `EstablishmentName` (sin establecimiento todavía: "").
- `internal/httpapi/clientversion.go` + `internal/lanlistener` (`WithNetwork`, exportado para las pruebas): **por la
  escucha de red `X-Client-Version` es obligatoria** (decisión de la fase, abajo): sin ella, 426
  `CLIENT_UPDATE_REQUIRED` con `min_client_version` y `server_version`. Por la escucha local nada cambia.
- `go.mod`: `golang.org/x/net v0.60.0` pasa a directa (ya estaba en `go.sum` y en la caché; sin descargas) por el
  cliente DNS-SD de las pruebas (`dns/dnsmessage`).
- `scripts/database.py`: `./internal/discovery` en la tupla de integración.

**Contrato v1** (`internal/apicontract/openapi.json`, `docs/API_V1.md`, `contract_integration_test.go`)

- Seis operaciones nuevas de la terminal que se empareja (las que usará Android), etiqueta «Emparejamiento», sin
  seguridad de dispositivo y con el parámetro `X-Pairing-Secret`: `createPairingRequest` (`POST pairing/requests`),
  `revealPairingNonce`, `getPairingRequest`, `renamePairingRequest`, `cancelPairingRequest` y
  `claimPairingCredential`. `ClientVersion` explica que por la escucha de red es obligatoria. Las rutas de
  administración de B (`devices/pairing-requests*`, `devices/sessions-summary`, `devices/{id}/configuration`) no
  entran: no son de clientes.
- `contractPairing` en el recorrido: un «Mesero contrato» Android pide el código con el compromiso, revela, cambia
  su nombre, consulta, la computadora principal aprueba con el código derivado (`administration.PairingCode` con la
  huella de una identidad temporal), reclama una sola vez (el segundo reclamo, 409 sin token) y otra solicitud se
  cancela. 26 operaciones recorridas.
- `docs/API_V1.md`: filas de la tabla, sección «Encontrar el servidor y emparejarse (fases B y C)» (mDNS, TXT,
  huella, código, cambio de IP) y la regla de `X-Client-Version` por la red. `API_CONTRACTS.md`: bloque «Etapa 9G ·
  descubrimiento y comportamiento en red (fase C)».

**Escritorio (Rust)**

- `discovery.rs` (nuevo): consulta mDNS de una sola vez con `std::net::UdpSocket` (puerto efímero →
  `224.0.0.251:5353`, se repite a la mitad de 1.5 s y, si faltan registros, una segunda pregunta por SRV, TXT o A),
  lector propio de mensajes DNS (nombres comprimidos con saltos solo hacia atrás y como máximo 16, límite de 255
  bytes, paquetes cortados sin pánico), `collect` (solo instancias completas con `server_id` UUID en minúsculas,
  `fp` de 16 hexadecimales, puerto y al menos una IPv4 de la red local; las repetidas por `server_id` juntan
  direcciones). `ServerLocator` en `TerminalState`: dirección vigente del servidor (`RwLock`), última búsqueda
  (React solo recibe `{id, name, paired}`) y **cambio de IP**: `relocate` (una búsqueda a la vez con un `Mutex`
  asíncrono y como máximo cada 10 s) → `candidates` (mismo `server_id` y misma huella corta, distinta de la que
  falló) → `probe` (`GET /api/v1/health/live` con `PinnedVerifier::pinned` de la huella completa) → `move_to`
  (valida con `check_paired`, guarda `server_url` en `connection.json` y cambia la dirección vigente). Comando
  `discovery_search`.
- `api.rs`: la petición se arma con la dirección vigente; con un **error de conexión** (no llegó al servidor) se
  intenta `relocate` y, si encontró el servidor, se repite la petición una sola vez en la dirección nueva
  (`discovery::rebase`). `realtime.rs` lee la dirección en cada intento y llama a `relocate` al fallar la conexión
  (el cursor se conserva). `export.rs` y `server_snapshot` usan la dirección vigente.
- `pairing.rs`: `pairing_start_found {serverId, name}` (servidor elegido de la búsqueda: prueba sus direcciones;
  antes de pedir nada fija el certificado con `GET version` y exige que su huella empiece como la anunciada, y
  después que el `server_id` de la solicitud sea el anunciado); `request_pairing` recibe esa expectativa;
  `SERVER_NOT_FOUND_MESSAGE` para `connection_error` de una terminal emparejada que no encuentra su servidor.
- `Info.plist` (nuevo, junto a `tauri.conf.json`; Tauri 2 lo combina con el del paquete y lo incrusta en las
  compilaciones de desarrollo): `NSLocalNetworkUsageDescription` («AD-EL POS busca y se conecta a la computadora
  principal del restaurante en la red local para tomar pedidos, cobrar y enviar las comandas a cocina.») y
  `NSBonjourServices` = `_adelpos._tcp`.
- `build.rs`, `permissions/diagnostics.toml` y `main.rs`: los dos comandos nuevos.

**Escritorio (React)**

- `pages/setup/FindServerPage.tsx`: el nombre de la terminal arriba; «Computadoras principales en la red» (busca al
  abrir y al volver de un código cancelado; «Buscar otra vez»; cada servidor es un botón grande «Pedir código a
  <nombre>»; sin resultados o sin permiso de red lo explica) y debajo «Escribir la dirección» (la de B, para redes
  que bloquean la búsqueda). `services/discovery.ts`.
- `navigation/ConnectionLostLayer.tsx` (nuevo) + `App.tsx`: con la sesión abierta y sin servidor, la capa «Sin
  conexión con el servidor» empieza debajo de la barra superior (que sigue a la vista con «Sin conexión»), el
  `main` queda `inert` (sin clics ni teclado, **montado** con lo capturado), dice cuándo es el próximo intento y
  ofrece «Reintentar»; el foco pasa a ese botón. En la captura la barra reducida es parte del contenido y queda
  cubierta. Al volver la conexión, `services/reconnection.ts` → `announceReconnection` pide de nuevo todas las
  consultas a la vista (`adel-server-change`, caja y `adel-connection-restored`, con el que `useRestaurantWorkspace`
  recupera el guardado incierto de la captura con su mismo `operation_id`). Los «Reintentar» propios de Mesas,
  Cocina y la barra de la captura ya no se pasan desde `App.tsx` (la capa los sustituye; los componentes los
  conservan para su uso aislado). El aviso persistente de la 9D se conserva.
- `hooks/useSession.ts`: sin conexión, la sesión se reintenta con espera creciente 1, 2, 4, 8 y después cada 10 s
  (`reconnectDelays`) y expone `retryAt`; en línea, cada 5 s como siempre.
- `styles.css`: bloque al final, solo tokens.

**Pruebas y puente**

- Go: `discovery_test.go` (nombres con acentos y largos, TXT, `Announcer` con un registrador simulado: cambio de
  nombre, publicación que termina sola, apagado y sistema sin implementación), `browse_test.go` (cliente DNS-SD de
  las pruebas con `dnsmessage`: consulta, respuesta tipo mDNSResponder con nombre con acentos, rechazo de otra
  consulta, paquetes cortados) y `announce_darwin_integration_test.go` (`integration`, macOS: **anuncio real y
  búsqueda acotados**; `TestHoldAnnouncementForLiveSearch` solo con `ADEL_DISCOVERY_HOLD_SERVER_ID`, para la prueba
  en vivo de Rust). `httpapi/clientversion_test.go` (por la red sin versión: 426; por la local: pasa).
  `administration/pairing_acceptance_test.go`: el ayudante y el SSE envían las cabeceras de cliente como Rust y una
  subprueba comprueba el 426 por la escucha de red real. `operations/stage9g_two_terminals_test.go` (nuevo, ver
  «Pruebas»).
- Rust: 8 pruebas nuevas en `discovery.rs` (incluida la de huella correcta e incorrecta con dos servidores TLS
  reales en 127.0.0.1) y una ignorada en vivo.
- Vitest: `pages/stage9gDiscovery.test.tsx` (4), `App.test.tsx` (capa «Sin conexión»), mocks de
  `stage9gPairing`/`stage9gModality` con `discovery_search`.
- `e2e/native-driver.ts`: `installDriver(page, { terminal })` (segunda terminal con su propia credencial),
  simulación de `discovery_search` (`setDiscovery('server'|'none'|'blocked')`, con el `server_id` real del Go
  temporal leído de `identity/identity.json` por `e2e/pairing.ts`) y de `pairing_start_found`.
  `e2e/workflow-stage9g-two-terminals.spec.ts` (nuevo, 2 pruebas). `workflow-stage9d-topbar-tables.spec.ts`, paso
  10: sin conexión en la captura comprueba la capa y su «Reintentar» (la barra reducida ya no tiene el suyo).

### Decisiones tomadas en la fase

1. **Anuncio con el respondedor mDNS del sistema, no uno propio.** Un respondedor propio en Go tendría que abrir un
   segundo socket en el puerto 5353: Go lo abre en `0.0.0.0:5353` (lo hace así aunque se pida el grupo
   multicast) con `SO_REUSEPORT`, y los datagramas **unicast** a ese puerto llegan a uno solo de los sockets (en BSD,
   según el orden interno del núcleo; no se comprobó en esta Mac), así que podría quitarle respuestas a mDNSResponder (AirPrint y el resto de Bonjour en la computadora
   principal). Con `dns-sd -R` no hay segundo socket y el sistema resuelve nombres repetidos, varias interfaces,
   IPv6, el cambio de IP de la propia computadora y la despedida; además el permiso «Red local» no aplica (anuncia
   mDNSResponder). Riesgos aceptados: `dns-sd` es una herramienta «para uso interactivo» (solo se usan sus
   argumentos; la salida solo va a la bitácora) y un proceso hijo que hay que vigilar (se relanza solo; si launchd
   detiene el servicio termina el grupo de procesos, así que no queda un anuncio huérfano; con `serve` de desarrollo
   un SIGKILL del servidor sí lo dejaría vivo hasta cerrarlo). Comprobado en esta Mac: mDNSResponder contesta la
   consulta de una sola vez por unicast con PTR, SRV, TXT, AAAA y A. **Windows (Fase W)**: el mismo `Announcer`
   con `announce_windows.go` sobre `DnsServiceRegister`/`DnsServiceDeRegister` de dnsapi.dll (Windows 10+), sin
   socket propio; hasta entonces el servidor de Windows no se anuncia y las terminales escriben la dirección.
2. **`X-Client-Version` obligatoria solo por la escucha de red** (D la dejó a B; B no la tomó): todo cliente que
   llega por la red es de la 9G o posterior (antes no había forma de conectarse por la red), así ninguna terminal ni
   Android se salta la versión mínima con solo omitir la cabecera; la ventana de la propia computadora, las
   herramientas y las pruebas locales no cambian. Respuesta: la de un cliente viejo (426, «Actualiza esta
   terminal»).
3. **El TXT no es una prueba de identidad**: la huella corta solo descarta pronto; la terminal siempre comprueba la
   huella completa en el intercambio TLS (`PinnedVerifier`), también al emparejarse con un servidor de la lista.
4. **Cambio de IP solo con error de conexión** y repetición única: la petición no llegó al servidor, así que
   repetirla no duplica nada (las escrituras llevan además su `operation_id`). Un tiempo de espera o un error de
   respuesta no disparan la búsqueda. La búsqueda bloquea esa petición ≈ 1.5–2.3 s una vez cada 10 s como máximo.
5. **La capa deja la barra superior usable** (navegación, usuario, pantalla completa: nada de eso escribe en el
   servidor) y cubre todo lo demás; en la captura cubre también la barra reducida. El aviso persistente de la 9D se
   conserva (dice el motivo que da Rust); la capa es el bloqueo.
6. Espera creciente de la sesión **hasta 10 s** (no 15 como el SSE): al volver la red la terminal se recupera en
   menos de 10 s sin tocar nada (las especificaciones de la 9D/9F esperan 15 s al estado «En línea»).
7. Nombres de instancia sin puntos («S.A.» → «SA»): `dnsmessage` y otros clientes rechazan una etiqueta con puntos.

### Pruebas ejecutadas (aisladas)

13/11/2026, en `wt/C` con `iso.sh` (clúster 55499, Go 8188, vista previa 1426, Rust en `cache/cargo-C`). Pantalla
del equipo desbloqueada (`CGSSessionScreenIsLocked` ausente).

| Suite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Resultado                                                                                                    |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `format` (Prettier, `gofmt`, `cargo fmt`; `docs/etapas/*.md` con el mismo md5 antes y después)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | aprobado (10 s)                                                                                              |
| `sync`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | aprobado                                                                                                     |
| `vet` (normal e `integration`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | aprobado                                                                                                     |
| `win` (`GOOS=windows` build, vet y vet `integration`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | aprobado                                                                                                     |
| Go unitarias con `-race` (`go test ./...`): `discovery` (nombres con acentos y largos, TXT, `Announcer`, cliente DNS-SD de las pruebas), `httpapi` (versión obligatoria por la red)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | aprobado (16 s)                                                                                              |
| Integración `./internal/discovery` (`TestAnnounceAndFindOnTheLocalNetwork`): anuncio real con Bonjour del sistema y nombre único «AD-EL POS prueba <hex> — Ñeli», encontrado por la consulta de una sola vez con su puerto, TXT y una IPv4 de la red local; al apagar se retira y no queda ningún `dns-sd`                                                                                                                                                                                                                                                                                                                                                                                                                                       | aprobado (5 s)                                                                                               |
| Integración `./internal/operations` (`TestStage9GTwoTerminals`, base propia): dos cajas con su terminal, usuario y caja: turnos propios con el evento en la otra y sin poder arquear el ajeno; mesa ocupada vista por la otra (SSE + `restaurant/state`) y `TABLE_HAS_ACCOUNT` con la cuenta; la misma cuenta: 409 `ACCOUNT_CONFLICT` «…otra terminal…», vista actualizada y dos confirmaciones simultáneas con un solo envío (3 unidades, no 6); preparación lista desde la otra (evento y estado) y revisión vieja rechazada; cobro que cierra la cuenta y libera la mesa en la otra, efectivo solo en su caja; arqueo que pausa solo su caja (la otra cobra; la pausada no) y tope de 100 comparado por caja (aviso solo en la que lo supera) | aprobado (13 s)                                                                                              |
| Integración `./internal/apicontract` (`TestContractV1`, 26 operaciones con las seis de emparejamiento)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | aprobado (7 s)                                                                                               |
| Integración `./internal/administration` (`TestStageTwoAcceptance` con `testPairingAndTerminals` y la subprueba nueva del 426 por la escucha de red real)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | aprobado (97 s)                                                                                              |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS`: el `host` en modo de pruebas con `--network-listen 127.0.0.1:8196`, sin anuncio)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | aprobado (42 s)                                                                                              |
| `cargo test --locked --offline` (8 nuevas: consulta, respuesta con acentos y un punto dentro del nombre, paquetes ajenos o cortados y puntero en ciclo, preguntas de seguimiento, selección por `server_id` y huella corta, **huella correcta e incorrecta con dos servidores TLS reales**, cambio de dirección que reescribe `connection.json` y rechaza una IP pública, lista para React sin direcciones)                                                                                                                                                                                                                                                                                                                                      | 75 aprobadas, 3 ignoradas (Keychain real y las dos en vivo)                                                  |
| Prueba en vivo Rust ↔ Go (`dtools-C/live-discovery-C.sh`): Go mantiene 12 s un anuncio «AD-EL POS prueba Rust <id> — Ñeli» y la búsqueda real de `discovery.rs` lo encuentra con su `server_id`, puerto 8202 y 2 IPv4; al terminar no queda ningún `dns-sd`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | aprobado                                                                                                     |
| Sondeo previo (Python, acotado a 3 s con un tipo de prueba): mDNSResponder contesta la consulta de una sola vez por unicast desde el puerto 5353 con el mismo identificador, PTR en respuestas y SRV, TXT, AAAA y A en adicionales (TTL 10)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | comprobado                                                                                                   |
| `tsc --noEmit`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | sin errores                                                                                                  |
| Vitest (`npm test`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | 92 archivos, 861 pruebas (nuevas: `stage9gDiscovery.test.tsx` ×4 y la capa «Sin conexión» en `App.test.tsx`) |
| Suite `app` (`tauri build --bundles app`, 97 s): `Contents/Info.plist` del `.app` con `NSLocalNetworkUsageDescription` (el texto en español) y `NSBonjourServices` = `["_adelpos._tcp"]`; `plutil -lint` correcto                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | aprobado                                                                                                     |
| Playwright Chromium: `restaurant`, `workflow-stage9d-colors`, `workflow-stage9d-topbar-tables`, `workflow-stage9f-kitchen-stations`, `workflow-stage9f-topbar-map`, `workflow-stage9g-modality`, `workflow-stage9g-terminals` ×2, `workflow-stage9g-two-terminals` ×2 y `workflow-zz-test-data`                                                                                                                                                                                                                                                                                                                                                                                                                                                  | 11 de 11 (3.2 min)                                                                                           |
| Playwright WebKit (las mismas)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | 11 de 11 (3.1 min)                                                                                           |

La especificación nueva también pasó sola en Chromium antes del ajuste final de la espera (2 de 2, 23 s).

### No verificado y lo que queda para otras fases

- **Fase W (otra computadora real)**, pasos sugeridos:
  1. Mac como servidor (`host` con la escucha `:8443`), Windows como terminal: en Windows «Buscar el servidor» debe
     listar «<establecimiento> — <computadora>» sin escribir nada (comprobar con `dns-sd -B _adelpos._tcp` en la
     Mac que el anuncio existe; si no aparece en Windows, revisar el firewall de Windows para UDP 5353 entrante en
     redes privadas). Emparejar eligiéndolo de la lista.
  2. **Cambio de IP real**: con la terminal emparejada, cambiar la IP de la Mac (renovar la concesión DHCP en
     Ajustes › Red › Detalles › TCP/IP › «Renovar concesión DHCP» o reservar otra IP en el módem; **parada**: es
     configuración de red) y comprobar que la terminal sigue sola (la primera operación tarda ≈ 2 s) y que
     `connection.json` de la terminal tiene la dirección nueva. Probar también con otro servidor AD-EL POS en la red
     con el mismo nombre: nunca debe aceptarse.
  3. **Apagar la red de verdad** (desconectar el cable o el Wi-Fi de la terminal): la capa «Sin conexión» aparece en
     ≤ 5 s, lo capturado se conserva, al volver se guarda solo; probar también apagar la Mac y encenderla.
  4. Windows como servidor: implementar `announce_windows.go` (`DnsServiceRegister`) y probar que una Mac lo
     encuentra; comprobar si el respondedor de Windows y la regla del firewall necesitan algo más.
- **Permiso «Red local» de macOS** en la aplicación instalada: al buscar por primera vez debe aparecer el aviso con
  el texto del Info.plist (verificado que las claves están en el `.app` compilado; con firma ad hoc puede volver a
  pedirse en cada compilación). Si se niega, la búsqueda falla con el aviso en español y la dirección escrita a mano
  tampoco conectará (el permiso cubre también las conexiones a la red local).
- **Exención del servicio**: el anuncio lo hace mDNSResponder, así que el permiso «Red local» no aplica al
  LaunchDaemon; la escucha 8443 del servicio sí recibe conexiones de la red (Fase G: comprobar con la instalación
  real).
- Solo IPv4 en la búsqueda de la terminal (las direcciones IPv6 del anuncio se ignoran) y solo por la interfaz de
  salida predeterminada del multicast (`std::net::UdpSocket` no elige interfaz): en una computadora con varias redes
  podría no encontrar el servidor de la otra red; queda la dirección escrita a mano.
- **Guía (Fase G)**: `docs/INSTALAR_EN_RED.md` debe recomendar asignar una IP fija a la computadora principal en el
  módem (§4.1), explicar el aviso «Red local» de macOS y qué hacer si la red bloquea la búsqueda.

### Para CLAUDE.md

- §3 Arquitectura: el servidor se anuncia en la red local (`internal/discovery`, `_adelpos._tcp`, TXT con
  `server_id`, `api`, `port` y `fp`) con el respondedor mDNS del sistema (macOS: `dns-sd -R` hijo del servidor;
  Windows: pendiente, `DnsServiceRegister`), solo con la escucha de red y nunca en modo de pruebas. La terminal
  busca con `discovery.rs` (consulta de una sola vez, sin crates) y, emparejada, vuelve a encontrar su servidor por
  `server_id` + huella si cambia de IP (`ServerLocator`: dirección vigente en `TerminalState`, que usan `api.rs`,
  `realtime.rs`, `export.rs` y `server_snapshot`).
- §6 Pruebas: `./internal/discovery` en la tupla (prueba acotada del anuncio real con nombre único; necesita macOS y
  red); `TestStage9GTwoTerminals` en `operations`; `installDriver(page, { terminal })` para una segunda terminal en
  otro contexto y `setDiscovery` en el puente; la búsqueda real de Rust se prueba a mano con
  `dtools-C/live-discovery-C.sh` (Go mantiene un anuncio 12 s). **Al nombrar datos de prueba, evitar prefijos que
  filtran otras especificaciones** (p. ej. «Caja 2»): la fase C usa «9G-C».
- §7 Seguridad: por la escucha de red `X-Client-Version` es obligatoria (426 sin ella); el anuncio mDNS nunca se usa
  para confiar (la huella completa sí); una terminal emparejada solo cambia de dirección hacia un servidor con la
  huella fijada y la guarda como última dirección.
- §11–12: la capa «Sin conexión con el servidor» (`navigation/ConnectionLostLayer.tsx`) bloquea con `inert` el
  `main` debajo de la barra superior y conserva lo capturado; al reconectar `announceReconnection` vuelve a pedir
  todo lo visible. Regla propuesta: **ninguna pantalla nueva pone su propio «Reintentar» de conexión**; la capa es
  el único lugar.
- §15: «Buscar el servidor» con la lista de computadoras encontradas y la dirección escrita a mano; reconexión con
  espera creciente (1, 2, 4, 8 y 10 s); `Info.plist` con `NSLocalNetworkUsageDescription` y `NSBonjourServices`.
- §19 Riesgos: `dns-sd` es una herramienta interactiva (formato no garantizado; solo se usan sus argumentos); un
  `serve` de desarrollo matado con SIGKILL deja su `dns-sd` vivo; la búsqueda es solo IPv4 y por la interfaz
  predeterminada; el permiso «Red local» de macOS es necesario para buscar y para conectarse.

**Fusión y verificación del coordinador** (13/11/2026): la rama `etapa-9g-c` salió de la fusión de A, D y B, antes de la
F. Se fusionó en `146ffc2` (conflictos solo de bloques agregados por ambas: trabajadores de `main.go` —con `gofmt` en
`65bb484`—, comandos de `build.rs`, `main.rs` y `diagnostics.toml`, tupla de integración, estilos y `API_CONTRACTS.md`).
Copia exacta de `65bb484`: Go unitarias con `-race`, integración de los 13 paquetes de la tupla (con `discovery`),
`cargo test` y Playwright (`restaurant`, `table-price-decrease`, `takeaway-quick-notes`, `workflow-stage9c-navigation`,
`workflow-stage9d-topbar-tables`, `workflow-stage9f-printing`, las seis `workflow-stage9g-*` y `workflow-zz-test-data`:
14 de 14 en Chromium y 14 de 14 en WebKit), además de formato, `sync`, `vet`, `GOOS=windows`, `tsc` y Vitest: todo
aprobado.

## Fase E · Impresión con varias terminales

Un agente, en su propio árbol de trabajo (`etapa-9g-e`, creada desde la fusión de A, D, B, F y C en `65bb484`), en
paralelo con las correcciones de la revisión de la fase B. 13/11/2026. Decisiones del coordinador D10 y D13. Sección
E de la `0023` (esquema 23, sin cambiar `config/product.json`). Pruebas en copias aisladas: clúster temporal 55499, Go
temporal 8190 (Playwright), vista previa 1428, Rust en `cache/cargo-E`. **Nunca se imprimió en una impresora real**:
el agente de Rust se probó con un ejecutor simulado de `lpstat`/`lpoptions`/`lp`, el de Playwright es un `lp`
simulado en el puente y la cola de las pruebas es `ADEL_E2E_SIN_IMPRESORA`, que no existe en ninguna computadora.

Commits: `6731013` (Go, migración y Rust con sus pruebas), `47e4d29` (interfaz, puente, Vitest y Playwright) y
`0e13097` (contrato, guía de cocina y el agente espera la credencial).

### Qué cambió

**Migración `0023`, sección «Fase E»** (al final, después de la F)

- `kitchen_printers.agent_device_id` → `devices` (FK con nombre) con `CHECK (agent_device_id IS NULL OR transport =
'agent')` e índice parcial: una impresora `agent` con terminal es la impresora USB de esa terminal; `destination` es
  el nombre de su cola en esa computadora. Las `agent` existentes (sin terminal, como «Destino controlado E2E») siguen
  válidas y sin imprimir.
- `devices.print_agent_seen_at` (última consulta del agente) y `devices.local_printers_reported_at`.
- `device_local_printers (device_id, queue)`: cola (patrón de Go), modelo (≤ 120), conexión (solo el esquema de
  `device-uri`, `^[a-z0-9+.-]{0,20}$`, nunca la dirección ni el número de serie) y `printer_type` de CUPS.
- `register_receipt_printers (register_id PK, printer_id NOT NULL)`: impresora de tickets propia de una caja (D10).
- Probada de 22 a 23 con una impresora de red y una `agent` sin terminal existentes (`checkStage9GPrintAgent` desde
  `TestStage9GAndroidUpgrade`: dos líneas agregadas a la prueba de D).

**Servidor Go**

- `kitchen/agent.go` (nuevo): `HandleAgent` con tres rutas solo para el agente:
  `PUT print-agent/printers` (reemplaza la lista de la terminal; audita `device.local_printers_reported` y publica
  `devices.printers.changed` solo si cambió), `POST print-agent/jobs/claim` (`claim_id`; la misma clave con la lease
  vigente devuelve el mismo intento; antes recoge los procesos vencidos de esa terminal; un trabajo por impresora a la
  vez con `pg_try_advisory_xact_lock(hashtextextended(impresora, 7820148307))` y «nadie en proceso»; mismos reintentos
  automáticos que el servidor, nunca el cajón; impresora o estación desactivada o fotografía inválida → intervención;
  entrega `{id, attempt_id, queue, title, data (base64), size, drawer, lease_until}`) y
  `POST print-agent/jobs/{id}/result` (`{attempt_id, outcome, code?, reference?, detail?}` con lista cerrada; repetido
  responde lo guardado; distinto, `409 PRINT_RESULT_CONFLICT`; lease vencida: `200 {recorded: false, reason:
"lease_expired"}`, trabajo incierto e historial `print.agent_late_result`). `AgentJobBytes` = los bytes de una
  impresora de red: pulso del cajón, `documents.EncodeESCPos` de la IR o `ESCPosBytes` del texto, con el final de
  papel de la impresora y las copias repetidas. `TerminalPrinters` (terminales con lo que informaron y sus impresoras
  de AD-EL POS) y `checkAgentPrinter` (al elegir terminal o cola: terminal existente y no revocada, cola informada y
  que sirve).
- `kitchen/worker.go`: el trabajador del servidor **nunca** toma un trabajo cuya fotografía es de una terminal; solo
  deja incierto el que su agente dejó en proceso con la lease vencida (`expireAgentLease`, «La terminal no informó el
  resultado a tiempo…»). El guardado del resultado pasa a `storeOutcome` (mismo código para el servidor y el agente:
  estados, espera creciente, respaldo tras tres errores confirmados, historial —con la terminal como autora en el
  agente— y eventos); el `result` del intento se combina con lo que ya tenía.
- `kitchen/print_problems.go`: problema `agent_offline` («La Caja 2 está desconectada»): trabajo `pending`,
  `processing` o `error` con reintentos pendientes de una terminal sin consultas en `AgentOfflineSeconds` (30 s) o
  revocada; `JobTerminalSQL` da el nombre de la terminal. Lo usan la cola, los documentos de la cuenta (`finance.go`,
  con `attempts` y `printer_snapshot` en su consulta lateral) y los de caja.
- `kitchen/cups_raw.go`: la regla de datos raw se separa en `rawQueueVerdict` (una sola regla) con dos textos:
  `EvaluateRawQueue` (cajón por CUPS del servidor, textos idénticos) y `EvaluateTerminalQueue` (impresoras de una
  terminal, en palabras del restaurante).
- `kitchen/printers.go`, `models.go`, `reads.go`, `handler.go`, `documents.go`, `commercial.go`: `Printer.AgentDeviceID`
  (y `TerminalName` solo en consultas); validación (cola con el patrón de Go, sin opción de corte CUPS ni marca
  ESC/POS); guardar con la terminal; `printerListQuery` con `terminal_name` en `kitchen/printers`, `kitchen/outputs` y
  la cola; `GET kitchen/terminal-printers` (`kitchen.printers.manage`); un trabajo para la impresora de una terminal
  queda `pending` (el de una `agent` sin terminal, en intervención como antes); `PrinterProblem` acepta la impresora
  de una terminal para tickets y `CompatibleDrawerPrinter` para el cajón.
- `administration/service.go` + `print_agent.go` (nuevo): `print-agent/*` después de `authenticateDevice` y antes de
  la sesión (el agente imprime aunque nadie haya iniciado sesión); auditoría con la terminal como autora.
  `devices.go`: `GET devices/{id}/configuration` llena `printers` y agrega `local_printers` (el hueco de la fase B).
- `payments`: `receiptPrinter` usa la impresora propia de la caja de la terminal que pide el documento y, sin ella, la
  general (precuentas, tickets, documentos de caja, reintentos, reimpresiones y el aviso inmediato de la 9F);
  `GET/PUT payments/settings` con `register_printers` (misma validación que la general, `usablePrinter`; auditado en
  `payments.settings_changed`); el cajón acepta la impresora de una terminal (`drawerConfig.agentDevice`,
  `DrawerPrinter.TerminalName`, compatible sin consultar CUPS del servidor); `DocumentRecord`/`CashDocumentRecord`
  con `terminal_name`. Mensaje de una `agent` sin terminal para el cajón: «…no tiene una terminal asignada, así que
  no puede entregar el comando ESC/POS de apertura…».
- `designer/samples.go`: la impresora de una terminal es compatible con formato y logotipo (`terminal_agent`).
- `maintenance`: firma «Impresora USB 9G-E» (`agent` + `ADEL_E2E_SIN_IMPRESORA`,
  `workflow-stage9g-print-agent.spec.ts`; la prueba de firmas admite `agent` con esa cola);
  `register_receipt_printers` es configuración que se desvincula («Impresora de tickets de «Caja X»: vuelve a la
  general»).

**Escritorio (Rust)**

- `print_agent.rs` (nuevo), arrancado en `main.rs` (`print_agent::start`, no arranca en Windows) y despertado por
  `realtime.rs` con cada mensaje del canal (`nudge`). Espera a que la terminal tenga credencial; informa sus
  impresoras al abrir y cada 5 min (`/usr/bin/lpstat -a` y `/usr/bin/lpoptions -p <cola>`, solo lecturas, 3 s cada
  una; sin impresoras, lista vacía; sin respuesta, no informa); consulta cada 3 s; entrega cada trabajo con
  `/usr/bin/lpstat -p <cola>` y `/usr/bin/lp -d <cola> -o raw -n 1 -t <título> <archivo>` (argumentos separados,
  `LC_ALL=C`, archivo `create_new` 0600 en la carpeta temporal que se borra, 12 s): referencia «cola-n» → aceptado;
  `lpstat -p` falla → `not_found`; no responde o `lp` no arranca → `unreachable`; `lp` empezó sin referencia, con
  error o sin terminar → incierto. Reclamo y resultado sin respuesta se repiten igual; recuerda los intentos ya
  entregados (128) para no imprimir dos veces el mismo. Sin crates nuevos (base64 y UUID propios; aleatorio de
  `rustls`/`ring`), sin comandos de Tauri nuevos (sin cambios en `build.rs` ni permisos). Credencial de la terminal sin
  sesión, `X-Client-Version` (obligatoria por la red) y relocalización del servidor como las demás peticiones.
- `api.rs`: operación `kitchen_terminal_printers`; prueba de que `print-agent/` no está en la lista de React.

**Escritorio (React)**

- `PrintersAdmin.tsx`: «Impresora USB de una terminal»: «Terminal donde está conectada» (Conectada/Desconectada) y
  «Impresora de esa terminal» (las que no sirven, deshabilitadas con el motivo; sin lista, «…todavía no informó sus
  impresoras»); sin terminal, «Nombre de la impresora en la terminal» y el aviso de que no imprime. La lista dice
  «Impresora USB de Caja 2». Texto de la página actualizado (ya no dice «todavía no se puede usar»).
- `services/kitchen.ts`: `printerLabel` («USB de caja · en Caja 2»), usado al elegir impresora en estaciones,
  respaldo, reenvío desde la cola, tickets por caja; `TerminalPrinterSet`/`TerminalQueue`.
- `DevicesPage.tsx`: «Impresoras de esta terminal» con sus impresoras y la sección «Impresoras de la computadora»
  (conectada o no, cuándo informó, cuál se puede usar y por qué no).
- `PaymentPoliciesForm.tsx`: «Impresora de tickets por caja» con «Impresora propia de <caja>» («Usar la impresora de
  tickets general» por omisión); `receiptPrinterOptions` acepta la de una terminal.
- `printProblems.ts`: `agent_offline` («La Caja 2 está desconectada.», «La terminal Mostrador está desconectada.» si el
  nombre no empieza por Caja/Terminal/Computadora) y su ayuda («Se imprimirá sola… no hace falta reintentar»);
  `PrintQueue.tsx`, `DocumentList.tsx` (también con el documento pendiente) y `CashDocumentActions.tsx` con la
  terminal. `cash.ts` («USB de Caja 2» en el cajón), `SampleDialog.tsx`, estilos al final de `styles.css` (tokens).

**Puente y pruebas de navegador**

- `native-driver.ts`: `printAgent.report`, `printAgent.run({outcome, code})` y `printAgent.calls` (agente simulado con
  la credencial de esa terminal, `lp` simulado que registra los argumentos de Rust).
- `e2e/workflow-stage9g-print-agent.spec.ts` (nueva). `kitchen-flow.ts`: el campo de la impresora sin terminal se
  llama «Nombre de la impresora en la terminal» (antes «Referencia del destino remoto»; los datos creados no cambian).

### Decisiones tomadas en la fase

1. **Una `agent` sin terminal sigue existiendo** (instalaciones anteriores, destinos de aceptación de las pruebas,
   Windows de la 9E): sus trabajos quedan en intervención con «Esta impresora todavía no puede usarse para imprimir.»
   como en la 9F. Por eso el CHECK es «solo una `agent` tiene terminal», no «toda `agent` tiene terminal».
2. **Rutas del agente sin sesión** (`print-agent/*`, solo credencial de la terminal): el agente debe imprimir aunque
   la caja esté en la pantalla de acceso (D13: «mientras la aplicación está abierta»). Cada terminal solo ve lo suyo.
3. **La fotografía decide qué agente reclama** (`printer_snapshot.agent_device_id`), como el trabajador del servidor
   usa la fotografía; «Reintentar» toma la configuración vigente (revisión 9F, M2).
4. **«Desconectada» = 30 s sin consultas del agente** (`print_agent_seen_at`), no los 2 min de «En línea» de
   Terminales (cualquier petición): el agente consulta cada 3 s, y una terminal que hace peticiones sin agente (una
   versión anterior, Android) no imprime. Terminales muestra ambos («En línea» en la tabla; «Conectada/Desconectada»
   en «Impresoras de la computadora»).
5. **Resultado con la lease vencida = 200 `recorded: false`** (no 409): así se confirma en la base el paso a
   «incierto» y el informe tardío en el historial (un error de cocina no puede pedir confirmar la transacción).
6. **Copias en los bytes** y `lp -n 1`: el agente envía un solo trabajo con todas las copias (como ESC/POS por red).
7. **Impresora de tickets de la caja de la terminal que pide el documento** (no la de la caja del turno): «el papel
   sale donde estás»; una terminal sin caja o una caja sin impresora propia usa la general.
8. **La cola se elige de lo que informó la terminal y se comprueba al elegirla o cambiarla**, no al editar otros datos
   (como la impresora de tickets en la revisión B3 de la 9F); Go conserva la regla y la interfaz solo la presenta.
9. **Sin OpenAPI**: las rutas del agente son exclusivas del escritorio y las de administración son de su interfaz; la
   aplicación Android no imprime por agente (sus documentos salen en la impresora que elige Go).
10. **Etiquetas de la caja propia «Impresora propia de <caja>»**: «Impresora de tickets de …» chocaba con el
    selector `/Impresora de tickets/` de `payments-flow.ts` y `workflow-stage9f-printing`.

### Pruebas ejecutadas (aisladas)

13/11/2026 en `wt/E` con `iso.sh` (Go 8190, vista previa 1428, Rust en `cache/cargo-E`); pantalla desbloqueada
(`CGSSessionScreenIsLocked` ausente).

| Suite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Resultado                                                                                                               |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `format` (Prettier, gofmt, cargo fmt; `docs/etapas/*.md` con el mismo md5 antes y después)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | aprobado (11 s)                                                                                                         |
| `sync`, `vet` (normal e `integration`), `win` (`GOOS=windows` build y vet)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | aprobados                                                                                                               |
| Go unitarias con `-race` (`go test ./...`; nuevas en `kitchen/agent_test.go`: validación de la impresora de una terminal, bytes del agente iguales a los de red y del pulso, resultados con la lista cerrada, regla de colas sin términos técnicos, problema `agent_offline`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | aprobado (21 s)                                                                                                         |
| Integración `./internal/operations` `TestStage9GPrintAgent` (nueva): informe de impresoras solo propio, sin credencial 401, lista inválida 400, auditoría solo al cambiar; impresora de terminal elegida solo de lo informado (ipp y no informada, 400); terminal desconectada con `agent_offline` y nombre; el servidor no toma el trabajo; la otra terminal no lo reclama ni lo confirma (404); reclamo con bytes ESC/POS de Go, reclamo repetido = mismo intento, un trabajo por impresora; aceptado, repetido y en conflicto (409); error confirmado con reintento a su turno; incierto que nadie reenvía; resultado con lease vencida y lease recogida por el servidor; desconectada y de vuelta; cajón por agente con el pulso exacto y sin reintento del error; impresora de tickets por caja (Sur → USB, Norte → general, agente sin terminal 422, auditoría, documento con `agent_offline` y terminal) | aprobado (16 s)                                                                                                         |
| Integración `./internal/operations` completa (con el ajuste del mensaje del cajón; la primera corrida falló en `C37` porque el mensaje nuevo no decía «ESC/POS»)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | aprobado (321 s)                                                                                                        |
| Integración `./internal/database` (`TestStage9GAndroidUpgrade` con la sección E), `./internal/administration`, `./internal/payments`, `./internal/designer`, `./internal/apicontract`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | aprobados (10, 117, 3, 23 y 9 s)                                                                                        |
| `cargo test --locked --offline` (8 nuevas en `print_agent.rs`: lectura de `lpstat -a` traducido y de `lpoptions`, argumentos separados y sistema vacío, `lp -o raw` sin intérprete y archivo borrado, reglas de resultado y colas peligrosas que nunca llegan a `lp`, base64/UUID/referencia, reintentos sin imprimir dos veces el mismo intento, trabajo dañado, informe como lo espera Go; y la ruta nueva en `api.rs`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | 88 aprobadas, 3 ignoradas                                                                                               |
| `tsc --noEmit`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | sin errores                                                                                                             |
| Vitest (`npm test`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | 94 archivos, 880 pruebas (nuevas: `pages/stage9gPrintAgent.test.tsx` ×7; `printersWindows.test.tsx` con el texto nuevo) |
| Playwright Chromium: `restaurant`, `workflow-stage9b-designer`, `workflow-stage9f-kitchen-stations`, `workflow-stage9f-printing`, `workflow-stage9g-terminals` ×2, `workflow-stage9g-print-agent`, `workflow-zz-test-data`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | 8 de 8 (2.8 min)                                                                                                        |
| Playwright WebKit (las mismas)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | 8 de 8 (2.8 min)                                                                                                        |
| `workflow-stage9g-print-agent` repetida tras los últimos ajustes                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Chromium 1/1, WebKit 1/1                                                                                                |

No se ejecutó la suite `app` (compilación `release` de Tauri): no hay comandos ni permisos nuevos y `cargo build`
compila el agente sin advertencias. No hay `rustup` con el destino de Windows en esta Mac: la compilación de
`print_agent.rs` para Windows (solo `cfg!`/`#[cfg]` triviales) queda para la Fase W.

### No verificado y lo que queda

- **Hardware (propietario)**: papel, logotipo, corte y cajón reales por una impresora USB conectada a otra Mac
  (agente real con `lp -o raw`); qué hace una impresora real con el pulso por `lp -o raw` desde la terminal; un trabajo
  grande con logotipo (los bytes viajan en base64, límite 4 MB).
- **Fase W (Windows)**: el agente no arranca en Windows. Punto de integración: `print_agent.rs` — `local_printers`
  (EnumPrinters con su puerto y la regla de `kitchen/windows_queue.go`: USB, COM, LPT, TCP/IP estándar; nunca
  virtuales, de otro equipo ni WSD) y `deliver` (StartDocPrinter tipo `RAW` + WritePrinter, con los mismos
  resultados: no se abrió → error, falló después de abrir → incierto); quitar el `cfg!(target_os = "windows")` de
  `start`. Probar Mac servidor + Windows terminal con su USB y al revés (§8.2, «a su USB si la hay»).
- **Carga (Fase W)**: cada terminal abierta consulta cada 3 s (una transacción con dos `UPDATE` de `devices`);
  medir con varias terminales junto al SSE.
- **Fase G**: guía `docs/INSTALAR_EN_RED.md` («Impresora USB en una caja»: conectarla e instalarla en esa
  computadora con su controlador, abrir AD-EL POS, registrarla en Cocina › Impresoras eligiendo la terminal; si la
  caja está apagada, lo pendiente sale al encenderla); CLAUDE.md §15 y §19.

### Para la fusión (coordinador)

- `0023`: la sección E va al final (después de la F); no toca D, B ni F.
- Zonas compartidas (cambios acotados): `administration/service.go` (bloque `print-agent/` después de `auth/login`),
  `administration/devices.go` (`deviceConfiguration`), `api.rs` (una operación y una prueba), `main.rs` (`mod
print_agent` y `print_agent::start`), `realtime.rs` (una línea), `native-driver.ts` (`randomUUID` y el bloque
  `printAgent` antes del `return`), `DevicesPage.tsx` (panel de configuración), `styles.css` (al final),
  `API_CONTRACTS.md` (al final), `database/stage9g_android_upgrade_test.go` (dos llamadas).

### Para CLAUDE.md

- §3 Arquitectura: agente de impresión de la terminal (`print_agent.rs`): impresoras USB de otra computadora con el
  transporte `agent` + `agent_device_id`; rutas `print-agent/*` solo con la credencial de la terminal (sin sesión);
  la cola durable de siempre con su lease; el trabajador de Go nunca envía un trabajo de una terminal. Lock
  7820148307 también en el reclamo del agente (transaccional, misma finalidad: un trabajo por impresora).
- §6 Pruebas: `TestStage9GPrintAgent` (`operations`); `printAgent.report/run/calls` del puente; la impresora de
  prueba «Impresora USB 9G-E» (`agent` + `ADEL_E2E_SIN_IMPRESORA`) con su firma; el agente de Rust solo con
  `CommandRunner` simulado (nunca `lp` real).
- §7 Seguridad: el agente ejecuta solo `/usr/bin/lpstat`, `/usr/bin/lpoptions` y `/usr/bin/lp` con argumentos
  separados y la cola validada con el patrón de Go; la conexión informada es solo el esquema de `device-uri`.
- §15 «Cocina»: impresora USB de una terminal (D13), «La Caja 2 está desconectada» (`agent_offline`, 30 s sin
  consultas del agente), impresora de tickets por caja con respaldo en la general (D10), Terminales con las impresoras
  de la computadora y el nombre de la terminal al asignar impresoras.
- Reglas permanentes propuestas: toda lista para elegir impresora usa `printerLabel` (dónde sale el papel); una
  impresora de terminal solo se elige de lo que esa terminal informó; ningún resultado del agente se acepta con la
  lease vencida.
- §19 Riesgos: el agente no existe todavía en Windows (Fase W); «Desconectada» (30 s del agente) y «En línea» (2 min,
  cualquier petición) son señales distintas; la carga de las consultas cada 3 s está sin medir.

**Fusión y verificación del coordinador** (13/11/2026): la rama `etapa-9g-e` salió de `65bb484` (antes de las
correcciones de B y F); se fusionó en `39b8b0c` con un solo conflicto de estilos agregados por ambas ramas (la sección E
de la 0023 quedó al final, después de la F, sin conflicto). Copia exacta de `39b8b0c`: formato, `sync`, `vet`,
`GOOS=windows`, `tsc`, Vitest, Go unitarias con `-race`, integración de los 13 paquetes (`operations` 329 s),
`cargo test` (89 aprobadas, 3 ignoradas) y Playwright (`restaurant`, `workflow-stage9b-designer`,
`workflow-stage9f-kitchen-stations`, `workflow-stage9f-printing`, las siete `workflow-stage9g-*` y
`workflow-zz-test-data`: 13 de 13 en Chromium y 13 de 13 en WebKit): todo aprobado.

## Revisión independiente (emparejamiento y cifrado, idempotencia y pedidos sin conexión, respaldos)

Un solo agente de solo lectura, en tres partes, sobre copias exactas; las correcciones las hicieron los agentes de cada
fase en ramas nuevas y el coordinador las fusionó y verificó.

### Parte 1 · Emparejamiento y cifrado (Fase B), sobre `b84415c`

Un agente de solo lectura, con una prueba desechable propia contra el clúster aislado (fuera del repositorio).

| Gravedad | Hallazgo                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Estado                                                           |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| Alto     | **Un intermediario podía «moler» solicitudes**: abrir solicitudes en el servidor eligiendo su número comprometido, calcular sin conexión el código que esperaría el servidor y cancelar las que no coincidían con el código fijo de la terminal interceptada, hasta igualarlo. El límite por origen contaba la IP exacta (la escucha acepta cualquier `fe80::/10` o ULA) y el cupo de 20 abiertas no contaba canceladas ni rechazadas: 3 000 de 3 000 creaciones y cancelaciones aceptadas en 3,3 s, ≈1–5 % de éxito por emparejamiento en lugar de 1 en 10⁶ (`pairing.go`, `lanlistener.go`). | confirmado (el ritmo y el desvío); el ataque completo, plausible |
| Medio    | **Llenar la base desde la red sin credencial**: cada creación y cancelación dejaba una fila (7 días) y dos movimientos de auditoría permanentes; el conteo recorría la tabla completa y el borrado de antiguas no tenía índice, todo bajo `LOCK TABLE` dentro de la transacción de 10 s.                                                                                                                                                                                                                                                                                                       | confirmado (crecimiento y recorrido); llenar el disco, plausible |
| Bajo     | Consultas de estado simultáneas (`setInterval` sin esperar y `pairing_status` sin serializar): la terminal podía decir «pide otro código» ya emparejada y dejar un dispositivo huérfano.                                                                                                                                                                                                                                                                                                                                                                                                       | plausible                                                        |
| Bajo     | Una identidad incompleta (corte entre escribir la llave e `identity.json`) impedía arrancar todo el servidor, también la venta local.                                                                                                                                                                                                                                                                                                                                                                                                                                                          | plausible                                                        |
| Bajo     | El filtro por ULA aceptaba nodos de VPN superpuestas (Tailscale `fd7a:115c:a1e0::/48`, ZeroTier) fuera del restaurante.                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | confirmado por lectura; impacto plausible                        |

Revisado sin hallazgos: derivación idéntica del código en Go, Rust y el puente; orden del compromiso; comparaciones en
tiempo constante; vencimiento e intentos bajo `FOR UPDATE`; rutas locales (`loopback && !FromNetwork`, marca puesta por la
propia escucha de red); filtro de orígenes (IPv4 mapeada, CGNAT, multidifusión, 6to4); entrega única de la credencial
(token de 256 bits, solo SHA-256 en la base, nunca llega a React); verificador `rustls` (huella SPKI, firma TLS 1.2 y 1.3,
sin reanudación, proxy ni redirecciones, en todos los clientes de Rust); revocación (peticiones y SSE); secretos fuera de
React, auditoría y bitácoras; restricciones de la sección B; apertura de la escucha de red solo con modalidad con
servidor.

### Correcciones de la parte 1 (Fase B)

Un agente, en el árbol nuevo `$SCR/wt/BF` (rama `etapa-9g-bfix`, desde la punta de `etapa-9g` en `1ba4833`, con A, D,
B y F ya fusionadas). 13–14/11/2026. La 0023 aún no se aplicó a ninguna instalación: se editó su sección B. Mismo
aislamiento que la fase B (clúster 55499, Go temporal 8186, vista previa 1424, Rust en `cache/cargo-B`, escucha HTTPS
de prueba en `127.0.0.1:8196`/`8197` y puertos efímeros); nada de la instalación real ni del Keychain real.

| #         | Hallazgo (verificado en el código)                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Corrección                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Prueba                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1 · ALTO  | **Intermediario que muele solicitudes.** Cierto: una vez que T revela nT, el código de T queda fijo y lo conoce M; M podía abrir solicitudes en S sin límite real (el límite por origen contaba la IP exacta y el enlace local/ULA da direcciones ilimitadas; el cupo de 20 no contaba canceladas ni rechazadas), calcular sin conexión el código de cada una y cancelar las que no coincidían. El comentario de `pairing.go` y el avance afirmaban «1 en 10⁶ por solicitud», engañoso. | **Tope global** de 30 solicitudes **creadas** cada 10 minutos, sin importar estado ni origen (canceladas y rechazadas siguen contando), `429 PAIRING_LIMIT_REACHED` con un mensaje claro para la terminal y un solo movimiento `device.pairing_limit_reached` por ventana. Límite por **grupo de origen** (`SourceGroup`: IPv4 exacta, /64 de IPv6, todo el enlace local IPv4 e IPv6 como un grupo; columna `source_group`). Terminales muestra en la computadora principal cuántas solicitudes se cancelaron, rechazaron o vencieron sin aprobarse en la ventana (indicio de un intento) y si se alcanzó el tope. Comentario de `pairing.go` y texto del avance corregidos: la probabilidad queda acotada por el tope (≤ 30 intentos por cada 10 minutos que tarde el administrador: ≤ 3×10⁻⁵ por ventana) y cada intento queda a la vista. | Integración `stage9g_tope_global_contra_un_intermediario_que_muele_solicitudes`: 12 crear+cancelar desde `fe80::` distintos → 5 aceptadas; 7 desde un mismo /64 → 5; 60 desde IPv4 y /64 distintos → exactamente 30 aceptadas en total y 40 `PAIRING_LIMIT_REACHED`; una terminal legítima también espera; un solo movimiento del tope y ninguno de las cancelaciones; Terminales informa 30 abandonadas y el tope; pasada la ventana se vuelve a pedir código. Unitaria `TestSourceGroups`; Vitest de los avisos. |
| 2 · MEDIO | **Llenar la base desde la red.** Cierto: cada crear+cancelar dejaba fila y dos movimientos permanentes; el conteo recorría toda la tabla (dos `FILTER` sin `WHERE`) y el `DELETE` por `created_at` no tenía índice, todo con la tabla bloqueada dentro de la transacción de 10 s.                                                                                                                                                                                                       | El tope global acota las filas. Conteos con `WHERE created_at > now() - ventana` sobre los índices nuevos `(created_at)` y `(source_group, created_at)`. Sin `DELETE` en la petición: trabajador `PairingCleaner` (al arrancar y cada hora) que borra por lotes de 500 con su propia sentencia (`CleanPairingRequests`). La creación ya no se audita; `device.pairing_requested` se anota al **revelar** (cuando la solicitud puede aprobarse) y la cancelación solo si ya estaba revelada.                                                                                                                                                                                                                                                                                                                                                  | Integración `stage9g_conteos_con_indices_y_limpieza_por_lotes`: con 20 000 filas antiguas, `EXPLAIN` de los dos conteos usa índice y sin `Seq Scan`; crear una solicitud tarda < 2 s; la limpieza borra las 20 000 y conserva la reciente. Aserciones de auditoría al revelar y en el compromiso falso.                                                                                                                                                                                                            |
| 3 · BAJO  | **Consultas de estado simultáneas.** Cierto: `setInterval` en React sin esperar y `pairing_status` sin serializar; una consulta lenta (diálogo del Llavero) hacía que la siguiente recibiera `claimed` y la pantalla dijera «pide otro código».                                                                                                                                                                                                                                         | Rust: una sola consulta en curso (`pairing_busy`, turno que se libera también con error); otra simultánea responde «en espera» sin llamar al servidor. La credencial reclamada se conserva en la sesión antes de guardarla: si el almacén seguro falla, la siguiente consulta reintenta guardarla sin reclamar otra vez (no queda un dispositivo huérfano). React: consultas encadenadas con `setTimeout` al terminar cada una; un error no detiene la espera.                                                                                                                                                                                                                                                                                                                                                                               | Rust `only_one_status_query_runs_at_a_time` (en curso → «en espera» sin red; el turno se libera con error). Vitest `encadena las consultas…`: una consulta lenta no se solapa con otras en 4 intervalos, un error de guardado se muestra y la siguiente empareja.                                                                                                                                                                                                                                                  |
| 4 · BAJO  | **Identidad incompleta impedía arrancar.** Cierto: un corte entre la llave e `identity.json` dejaba la identidad incompleta para siempre y `serveHTTP` terminaba con error (launchd en bucle, sin vender).                                                                                                                                                                                                                                                                              | Creación **atómica**: los tres archivos en una carpeta temporal junto a la definitiva, `fsync` y `rename` de la carpeta; las temporales de un intento interrumpido se descartan; una carpeta vacía se acepta y una con otros archivos no se toca. Si aun así la identidad no carga (archivos cambiados a mano), el servidor **sigue atendiendo la escucha local**: sin escucha de red, `pairing/*` responde `503 PAIRING_UNAVAILABLE` («…su identidad necesita revisión de soporte»), `server_identity_unavailable` en la bitácora y aviso en Terminales (`pairing_available: false`).                                                                                                                                                                                                                                                       | Unitaria `TestIdentityCreationIsAtomic`. Integración del `host` real: con `identity.json` apartado el servicio queda listo, la bitácora lo explica (`network_listen_disabled`), el 8196 no se abre y el emparejamiento responde 503 con el mensaje; después se repone. Vitest del aviso.                                                                                                                                                                                                                           |
| 5 · BAJO  | **VPN con ULA.** Cierto: se aceptaba cualquier ULA/privada/enlace local, también la de un túnel (Tailscale `fd7a:115c:a1e0::/48`, ZeroTier).                                                                                                                                                                                                                                                                                                                                            | Solo loopback u orígenes de clase aceptable **dentro de la subred de una interfaz activa que no sea punto a punto** (`net.FlagPointToPoint`), evaluado al aceptar con la lista de interfaces releída cada 30 s y, si un origen no coincide, como mucho cada 2 s (DHCP). Riesgo que queda (documentado en `lanlistener.go`, el avance y `API_CONTRACTS.md`): una VPN que se presenta como interfaz normal con una subred privada.                                                                                                                                                                                                                                                                                                                                                                                                             | Unitarias con interfaces simuladas: red de otra LAN, red y ULA de un túnel punto a punto, Internet y CGNAT rechazados; misma red, enlace local y ULA de la Wi-Fi aceptados; relectura a los 2 s/30 s; error al leer interfaces → solo loopback; `SystemNetworks` sin loopback.                                                                                                                                                                                                                                     |
| extra     | Al probar el punto 1 apareció un defecto: el origen de una terminal de **enlace local IPv6** llega con zona (`fe80::1%en0`) y `net.ParseIP` la rechazaba (`PAIRING_SOURCE_UNKNOWN`): una terminal así no podía emparejarse.                                                                                                                                                                                                                                                             | `http.go` quita la zona antes de leer la IP de origen.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Cubierto por la prueba del tope (orígenes `[fe80::x%en0]`).                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

Además: el plazo de `TestStageTwoAcceptance` pasa de 90 a 180 s (la aceptación ahora inserta miles de filas) y la
prueba del SSE usa su propio contexto.

**Archivos**: `backend/internal/administration/pairing.go` (comentario del protocolo, límites, `SourceGroup`,
`pairingLimitReached`, `pairingUnavailable`, auditoría al revelar, contadores de la lista), `pairing_cleanup.go`
(nuevo), `http.go` (zona IPv6), `service.go` (`IdentityProblem`), `internal/lanlistener/lanlistener.go` (`peerFilter`,
`SystemNetworks`; `Filter` recibe las interfaces), `internal/localsetup/identity.go` (creación atómica),
`cmd/ad-el-pos-server/main.go` (identidad tolerante y trabajador de limpieza), sección B de la `0023` (`source_group` e
índices), `desktop/src-tauri/src/pairing.rs` y `main.rs` (`pairing_busy`, credencial reclamada conservada,
`startup_tests::test_state`), `FindServerPage.tsx` (consultas encadenadas), `DevicesPage.tsx`, `models.ts` y
`styles.css` (avisos), pruebas en `pairing_acceptance_test.go`, `pairing_test.go`, `lanlistener_test.go`,
`identity_test.go`, `host_unix_*integration_test.go`, `stage9gPairing.test.tsx`; `API_CONTRACTS.md` y el avance.

#### Pruebas ejecutadas (aisladas)

| Suite                                                                                                          | Resultado                                                                                                              |
| -------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `format` (`docs/etapas` intacto por md5), `sync`, `vet`, `win`                                                 | aprobados                                                                                                              |
| Go unitarias `-race` (`go test ./...`)                                                                         | aprobado (16 s)                                                                                                        |
| Integración `./internal/administration` (`TestStageTwoAcceptance` con las dos pruebas nuevas)                  | aprobado (116 s)                                                                                                       |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS` con la identidad dañada)                        | aprobado (46 s)                                                                                                        |
| `cargo test --locked --offline`                                                                                | 73 aprobadas, 2 ignoradas                                                                                              |
| Prueba en vivo Rust → Go (`dtools/live-pairing-BF.sh`, escucha de red real con el filtro nuevo desde loopback) | aprobada: misma huella que `openssl`, HTTP 400, TLS 1.1 rechazado, código de Rust igual al derivado de los datos de Go |
| `tsc`                                                                                                          | sin errores                                                                                                            |
| Vitest                                                                                                         | 92 archivos, 870 pruebas                                                                                               |
| Playwright `restaurant`, `workflow-stage9g-terminals` (2), `workflow-zz-test-data`                             | Chromium 4/4 (1.7 min), WebKit 4/4 (1.6 min, pantalla desbloqueada)                                                    |

#### Lo que queda

- Fase W/G: el filtro por subred se probó con interfaces simuladas y desde loopback; con otra computadora real falta
  comprobar una terminal en la Wi-Fi (aceptada) y otra por una VPN de túnel (rechazada).
- Con el tope global, un atacante en la LAN puede impedir emparejar durante 10 minutos (riesgo aceptado y visible en
  Terminales y en el Registro de movimientos).

#### Para CLAUDE.md

- §7: el emparejamiento tiene un tope global de 30 solicitudes cada 10 minutos (contando canceladas y rechazadas) y 5
  por grupo de origen (IPv4, /64 de IPv6, enlace local); es lo que acota a un intermediario, no el compromiso solo.
  La escucha de red acepta solo orígenes dentro de la subred de una interfaz local que no sea punto a punto.
- §3: sin identidad cargable el servidor sigue atendiendo la escucha local sin red ni emparejamiento (503); la identidad
  se crea con una carpeta temporal renombrada; `PairingCleaner` borra por lotes las solicitudes de más de 7 días.
- Regla permanente propuesta: toda tabla que se llena desde rutas sin credencial lleva un tope global por ventana,
  conteos por índice y limpieza fuera de la petición.

**Fusión y verificación del coordinador**: la rama `etapa-9g-bfix` salió de `1ba4833` (sin la Fase C). Se fusionó en
`6e92f7a` (conflictos: limpieza de solicitudes junto al anuncio en la red en `main.go`, filtro por subred junto a la marca
de la escucha de red en `lanlistener.go`, estado del emparejamiento junto a la búsqueda en `main.rs` y estilos). Copia
exacta de `6e92f7a`: formato, `sync`, `vet`, `GOOS=windows`, `tsc`, Vitest, Go unitarias con `-race`, integración de los
13 paquetes, `cargo test` (81 aprobadas, 3 ignoradas) y Playwright 14 de 14 en Chromium y en WebKit: todo aprobado.

### Partes 2 y 3 · Idempotencia y pedidos sin conexión (Fase D) y respaldos programados (Fase F), sobre `b84415c`

El mismo agente de solo lectura, con una prueba desechable propia de `backupschedule` contra el clúster aislado.

| Gravedad | Fase | Hallazgo                                                                                                                                                                                                                                                                                            | Estado                                  |
| -------- | ---- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| Medio    | D    | «Solo cambios desde X» perdía los productos archivados cuando cambiaba una lista de precios o la tasa general de IVA (la rama «todos los productos» del delta conservaba el filtro `NOT p.archived`; `catalog/sync.go`).                                                                            | confirmado por lectura                  |
| Bajo     | D    | Una base restaurada reutilizaba números de versión del catálogo (la secuencia vuelve con el respaldo): un cliente al día podía no recibir cambios nuevos.                                                                                                                                           | plausible                               |
| Bajo     | D    | El borrador aceptaba productos agotados (la confirmación sí los rechazaba; ya pasaba con «Pausar»).                                                                                                                                                                                                 | confirmado por lectura                  |
| Bajo     | D    | El reenvío confirmado de un pedido sin conexión recibía `IDEMPOTENCY_CONFLICT` sin el resultado si antes había entrado una repetición tardía del original.                                                                                                                                          | plausible                               |
| Bajo     | D    | «Agotado» de un componente o complemento no se reflejaba en la descarga de los combos y grupos que lo contienen (solo presentación: Go rechaza al recibir).                                                                                                                                         | confirmado por lectura                  |
| Medio    | F    | La retención y la limpieza de `.partial` solo miraban el patrón de nombre (la base se llama igual en todas las instalaciones) y ordenaban por nombre: en una carpeta compartida (USB, NAS, `serve` y servicio) borraban respaldos ajenos, su `.partial` en curso y hasta el respaldo recién creado. | confirmado (reproducido)                |
| Medio    | F    | En el servicio de macOS el respaldo se creaba con `pg_dump --file` (0644 por la umask de launchd, sin `O_EXCL`, siguiendo enlaces) y solo pasaba a 0600 al terminar.                                                                                                                                | confirmado por lectura                  |
| Medio    | F    | Sin aviso si el respaldo automático deja de intentarse: una ejecución fechada en el futuro (reloj adelantado y corregido) satisfacía todos los días siguientes; un fallo nuevo quedaba detrás de un éxito futuro; una fila `running` huérfana.                                                      | confirmado (reloj); lo demás, plausible |
| Bajo     | F    | La validación de la carpeta no revisaba las antecesoras (subcarpetas de los datos de PostgreSQL o del servicio) ni resolvía enlaces simbólicos.                                                                                                                                                     | plausible                               |

Revisado sin hallazgos (D): la clave se busca bajo el lock exclusivo antes de cualquier efecto y es la llave primaria de
`restaurant_operations` (un uso simultáneo entre módulos aborta con 23505); huella con método, ruta y cuerpo canónico;
el pedido sin conexión valida y acepta en una transacción con 7820148305 exclusivo y 7820148304 compartido, siempre con
precios del servidor, y la confirmación exige la versión vigente y cubrir cada cambio; versión del catálogo monótona en
orden de confirmación; orden de locks consistente; «Agotado» cumplido en vista previa, confirmación, edición y pedido sin
conexión, con vencimiento por el corte; `restaurant/state` con los mismos permisos de las listas; el contrato falla ante
rupturas. (F): programación con zona, corte y cambio de horario; lock 7820148309 en conexión propia que siempre se
suelta; un respaldo no verificado no cuenta ni dispara la retención; la carpeta solo se elige desde la computadora
principal y Go la valida; la contraseña solo en `PGPASSWORD`; aviso persistente; «Respaldar ahora» idempotente.

Pendiente de la computadora real: un `LaunchDaemon` sin permiso de privacidad para «Volúmenes extraíbles» puede recibir
`EPERM` al escribir en una USB (la prueba de escritura la rechazaría como «sin permiso»): comprobar en la Fase G.

### Correcciones de la parte 2 (Fase D)

Un agente (13/11/2026), en el árbol `$SCR/wt/DF`, rama `etapa-9g-dfix` desde la punta de `etapa-9g-merge` (`dc638d1`).
Commits `e9d35fc` (correcciones) y `9eb8a49` (especificación de Playwright). Se editó solo la sección D de la 0023 (no aplicada aún en ninguna instalación); las secciones B y F
quedan intactas. Cada hallazgo se verificó en el código antes de corregirlo; los cinco se confirmaron.

#### Hallazgo → corrección → prueba

1. **MEDIO · «Solo cambios desde X» perdía productos archivados** (`catalog/sync.go`). Confirmado: con `price_list` o
   `tax` en la bitácora, la rama «todos los productos» filtraba `NOT p.archived` y un producto archivado después de X
   no llegaba ni con `archived` ni en `deleted`.
   **Corrección**: el plan de la descarga (`catalog/sync_dependents.go`, `syncPlan.products`) entrega en esa rama los
   productos vigentes **más** los archivados que cambiaron después de X.
   **Prueba** (`catalog/stage9g_test.go`, subprueba de revisión): se archiva la hamburguesa y después se guarda la lista
   predeterminada → llega con `archived: true` junto con todos los vigentes. En el otro orden, primero se cambia el IVA
   general (8 %) y después se archiva el combo → llega archivado, con la tasa nueva en los vigentes. Un archivado que
   no cambió después de X no vuelve a llegar.
2. **BAJO · Una base restaurada reutilizaba versiones** (`catalog_version_seq`). Confirmado: tras restaurar, los
   números que siguieron al respaldo se repiten con otro contenido.
   **Corrección**: **época del catálogo**. La 0023 (sección D) crea `app.catalog_epoch` (una fila con un UUID) y
   `catalog.Epoch` la combina con el OID de la base y el `system_identifier` del clúster
   (`md5(…)::uuid`). Restaurar con el comando `restore` (DROP/CREATE de la base) o pasar los datos a otro clúster (la
   adopción de la fase A) la cambia **sola**, sin depender de que cada procedimiento de restauración se acuerde de
   regenerarla: me pareció más seguro que cambiar el UUID en cada comando de restauración, que olvidaría un
   procedimiento manual que recree la base. `catalog/sync` devuelve `catalog_epoch`, **exige `epoch` cuando `since` > 0**
   (400 sin ella) y, con otra época, entrega todo con `reset: true`; `restaurant/state` también la devuelve.
   `docs/API_V1.md` explica qué hace el cliente: guardar versión y época y, con `reset`, reemplazar todo lo guardado.
   Límite conocido: una restauración que no recree la base (por ejemplo, `pg_restore --clean` sobre la misma) no la
   cambia; ningún procedimiento del proyecto lo hace.
   **Pruebas**: `catalog/stage9g_test.go` (sin época → 400; época inválida → 400; otra época → todo con `reset`; misma
   época → solo cambios) y **restauración real** en `cmd/ad-el-pos-server/host_unix_integration_test.go`
   (`TestServiceHostOnMacOS`): la época leída antes del respaldo cambia después de `restore`, y el UUID guardado se
   conserva. `database/stage9g_android_upgrade_test.go`: la tabla nace con una sola fila.
3. **BAJO · El borrador aceptaba productos agotados** (`operations/drafts.go`). Confirmado: `PUT …/draft` y abrir una
   cuenta con `items` no ejecutan el motor.
   **Corrección**: `catalog.RejectSoldOut` (motor con la excepción de disponibilidad, que nunca pasa por alto
   «Agotado») y `operations.rejectSoldOutCapture` en `saveDraft` y `openAccount`: solo revisan lo que se agrega (partida
   nueva, con otro producto, presentación o personalización, o con más unidades). Una partida ya guardada que se agotó
   después se conserva igual o con menos unidades (el mesero puede quitarla; la confirmación la sigue rechazando). Otros
   rechazos del catálogo se ignoran aquí (los dicen la cotización y la confirmación, como antes). La edición de partidas
   confirmadas no cambia.
   **Pruebas**: `operations/stage9g_offline_test.go` (abrir cuenta con un agotado → 409 sin crear nada; conservar y
   reducir → 200; aumentar, otra partida del agotado y combo con el componente agotado → 409 «Papas producto está
   agotado» sin cambiar la cuenta; disponible otra vez → se guarda) y Playwright `workflow-stage9g-sold-out.spec.ts`:
   con la limonada en la cuenta, otra terminal la agota y la cuenta ya avisa (cálculo de Go); «Agregar una unidad» no se
   guarda (segundo aviso igual, del borrador rechazado, y «Por guardar…»); al quitar la unidad de más queda «Guardado»
   con un solo aviso. Nota: el aviso del borrador rechazado y el del cálculo dicen lo mismo («Limonada 9G está agotado.
   Elige otro producto.»); no hizo falta cambiar la interfaz.
4. **BAJO · Reenvío confirmado con `IDEMPOTENCY_CONFLICT` sin resultado** (`operations/handler.go`). Confirmado.
   **Corrección**: en `replay`, si la clave de `POST restaurant/offline-orders` ya está guardada **por el mismo usuario y
   la misma terminal** con otro contenido, 409 **`OFFLINE_ORDER_ALREADY_ACCEPTED`** con `details.status:
"already_accepted"` y `details.result` (la respuesta guardada: `order_id`, `account_id`, cuenta completa). Otro
   remitente sigue recibiendo `IDEMPOTENCY_CONFLICT` sin datos. Contrato: esquema `OfflineOrderAlreadyAccepted` en el 409
   de `submitOfflineOrder`; `docs/API_V1.md` (doble envío y tabla de errores).
   **Pruebas**: `operations/stage9g_offline_test.go` reproduce el caso exacto (original rechazado por precio → el precio
   vuelve → la repetición tardía entra → el reenvío confirmado recibe el pedido que entró; un solo pedido; otro usuario
   recibe el conflicto sin el pedido) y el envío tardío del original después de la confirmación. `TestContractV1`
   valida la respuesta contra el contrato.
5. **BAJO · «Agotado» no se reflejaba en combos ni grupos** (`sync.go`). Confirmado.
   **Corrección**: la descarga de cambios agrega, con las relaciones vigentes, los combos que ofrecen un producto que
   cambió (también como predeterminado, y combos de combos) y los grupos cuyos complementos son ese producto o cuyos
   complementos, porciones o ingredientes cambiaron. Cada opción de ranura de combo y de grupo lleva `available_now` y
   `sold_out` (mismas reglas que el cálculo), y un combo cuya ranura obligatoria se queda sin opciones disponibles sale
   con `available_now: false` y su motivo. Se resolvió **al leer** a partir de lo que anotó la bitácora (la versión ya
   sube con el producto agotado) en lugar de anotar combos y grupos en los disparadores: cubre cualquier causa
   (agotado, desactivación, categoría, complemento) sin que los disparadores conozcan las relaciones.
   **Prueba** (`catalog/stage9g_test.go`): combo con ranura obligatoria de papas y grupo con el complemento papas; al
   agotar las papas, la descarga de cambios trae papas y combo (no disponible, opción `sold_out`) y el grupo (opción no
   disponible y agotada), y la descarga completa marca el combo; al reactivarlas, el combo vuelve a estar disponible.

#### Pruebas ejecutadas (13/11/2026, árbol DF, clúster 55499, Go 8185, vista previa 1423)

| Suite                                                                                                 | Resultado                                                                                                                                                                      |
| ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `format`, `sync`, `vet`, `win` (`GOOS=windows` build y vet)                                           | aprobado                                                                                                                                                                       |
| Go unitarias con `-race`                                                                              | aprobado (16 s), incluidas las del contrato                                                                                                                                    |
| Integración con `-race`                                                                               | `apicontract` 10 s, `database` 15 s, `catalog` 22 s, `cmd/ad-el-pos-server` 45 s (restauración real con la época), `administration` 120 s, `operations` 316 s: todas aprobadas |
| Playwright `restaurant.spec.ts`, `workflow-stage9g-sold-out.spec.ts`, `workflow-zz-test-data.spec.ts` | Chromium 3/3 (1.6 min), WebKit 3/3 (1.6 min); pantalla desbloqueada                                                                                                            |

La primera corrida de Playwright falló en la especificación de «Agotado» porque esperaba que el aviso desapareciera al
quitar la unidad de más; el aviso que quedaba era el del cálculo (correcto: la limonada sigue agotada en el borrador). Se
ajustó la especificación para distinguir los dos avisos; `workflow-zz-test-data` falló en esa corrida solo en cascada
(el puente exige una base sin configurar después de un fallo) y aprobó en la siguiente. No se tocó la interfaz (sin
`tsc`/Vitest nuevos: los archivos de `desktop/src` no cambiaron).

#### Para el coordinador y CLAUDE.md

- `catalog/sync` exige `epoch` con `since` > 0 (cambio del contrato antes de que exista ningún cliente Android).
- Código nuevo de error `OFFLINE_ORDER_ALREADY_ACCEPTED` (409).
- `RejectSoldOut` toma el lock 7820148304 compartido después de 7820148305 (orden de siempre).
- Fase G: en `docs/INSTALAR_EN_RED.md`, al explicar la restauración, decir que después las terminales y teléfonos
  vuelven a descargar el catálogo completo por sí solos (época nueva).

**Fusión y verificación del coordinador**: la rama `etapa-9g-dfix` salió de `dc638d1` (sin la Fase E); se fusionó sin
conflictos en `792e877` y quedó cubierta por la regresión final de la Fase G (abajo).

### Correcciones de la parte 3 (Fase F)

Un agente (14/11/2026), en el árbol `$SCR/wt/FF` (rama `etapa-9g-ffix`, desde `etapa-9g-merge` en `6e92f7a`, con A, D,
B, C, F y las correcciones de B), en paralelo con la Fase E. La 0023 todavía no se aplicó a ninguna instalación: se
editó solo su sección F (la E no se tocó). Commit: `3948fdc`. Pruebas aisladas: clúster 55499, Go 8187, vista previa
1425, `TMPDIR=$SCR/tmp-F`.

Cada hallazgo se comprobó primero en el código.

#### 1. MEDIO · Retención y limpieza de `.partial` sin distinguir la instalación

- **Comprobado**: `Retain` y `CleanPartials` (`files.go`) decidían qué era «propio» solo por el patrón
  `<base>-<fecha>-<hex>-automatico.dump`, y la base se llama igual en todas las instalaciones; se ordenaba por el
  nombre (hora UTC de quien lo creó). Con archivos ajenos de fechas posteriores en una carpeta compartida, se borraba
  lo propio, incluso el respaldo recién hecho.
- **Corrección**:
  - La retención (`runner.retain`) solo considera lo registrado en `backup_runs`: `kind='automatic'`,
    `status='succeeded'`, misma carpeta, `removed_at IS NULL`, `seq` menor que el recién hecho (nunca el actual),
    ordenado por `seq` (columna nueva `GENERATED ALWAYS AS IDENTITY`). `toRemove` es la decisión pura.
  - Borra con `removeRegular`: solo archivos regulares, nunca enlaces ni carpetas. Un archivo que ya no está se anota
    como borrado.
  - El archivo temporal se registra antes de crearse (`partial_file` y `directory` en la fila en curso; restricción
    `backup_runs_partial_running_check`). Al recuperar una ejecución interrumpida (`markInterrupted`, siempre con el
    lock) se borra exactamente ese archivo y nada más. Desapareció la limpieza por patrón.
  - Nombre `<base>-<instalación>-<fecha>-<hex>-<etiqueta>.dump`. `<instalación>` son 8 caracteres hexadecimales del
    `server_id` de la identidad de la Fase B (`main.go` lo fija en el ejecutor antes de arrancar el trabajador) o, sin
    identidad, de `backup_settings.installation_id` (columna nueva, `gen_random_uuid()`).
  - Tras un fallo de `Dump`, el ejecutor ya no borra el nombre (podía ser un archivo ajeno): `Dump` limpia lo que
    crea.
- **Prueba**: `TestSharedFolderKeepsOtherInstallationsFiles`, el caso del revisor como prueba de integración.
  - Carpeta compartida con dos automáticos de «otra instalación» con fechas de 2030, uno con el formato anterior sin
    identificador, un `.partial` ajeno y un manual ajeno; `keep=2`; cuatro días de respaldos.
  - Resultado: nunca se borra un ajeno ni el recién hecho, quedan exactamente dos propios y se anotan dos borrados.
    Sin identidad, el nombre lleva el `installation_id`.
- **Otras pruebas**:
  - `fila_en_curso_huerfana`: se borra el `.partial` propio registrado y se conserva el ajeno del mismo formato.
  - Unitarias `TestToRemove` y `TestRemoveRegular`: enlace, carpeta y nombres con `..` o barras rechazados.

#### 2. MEDIO · Respaldo legible para otros y sin `O_EXCL` en el servicio

- **Comprobado**:
  - `standalone.DumpFile`, y también `dumpLegacy` de `adopt-legacy`, usaban `pg_dump --file`. Eso crea el archivo con
    la máscara del proceso, sigue enlaces, trunca y solo al final pasa a 0600. Lo usaban el respaldo manual, el de
    antes de migrar, el de seguridad de `restore` y los programados del `host`.
  - El plist ya fija `Umask` 63 (077): en el servicio el archivo ya nacía 0600. Desde una consola (umask 022) nacía
    0644 hasta el `chmod` final.
- **Corrección**:
  - `fileacl.CreatePrivate`. En macOS y Unix: `O_WRONLY|O_CREATE|O_EXCL|O_NOFOLLOW`, 0600 desde el primer byte. En
    Windows: `O_EXCL` y la ACL privada antes de escribir.
  - `standalone.dumpToFile`: `pg_dump` escribe en su salida estándar hacia ese archivo, con la salida de error aparte.
    Lo usan `DumpFile` (y por él `Backup`: manual, antes de migrar y seguridad de `restore`) y `dumpLegacy`.
  - `maintenance.DumpBackup.Dump` (`serve`) también usa `CreatePrivate` en lugar de `OpenFile` más `Protect` al final.
  - El plist no necesitó cambios.
- **Prueba**:
  - `TestCreatePrivateIsPrivateExclusiveAndDoesNotFollowLinks` (máscara 0, archivo existente, enlace y enlace
    colgante sin crear su destino).
  - `TestDumpFileIsPrivateFromTheFirstByte`: un `pg_dump` simulado lee los permisos del archivo antes de escribir y ve
    `-rw-------` con la máscara 022; tampoco reemplaza un respaldo existente ni sigue un enlace (el archivo apuntado
    queda intacto).
  - `TestServiceHostOnMacOS` ejercita los respaldos reales del `host`: `backup`, antes de migrar, `restore` con
    respaldo de seguridad y `adopt-legacy`.

#### 3. MEDIO · Sin aviso si el respaldo deja de intentarse

- **Comprobado**:
  - `slotHistory` contaba `started_at >= slot` sin límite superior, así que un respaldo «del futuro» detenía los días
    siguientes.
  - `lastRun` ordenaba por `started_at`.
  - Una fila `running` que `finish()` no pudo cerrar bloqueaba `Due` y la pantalla decía «en curso».
  - Un `Plan` que fallaba siempre solo quedaba en la bitácora.
- **Corrección**:
  - **Reloj adelantado**: `slotHistory` y `lastRun` ignoran `started_at > ahora + 5 min` (`ClockTolerance`).
  - **Orden**: por `seq` en `lastRun`, la lista y el conteo de fallos desde el último correcto.
  - **Filas huérfanas**:
    - Una fila en curso de más de `StaleAfter` (30 + 5 min) no cuenta como en curso ni detiene `Due`.
    - En cada revisión, si hay filas en curso y el lock está libre, el trabajador las cierra como `interrupted`; con el
      lock tomado ningún respaldo vive, así que toda fila en curso es un resto.
    - Al arrancar sigue la recuperación de siempre.
    - Un intento interrumpido cuenta para el máximo de 3, pero no hace esperar la hora de reintento.
  - **Aviso nuevo `overdue`** («No hay un respaldo reciente de la base de datos»):
    - Cuándo: respaldos activos, establecimiento configurado y sin respaldo correcto en más de 26 h
      (`OverdueAfter`), contadas desde lo más reciente entre el último correcto y la activación
      (`backup_settings.enabled_at`, nueva; se renueva al reactivar) o la configuración del establecimiento.
    - `reason`: `stale`, con la fecha del último correcto, o `never`.
    - Prioridad: el fallo primero, luego el atraso, luego la carpeta no disponible.
    - Se ve en la pantalla de inicio con el mismo estilo rojo que el fallo, y en Mantenimiento y en «Solo servidor».
    - Si `Plan` falla siempre, este aviso lo cubre.
- **Prueba**:
  - `TestClockAheadAndOverdueAlert`:
    - Un respaldo con el reloj 5 días adelantado; al corregirlo (27 h después de configurar) hay aviso `overdue/never`,
      el respaldo «del futuro» no es el último y la programación vuelve a tocar.
    - Un fallo posterior se avisa como `failed` aunque haya un éxito con fecha futura; un correcto quita el aviso.
    - 30 h después hay `overdue/stale` con la fecha del último correcto.
    - Desactivados no avisan; al reactivar se mide desde entonces.
  - `fila_en_curso_huerfana`: una fila en curso huérfana no bloquea el día; la revisión la cierra y respalda.
  - Unitarias `TestAlertFor` (fallo, reciente, atrasado, recién activados, nunca, desactivados, sin establecimiento,
    carpeta) y `TestLatestPast`.
  - Vitest: textos del aviso atrasado y su estilo.

#### 4. BAJO · Validación de la carpeta

- **Comprobado**: solo se rechazaba la carpeta que tenía `PG_VERSION` ella misma, no `pg_wal`, `base` ni la carpeta
  del servicio (`identity/`, `logs/`); los enlaces se guardaban sin resolver.
- **Corrección**:
  - `ValidateDirectory(directory, protected)` resuelve con `EvalSymlinks` y guarda la ruta real.
  - Rechaza cualquier carpeta con `PG_VERSION` en ella o en una superior (`postgres_data`).
  - Rechaza cualquier carpeta dentro de las protegidas (`server_data`, «Esa carpeta guarda los datos del servidor de
    AD-EL POS; elige otra para los respaldos.»).
  - `ScheduledBackuper.ProtectedDirectories()`: la carpeta de datos del servicio (`installationBackup`, ahora con una
    aserción en compilación) o la carpeta de la configuración con `serve` (`DumpBackup.ConfigDirectory`).
  - Elegir la predeterminada sigue siendo volver a ella.
- **Prueba**:
  - `TestValidateDirectory`: `pg_wal`, otra carpeta de datos de PostgreSQL, la carpeta del servidor, `identity`,
    `logs`, enlaces hacia ellos (rechazados) y un enlace hacia la USB (guardado resuelto).
  - `TestStage9GBackupRoutes`: las mismas por HTTP; un enlace a la carpeta ya elegida no cambia nada ni se audita.
  - Playwright: elegir la carpeta del servidor temporal se rechaza. La carpeta «USB» de la prueba pasó fuera de ella.

#### 5. Para la Fase G (sin poder probarlo aquí) · USB y privacidad de macOS

Un `LaunchDaemon` no recibe la ventana de permiso de privacidad de macOS. Al escribir en una USB o un disco externo
(«Volúmenes extraíbles») o en una carpeta de red puede recibir `EPERM`. Qué verá el restaurante:

- Al **elegir** la carpeta en Mantenimiento: «El servidor no puede guardar archivos en esa carpeta. Elige otra o revisa
  sus permisos.», porque Go prueba a escribir con la cuenta del servicio. No se guarda nada.
- Si el permiso se pierde **después**: el respaldo queda en la carpeta predeterminada y aparece el aviso «La carpeta de
  los respaldos no estaba disponible… no permite guardar archivos» en la pantalla de inicio.
- Qué hacer:
  - En la computadora principal, Ajustes del Sistema › Privacidad y seguridad › **Acceso total al disco** › «+» y
    agregar `/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server` (con ⌘⇧G para escribir la ruta); activarlo.
  - Reiniciar el servicio desde Mantenimiento («Reiniciar») y volver a elegir la carpeta.
  - Con firma ad hoc, el permiso puede tener que darse otra vez tras instalar una versión nueva.
  - Comprobarlo en la Fase G con una USB real antes de anotarlo en `docs/INSTALAR_EN_RED.md`.

#### Pruebas ejecutadas

Sobre el commit `3948fdc`, con la pantalla desbloqueada (`CGSSessionScreenIsLocked` ausente):

| Suite                                                                                                                                   | Resultado                                                                                                    |
| --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `format` (`docs/etapas` con el mismo md5), `sync`, `vet` (normal e `integration`), `win` (`GOOS=windows` build y vet)                   | aprobado                                                                                                     |
| Go unitarias con `-race` (`go test ./...`)                                                                                              | aprobado (17 s); nuevas o cambiadas en `backupschedule`, `fileacl` y `standalone`                            |
| Integración `./internal/backupschedule`                                                                                                 | aprobado, 10 s (incluido el respaldo real con `pg_dump`/`pg_restore` contra 55499 y las tres pruebas nuevas) |
| Integración `./internal/administration` (todo el paquete, `TestStageTwoAcceptance` incluida)                                            | aprobado, 121 s                                                                                              |
| Integración `./internal/database` (`checkStage9GBackups` con las columnas nuevas y 3 rechazos más de `partial_file`)                    | aprobado, 12 s                                                                                               |
| Integración `./cmd/ad-el-pos-server` (`TestServiceHostOnMacOS`: `backup`, antes de migrar, `restore` y `adopt-legacy` con `dumpToFile`) | aprobado, 47 s                                                                                               |
| `tsc --noEmit` / Vitest                                                                                                                 | sin errores / 93 archivos, 876 pruebas (13 en `stage9gBackups.test.tsx`)                                     |
| Playwright `restaurant`, `workflow-stage9g-backups`, `workflow-zz-test-data`                                                            | Chromium 3/3 (1.5 min), WebKit 3/3 (1.5 min)                                                                 |
| `cargo`                                                                                                                                 | no se ejecutó: no se tocó Rust                                                                               |

#### Para el revisor y para CLAUDE.md

- 0023, sección F: `backup_settings.enabled_at` e `installation_id`; `backup_runs.seq` (identidad) y `partial_file`;
  índices `backup_runs_running_idx (seq)` y `backup_runs_retention_idx`.
- Regla de respaldos (§15): la retención y la limpieza de restos solo tocan archivos registrados por esta instalación
  en `backup_runs`, y los nombres llevan el identificador de la instalación. Todo respaldo nace con
  `fileacl.CreatePrivate`. Ejecuciones «del futuro» ignoradas, orden por `seq`, aviso `overdue` a las 26 h. La carpeta
  elegida se guarda resuelta y fuera de los datos de PostgreSQL y del servidor.
- Fase W: verificar `fileacl.CreatePrivate` en Windows (la ACL con el archivo abierto, `pg_dump` a la salida estándar
  en binario) y una USB FAT o exFAT (sin ACL, ya anotado en F).
- Fase G: el permiso «Acceso total al disco» del servicio para una USB (punto 5).

**Fusión y verificación del coordinador**: la rama `etapa-9g-ffix` salió de `6e92f7a`; se fusionó sin conflictos en
`dc638d1`. Copia exacta: formato, `sync`, `vet`, `GOOS=windows`, Go unitarias con `-race`, integración de los 13
paquetes, `tsc`, Vitest y Playwright (`restaurant`, `workflow-stage9f-printing`, las seis `workflow-stage9g-*` y
`workflow-zz-test-data`: 10 de 10 en Chromium y 10 de 10 en WebKit): todo aprobado.

## Fase G · Versión, documentación, regresión final, instalación y fusión

### Versión 0.10.0

`c4cc94d`: `config/product.json` → versión **0.10.0** y, como pidió la Fase D, `min_client_version` y
`min_server_version` también **0.10.0** (el escritorio 0.10.0 usa rutas que un servidor 0.9.5 no tiene, y un cliente
0.10.0 es el primero que se identifica); `scripts/dev.sh sync` y la versión de la raíz en `Cargo.lock` y
`package-lock.json` (como en la 9F). Esquema 23 (`0023_network_terminals_and_android.sql`, secciones D, B, F y E).

### Documentación

Un agente, solo Markdown (`dc5fe19`, fusionado en `d9e1f1a`): `CLAUDE.md` (versión y esquema, arquitectura servidor y
terminales, modalidades, emparejamiento y cifrado —D1 sustituye la regla de `0.0.0.0`—, idempotencia y pedidos sin
conexión, agente de impresión, respaldos, lock 7820148309, estructura, comandos, pruebas seguras, reglas permanentes de la
9G, §13–14, §15 «Etapa 9G», §16, §17 y §19), `docs/INSTALAR_EN_RED.md` (nuevo, para el propietario),
`docs/VERIFICATION_STAGE9G.md` (nuevo) y el índice de `docs/VERIFICATION.md`, `docs/NOTAS_WINDOWS.md` («Etapa 9G: qué hacer
en la Fase W»), `ARCHITECTURE.md`, `AGENTS.md`, `README.md`, `NEXT_STEPS_FOR_SOL.md`, `DEVELOPMENT_MACOS.md`,
`docs/DEPENDENCIES.md` y las guías de catálogo, operación, cobros, navegación e impresión; `docs/API_V1.md` lo escribió la
Fase D. Al agente, el sistema de permisos le bloqueó una edición de `API_CONTRACTS.md` (se le había pedido no tocar el
contrato); el coordinador corrigió después cuatro incoherencias de documentación que señaló (`eb7ab4a`: identidad que ya
no impide vender, época del catálogo, pedido sin conexión ya aceptado y borrador sin agotados, cabecera de versión
obligatoria por la red, impresoras de la terminal y `PAIRING_LIMIT_REACHED` en `API_V1.md`), sin cambiar el contrato
(prueba de contrato aprobada).

### Regresión final

Ejecutada por el coordinador con el arnés aislado sobre una copia exacta de `c4cc94d` (`git archive`; después solo
cambió Markdown: documentación y `eb7ab4a`, con formato y prueba de contrato aprobados), clúster PostgreSQL temporal
55499, Go temporal en 8183 y vista previa en 1421, 13–14/11/2026: **1757 s, todo aprobado**.

| Suite                                                      | Resultado                                                                                                                                                                                                                                                                   |
| ---------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato (Prettier, `gofmt`, `cargo fmt`)                   | aprobado                                                                                                                                                                                                                                                                    |
| Metadatos (`sync-metadata --check`)                        | aprobado                                                                                                                                                                                                                                                                    |
| `go vet` (normal e `integration`)                          | aprobado                                                                                                                                                                                                                                                                    |
| `GOOS=windows GOARCH=amd64` build, vet y vet `integration` | aprobado                                                                                                                                                                                                                                                                    |
| Go unitarias con `-race`                                   | 31 paquetes `ok` (nuevos: `lanlistener`, `discovery`, `backupschedule`, `apicontract`)                                                                                                                                                                                      |
| Integración PostgreSQL con `-race`                         | 13 de 13: `operations` 327 s, `reports` 130 s, `administration` 117 s, `reports/operational` 44 s, `cmd/ad-el-pos-server` 42 s, `designer` 24 s, `catalog` 23 s, `identity` 20 s, `database` 11 s, `backupschedule` 9 s, `apicontract` 9 s, `discovery` 5 s, `payments` 3 s |
| `tsc --noEmit`                                             | sin errores                                                                                                                                                                                                                                                                 |
| Vitest (`npm test`)                                        | 94 archivos, 883 pruebas aprobadas                                                                                                                                                                                                                                          |
| `npm run build`                                            | correcto                                                                                                                                                                                                                                                                    |
| `cargo test --locked --offline`                            | 89 aprobadas, 3 ignoradas                                                                                                                                                                                                                                                   |
| Playwright Chromium (todas las especificaciones)           | 32 de 32 (9.9 min)                                                                                                                                                                                                                                                          |
| Playwright WebKit (todas las especificaciones)             | 32 de 32 (9.8 min)                                                                                                                                                                                                                                                          |
| `.app` (`tauri build --bundles app`)                       | 0.10.0, Mach-O arm64, 14.02 MiB, `Info.plist` con `NSLocalNetworkUsageDescription` y `NSBonjourServices`                                                                                                                                                                    |

La pantalla de la Mac estaba desbloqueada al empezar y quedó bloqueada al terminar (después de Playwright, que aprobó en
ambos motores). Rust para Windows no se compiló (el destino MSVC no está instalado en la Mac): lo nuevo de Rust propio de
Windows queda detrás de `#[cfg]` y se compila en la Fase W.

### Instalación real (regla de autonomía de §0)

Sobre `4687930` con el árbol limpio, después de la regresión final aprobada (14/11/2026). La instalación real sigue
siendo la de `.local` (servidor `serve` y PostgreSQL 55432 del proyecto): pasarla al servicio exige instalar el `.pkg`
con la contraseña de administrador, que es una parada del §0 de la especificación (abajo).

1. **Estado previo** (servidor 0.9.5, esquema 22), comprobación de solo lectura con un script del directorio temporal
   (`default_transaction_read_only=on`, solo conteos): migraciones 1–22, 136 tablas (con `schema_migrations`), 937 filas,
   154 permisos, cuentas `closed:2, open:7, voided:1`, turnos `closed:1, open:1`, **0 turnos en arqueo, 0 trabajos de
   impresión sin terminar**, 1 terminal activa y 0 sesiones activas. Repetida justo antes de detener el servidor:
   idéntica.
2. **Respaldo** `backups/adel_pos-20261114T060151Z-ec289c.dump` (677 970 bytes, catálogo legible).
3. **Ensayo de restauración**: «Restauración completa verificada: 135 tablas con conteos idénticos».
4. **Servidor detenido** de forma ordenada (`server-stop`).
5. **Migración**: `./scripts/dev.sh migrate` → `applied:1`; repetición → `applied:0`.
6. **Servidor 0.10.0 iniciado** (`server-start`): readiness 200; `api/v1/version` → **0.10.0 / esquema 23**,
   `test_mode: false`, `min_client_version` 0.10.0; identidad del servidor creada en `.local/identity`; solo escucha en
   `127.0.0.1:8181` (sin `network_listen_address`: sin escucha de red ni anuncio, como corresponde a `serve`).
7. **Comprobación posterior** de solo lectura, comparada con la previa: solo cambian las migraciones (1–23), las tablas
   nuevas de la 0023 (`backup_runs` 0, `backup_settings` 1, `catalog_change_log` 0, `catalog_epoch` 1,
   `device_local_printers` 0, `device_pairing_requests` 0, `register_receipt_printers` 0), `permissions` 154 → 155 y
   `role_permissions` 160 → 161 (`catalog.sold_out.manage` al administrador). Cuentas, pagos, caja, cocina, inventario,
   catálogo, auditoría y ventas: **idénticos**. Ningún dato de operación se borró ni se reescribió.
8. **Aplicación**: `./scripts/dev.sh build-macos` → `AD-EL POS.app` 0.10.0 (13.98 MiB); `./scripts/dev.sh open-app` →
   la aplicación 0.10.0 quedó abierta. Como aún no existe `connection.json`, al abrirla pregunta «¿Cómo se usará esta
   computadora?» con **«Servidor y terminal»** sugerida (sin servicio instalado guarda solo la conexión local, como hasta
   la 9F).

**Pendiente del propietario (parada del §0 de la especificación)**: instalar el `.pkg` 0.10.0 (ventana de contraseña de
administrador del Instalador y aviso «Elementos en segundo plano añadidos»), pasar los datos de `.local` a
`/Library/Application Support/AD-EL POS` con `adopt-legacy`, y comprobar que el servicio sigue al cerrar la ventana y que
arranca sin iniciar sesión (reiniciar la Mac, otra parada). Ajuste al procedimiento de la Fase A: como la aplicación
0.10.0 ya guarda su modalidad en `connection.json`, la elección del primer arranque no vuelve a aparecer; para habilitar
el servidor después de pasar los datos se quita ese archivo antes de abrir la aplicación instalada (o se vuelve a
ejecutar el instalador con «Servidor del restaurante»).
