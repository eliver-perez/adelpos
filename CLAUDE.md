# CLAUDE.md · AD-EL POS

Guía de trabajo para Claude en este repositorio. Resume decisiones vigentes y remite a la documentación existente en lugar de copiarla. **`AGENTS.md` sigue siendo obligatorio** y prevalece en convenciones; si este archivo y otro documento se contradicen, señalarlo al propietario en lugar de resolverlo por cuenta propia.

Última revisión: 8 de noviembre de 2026, al terminar la etapa 6 (cobros, pagos y devoluciones; avance por fases en [docs/ETAPA_6_AVANCE.md](docs/ETAPA_6_AVANCE.md)), después del diagnóstico de las etapas 1–5 ([docs/DIAGNOSTICO_ETAPAS_1_5.md](docs/DIAGNOSTICO_ETAPAS_1_5.md)). Versión 0.6.0, API v1, esquema PostgreSQL 6. La instalación real se migró a esquema 6 el 8 de noviembre de 2026 con autorización del propietario (respaldo y ensayo de restauración previos).

---

## 1. Qué es AD-EL POS

Punto de venta comercial para restaurantes, optimizado para pantallas táctiles. Reconstruye y amplía un sistema anterior en C# / Windows Forms / SQL Server usado en un restaurante real. **No se copia la apariencia de Windows Forms**: se conservan y mejoran sus funciones con una interfaz moderna.

Este repositorio es el **Proyecto 1 — Servidor + POS** (ver §16). Nombre provisional del producto: "AD-EL POS"; identificador técnico `ad-el-pos`; nombre inicial del establecimiento en instalaciones nuevas: **ADEL POS** (no confundir marca técnica con el establecimiento).

## 2. Tecnologías

| Capa       | Tecnología                                                                             |
| ---------- | -------------------------------------------------------------------------------------- |
| Backend    | Go (monolito modular), `pgx/v5`, `shopspring/decimal`, `x/crypto` (Argon2id), `x/term` |
| Datos      | PostgreSQL 18 (clúster exclusivo del proyecto, esquema `app`)                          |
| Frontend   | React 19, TypeScript 5.9 estricto, Tailwind CSS 4, Vite 8, Vitest                      |
| Escritorio | Tauri 2 (Rust), `reqwest` + rustls, `keyring` (Keychain de macOS)                      |
| Transporte | HTTP JSON versionado `/api/v1` + SSE autenticado (`/api/v1/events`)                    |
| Pruebas    | `go test -race`, Vitest + jsdom, Playwright (Chromium/WebKit), `cargo test`            |

Versiones exactas y fuentes: [docs/DEPENDENCIES.md](docs/DEPENDENCIES.md). Las herramientas locales (Go, Node, Rust) viven en `.tools/` y `scripts/env.sh` las antepone al `PATH`. **No actualizar dependencias por inferencia**: los lockfiles (`go.sum`, `package-lock.json`, `Cargo.lock`) son la referencia.

## 3. Arquitectura

```
React/TS (WebView) ──IPC: operaciones enumeradas──▶ Rust/Tauri ──HTTP JSON v1 (TLS en LAN)──▶ Go ──pgxpool──▶ PostgreSQL
                                                     └─ Keychain (tokens duraderos)
```

- **Go es la autoridad** de negocio, permisos, sesiones, dinero (precios en `catalog`, finanzas en `payments`), cocina y eventos. Toda operación SQL vive en `backend/`. Ningún código de escritorio accede a PostgreSQL.
- **Tauri no inicia ni detiene Go ni PostgreSQL.** Go funciona sin ventana (trabajadores de impresión y planificador incluidos).
- **Rust** custodia credenciales en Keychain, valida el destino (solo desde archivo local, nunca desde React) y expone una **lista cerrada** de operaciones API (`desktop/src-tauri/src/api.rs`). Los tokens nunca llegan al WebView.
- **React** presenta y recoge datos; no calcula dinero (los importes llegan como cadenas decimales calculadas en Go).
- Modos de instalación: `server-terminal`, `terminal-only`, `server-only`.
- Detalle y límites: [ARCHITECTURE.md](ARCHITECTURE.md). Contratos: [API_CONTRACTS.md](API_CONTRACTS.md).

### Composición del backend

`administration` autentica cada petición (dispositivo + sesión + permisos leídos de PostgreSQL), abre **una transacción por operación** y delega por prefijo (`backend/internal/administration/routes.go`):

- `catalog/*` → `catalog.Handle` · `restaurant/*` → `operations.Handle` · `kitchen/*` → `kitchen.Handle` · `payments/*` → `payments.Handle` (con autorizador de supervisor, `administration/supervisor.go`) · `events` / `events/snapshot` → SSE en `administration/events.go`.
- Cada módulo recibe la misma `pgx.Tx`, un `Actor` con permisos efectivos y un callback de auditoría que escribe en la misma transacción.

Locks consultivos en uso (no reutilizar números para otro fin; respetar el orden de adquisición):

| Clave      | Uso                                                                                                                                                                        |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 7820148301 | Migrador                                                                                                                                                                   |
| 7820148302 | Asistente inicial y aprovisionamiento de `sa`                                                                                                                              |
| 7820148303 | Usuarios/sesiones (`administration/users.go`)                                                                                                                              |
| 7820148304 | Catálogo: compartido en lecturas/cálculos, exclusivo en escrituras                                                                                                         |
| 7820148305 | Operación del restaurante, **cocina y cobros** (incluye el planificador y `payments/*`): compartido en lecturas, exclusivo en escrituras; se toma **antes** que 7820148304 |
| 7820148306 | Publicación ordenada del outbox `realtime_events`                                                                                                                          |
| 7820148307 | Semilla de `hashtextextended` para el lock de sesión por impresora                                                                                                         |

## 4. Estructura real del repositorio

La raíz git es `ad-el-pos/` (dentro de la carpeta de trabajo `ADEL POS/`).

```text
ad-el-pos/
├── AGENTS.md  ARCHITECTURE.md  API_CONTRACTS.md  README.md
├── DEVELOPMENT_MACOS.md  NEXT_STEPS_FOR_SOL.md  CLAUDE.md
├── backend/
│   ├── cmd/ad-el-pos-server/      # main.go (serve/migrate/prepare-setup/provision-sa/reset-sa/tls-init/create-enrollment/check-config/version), local.go
│   ├── internal/administration/   # identidad, permisos, usuarios, roles, terminales, cajas, ajustes, preferencias, auditoría, SSE, despacho de rutas
│   ├── internal/catalog/          # catálogo, disponibilidad, imágenes y motor decimal único (Calculate, QuoteForOrder)
│   ├── internal/operations/       # áreas, mesas, cuentas, borradores, lotes, versiones, unidades, división/transferencia/combinación
│   ├── internal/kitchen/          # estaciones, rutas, liberaciones, tareas, documentos, cola y transportes de impresión, Worker
│   ├── internal/payments/         # saldos, pagos, ajustes, IVA, devoluciones, correcciones, turno mínimo, documentos comerciales, entregas
│   ├── internal/events/           # outbox, secuencia, replay
│   ├── internal/httpapi/          # diagnósticos, CORS, request ID, errores
│   ├── internal/config/  database/  localsetup/  licensing/  product/
│   └── migrations/                # 0001–0006 embebidas; INMUTABLES
├── desktop/
│   ├── src/                       # App.tsx, components/, hooks/, pages/, services/, styles.css, generated/
│   ├── src-tauri/                 # main.rs, api.rs, vault.rs, connection.rs, realtime.rs, window_check.rs, tauri.conf.json, capabilities/, permissions/
│   └── e2e/                       # Playwright: restaurant.spec.ts, kitchen-flow.ts, native-driver.ts
├── config/                        # product.json (canónico) y plantillas sin secretos
├── scripts/                       # dev.sh, env.sh, database.py, server-process.py, browser-tests.py, restore-check.py, sync-metadata.mjs
├── tests/runtime_smoke.py         # prueba operativa opt-in (interrumpe servicios)
├── ops/launchd/                   # plantilla .plist NO instalada
├── assets/brand/                  # PNG originales de marca (no modificar)
└── docs/                          # CATALOG, RESTAURANT_OPERATIONS, KITCHEN_AND_PRINTING, PAYMENTS_AND_REFUNDS, ETAPA_6_AVANCE, VERIFICATION*, CODING_CONVENTIONS, DEPENDENCIES, DIAGNOSTICO_ETAPAS_1_5, etapas/
```

Excluidos de git (no versionar ni borrar): `.local/` (clúster, secretos, logs, evidencias), `.tools/`, `.cache/`, `bin/`, `backups/`, `desktop/node_modules/`, `desktop/dist/`, `desktop/src-tauri/target/`, `desktop/src-tauri/gen/`.

Archivos **generados** desde `config/product.json` por `scripts/sync-metadata.mjs` (no editarlos a mano; editar `product.json` y ejecutar `./scripts/dev.sh sync`): `backend/internal/product/metadata.json`, `desktop/src/generated/product.json`, `desktop/src/generated/connection.json`, y los campos de nombre/versión/identificador en `tauri.conf.json`, `desktop/package.json`, `Cargo.toml` e `index.html`.

## 5. Comandos de desarrollo macOS

Todos desde la raíz del repositorio. Fuente: `scripts/dev.sh`. **Revisar el efecto antes de ejecutar**: varios actúan sobre la instalación real.

| Comando `./scripts/dev.sh …`              | Efecto real                                                                                                                                                     |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `format-check`                            | Solo lectura: Prettier, gofmt, `cargo fmt --check`.                                                                                                             |
| `format`                                  | **Escribe** fuentes (Prettier, gofmt, cargo fmt).                                                                                                               |
| `sync`                                    | **Escribe** metadatos generados desde `config/product.json`.                                                                                                    |
| `install`                                 | `npm ci` + sync + `go mod download` (requiere red).                                                                                                             |
| `db-init`                                 | Crea clúster exclusivo `.local/postgres` (puerto 55432), secretos 0600, rol `adel_app`, base `adel_pos`, `.local/server.json`. Idempotente.                     |
| `db-start` / `db-stop`                    | Inicia/detiene el clúster del proyecto.                                                                                                                         |
| `backup`                                  | `pg_dump` custom en `backups/` + `pg_restore --list`.                                                                                                           |
| `build-server`                            | sync + compila `bin/ad-el-pos-server`.                                                                                                                          |
| `migrate`                                 | Compila y **aplica migraciones pendientes a la base configurada** (real por defecto).                                                                           |
| `check-config`                            | Valida la configuración del servidor.                                                                                                                           |
| `prepare-setup`                           | Crea `.local/provisioning-token` si no existe.                                                                                                                  |
| `provision-sa` / `reset-sa`               | **Solo el propietario**, en terminal interactiva. Claude nunca los ejecuta ni pide la contraseña.                                                               |
| `tls-init --lan-ip IP`                    | Genera CA/certificados en `.local/tls/`; se niega a reemplazarlos.                                                                                              |
| `create-enrollment`                       | Emite y audita (en la base real) una autorización local de 10 min en `.local/enrollment-code`.                                                                  |
| `server`                                  | Go en primer plano.                                                                                                                                             |
| `server-start` / `server-stop` / `status` | Go como proceso independiente (PID `.local/server.pid`, log `.local/server.log`).                                                                               |
| `desktop`                                 | `tauri dev` con Vite en 127.0.0.1:1420.                                                                                                                         |
| `open-app`                                | Ejecuta el `.app` compilado.                                                                                                                                    |
| `build-macos`                             | `tauri build --bundles app` → `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`.                                                                    |
| `test`                                    | format-check + sync `--check` + `go test -race` + `go vet` + Vitest + `npm run build` (**escribe `desktop/dist`**) + `cargo test --locked` (escribe `target/`). |
| `test-integration`                        | Crea y elimina bases `adel_pos_test_*` **en el clúster del proyecto (55432)**. Requiere `db-start`.                                                             |
| `test-browser`                            | Go temporal en 8183 + bases temporales en el clúster del proyecto + Playwright Chromium/WebKit; artefactos en `.local/browser-artifacts/`.                      |

Otros:

- `python3 scripts/restore-check.py backups/NOMBRE.dump`: restaura en base temporal y compara conteos de todas las tablas; escribe `.local/stageN-restore-verification.json`.
- `python3 tests/runtime_smoke.py [--terminal-only]`: **detiene y reinicia PostgreSQL y Go reales y cierra/reabre Tauri**. Solo con autorización expresa y sin trabajo real en curso.
- Variables: `ADEL_SERVER_CONFIG`, `ADEL_DB_PASSWORD`, `ADEL_TERMINAL_CONFIG`, `ADEL_PG_BIN`, `ADEL_TEST_PACKAGE`, `ADEL_TEST_RUN`, `ADEL_WINDOW_VERIFICATION_FILE`, `ADEL_HARDWARE_QUEUE` / `ADEL_HARDWARE_SEND` / `ADEL_HARDWARE_PREVIEW` / `ADEL_HARDWARE_REPORT`.
- Puertos: Go real 8181, PostgreSQL 55432, Vite 1420, vista previa e2e 1421, Go temporal de smoke 8182, Go temporal e2e 8183.

Arranque diario y actualización de instalaciones: [README.md](README.md) y [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md) (respaldo → `restore-check.py` → `server-stop` → `migrate` → `server-start` → `build-macos` → `open-app`).

## 6. Pruebas seguras

Principio: **nunca alterar la instalación real** (`adel_pos`, `.local`, Keychain, servicios activos) para probar. Antes de ejecutar algo, leer el script.

- Sin efectos sobre datos: `format-check`, `sync --check` (`node scripts/sync-metadata.mjs --check`), `go vet`, `go test -race ./...` (las pruebas con PostgreSQL llevan `//go:build integration` y exigen `ADEL_TEST_DATABASE_URL` hacia una base cuyo nombre empiece por `adel_pos_test_`).
- `test-integration` y `test-browser` usan bases desechables, pero dentro del clúster real del proyecto. Aceptable con autorización; si se pide aislamiento total, usar el procedimiento siguiente.
- **Procedimiento aislado usado en el diagnóstico** (no deja rastros en el repositorio): copiar el repositorio al directorio temporal de la sesión excluyendo `.git`, `.local`, `.cache`, `.tools`, `backups`, `bin`, `target`, `dist`, `gen` (con exclusiones ancladas en rsync, p. ej. `--exclude='/bin/'`, para no omitir los `bin/` de `node_modules`); crear un clúster PostgreSQL temporal con `initdb` en otro puerto (p. ej. 55499, solo TCP loopback) y un rol `adel_app`; exportar `GOCACHE`, `CARGO_TARGET_DIR` y `npm_config_cache` hacia el directorio temporal, `GOMODCACHE` al del proyecto, `GOFLAGS=-mod=readonly`, `GOPROXY=off`; ejecutar en la copia `go test -race -tags=integration <paquete>` con `ADEL_TEST_DATABASE_URL=postgresql://adel_app:…@127.0.0.1:55499/adel_pos_test_…`, Vitest, `npm run build`, `cargo test --locked --offline`, y Playwright con un Go temporal en 8183 configurado contra ese clúster.
- Impresión física: **solo** `ADEL_TEST_PACKAGE=./internal/operations ADEL_TEST_RUN=TestControlledCUPSPrinter ADEL_HARDWARE_QUEUE=NOMBRE` y, para enviar realmente, `ADEL_HARDWARE_SEND=1`. Nunca en suites ordinarias ni dirigido a la impresora predeterminada por inferencia.
- Al añadir un paquete Go con pruebas de integración, **agregarlo a la tupla `packages` de `scripts/database.py`**; si no, `test-integration` no lo ejecuta.
- El puente de pruebas Playwright (`desktop/e2e/native-driver.ts`) **descubre las rutas leyendo `api.rs` con expresiones regulares**: las operaciones nuevas deben conservar el formato `"op" => Some((Method::X, "ruta"))` o el patrón `format!` existente.
- Las pruebas de navegador no sustituyen la ventana nativa, Keychain real, LAN con otra máquina ni hardware táctil; distinguir siempre esas evidencias (ver [docs/VERIFICATION.md](docs/VERIFICATION.md)).

## 7. Seguridad (resumen operativo)

Reglas completas en [AGENTS.md](AGENTS.md) y [ARCHITECTURE.md](ARCHITECTURE.md). En la práctica:

- No leer ni imprimir secretos de `.local/` (contraseñas DB, token de aprovisionamiento, `server.json`, claves TLS, códigos de enrolamiento) ni del Keychain. No copiar secretos a argumentos, variables visibles, logs o documentación.
- `sa`: identidad reservada, invisible e inmodificable desde la administración ordinaria; su contraseña la define solo el propietario con `provision-sa`. **No modificar su mecanismo** hasta la etapa 10.
- `admin/admin`: política aprobada; se crea una sola vez en el asistente, advierte sin obligar y nunca se restablece.
- Toda operación protegida valida en Go dispositivo, sesión y permisos vigentes en cada petición (sin caché de autorizaciones).
- Tokens: 256 bits; en BD solo SHA-256; contraseñas Argon2id. Auditoría en la misma transacción, sin contraseñas, hashes ni tokens.
- HTTP solo en loopback; LAN exige IP privada explícita, `allow_lan` y TLS con CA propia verificada. No escuchar en `0.0.0.0` ni exponer a Internet.
- Mantener CSP y capabilities de Tauri restringidas; no abrir rutas arbitrarias en el puente ni un cliente web alternativo de producción.
- No ejecutar comandos de shell construidos con datos del cliente (CUPS usa argumentos separados).

## 8–10. Convenciones de código

Obligatorias y detalladas en [AGENTS.md](AGENTS.md) y [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md). Puntos que no se deben olvidar:

- **Prohibido ofuscar, minificar o compactar código fuente** para reducir líneas. La claridad prevalece sobre el número de líneas.
- Go: gofmt (tabulaciones), funciones y bloques `if/else/for/switch` en varias líneas; no concentrar condiciones u operaciones completas en una línea.
- TS/JS/React: Prettier (dos espacios, ver `.prettierrc.json`); cada etiqueta JSX anidada en su propia línea; expandir objetos, condiciones y expresiones complejas que Prettier no expanda.
- Rust, Python y shell: indentación y convenciones propias (`cargo fmt`; Black se usó para Python).
- **Comentarios explicativos en español** para reglas de negocio, validaciones, seguridad, transacciones y lógica compleja; no comentar lo obvio. Mensajes al usuario en español.
- Nombres descriptivos, funciones con responsabilidad clara, evitar archivos o funciones gigantes. Ejecutar `./scripts/dev.sh format` y `format-check` antes de entregar sin cambiar lógica al reformatear.
- Las migraciones SQL aplicadas se excluyen del reformateo.
- Nota: parte del código heredado ya contiene líneas densas (≈1,7 % de las líneas Go superan 140 caracteres, sobre todo SQL y pruebas). No reformatearlo de forma incidental; aplicar las reglas al código nuevo o tocado.

## 11–12. Patrones visuales, temas y pantalla completa

- **Secciones colapsables: siempre `components/Accordion.tsx`** (encabezado completo con título, resumen opcional y chevron; controles montados al cerrar; `inert`/aria; revelado automático de errores; movimiento reducido). No reintroducir `<details>/<summary>` ni convertir todo en acordeones.
- **Teclado virtual**: `components/VirtualKeyboard.tsx`, según preferencia de terminal `auto | always | disabled`; excluye contraseñas y reserva espacio para el formulario.
- **Marca**: `BrandLogo` usa los PNG originales de `assets/brand/` según el tema; no modificar ni sustituir por el futuro logotipo del restaurante.
- **Temas claro/oscuro**: preferencia por usuario guardada en el servidor; `App.tsx` la aplica como `document.documentElement.dataset.theme` y `desktop/src/styles.css` define los tokens bajo `:root` y `:root[data-theme='dark']` (Tailwind 4 vía `@tailwindcss/vite`). Usar esos tokens en pantallas nuevas y comprobar ambos temas.
- **Pantalla completa nativa al iniciar** (`tauri.conf.json` → `fullscreen: true`) con controles visibles de salida/regreso (`components/WindowControls.tsx`) y modo ventana normal. **Sin modo quiosco** ni bloqueo de Spaces/controles de macOS.
- **1280×720 es un escenario de prueba, no un tamaño fijo.** Distribuciones fluidas que aprovechen todo el monitor; conservar formularios y selecciones al redimensionar; desplazamientos internos para mantener visibles total y acción principal.
- Optimizado para táctil: objetivos de toque amplios, sin depender de hover.

## 13–14. Plan de diez etapas y estado

Clasificación del diagnóstico del 8/11/2026 (evidencia completa en [docs/DIAGNOSTICO_ETAPAS_1_5.md](docs/DIAGNOSTICO_ETAPAS_1_5.md)):

| Etapa | Alcance                                  | Estado reportado | Estado verificado (8/11/2026)                                                                                                                                                                                                                                                                                                                                                                                                             |
| ----- | ---------------------------------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Arquitectura y cimientos                 | Terminada        | Implementada con verificaciones pendientes (código y pruebas aprobadas; ciclo nativo de procesos/modos no re-ejecutado).                                                                                                                                                                                                                                                                                                                  |
| 2     | Administración y seguridad               | Terminada        | Implementada con verificaciones pendientes (aceptación PostgreSQL aprobada; Keychain real y ventana nativa no re-ejecutados; H1 y M1 corregidos con pruebas negativas).                                                                                                                                                                                                                                                                   |
| 3     | Catálogo y precios                       | Terminada        | Comprobada (motor y aceptación PostgreSQL aprobados; defecto de límite M5).                                                                                                                                                                                                                                                                                                                                                               |
| 4     | Operación del restaurante                | Terminada        | Comprobada (aceptación PostgreSQL y Playwright Chromium/WebKit aprobadas; sin pantalla táctil física; defecto M2 y menores de interfaz).                                                                                                                                                                                                                                                                                                  |
| 5     | Cocina, comandas e impresión             | Terminada        | Implementada con verificaciones pendientes (cola/SSE/preparación aprobadas con transportes controlados; papel físico nunca confirmado; fullscreen nativo no re-ejecutado; M3 y M4 corregidos; M8 resuelto en etapa 6).                                                                                                                                                                                                                    |
| 6     | Cobros, pagos y devoluciones             | Implementada     | Implementada con verificaciones pendientes (fases 6A–6D en commits locales de `etapa-6`; aceptación PostgreSQL, Vitest, Rust y Playwright Chromium/WebKit aprobados en entorno aislado; 0006 aplicada a la instalación real con autorización tras respaldo y ensayo de restauración; servidor y `.app` 0.6.0 iniciados; recorrido de cobro en la ventana nativa, Keychain, pantalla táctil e impresión física de tickets no verificados). |
| 7     | Administración de caja y cortes          | Pendiente        | —                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| 8     | Inventarios y compras                    | Pendiente        | —                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| 9     | Reportes, indicadores y documentos       | Pendiente        | —                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| 10    | Licenciamiento, respaldos y distribución | Pendiente        | —                                                                                                                                                                                                                                                                                                                                                                                                                                         |

Alcance funcional de cada etapa: el propietario lo entregó en el prompt de diagnóstico y está resumido en el documento de diagnóstico. Respetar las fronteras: no implementar de forma incidental funciones de etapas posteriores (cierre de turno y arqueo, inventario, facturación electrónica, Android, KDS, licencias).

## 15. Decisiones funcionales vigentes

- **Dinero**: `numeric` en PostgreSQL (`numeric(18,2)` en unidades), `shopspring/decimal` en Go, cadenas en JSON; redondeo comercial mitad alejándose de cero; residuos de centavos asignados a los primeros ordinales. `catalog.Calculate` y `QuoteForOrder` son la única autoridad de precios. Moneda única MXN (CHECK en `app.establishment`). `payments` es la única autoridad financiera; el repartidor de centavos `payments.allocate` (residuo mayor, desempate por posición) conserva el total exacto.
- **IVA (decisión del propietario)**: incluido en los precios. Tasa del establecimiento de lista cerrada 16/8/0 (`CHECK` en SQL, validación en Go, botones en la interfaz; nunca captura libre), copiada a cada versión confirmada (`restaurant_item_versions.tax_rate`) y nunca recalculada al cambiar la configuración. Desglose por tasa: base = total/(1+tasa) redondeada, IVA = total − base. Los descuentos generales y las devoluciones se reparten entre tasas en proporción; las devoluciones congelan su desglose; los cargos llevan la tasa vigente; las propinas no llevan IVA. Precuentas y tickets no son CFDI.
- **Equivalencia por valor** con piso en el precio base, extras explícitos separados, tramos incrementales; mínimo de grupo predeterminado cero (se puede retirar todo). Ver [docs/CATALOG.md](docs/CATALOG.md).
- **Pedidos**: borrador persistido ≠ editor local. Confirmar es explícito, compara huella (`expected_fingerprint`) y guarda snapshot comercial por lote. Cada unidad comercial tiene UUID, ordinal y centavos propios (`app.restaurant_item_units`). Dividir/transferir/combinar mueve las mismas unidades sin recalcular ni generar preparación. Ver [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md).
- **Escrituras operativas**: `operation_id` UUID + `revision`; resultado idempotente en `app.restaurant_operations`, auditoría, `restaurant_events` y `realtime_events` en la misma transacción. Nunca regenerar el UUID tras una respuesta incierta.
- **Estados independientes**: preparación, pago (`payment_state`: `not_recorded`, `partial`, `paid`) y entrega (`delivery_state`: `not_delivered`, `delivered`) no se mezclan; los dos últimos los deriva `payments` de registros reales. Pagado no implica entregado; solo se entrega un pedido para llevar liquidado. Ventanilla es un origen de `takeaway`. La cuenta admite `closed`: salón al liquidarse sin borrador, para llevar al entregarse liquidada; una devolución que deja saldo la reabre.
- **Partidas con pago (decisión del propietario)**: una unidad con asignación neta de pago o descuento manual vigente no se transfiere, divide, combina, cancela, edita ni anula; ninguna operación deja la cuenta con total menor que lo pagado; combinar exige origen sin pagos; anular exige cobros netos en cero. Guardas en `payments/guards.go`, usadas por `operations/payment_guards.go`.
- **Cobros**: registros financieros inmutables con compensaciones enlazadas (devoluciones, cancelaciones de ajustes, reversos de corrección). Pagos mixtos = pagos sucesivos; efectivo con `recibido ≥ importe + propina`; el excedente nunca se vuelve propina. Turno mínimo: uno abierto por caja, requerido para cobrar o devolver; `shared_shifts` falso por omisión. Una corrección directa solo procede con el turno original abierto. La autorización de supervisor es puntual (sin sesión, con límite de intentos, sin autoautorización) y queda ligada al `operation_id`; la huella idempotente excluye su contraseña. Límite de descuento por rol en `role_discount_limits`.
- **Cocina**: una liberación canónica por unidad (`kitchen_unit_releases`), base del consumo teórico futuro. Cola de impresión: intento/lease confirmados antes de imprimir, serialización por impresora, `uncertain` exige resolución humana y nunca se reenvía solo; `submitted` = aceptación CUPS/TCP, **no** papel físico. Ver [docs/KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md).
- **Tiempo real**: SSE (no WebSocket) para invalidaciones; comandos por HTTP idempotente; Rust entrega frames por `Channel`; el sondeo sigue como recuperación.
- **Cajas**: el asistente crea **Caja 1** sin turnos ni fondos; las terminales pueden asociarse a una caja (`devices.register_id`). Una caja con turno abierto no se desactiva y su terminal no cambia de caja hasta el cierre (etapa 7).
- **Futuras (ya decididas por el propietario)**: las diferencias de caja no bloquean el cierre ni se exige liquidar cuentas para cerrar turno (etapa 7); existencias teóricas negativas no bloquean ventas.

## 16. Separación de proyectos

1. **Servidor + POS** — este repositorio (Go, PostgreSQL, React/Tauri).
2. **Android** (mesero / para llevar, SQLite y sincronización offline) — repositorio independiente.
3. **KDS** (estaciones de cocina) — aplicación independiente; consumirá los contratos ya preparados (`/kitchen/board`, `/kitchen/tickets`, `/kitchen/tickets/{id}/ack`, `/kitchen/states`, `/events`).
4. **Servidor de licencias** — servicio independiente; aquí solo existe el límite `internal/licensing`.

No desarrollar los proyectos 2–4 aquí. La futura herramienta de migración SQL Server → PostgreSQL vivirá en este repositorio pero **separada de la aplicación comercial** (sin compartir binario ni rutas API).

## 17. Compatibilidad y distribución

- Desarrollo y pruebas actuales: **solo macOS Apple Silicon** (macOS 26.5.2; el bundle declara mínimo 14.0). Intel no probado.
- **Windows no está probado ni preparado**; no afirmar compatibilidad. Prepararlo solo cuando el propietario lo pida, pero diseñar sin bloquearlo: rutas con `filepath`, sin supuestos de señales POSIX ni `/dev/tty` en código nuevo, transportes de impresión detrás de interfaces (hoy CUPS usa `/usr/bin/lp*`), Keychain abstraído en `vault.rs`.
- Producción bloqueada por diseño: `licensing.CheckStartup` solo permite `environment=development` + `license_mode=development`.
- Bundle ID provisional `local.adel.pos.dev`; sin firma/notarización, sin DMG ni instalador; launchd solo como plantilla.

## 18. Leer antes de desarrollar

1. [AGENTS.md](AGENTS.md) (obligatorio) y [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md).
2. [ARCHITECTURE.md](ARCHITECTURE.md), [API_CONTRACTS.md](API_CONTRACTS.md), [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md).
3. Según el área: [docs/CATALOG.md](docs/CATALOG.md), [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md), [docs/KITCHEN_AND_PRINTING.md](docs/KITCHEN_AND_PRINTING.md), [docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md).
4. [docs/VERIFICATION.md](docs/VERIFICATION.md) (qué está probado y qué no) y [docs/DIAGNOSTICO_ETAPAS_1_5.md](docs/DIAGNOSTICO_ETAPAS_1_5.md) (riesgos abiertos).
5. Código de referencia para nuevos módulos: `backend/internal/administration/routes.go` (delegación de rutas), `backend/internal/administration/delegation.go` (reglas de delegación de permisos), `backend/internal/operations/handler.go` (locks, idempotencia), `backend/internal/operations/context.go` (permisos del módulo), `backend/internal/events/store.go` (outbox), `desktop/src-tauri/src/api.rs` (lista cerrada), `desktop/src/hooks/useRestaurantMutation.ts` y `desktop/src/hooks/pendingOperations.tsx` (reintento con mismo UUID y almacén global de pendientes), `backend/internal/payments/handler.go` (rutas, permisos e idempotencia financiera).

## 19. Riesgos y limitaciones conocidos

Lista priorizada y evidencias en [docs/DIAGNOSTICO_ETAPAS_1_5.md](docs/DIAGNOSTICO_ETAPAS_1_5.md) (IDs H/M/B). **Corregidos**: H1, M1, H2, M3, M4 y B1 antes de la etapa 6 (§11 del diagnóstico); H3, M8 y B9 durante la etapa 6, M2 y M11 en la captura de cobros y B14 en la autorización de supervisor (§12 del diagnóstico). Los demás siguen abiertos; corregirlos solo cuando el propietario lo solicite. Resumen para no olvidar al desarrollar:

- **H1 y M1 · corregidos**: la delegación vive en `administration/delegation.go`. Nadie concede, asigna o edita permisos efectivos que no posee, administra a usuarios con más permisos ni modifica su propio rol o excepciones; debe permanecer un administrador completo (todos los permisos registrados, por permisos efectivos). Todo permiso nuevo de etapas futuras amplía ese conjunto: concederlo al rol `administrator` en la misma migración, o los administradores dejarán de contar como completos.
- **H2 · corregido**: `runProtected` ejecuta cada operación autenticada en un savepoint y un 403 revierte todo lo escrito (solo se confirma la auditoría `access.denied`). La regla sigue siendo autorizar **antes de la primera escritura**; el savepoint es una red de seguridad, no un sustituto.
- **H3 · resuelto**: las guardas de partidas con pago (§15) viven en `payments/guards.go`; toda operación nueva que mueva, cancele o recotice unidades debe llamarlas en la misma transacción.
- **M2 y M11 · resueltos en cobros**: importes en texto con `inputMode="decimal"` y `services/amountInput.ts`; teclado numérico fijo en el cobro y exclusión del teclado flotante con `data-virtual-keyboard="off"`. Siguen abiertos fuera de cobros: la cantidad del producto (`type="number"`), el espacio reservado solo en modo operación y objetivos táctiles menores de 44 px en pantallas anteriores.
- **M8 · resuelto**: `hooks/pendingOperations.tsx` conserva operación, cuerpo y `operation_id` a nivel de aplicación (cobros con clave `payments:<cuenta>`, cocina con `kitchen`) y descarta la contraseña de autorización. No sobrevive al cierre del proceso.
- **Cocina · M3 y M4 corregidos**: editar datos de la cuenta ya no revierte la reprogramación de cocina (solo un cambio de `scheduled_at` la sustituye); el respaldo exige tres errores confirmados consecutivos.
- **Catálogo**: el límite de 200 productos por cotización cuenta todos los productos ligados a complementos (M5).
- **Contrato**: restaurante devuelve `ACCOUNT_CONFLICT`, aunque la documentación dice `REVISION_CONFLICT` (B2).
- **Escrituras serializadas globalmente**: restaurante, cocina, planificador y cobros comparten el lock exclusivo 7820148305. Correcto para consistencia; no medido bajo carga.
- **Instalación real en esquema 6** (8/11/2026): respaldo `backups/adel_pos-20261108T191622Z-57c0f5.dump` con restauración verificada (63 tablas idénticas), `migrate` `applied:1` y repetición `applied:0`, readiness 200 con esquema compatible; servidor y `.app` 0.6.0 iniciados. La 0006 es ahora inmutable; la siguiente ampliación requiere 0007.
- **Cobros sin cierre de turno**: `expected_cash` no es un arqueo; un turno abierto permanece abierto hasta la etapa 7 y bloquea desactivar su caja o cambiar la caja de su terminal.
- **Autorización reintentada**: un reintento incierto de una operación autorizada viaja sin contraseña; si el original no se confirmó, Go responde `403 AUTHORIZATION_REQUIRED` y el supervisor debe autorizar de nuevo.
- **SSE por sondeo de BD**: cada conexión consulta PostgreSQL cada ~750 ms (flujos de 30 s, medidos); límite de 2 conexiones por dispositivo mantenido en memoria del proceso.
- **Apagado (reproducido)**: con un flujo SSE abierto, `Shutdown` agota sus 5 s y el proceso termina con código 1 (relevante para futuros servicios del sistema).
- **Sin retención** de auditoría, `realtime_events`, `restaurant_operations`, sesiones ni intentos; los snapshots por lote crecen sin límite.
- **Respuestas IPC limitadas a 2 MiB**; cuentas/historiales muy grandes requieren paginación de detalle.
- **Impresión**: aceptación CUPS de `Printer_POS_80` (trabajo 219) sin confirmación física de papel ni corte; ESC/POS TCP sin impresora de red real; transporte agente no implementado.
- **Sin pruebas** de LAN con otra máquina, muchas terminales, corte eléctrico, pantalla táctil física, Mac Intel ni Windows.
- **Pendientes de distribución**: usuario de BD de ejecución separado del migrador, rotación de certificados, respaldos periódicos, firma/notarización, servicio sin sesión, licencias.

## 20. Criterios para agregar funcionalidad sin romper lo anterior

1. **Esquema**: nunca editar 0001–0006. La siguiente migración es `0007_…sql`, transaccional, sin datos ficticios. Las restricciones CHECK sin nombre (p. ej. estados de `restaurant_accounts`, reemplazado en la 0006) se modifican con `ALTER TABLE … DROP/ADD CONSTRAINT` en una migración nueva. Antes de aplicarla a la instalación real: `backup` + `restore-check.py` y autorización del propietario.
2. **Permisos**: añadir la clave al `Permissions` del módulo (se agrega al registro central en `administration/permissions.go`) **y** en la migración (`INSERT INTO app.permissions` + concesión al rol `administrator`), porque las instalaciones existentes no vuelven a ejecutar el asistente. No asignar por inferencia a otros roles.
3. **Rutas**: delegar desde `administration/routes.go` con la misma transacción, `Actor` y callback de auditoría; **comprobar todos los permisos antes de cualquier escritura** (un 403 revierte la operación mediante savepoint, pero no debe usarse como control de flujo); validar UUID, `revision` y `operation_id`; registrar el resultado idempotente en la misma transacción; publicar en `realtime_events` dentro de la transacción; respetar el orden de locks (§3); traducir los errores del módulo a `Fault` en `routes.go` (como ya se hace con `payments.Error` y `kitchen.Error`).
4. **Dinero**: solo `decimal` en Go y cadenas en JSON; reutilizar el motor de `catalog` y los importes históricos de `restaurant_item_units`; nunca aritmética monetaria con `number` en TypeScript.
5. **Puente nativo**: cada operación nueva se agrega a la lista cerrada de `api.rs` (con prueba Rust), respetando el formato que lee el driver e2e; nunca rutas arbitrarias ni tokens en React.
6. **Frontend**: tipos en `desktop/src/services/*.ts`; reutilizar `Accordion`, `VirtualKeyboard`, `Forms`, `DecimalField`, `useResource`/`useLiveResource`, `useRestaurantMutation`; conservar selección local ante sondeo/conflicto; las operaciones inciertas que deban sobrevivir a la navegación no pueden vivir en el estado de una página que se desmonta; importes en campos de texto `inputMode="decimal"`, nunca `type="number"`; objetivos táctiles ≥ 44 px; temas claro/oscuro con tokens existentes; layout fluido.
7. **Pruebas**: unitarias + aceptación con PostgreSQL (`//go:build integration`) para transacciones, concurrencia e idempotencia; actualizar `scripts/database.py` si hay paquete nuevo; Vitest para componentes; ampliar Playwright si cambia el recorrido operativo. Repetir las suites de etapas anteriores.
8. **Documentación**: actualizar README, ARCHITECTURE, API_CONTRACTS, la guía del módulo y una nueva `docs/VERIFICATION.md` (archivando la anterior como `VERIFICATION_STAGE6.md`) **solo con lo realmente ejecutado**; no presentar pendientes como completos. Versionado en `config/product.json` + `sync`.
9. **Alcance**: no modificar `sa`, admin/admin, marca ni etapas anteriores sin solicitud expresa; no adelantar funciones de etapas futuras; preguntar ante contradicciones.
