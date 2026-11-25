# Etapa K1 · Avance

Aplicación de cocina (KDS) para Android y la corrección del aviso falso «Reconectando» del escritorio:

- una aplicación Android que funciona **solo como pantalla de cocina**, para tabletas en horizontal, conectada a la
  computadora principal;
- lo compartido con la aplicación de meseros en `android-core/`;
- el aviso «Reconectando» que aparecía cada unos 30 segundos con la computadora en línea (§7).

El modo Cocina del escritorio no se toca en esta etapa.

Especificación: [docs/etapas/ETAPA_K1_KDS_ANDROID_v3.md](etapas/ETAPA_K1_KDS_ANDROID_v3.md).

Rama `etapa-k1`, creada desde `main` (`2f715d4`, con la L1 fusionada):

- **Al empezar**: servidor 0.15.0 (esquema 28) y aplicación de meseros 0.4.0 (`versionCode` 4).
- **Al terminar**: servidor **0.16.0**, aplicación de cocina **0.1.0** (`com.adelpos.cocina`, `versionCode` 1) y
  aplicación de meseros **0.4.1**.

Cómo se trabaja:

- Coordinador y agentes, cada uno en su árbol de trabajo. **Máximo dos agentes trabajando a la vez**; los agentes no
  lanzan otros.
- Se prueba en copias aisladas: el clúster temporal 55499 y un Go temporal en modo de pruebas.
- Android, en `Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal.
- **Ninguna prueba toca el servicio real de la Mac** ni llama a `licensing.sicrat.com`.
- Ningún commit lleva líneas de atribución.

---

## Fase 0 · Verificación, estructura del proyecto y plan

Agente único de la Fase 0, 24/11/2026, en `$SP/wt/k1-p0` (rama `etapa-k1-p0`, desde `etapa-k1` `eb1fe7b`). Sin
cambios en el repositorio: solo lectura del código, una prueba de concepto de Gradle fuera del repositorio
(`$SP/k1/spike/`) y un Go temporal (`ISO_TAG=P0`, 8201/8461) para medir el canal de eventos. Las rutas son del árbol
(`backend/…`, `desktop/…`, `android/…`); los números de línea son los de `eb1fe7b`.

### Resumen

- **El KDS de la 9F ya tiene casi todo lo de Go**: tareas por unidad y estación, liberaciones, estaciones por terminal,
  avance a «Listo» con revisiones, cancelaciones y modificaciones integradas, el aviso al mesero y los eventos por
  estación. **No existen**: la tarjeta de pedido, «Recuperar» (Go prohíbe retroceder un «Listo»), la lista de
  recientes, los límites de color generales (hoy son por estación) ni el cierre automático de una tarjeta.
- **«Recuperar» y volver a avisar chocan con `UNIQUE (unit_id, version_id)`** de `app.ready_notices`: hoy un segundo
  «Listo» de la misma preparación no avisa. Propuesta: recuperar borra los avisos de esas unidades (el que no llegó ya no
  llega) y volver a marcar inserta uno nuevo; la restricción se queda.
- **Envío por estación ya existe** desde la 0005: `kitchen_stations.reception` (`printer`, `kds`, `both`) con
  «Recepción de comandas» en Cocina › Estaciones. Solo se verifica; la impresión no cambia.
- **El tipo de dispositivo `kitchen` ya está reservado** en los `CHECK` de la 0023 y en `DeviceKinds`, sin ningún uso
  real. Propuesta: la pantalla de cocina Android es `kind = 'kitchen'` con `X-Client-Kind: kitchen`, sin usuario ni
  sesión (como el agente de impresión y los avisos de listo), con una lista cerrada de rutas en un punto único de
  `dispatch`.
- **Causa del «Reconectando» falso (reproducida)**: Go cierra **siempre** el canal `GET /api/v1/events` a los
  **30,0 s** (`administration/events.go:118`) y el puente de Rust avisa `{"connection":"reconnecting"}` en **todo** fin
  de canal, también en ese cierre normal (`desktop/src-tauri/src/realtime.rs:132`), espera 500 ms y vuelve a abrir. La
  barra lo traduce a «Reconectando» (`desktop/src/navigation/serverState.ts:25-28`). Medido: hueco de ~0,6 s cada
  30 s. Playwright nunca lo vio porque su puente no manda «reconnecting» (`desktop/e2e/native-driver.ts:769`).
- **Estructura**: `android-core/` como build de Gradle propio incluido con `includeBuild` desde `android/` y `cocina/`,
  con **un solo catálogo de versiones** (el de `android-core`). Probado sin red con Gradle 9.9.1, AGP 9.5.1 y Kotlin
  2.4.21: compila, los bloqueos del build incluido se respetan y la verificación la decide el archivo del build raíz.

### 1. KDS de la 9F (servidor y escritorio)

#### Rutas, permisos, eventos y tablas

- **Rutas que usa Preparación** (`desktop/src/pages/kitchen/PreparationBoard.tsx`):
  - `GET kitchen/board?station_id&state&page` (`:43-45`, operación `kitchen_board` de `desktop/src-tauri/src/api.rs:77`);
  - `POST kitchen/states` (`:72`, `kitchen_save_states`);
  - administrativas: `POST kitchen/releases/{id}/release`, `…/schedule` y `POST kitchen/changes/{id}/retry`
    (`:271`, `:311`);
  - el cupo: `POST kitchen/display-mode {active}` (`PreparationSeat.tsx:9`, `licenseOperations.ts:92`).
- **Permisos** (`backend/internal/kitchen/handler.go:21-44`, rutas en `:148-212`): `kitchen.states.read` (tablero),
  `kitchen.states.manage` (avanzar), `kitchen.stations.all` (todas las estaciones y lo administrativo),
  `kitchen.kds.ack` (`POST kitchen/tickets/{id}/ack`), `events.read`.
- **Escrituras**: lock 7820148305 exclusivo (`handler.go:72`), idempotencia en `app.restaurant_operations` con
  `actor_id` del usuario (`handler.go:95-99`, `:119-121`).
- **Eventos** (solo invalidación, sin contenido; todos por el canal que ya existe):
  - `kitchen.released`, ámbito `kitchen`, sin estación (`kitchen/releases.go:188`);
  - `kitchen.ticket.created` por estación: comanda de un envío, de una modificación o de una cancelación
    (`kitchen/documents.go:157`);
  - `kitchen.state.changed` por estación al avanzar (`kitchen/states.go:135-138`) y `kitchen.states.changed` sin
    estación (`handler.go:129-134`);
  - `kitchen.change.pending` por estación (`kitchen/changes.go:105`);
  - `restaurant.account.preparation_changed`, ámbito `restaurant` o `takeaway` (`kitchen/preparation.go:112-123`).
  - **Ojo**: un evento de `kitchen` **sin estación** solo llega a quien tiene `kitchen.stations.all`
    (`events/store.go:112-114`).
- **Tablas** (0005): `kitchen_settings` (`:2-16`), `kitchen_stations` con `reception` (`:38-55`),
  `kitchen_device_stations` (`:72-77`), `kitchen_releases`, `kitchen_unit_releases`, `kitchen_tasks` (`:127-142`),
  `kitchen_tickets`, `kitchen_kds_receipts` (`:155-162`, con `user_id NOT NULL`) y `kitchen_history` (`:199-209`,
  `operation_id` con llave hacia `restaurant_operations`). 0022 agrega `devices.kitchen_all_stations` y
  `devices.kitchen_sound_alert` (`:45-50`) y las estaciones por usuario.

#### Unidad de trabajo y su relación con «platillo» y «pedido» (§4)

- **`kitchen_tasks`**: una fila por **unidad × estación** (`UNIQUE (unit_id, station_id)`), con su versión, estado
  (`released`, `preparing`, `ready`, `cancelled`), instrucciones, `released_at` y `ready_at`.
- **`kitchen_releases`**: una por cada «Enviar a cocina» (evento `order.confirmed`, `releases.go:296-312`) y otra por
  cada **agregado** de una modificación (`changes.go:115-119`, `createRelease` con `added_unit_ids`).
- **«Platillo» de la §4** = las tareas de una misma partida en una estación (el escritorio las agrupa por liberación,
  estación, partida, versión y estado: `PreparationBoard.tsx:49-53`; una cantidad de 3 son tres tareas).
- **«Pedido» (tarjeta) de la §4**: **no existe**. El escritorio muestra una tarjeta por platillo, no por pedido.
  Propuesta en «Decisiones propuestas», 5.

#### Qué existe de cada cosa de la §4

| Pieza                        | Hoy                                                                                                                                                                                                                              |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Marcar un platillo listo     | Sí: `POST kitchen/states` con `tasks[{id, revision}]` y `state: ready` (`states.go:57-87`).                                                                                                                                      |
| Marcar el pedido completo    | Solo «por estación» en las estaciones con seguimiento `order` (`release_id` + `station_id`, `states.go:34-55`); con seguimiento `item` se rechaza tocar el pedido y en `order` se rechaza tocar un platillo (`states.go:83-85`). |
| Recuperar (deshacer «Listo») | **No.** Go lo prohíbe: «No se puede retroceder…» (`states.go:109-111`; `docs/KITCHEN_AND_PRINTING.md:111`).                                                                                                                      |
| Lista de recientes           | Solo el filtro «Listos» del tablero (`state=ready`, sin ventana de tiempo, `reads.go:136-137`).                                                                                                                                  |
| Cancelaciones                | La tarea pasa a `cancelled` (`changes.go:166`) y sale una comanda de cancelación. El tablero por omisión **las oculta** (`reads.go:137`); con «Todos los estados» se ven como «Cancelado».                                       |
| Agregados después del envío  | Son otra liberación; el escritorio no los distingue (solo el aviso sonoro, `preparationModel.ts:103-117`).                                                                                                                       |
| Modificaciones               | Si cambia la receta, la tarea vuelve a `released` con `ready_at` vacío y una versión nueva (`changes.go:180-191`); el «antes» solo queda en la comanda de modificación.                                                          |
| Modificadores y notas        | Sí: `instructions.parts` (variante, ingredientes, grupos, indicaciones rápidas, notas) e `instructions.notes` (`kitchen/models.go:135-160`).                                                                                     |
| Mesa / para llevar y cliente | El tablero da `account_name`, `table_name` y `folio` (`reads.go:124-129`), no `mode` ni `customer_name`; la liberación guarda el contexto completo (`context_snapshot`, `releases.go:47-53`).                                    |
| Mesero                       | No en el tablero. La liberación guarda quien envió (`employee`); la cuenta tiene su responsable (`assigned_user_id`, el que usa el aviso).                                                                                       |
| Hora de envío y espera       | `released_at` y `elapsed_seconds` calculado por Go (`reads.go:129`), con `server_time`.                                                                                                                                          |
| Cierre automático del pedido | No existe como tarjeta; Go sí calcula que la **cuenta** está lista (`states.go:158-210`).                                                                                                                                        |

#### Límites de color por tiempo

- Hoy son **por estación**: `target_minutes`, `warning_minutes` y `overdue_minutes` de `kitchen_stations`
  (`0005:46-48`), y la regla del escritorio es una función de presentación (`preparationModel.ts:14-29`: ámbar desde la
  advertencia o el 75 % del objetivo, rojo al pasar el objetivo; decisión D7 de la 9F).
- **No hay** límites generales de 10 y 20 minutos. La §4 y la §6 los piden en la configuración general de cocina,
  comunes a todas las pantallas: van como columnas nuevas de `kitchen_settings` en la 0029. El modo Cocina del
  escritorio sigue con los de cada estación.

#### `internal/stationscope`

- Terminal: `devices.kitchen_all_stations` o su selección en `kitchen_device_stations`; usuario:
  `users.kitchen_all_stations` o `kitchen_user_stations`; con `kitchen.stations.all`, todas
  (`stationscope/scope.go:66-125`). Se ve la intersección (D6 de la 9F). La usan cocina (`kitchen/context.go:146-157`)
  y el canal de eventos (`administration/events.go:42-49`).
- **Pantalla sin usuario**: `stationscope.Load(ctx, tx, deviceID, "", true)` da exactamente la selección de la
  pantalla (con `allByPermission` el lado del usuario es «todas»). No hace falta un paquete nuevo. Las estaciones de una
  pantalla se eligen con lo que ya existe (`PUT kitchen/access`, Cocina › Estaciones › Terminales de cocina,
  `TerminalStations.tsx`); la §6 pide además verlas y cambiarlas en Terminales.

#### Qué lógica está en Go y cuál en el escritorio

- **Go**: el alcance por estación, el tablero (filtros, tiempos, seguimiento heredado, precios escondidos), las reglas
  de avance (revisiones, `item` u `order`, nada de retroceder, versión pendiente), liberar y programar, integrar
  modificaciones y cancelaciones, el estado de preparación de la cuenta, el registro de los avisos y los eventos.
- **Escritorio** (solo presentación): agrupar tareas en tarjetas, el color por tiempo, qué partes se muestran
  (`visibleParts`), el texto de la cuenta y la mesa, la detección de llegadas para el sonido y el aviso sin estaciones
  en común (`preparationModel.ts`).
- Para la aplicación de cocina, **la tarjeta de pedido, «NUEVO», «CANCELADO», el cierre automático, los recientes y
  recuperar se calculan en Go** (paquete `kitchen`, con las mismas funciones de la 9F). El dispositivo solo calcula el
  color con el reloj, con los límites, `sent_at` y `server_time` que da Go, como hace el escritorio.

### 2. Aviso de platillo listo de la A2

- **Cómo llega**: toda escritura de cocina termina en `publishPreparation` (`handler.go:116`) →
  `recordReadyNotices` (`kitchen/preparation.go:159-199`) inserta en `app.ready_notices` cada unidad cuyas tareas vivas
  de su versión están listas y la última se marcó en esta transacción (`last_ready = now()`), con el responsable de la
  cuenta y el dispositivo fijado (`:180-187`: `kind = 'android'`, avisos activados y `app_version` 0.2.0 o posterior).
  El dispositivo lee `GET ready-alerts` y `GET ready-alerts/stream` (`administration/ready_alerts.go`; revisión cada
  750 ms, `: ping` cada 15 s, 5 minutos de vida, `:40-44`) con solo su credencial.
- **Restricción**: `CONSTRAINT ready_notices_unit_version_key UNIQUE (unit_id, version_id)`
  (`0026_android_a2.sql:78`) y `ON CONFLICT (unit_id,version_id) DO NOTHING` (`preparation.go:191`).
- **Qué pasaría hoy con recuperar y volver a marcar** (si se permitiera): recuperar no inserta nada (bien: no avisa).
  Volver a marcar pone `ready_at = now()` (si recuperar lo dejó vacío; `states.go:115-117` conserva un `ready_at`
  anterior) y `recordReadyNotices` intenta insertar la misma `(unit_id, version_id)`: **conflicto y ningún aviso**. Choca
  con la §3 («si se vuelve a marcar, sí»).
- **Propuesta** (Decisión propuesta 8): recuperar **borra** las filas de `ready_notices` de esas unidades y versión, en
  la misma transacción y con el mismo lock 7820148305. Así:
  - un aviso que el dispositivo todavía no leyó ya no llega (no avisa de algo que se recuperó);
  - uno ya mostrado no se retira (el mesero ya lo vio; recuperar no manda nada);
  - volver a marcar inserta una fila nueva (otro `sequence` y otra `alert_key`): llega como aviso nuevo;
  - la restricción única se queda y sigue impidiendo repetir la misma preparación.

  La alternativa (una columna de ronda en la llave) deja vivo el aviso recuperado y obliga a filtrarlo al leer.

### 3. Emparejamiento y dispositivos

#### Flujo y tipo de dispositivo

- **Flujo** (9G, D2 y D5): `POST pairing/requests` con `device_kind`, `os`, `app_version` y el compromiso; revelar el
  número; aprobar con el código en Terminales (`devices/pairing-requests…`, `devices.manage`, solo por la escucha
  local); reclamar la credencial una vez (`administration/pairing.go`). La aplicación de meseros lo hace en
  `PairingRepository` con `deviceKind = ClientIdentity.KIND` (`android/core/data/…/access/PairingRepository.kt:352`).
- **Tipo**: `devices.kind CHECK (kind IN ('desktop', 'android', 'kitchen'))` y lo mismo en
  `device_pairing_requests.device_kind` (`0023:218-220`, `:249`); `DeviceKinds` (`pairing.go:90-92`). **`kitchen` está
  reservado y nadie lo usa**: el escritorio siempre empareja como `desktop` (`desktop/src-tauri/src/pairing.rs:497`) y
  Terminales ya tiene la etiqueta «Cocina» (`desktop/src/pages/DevicesPage.tsx:54`).
- **`X-Client-Kind`**: `httpapi.ClientKinds = {"desktop", "android"}` (`httpapi/clientversion.go:53`); otro valor es
  `400 CLIENT_KIND_INVALID` en toda ruta versionada, también el emparejamiento (`:143-150`). `matchClientKind`
  (`administration/service.go:474-491`) exige `android` a un dispositivo `android` y `desktop` a `desktop` **y a
  `kitchen`**; si no, `400 CLIENT_KIND_MISMATCH`.
- **Versión mínima por tipo**: `config/product.json` `min_client_versions {desktop: 0.10.0, android: 0.1.0}` →
  `GET version` (`httpapi/router.go:123-126`); un tipo sin mínima usa la general.

**Cómo se agrega la pantalla de cocina** (Decisión propuesta 3): tipo de dispositivo `kitchen` (el reservado),
`X-Client-Kind: kitchen` (se agrega a `ClientKinds`), `min_client_versions.kitchen = "0.1.0"`, `matchClientKind`
exige `kitchen` a los dispositivos `kitchen`, capacidad nueva `kitchen-display-v1` y la etiqueta «Pantalla de cocina»
en Terminales. Un servidor 0.15.0 responde `CLIENT_KIND_INVALID` a la aplicación de cocina: ella lo dice como
«Actualiza el servidor del restaurante», igual que hace la de meseros (`android/CLAUDE.md:374-380`).

#### Sesión de dispositivo sin PIN con solo permisos de cocina

- **Lo parecido que ya existe**:
  - el agente de impresión: `Principal{DeviceID}` sin usuario, con su propio punto de entrada `kitchen.HandleAgent`
    (`administration/print_agent.go:15-27`, despachado en `service.go:413`);
  - los avisos de listo: solo la credencial del dispositivo (`service.go:368`, `:418`);
  - `WaiterPermissions`: una lista cerrada que recorta los permisos de una sesión con PIN (`waiter_scope.go:13-57`).
  - La auditoría ya sabe registrar un autor «terminal / device» sin usuario (`service.go:213-218`).
- **Lo que impide una sesión normal sin usuario**: `sessions.user_id`, `restaurant_operations.actor_id`
  (`0004:83-91`) y `kitchen_kds_receipts.user_id` son `NOT NULL` con llave a `users`; el cupo de cocina exige
  `session_id` (`0028:93-97`).
- **Propuesta** (Decisión propuesta 4): **principal de dispositivo**, sin usuario ni sesión:
  1. En `dispatch`, justo después de `authenticateDevice` y `matchClientKind` (`service.go:373-379`) y **antes** de
     `auth/login`, `auth/pin-users`, `auth/pin-login`, la credencial de envío, `print-agent/*`, `ready-alerts`,
     `license/status` y `authenticateSession` (`:384-426`): si el dispositivo es `kitchen`, se atiende **solo**
     `service.kitchenDisplay(…)` y todo lo demás responde `403 KITCHEN_DISPLAY_ONLY` (auditado, como `access.denied`).
     La lista cerrada vive en un solo lugar: `kitchen-display/*`, `events` y `events/snapshot` y el resumen de la
     licencia. Aunque la tableta mande un `Authorization` robado, nunca llega a `authenticateSession`.
  2. Al revés: `kitchen-display/*` con un dispositivo que no es `kitchen` responde `403 KITCHEN_DISPLAY_REQUIRED`.
  3. Los permisos del principal salen de una **lista cerrada en código** (`KitchenDisplayPermissions`:
     `kitchen.states.read`, `kitchen.states.manage`, `events.read`), nunca de un rol: nadie puede ampliarlos desde
     Personal.
  4. Las estaciones: `stationscope.Load(deviceID, "", true)` y además solo las estaciones con `reception` `kds` o
     `both` (punto 6).
  5. La lógica de cocina se llama con un punto de entrada nuevo del paquete `kitchen` (`HandleDisplay`, como
     `HandleAgent`) que reutiliza las funciones de la 9F (tablero, avance, alcance, avisos), sin copiarlas.
  6. Idempotencia sin `restaurant_operations`: las marcas son **idempotentes por estado** (marcar listo lo que ya está
     listo no cambia nada; cada tarea va con su `version_id`, y una versión distinta no se marca y se informa).
     `kitchen_history` se escribe con `operation_id` vacío y el `device_id` (la columna admite `NULL`).
  - Alternativa descartada: un usuario interno por pantalla. Habría que esconderlo en cada lista de personas (hoy hay
    nueve consultas con `NOT u.support`) y un rol editable podría darle más permisos.

#### Versión instalada (L1-D64)

- **Hoy**: `devices.app_version` (`0023:223-224`, `CHECK length <= 40`) se escribe solo al reclamar el emparejamiento
  (`pairing.go:547-548`); Terminales la muestra como «versión …» (`DevicesPage.tsx:480`). Su único uso de negocio es
  elegir el dispositivo del aviso (`preparation.go:185-186`).
- **Propuesta** (Decisión propuesta 9): `Request` lleva también `X-Client-Version` (como ya lleva `ClientKind`,
  `administration/http.go:119`, `events.go:63`, `ready_alerts.go:288`), y `authenticateDevice` (`service.go:444-468`),
  que ya actualiza `last_seen_at` en cada petición autenticada, guarda la versión cuando cambia
  (`… app_version = $2 WHERE id = $1 AND app_version IS DISTINCT FROM $2`, solo una versión que `httpapi` ya validó y
  de 40 caracteres o menos). Sirve para las tres aplicaciones (escritorio, meseros y cocina) sin cambiar ningún
  cliente: todas mandan `X-Client-Version` en cada petición. Terminales dice «Versión instalada». Prueba: actualizar
  encima y ver la versión nueva sin emparejar (§8).

#### Revocar

- `POST devices/{id}/revoke` marca `revoked_at`, cierra sus sesiones y sus credenciales de envío
  (`administration/devices.go:133-150`). Desde ahí `authenticateDevice` responde `401 DEVICE_REVOKED`
  (`service.go:456-458`).
- Los canales lo notan en menos de un segundo, porque cada lectura vuelve a autenticar: `events` manda
  `event: interrupted` con `{error, status}` y termina (`events.go:131-137`); `ready-alerts/stream` manda
  `interrupted` con `{code}` (`ready_alerts.go:348-352`).
- La aplicación de meseros lo recibe en el punto único `report(...)`: `DEVICE_REVOKED` retira el dispositivo (pasa lo
  no enviado a la bitácora, borra sus datos y destruye la llave) y muestra «dispositivo retirado»
  (`android/CLAUDE.md:364-373`).
- La de cocina hace lo mismo con el código compartido (`android-core`) y vuelve a la pantalla de emparejamiento.

### 4. Licencia y cupo `kitchen_displays`

- **Hoy** (L1-D7, L1-D13, L1-D28):
  - el cupo lo ocupa el escritorio al entrar a Preparación con `POST kitchen/display-mode {active: true}`
    (`administration/license_seats.go:150-179` → `licenseseats.SetKitchenDisplay`, `store.go:249-258`);
  - la fila apunta a la sesión que entró y cuenta mientras esa sesión siga vigente (`store.go:22-31`, `0028:79-97`);
  - se libera al salir, al cerrar o vencer esa sesión, con «Liberar cupo» (`POST devices/{id}/seat/release`,
    `license_seats.go:181-216`, que cierra sus sesiones y llama a `ReleaseDevice`, `store.go:296-304`) y al revocar
    (L1-D36);
  - la clasificación única es `licenseseats.ForOperation` (`operations.go:80-109`), aplicada en `withSeats`;
  - lock 7820148311 siempre al final (`seats.go:110-115`).
- **Cómo encaja la pantalla Android** (Decisión propuesta 6):
  - **Ocupar al conectarse**: una ruta propia, `POST kitchen-display/connect`, clasificada en `ForOperation` con un
    efecto nuevo que ocupa `kitchen_displays` con el mismo `claim` y el mismo lock (asignación atómica). Las lecturas de
    pedidos exigen el cupo: sin él, `409 LICENSE_SEAT_UNAVAILABLE` con `in_use` y `limit`, y la tableta muestra el texto
    de la §6.
  - **Ligado al dispositivo, no a una sesión**: la 0029 deja `session_id` vacío para `kitchen_display` y la condición
    «en uso» (`inUseCondition`, `store.go:22-31`) cuenta esa fila mientras el dispositivo no esté revocado. Un corte de
    red no libera nada (no hay sesión que venza).
  - **Liberar**: al revocar (la condición deja de contarla y la revocación la borra) y con «Liberar cupo»
    (`ReleaseDevice` ya borra la fila de cocina). Qué hace la tableta después de «Liberar cupo» es la Duda 5.
- **Tabla de operaciones** (`licensing/operations.go`):
  - `RowKitchen` (F5) se permite en Vigente, Tolerancia, Vencida y Revocada y se niega sin activar (`:97-110`). Avanzar
    estados es `RowKitchen` con fecha (`:419-420`). **Toda lectura de cocina, también el tablero, es `RowRecords`, que
    ningún estado bloquea** (`:410-414`).
  - Para la §2 («Sin activar» no muestra pedidos), las lecturas `kitchen-display/*` se clasifican `RowKitchen` sin
    fecha (no `RowRecords`), y las marcas y recuperar, `RowKitchen` con fecha.
  - `events` es `RowAlways` (`:157-160`): el canal sigue abierto sin activar, pero solo lleva invalidaciones.
  - Con «Hora del equipo incorrecta», marcar y recuperar se bloquean (`SYSTEM_CLOCK_INVALID`, L1-D14), como el modo
    Cocina del escritorio: la tableta lo dice y sigue mostrando los pedidos (Duda 7).
  - Emparejar es `RowAdministration` (`:163-169`): sin activar no se empareja, como cualquier terminal.

### 5. Aplicación de meseros y lo compartible

#### Inventario de lo que la §1 manda a `android-core/`

Paquetes actuales bajo `com.adelpos.meseros.` (módulos `core:domain` JVM y `core:data` Android):

| Pieza                        | Archivos actuales                                                                                                                                                                                                                                                                      | Depende de                                                                               | Mezclado con meseros (separar)                                                                                                                                                                                                                                    |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Emparejamiento               | `core.domain.pairing.*` (`Pairing.kt`, `PairingProtocol.kt`), `core.domain.server.*` (dirección, anuncio NSD, cambio de IP, compatibilidad), `core.data.access.PairingRepository`, `core.data.discovery.*` (`NsdServerBrowser`, `HttpsServerProbe`), DTO de `core.data.api.AccessDtos` | `PinnedHttpClient`, `FirstSeenTrustManager`, `SecretSealer`, Room (`PairedServerEntity`) | `PairingRepository.kt:20` y `:236-252` (`IdleLock.DEFAULT_SECONDS` y `PairedServerEntity` de `AppDatabase`); `ServerCompatibility.kt:95` (capacidades de meseros); `AccessDtos.kt` (DTO de PIN junto a los del emparejamiento); `ClientIdentity.KIND = "android"` |
| TLS con certificado fijado   | `core.data.api.PinnedHttpClient`, `ServerAnswersInterceptor`, `ClientHeadersInterceptor`, `ClientIdentity`, `CallAwait`; `core.data.security.PinnedTrustManager`, `FirstSeenTrustManager`, `KeystoreSecretSealer`; `core.domain.security.SecretSealer`                                 | OkHttp, Keystore, `ServerAnswers`                                                        | Casi nada: el tipo de cliente debe ser un parámetro (`ClientIdentity.kt:13`)                                                                                                                                                                                      |
| Cliente de la API v1         | `core.data.api.ServerApi`, `ApiProblem`, `ApiJson`, `VersionInfo`, `MoneySerializer`; `core.domain.error.AppError`, `LicenseRefusal`, `LicenseMessages`; `core.data.access.PairedConnection`; `core.data.license.LicenseStatusRepository`                                              | `ServerTransport`, `ServerRelocator`                                                     | `PairedConnection.kt:3-16` trae la entrada con PIN (`PinLoginRequest`, `PinUsersResponse`); `LicenseMessages` tiene textos de la cola                                                                                                                             |
| Canal de eventos (SSE)       | `core.data.realtime.RealtimeClient`, `RealtimeDtos`, `AccessInterruptionMapping`; `core.domain.realtime.*` (`RealtimeFeed`, `ReconnectBackoff`); `core.data.session.ServerAccess` (`AuthorizedServer`, `ServerTransport`)                                                              | `AuthorizedServer.credentials` (cabeceras)                                               | `RecoveryStateCursorSeed` lee `restaurant/state` (de meseros); `DeviceServerAccess` depende de `AccessRepository` (mesero con sesión)                                                                                                                             |
| Estado de conexión de la A3  | `core.domain.connection.ServerLink`, `ServerLinkTracker`, `LinkState`, `DefaultNetworkTracker`; `core.data.network.ServerLinkMonitor`, `ConnectionMonitor`                                                                                                                             | `ServerAnswers` (alimentado por `ServerAnswersInterceptor`)                              | `ConnectionSummary` y `PendingOrdersSource` (los pedidos por enviar) son de meseros                                                                                                                                                                               |
| Borrado y retiro             | `core.data.wipe.DeviceDataWipe`; la parte de retiro y «Emparejar de nuevo» de `AccessRepository` (816 líneas)                                                                                                                                                                          | Room, Keystore                                                                           | `AccessRepository` mezcla emparejado, meseros conocidos, PIN y bitácora: se queda en `android/` y usa las piezas del núcleo                                                                                                                                       |
| Pantallas (opcional, Duda 9) | `core.ui.theme.*` (`AdelPalette`, `AdelTheme`), `core.ui.icons.AdelIcons`, `components/AdelButtons`, `AdelTopBar`, `ConnectionIndicator`, `LicenseNotices`; `feature.access.findserver.*` y `status.*`                                                                                 | Compose                                                                                  | `core:ui` tiene además el catálogo, el PIN y los estados de mesa (de meseros)                                                                                                                                                                                     |

Quién depende de esto hoy: `app` (contenedor), `feature:access`, `core:capture`, `feature:waiter`, `feature:takeaway` y
`core:data` (catálogo, cola, precuenta y avisos de listo).

#### Lo que no puede cambiar al moverlo (0.4.1 encima de 0.4.0 sin desemparejar)

1. **`applicationId`** `com.adelpos.meseros`, la firma de desarrollo (huella del certificado
   `fb6a4bac…7c86`, `android/CLAUDE.md:775-779`) y `versionCode` mayor (0.4.1 → 5).
2. **Alias del Keystore** `adel-pos-secrets-v1` (`KeystoreSecretSealer.kt:170`) y el formato del sobre (AES-GCM,
   IV de 12 bytes, etiqueta de 16, versión 1, `:171-176`). Los propósitos como dato autenticado: `device-credential`
   (`AccessRepository.kt:802`), `pin-verifier:<id>` (`core/domain/…/access/PinVerifier.kt:109`) y `offline-credential:<id>` (`AccessRepository.kt:806`). Si `KeystoreSecretSealer`
   cambia de paquete, estos textos se fijan como constantes y no se derivan del paquete.
3. **Room**: los archivos `adel-pos.db` (`AppDatabase.kt:76`) y `catalog.db` (`CatalogDatabase.kt:41`); `AppDatabase`
   versión 6 y `CatalogDatabase` versión 1 con el mismo esquema (el hash de identidad); los nombres de tablas y columnas
   (`paired_server`, `known_waiters`…). **`AppDatabase` y `CatalogDatabase` se quedan en `android/`** con su nombre
   completo de clase, porque las carpetas de esquemas exportados llevan ese nombre
   (`core/data/schemas/com.adelpos.meseros.core.data.db.AppDatabase/`, el que busca `MigrationTestHelper`).
4. **WorkManager**: el trabajador `com.adelpos.meseros.core.data.waiter.OrderSendWorker` (WorkManager guarda el
   nombre de la clase de cada trabajo pendiente: un pedido encolado por la 0.4.0 lo necesita después de actualizar) y
   los nombres únicos `adel-orders-send` y `adel-orders-periodic` (`OrderSendWork.kt:35`, `:141-142`). Son de meseros:
   no se mueven.
5. **Componentes del manifiesto**: `ReadyAlertsService`, `ReadyAlertsBootReceiver` y el proveedor de `app`, y los
   identificadores de los canales de notificación (`ready/ReadyAlertNotifications.kt:86-137`). No se mueven.
6. **Preferencias**: no hay `SharedPreferences` ni DataStore; las preferencias viven en Room (`device_preferences`).
7. **Clases serializadas**: no hay serialización polimórfica ni `Parcelable`; los JSON guardados en Room (destino y
   revisión de `pending_orders`) son de meseros y no cambian. La navegación con tipo solo vive en el estado guardado.
8. **Red y pruebas**: los tipos NSD `_adelpos._tcp` y `_adelpostest._tcp`, `X-Client-Kind: android`,
   `com.adelpos.meseros.AdelTestRunner` y los paquetes de pruebas que usa `scripts/android-e2e.sh`.
9. **R8**: hoy solo `-dontobfuscate` (`app/proguard-rules.pro`). Los módulos de `android-core` necesitan sus
   `consumerProguardFiles` si alguna regla depende del paquete.

**Si el paquete Kotlin cambia** (`com.adelpos.core…`), se fijan explícitamente: el alias del Keystore, los tres
propósitos, los nombres de los archivos de Room, las clases de las bases (se quedan en `android/`), el trabajador y sus
nombres únicos (se quedan), los tipos NSD y el tipo de cliente de cada aplicación. Lo demás (clases de OkHttp, del
canal o del estado de conexión) no se guarda en el dispositivo y puede cambiar de paquete.

#### Pruebas que deben pasar igual antes y después

- **JVM**: `core:domain` 26 clases (`ServerLinkTrackerTest`, `PairingProtocolTest`, `ServerAddressTest`,
  `ServerCompatibilityTest`, `DefaultNetworkTrackerTest`, `RealtimeTest`, `LicenseMessagesTest`…); `core:data` 28
  (`PinnedHttpClientTest`, `CallAwaitTest`, `RealtimeClientTest`, `PairingRepositoryTest`, `AccessRepositoryTest`,
  `ServerApiTest`, `LicenseStatusTest`…); `core:ui` 5 (`PlainLanguageTest`, `ColorsOnlyInPaletteTest`…); y los
  `ViewModel` de `feature:*`.
- **Instrumentadas**: `core:data` 6 (`AppDatabaseMigrationTest`, `CatalogDatabaseMigrationTest`,
  `KeystoreSecretSealerTest`, `NsdServerBrowserTest`, `PinVerifierOnDeviceTest`, `ServerApiOnDeviceTest`), `core:ui` 7
  y `app` 26.
- **Contra el Go temporal**: `scripts/android-e2e.sh --suite all` (access 1–5 y 6–7, catalog, waiter, field,
  takeaway, precheck, alerts, adjustments, license; `connection` aparte, `android-e2e.sh:189-214`).
- **Lint** y **ktfmt** de cada módulo.
- Hay que **sumar**: una prueba de que una base y una credencial de la 0.4.0 abren con la 0.4.1 (instalar el APK 0.4.0
  emparejado, instalar encima el 0.4.1, comprobar que sigue emparejado, con sus meseros y su cola), en un emulador.

### 6. Estaciones: impresora, pantalla o ambas

- **Ya existe** desde la 0005: `kitchen_stations.reception IN ('printer','kds','both')` con
  `CHECK (reception = 'kds' OR printer_id IS NOT NULL)` (`0005:44`, `:54`), validado en `kitchen/stations.go:45-56` y
  elegido en Cocina › Estaciones con «Recepción de comandas»: «Solo impresora», «Solo pantalla de cocina» e «Impresora y
  pantalla de cocina» (`desktop/src/pages/kitchen/StationsAdmin.tsx:156-166`; `docs/TERMINAL_DE_COCINA.md:13-14`).
- **La impresión ya lo respeta**: una estación `kds` no imprime su comanda (`kitchen/documents.go:147`,
  `kitchen/releases.go:126`) y la comanda guarda su recepción para el acuse KDS (`reads.go:361-364`).
- **Las tareas se crean igual en las tres** (`releases.go:151-168`), así que el modo Cocina del escritorio sigue viendo
  todas las estaciones, como hoy.
- **Cambio de la K1**: la pantalla Android muestra solo las estaciones `kds` y `both` (filtro en Go), y Terminales
  ofrece solo esas para una pantalla de cocina. Sin migración y sin tocar la impresión. Hay que verificarlo con pruebas
  (§8: «impresora, pantalla o ambas»).

### 7. Estructura del proyecto (§1): mecanismo elegido y prueba de concepto

#### Mecanismo

**Composite build**:

- `android-core/` es un build de Gradle propio, con su `settings.gradle.kts`, wrapper, bloqueos y verificación, y
  `group = "com.adelpos.core"`.
- `android/settings.gradle.kts` y `cocina/settings.gradle.kts` lo incluyen con `includeBuild("../android-core")`.
- Los módulos de las aplicaciones piden `implementation("com.adelpos.core:<módulo>")` y Gradle lo sustituye por el
  proyecto incluido.

#### Qué se ejecutó (todo en `$SP/k1/spike/`, con `--offline`, guion `$SP/k1/spike-run.sh`)

Proyectos mínimos que imitan los tres:

- `android-core` con un módulo Kotlin/JVM (`:domain`) y una biblioteca Android (`:net`, con OkHttp y
  kotlinx-serialization);
- `android` (`com.adelpos.meseros`) y `cocina` (`com.adelpos.cocina`), cada uno con `:app` que depende de
  `com.adelpos.core:net`.

Wrapper y catálogo copiados de `android/`, sin cambiar versiones; caché de configuración, caché de compilación y
compilación en paralelo activas como en el proyecto real.

| Prueba | Qué                                                                                                                             | Resultado                                                                                                                                                                                  |
| ------ | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1      | `android-core` solo: `:domain:build :net:testDebugUnitTest :net:assembleDebug ktfmtCheck --write-locks`                         | Correcto. Bloqueos de raíz, `settings`, `buildscript` y por módulo; prueba JVM con su XML.                                                                                                 |
| 2      | `android` y `cocina`: `:app:assembleDebug :app:assembleRelease ktfmtCheckAll --write-locks`                                     | Correcto en los dos. APK debug y release. `dexdump` confirma `com.adelpos.core.net.ClientKind` y `com.adelpos.core.domain.LinkWordKt` dentro del APK. Sin avisos de AGP cargado dos veces. |
| 2b     | `ktfmtCheckAll`: `gradle.includedBuild("android-core").task(":ktfmtCheck")` más el de cada módulo propio                        | Correcto: corre `:android-core:ktfmtCheck` desde el consumidor. El nombre del build incluido es el de su carpeta (`android-core`), no su `rootProject.name`.                               |
| 3      | Verificación: `--write-verification-metadata sha256` en `android/`                                                              | El archivo de `android/` incluye también lo que resuelve `android-core` (AGP, Kotlin, ktfmt, OkHttp): 244 componentes.                                                                     |
| A1     | `android-core/gradle/verification-metadata.xml` vacío y compilar desde `android/`                                               | Correcto: **en un build compuesto manda el archivo del build raíz**; el del incluido se ignora.                                                                                            |
| A2     | El mismo archivo vacío y compilar `android-core` solo                                                                           | Falla «Dependency verification failed»: el build solo usa el suyo.                                                                                                                         |
| B      | Quitar `okhttp 5.5.1` del archivo de `android/`                                                                                 | Falla en `:android-core:net` y `:android-core:domain`: el raíz verifica las dependencias del incluido.                                                                                     |
| C      | `:app:lintDebug :android-core:net:lintDebug :android-core:net:testDebugUnitTest` desde `android/`                               | Correcto: Lint y las pruebas del incluido se corren desde el consumidor.                                                                                                                   |
| D/D2   | Bloqueo de `android-core/net` alterado (`okhttp 5.4.0`) y correr desde `android/` (pruebas del incluido y `:app:assembleDebug`) | Falla en los dos con «Dependency Locking»: **los bloqueos del incluido se respetan** desde el consumidor.                                                                                  |
| E      | `android` y `cocina` a la vez sobre el mismo `android-core`                                                                     | Correcto los dos (Gradle serializa lo compartido).                                                                                                                                         |

**Qué no funcionó a la primera**:

- `gradle.includedBuild("adel-pos-android-core")`: el nombre es el de la carpeta.
- Una aplicación mínima con solo `core-ktx` pidió `androidx.collection:collection-jvm:1.4.2`, que no está en la caché:
  en el proyecto real Compose sube esa versión. Se quitó de la prueba. En la Fase B, un módulo nuevo con un subconjunto
  de dependencias puede resolver versiones que no están en la caché: hay que regenerar bloqueos y verificación con red.
- ktfmt pidió formatear los `.kts` que escribí a mano.

**No probado**: Room/KSP dentro del build incluido (la propuesta lo deja en `android/`), Compose en el incluido y la firma
(es la misma configuración de AGP de hoy, sin relación con el build compuesto).

#### Propuesta de estructura (Decisión propuesta 2)

- **Módulos de `android-core/`** (`com.adelpos.core…`):
  - `model` (Kotlin/JVM): conexión, emparejamiento, dirección y anuncio del servidor, compatibilidad con capacidades
    por aplicación, tiempo real, errores y mensajes de la licencia, `SecretSealer`, `Hex`;
  - `network` (biblioteca Android): cliente con huella fijada, interceptores, `CallAwait`, `ClientIdentity` con el tipo
    como parámetro, `ServerApi` base, `ApiProblem`, NSD y sonda, `PairingRepository` con una interfaz de
    almacenamiento, `RealtimeClient`, `ServerLinkMonitor`, `ConnectionMonitor`, `KeystoreSecretSealer`,
    `DeviceDataWipe` y `LicenseStatusRepository`;
  - `ui` (biblioteca Compose), si se acepta la Duda 9: paleta y tema, íconos, botones, barra con indicador y las
    pantallas de buscar el servidor, del código y de estado.
- **Lo de meseros** se queda en `android/` con sus paquetes (`com.adelpos.meseros…`): `AppDatabase` (con el emparejado
  en `paired_server`, que implementa la interfaz de almacenamiento), `AccessRepository`, PIN, catálogo, mesas, captura,
  cola y avisos de listo.
- **La de cocina** guarda su emparejado en su propia base Room. Paquetes y `namespace` `com.adelpos.cocina…`,
  `applicationId` `com.adelpos.cocina`, nombre «AD-EL POS Cocina», ícono propio.
- **Catálogo de versiones**: **uno solo**, `android-core/gradle/libs.versions.toml` (el build incluido lo importa
  solo); `android/` y `cocina/` lo toman con
  `versionCatalogs { create("libs") { from(files("../android-core/gradle/libs.versions.toml")) } }`. Se mueve el de
  `android/gradle/` (Gradle no deja tener ese archivo y además crear `libs`). Así las versiones no divergen.
- **Bloqueos**: cada build con los suyos (`gradle.lockfile`, `buildscript-…`, `settings-…`). El de cada aplicación
  incluye las dependencias que le llegan de `android-core`, así que un cambio de dependencia en `android-core` regenera
  los bloqueos de los tres.
- **Verificación**: una por build raíz (`android/`, `cocina/` y `android-core/` cuando se compila solo), cada una
  completa para lo que resuelve. El orden para regenerarlas va en `android-core/CLAUDE.md`.
- **Firma**: la misma configuración (`adelpos.signingProperties` o `ADELPOS_SIGNING_PROPERTIES`, la comprobación de
  «fuera del repositorio» de `app/build.gradle.kts:28-46`) en las dos aplicaciones, con la llave de
  `~/.android/adel-pos-dev/`. Se puede compartir en un script de Gradle de `android-core` o copiar las 30 líneas.
- **Versión de cocina** en `cocina/gradle.properties` (`adelpos.versionName=0.1.0`, `adelpos.versionCode=1`); meseros
  0.4.1 y 5.
- **Guiones**:
  - `dev.sh android-*` recorren `android/` y `cocina/` (y las tareas de `android-core` por el build compuesto);
  - `scripts/android-e2e.sh` con una suite `kitchen`, `APP_ID`, `TEST_PACKAGE` y `TEST_RUNNER` de cocina y la
    desinstalación de sus paquetes de prueba;
  - `iso.sh gradle` ya admite `ISO_GRADLE_DIR=cocina|android-core`;
  - `sync-check` no toca Android (`scripts/sync-metadata.mjs` solo sincroniza `config/product.json` con Go, el
    escritorio y Tauri): no cambia, salvo `min_client_versions.kitchen`;
  - el formato Prettier necesita **exclusiones nuevas** (no las hice: van en la Fase B): en `.prettierignore`,
    `cocina/**/build/`, `cocina/.gradle/`, `cocina/.kotlin/`, `android-core/**/build/`, `android-core/.gradle/`,
    `android-core/.kotlin/` y los esquemas de Room de cocina; en `.gitignore`, lo mismo más los dos
    `local.properties`.
  - ktfmt y Lint en cada build.

### 8. El «Reconectando» falso del escritorio (§7)

#### Lo que se midió (Go temporal `P0`, sin licencia, con el asistente hecho)

| Medición                                                                                                                 | Resultado                                                                                                                                                              |
| ------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `curl -N` a `http://127.0.0.1:8201/api/v1/events` con sesión de administrador (`$SP/k1/repro/sse.sh`)                    | HTTP/1.1 200; **Go cierra el canal a los 30,0 s** (`time_total=30.010`, `curl` sale con 0: fin normal); 41 mensajes `changes`, uno cada 750 ms aunque no haya eventos. |
| Lo mismo por la escucha de red, `https://127.0.0.1:8461` (como una terminal por red)                                     | Igual: 30,0 s, HTTP/1.1, 41 mensajes.                                                                                                                                  |
| El ciclo de `realtime.rs` imitado con `curl` (fin, 500 ms, abrir otra vez; `$SP/k1/repro/loop.sh`, 4 vueltas por la red) | Cada canal dura 30,04 s; **hueco sin canal de 0,62 a 0,63 s** entre el fin de uno y la respuesta del siguiente (TLS ~0,03 s; incluye ~0,08 s del guion).               |

#### La causa, con el código

1. **Go**: `lifetime := time.NewTimer(30 * time.Second)` y `case <-lifetime.C: return` (`administration/events.go:118`,
   `:128-129`). Es a propósito: el canal se renueva cada 30 s y vuelve a revisar permisos y estaciones en cada lectura
   (`docs/KITCHEN_AND_PRINTING.md:121`, «duración 30 segundos con reconexión»).
2. **Rust**:
   - al terminar el cuerpo (`response.chunk()` devuelve `Ok(None)`, `realtime.rs:105-107`) no distingue un fin normal
     de una falla;
   - manda `{"connection":"reconnecting"}` (`:132`), duerme `delay` (500 ms, `:135`, puesto en 500 con el éxito
     anterior, `:99`), lo duplica (`:136`) y al responder Go manda `{"connection":"connected"}` (`:100`).
3. **React**:
   - `useRealtime` pasa el estado a «Recuperando eventos · sondeo activo» (`hooks/useRealtime.ts:60-65`);
   - `serverState` lo convierte en `reconnecting` (`navigation/serverState.ts:25-28`);
   - la barra superior muestra el recuadro ámbar «Reconectando» (`TopBar.tsx:144`; en la barra de la captura, solo
     cuando no es «En línea», `App.tsx:857`).
   - Con el `connected` siguiente, `changed = true` (`useRealtime.ts:67`) dispara `adel-server-change` y **todas** las
     pantallas montadas vuelven a consultar a Go cada 30 s (24 escuchas, `useLiveResource.ts:75`).
4. **Por qué las pruebas no lo vieron**: el puente de Playwright vuelve a abrir el canal sin mandar «reconnecting»
   (`desktop/e2e/native-driver.ts:769`, solo `connected`), y Vitest prueba `useRealtime` con mensajes puestos a mano.

**El valor que coincide con ~30 s** es ese `30 * time.Second`.

**Descartados**:

- `WriteTimeout` de 15 s: el canal adelanta el plazo de escritura en cada mensaje
  (`SetWriteDeadline(+10 s)`, `events.go:102`).
- `ReadTimeout` de 15 s: medido, el canal dura 30 s y no 15.
- `IdleTimeout` de 60 s (`cmd/ad-el-pos-server/main.go:282-288`, `lanlistener/lanlistener.go:232-237`): no aplica a un
  canal ocupado.
- Plazo de `reqwest` de 40 s (`realtime.rs:81`): mayor que 30.
- La consulta de la sesión cada 5 s (`useSession.ts:117-119`): no produce «Reconectando».

**Un problema aparte, no medido**: una sola consulta de la sesión que falla pone «Sin conexión» y la capa que bloquea al
instante (`useSession.ts:82-85`, `App.tsx:313-327`), sin la tolerancia de la A3. Y `reqwest` (0.12, solo HTTP/1.1,
`Cargo.toml:15`) guarda las conexiones libres 90 s mientras Go las cierra a los 60 s: el mismo riesgo que causó los
avisos de la A2 en Android. No lo reproduje.

#### Corrección propuesta (Decisión propuesta 10)

1. **La causa, en Rust**: el fin normal de un canal que respondió 200 (cuerpo terminado sin error) se vuelve a abrir
   **al momento**, desde el mismo cursor, **sin** mandar `reconnecting` ni esperar y sin un `connected` nuevo (que hoy
   recarga todo cada 30 s). Solo una falla (error de conexión, plazo vencido, cuerpo cortado o un 5xx) cuenta como
   falla.
2. **Go no se toca**: los 30 s siguen (renovación y revisión de permisos). Así Android y la aplicación de cocina no
   cambian (§7.5). Rust y React deben dar por normal que el canal se renueva.
3. **Un solo estado de conexión con la tolerancia de la A3** (`desktop/src/navigation/serverLink.ts`, reglas puras
   como `ServerLinkTracker`):
   - lo alimentan la consulta de la sesión, cada respuesta de `services/api.ts` (cualquier respuesta HTTP es un éxito)
     y los mensajes del canal;
   - «Sin conexión» solo con 3 fallas seguidas y 20 s sin ningún éxito, o si el sistema dice que no hay red
     (`navigator.onLine` y los eventos `offline`/`online` del WebView);
   - «En línea» con el primer éxito.
   - Lo leen la barra (`ServerStatus`), la capa «Sin conexión con el servidor», el aviso persistente y «Reintentar» de
     Mesas y Preparación.
   - «Reconectando» queda solo para el canal de eventos caído **más de la tolerancia** mientras Go sí responde, o se
     retira (Duda 11).
4. **Eventos perdidos**: el cursor ya hace que el canal siga desde el último evento (`after`, `events.go:64-66`). Al
   pasar a «En línea» después de un «Sin conexión», se vuelve a consultar todo, como hoy (`announceReconnection`).
5. **Preventivo**: `pool_idle_timeout(30 s)` en los dos clientes `reqwest` (`main.rs:214-226` y
   `pairing::pinned_client`), como el pool de 30 s de OkHttp en la A3.
6. **Terminales por red**: es el mismo código (Rust y React); se prueba contra la escucha HTTPS del Go temporal.
7. **Pruebas**:
   - en Rust, el ciclo del canal con un servidor de prueba en 127.0.0.1 que corta cada 2 s (ningún `reconnecting` en
     el fin normal) y que no responde de 1 a 5 s (dentro de la tolerancia);
   - el puente de Playwright imita las mismas reglas;
   - Playwright `workflow-stagez-k1-reconnect.spec.ts` con el Go temporal pausado de 1 a 5 s y más de 20 s
     (`kill -STOP` y `-CONT` del Go temporal, sin tocar la red de la Mac);
   - y los 10 minutos sin uso con el canal real: una prueba de Rust ignorada por omisión contra el Go temporal (local y
     por la red), que se corre en la Fase E y en la F.

### 9. Plan por fase

Dos agentes a la vez como máximo: **S** (servidor y escritorio) y **M** (Android). La revisión independiente ocupa el
lugar de uno mientras el otro sigue.

| Fase            | Agente                     | Contenido y archivos                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Pruebas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| --------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A**           | S                          | **0029, sección «Fase A»**: límites de color en `kitchen_settings` (`display_warning_minutes` 10, `display_late_minutes` 20, con sus `CHECK`); ajustes de la pantalla en `devices` (`kitchen_display_text_size` `normal`/`large`, `kitchen_display_theme` `dark`/`light`, sonido y, según la Duda 6, volumen); `license_seats_link_check` admite `kitchen_display` sin sesión. Recientes y recuperar se derivan de `kitchen_tasks` (sin tabla nueva, Decisión propuesta 5). Go: `httpapi.ClientKinds` + `kitchen`; `product.json` `min_client_versions.kitchen` (lo sincroniza `sync-metadata`); `matchClientKind`; punto único en `dispatch` (`service.go`); `administration/kitchen_display.go` (rutas); `kitchen.HandleDisplay` (tarjetas, marcar, pedido completo, recientes, recuperar; reutiliza `saveStates` y el tablero); `recordReadyNotices` y el borrado al recuperar; `licenseseats` (efecto nuevo, `inUseCondition`); `licensing/operations.go` (clases de `kitchen-display/*`); `authenticateDevice` (versión instalada); eventos para la pantalla (Decisión propuesta 7); capacidad `kitchen-display-v1`; `openapi.json` y `docs/API_V1.md`. | `go-unit`; `go-int` de `./internal/administration`, `./internal/kitchen`, `./internal/licenseseats`, `./internal/licensing`, `./internal/apicontract` (`TestContractV1`) y `./cmd/ad-el-pos-server` (con el candado); pruebas nuevas `TestStageK1KitchenDisplay*`: emparejar como pantalla, la sesión de cocina no captura, no cobra ni administra (cada familia de rutas), filtrado por estación y por `reception`, eventos de pedido nuevo, agregado, cancelación y marca desde otra pantalla, aviso al marcar y no al recuperar (y sí al volver a marcar), estado completo con cursor sin pérdidas ni duplicados, cupo y estados de licencia, versión instalada; la migración 28 → 29 (`TestStageK1Upgrade`). |
| **Revisión R1** | revisor (con M trabajando) | Solo el emparejamiento como pantalla de cocina y la autorización de sus rutas: el punto único de `dispatch`, la lista cerrada, `CLIENT_KIND_MISMATCH`, canal de eventos sin sesión, la contraseña del administrador y el cupo.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Las de la Fase A más las que pida.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| **B**           | M, en paralelo con A       | `android-core/` (build propio, `CLAUDE.md`, catálogo único), mover lo compartido **sin cambiar comportamiento** (con `git mv` y paquetes nuevos; lo fijo de la sección 5); `cocina/` (proyecto, `CLAUDE.md`, ícono, nombre, firma, emparejamiento, conexión, pantalla vacía de pedidos); `android/CLAUDE.md`; `dev.sh`, `android-e2e.sh`, `.prettierignore` y `.gitignore`. La parte de cocina que habla con Go espera a que A esté en `etapa-k1`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Antes y después del movimiento, en las mismas condiciones: JVM, Lint, ktfmt, instrumentadas en `Pixel_Tablet` y `ADEL_A1_Tablet_API26` (contando los XML) y `android-e2e.sh --suite access` y `alerts`; la prueba de actualizar 0.4.0 → 0.4.1 sin desemparejar; JVM de `android-core` y de `cocina`.                                                                                                                                                                                                                                                                                                                                                                                                             |
| **C**           | M                          | Pantalla de pedidos (§4): tarjetas, encabezado, platillos, cancelados, «NUEVO», colores con reloj simulado, marcar platillo y pedido, cierre automático, «Recientes» y recuperar, muchos pedidos con el indicador, tamaños de letra y temas.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | JVM (reglas puras de presentación); instrumentadas en los dos emuladores en horizontal; suite `kitchen` contra el Go temporal; capturas de las pantallas nuevas.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| **D**           | M                          | Conexión tolerante (el estado de `android-core`), reintento silencioso de una marca, aviso «Sin conexión con la computadora principal. Reconectando…» y bloqueo de marcas, sonidos y animación, pantalla siempre encendida, arranque directo, ajustes con pulsación larga y contraseña de administrador, `docs/COCINA_INSTALAR.md` (fijar la aplicación).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | JVM; instrumentadas; `kitchen` con cortes de 1 a 5 s (Go pausado con `kill -STOP`/`-CONT` y, en Android 11+, modo avión; en Android 8, `adb root` + `svc`) sin ningún aviso, un corte largo con aviso y recuperación, una marca durante un corte corto que se reintenta; la prueba de punta a punta con las dos aplicaciones en el mismo emulador.                                                                                                                                                                                                                                                                                                                                                               |
| **E**           | S, en paralelo con C y D   | Terminales (§6): para una pantalla de cocina, estaciones (solo `kds`/`both`), letra, tema, sonido, conexión, versión instalada, cupo y «Revocar»; límites de color en la configuración general de cocina; el aviso de la tableta sin cupo; todo escondido por la capacidad. «Reconectando» (§7): `realtime.rs`, el estado único en React, `native-driver.ts`, `pool_idle_timeout`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Vitest; `cargo` (con las pruebas nuevas del canal); Playwright `workflow-stagez-k1-terminals.spec.ts` y `workflow-stagez-k1-reconnect.spec.ts` en Chromium y WebKit; la prueba de Rust de 10 minutos contra el Go temporal (local y por la red); `go-int` si toca Go.                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| **F**           | un agente                  | Regresión completa una sola vez; `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md`, `docs/ANDROID_INSTALAR.md`, `docs/COCINA_INSTALAR.md`; versiones (`config/product.json` 0.16.0, meseros 0.4.1/5, cocina 0.1.0/1); APK de las dos con la misma llave; `.pkg` 0.16.0 (parada); fusión y push.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Todo: Go, escritorio (Vitest, `cargo`, Playwright en los dos motores), Android de las dos aplicaciones en los dos emuladores, `android-e2e.sh --suite all` y `kitchen`, la actualización 0.4.0 → 0.4.1.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

#### Rutas nuevas de la API (propuesta)

Solo con la credencial de un dispositivo `kitchen`, sin sesión:

- `POST kitchen-display/connect`: ocupa el cupo y devuelve la configuración (estaciones, letra, tema, sonido, límites
  de color, `server_time` y el resumen de la licencia). Repetirlo deja lo mismo.
- `GET kitchen-display/orders`: el estado completo de las tarjetas **con el cursor de eventos leído en la misma
  transacción**, como `event_cursor` de `restaurant/state`. El canal se abre desde ahí: nada se pierde ni se repite.
- `POST kitchen-display/items/ready` `{tasks: [{id, version_id}]}` y `POST kitchen-display/orders/ready`
  `{account_id, tasks: [...]}`: idempotentes por estado.
- `GET kitchen-display/recent`: lo marcado en los últimos 30 minutos en sus estaciones.
- `POST kitchen-display/recover` `{tasks: [...]}`: regresa a «En preparación» o «Recibido» (su estado anterior) y borra
  sus avisos.
- `POST kitchen-display/settings-access` `{username, password}`: comprueba la contraseña de un administrador.
- `GET events` y `events/snapshot` con el principal de la pantalla.
- `GET license/status` (el resumen del dispositivo, que ya existe).

**Permisos**: el principal tiene la lista cerrada de la sección 3; ninguna ruta de sesión le sirve. En Terminales (con
sesión): `PATCH devices/{id}` con los ajustes de la pantalla (`devices.manage`), estaciones con `PUT kitchen/access`
(`kitchen.stations.manage`) o en el mismo `PATCH`, y límites de color en `PUT kitchen/timing`
(`kitchen.timing.manage`).

**Eventos**: se reutilizan `kitchen.ticket.created`, `kitchen.state.changed` y `kitchen.change.pending` (por estación)
y `license.changed`. Se agrega uno para «cambió la configuración de esta pantalla» (Decisión propuesta 7). La revocación
llega como `interrupted` con `DEVICE_REVOKED`.

**Capacidad**: `kitchen-display-v1` en `GET version`, con `client_kinds` y `min_client_versions.kitchen`.

**Códigos de error**:

- nuevos: `KITCHEN_DISPLAY_ONLY` (403), `KITCHEN_DISPLAY_REQUIRED` (403) y uno para una marca que ya no se puede
  recuperar (409, por ejemplo `KITCHEN_NOT_RECOVERABLE`);
- los de siempre: `LICENSE_SEAT_UNAVAILABLE` (409, `seat: kitchen_displays`, `in_use`, `limit`), `LICENSE_*` y
  `SYSTEM_CLOCK_INVALID` de la puerta, `STATION_SCOPE_DENIED`, `CLIENT_KIND_MISMATCH`, `DEVICE_REVOKED`,
  `AUTHORIZATION_INVALID` y `AUTHORIZATION_THROTTLED`.

#### Contraseña de un administrador desde la tableta (§5.4)

Se reutiliza `supervisorAuthorizer` (`administration/supervisor.go:16-67`):

- valida usuario y contraseña en la misma transacción **sin crear ni reemplazar sesiones**;
- comparte el límite de 5 intentos en 15 minutos del inicio de sesión (`login_attempts` por dispositivo y usuario);
- nunca registra la contraseña;
- devuelve los permisos efectivos.

La ruta exige `devices.manage` («administrador» de la §5.4), responde solo `{authorized: true}` y deja la auditoría
`kitchen_display.settings_opened`. La tableta no recibe ningún token. Los ajustes que muestra (dirección, estado,
versión y «Volver a emparejar») son locales.

#### Pruebas de punta a punta (§8)

- **Las dos aplicaciones en el mismo emulador** (`Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal), con
  `android-e2e.sh --suite kitchen`. El guion orquesta tres instrumentaciones en orden:
  1. la de meseros empareja, entra con PIN y «Enviar a cocina»;
  2. la de cocina (emparejada antes como pantalla, con su `CocinaTestRunner`) ve la tarjeta y la marca;
  3. la de meseros comprueba la notificación «Platillos listos».

  Instalar las dos con la misma llave de pruebas y desinstalar antes sus paquetes de prueba.

- **Cortes de red de 1 a 5 s**: pausar el Go temporal con `kill -STOP` y `-CONT` desde el guion (sirve en Android 8 y
  17 y no toca la red de la Mac), y además el modo avión real en Android 17. El corte largo, más de 30 s.

### 10. Dudas

Numeradas. **P** = la decide el propietario; **C** = la puede decidir el coordinador.

1. **(C)** **Qué es una tarjeta de «pedido»**: la cuenta en las estaciones de esa pantalla (lo agregado después entra
   como «NUEVO») o cada envío por separado. La especificación habla de tarjetas de pedido con «agregados después del
   envío» dentro: propongo la cuenta (Decisión propuesta 5).
2. **(C)** **Estaciones con seguimiento «por pedido»** (`tracking = order`): hoy Go rechaza marcar un platillo en ellas
   (`states.go:83-85`). La §4 pide tocar un platillo. Propongo respetar la regla de Go: en esas estaciones tocar un
   platillo no lo marca y la tarjeta solo ofrece el botón del pedido. Lo predeterminado es «por platillo».
3. **(C)** **Recuperar cambia una regla de la 9F** («no se puede retroceder una tarea lista»): solo para la pantalla
   Android, dentro de los 30 minutos de «Recientes» y con su propia ruta. El modo Cocina del escritorio sigue igual.
4. **(C)** **Mesero de la tarjeta**: el responsable actual de la cuenta (el que recibe el aviso) o quien envió el
   pedido (`context_snapshot.employee`). Propongo el responsable, con quien envió si la cuenta no tiene.
5. **(P)** **«Liberar cupo» de una pantalla conectada**: con el cupo ligado al dispositivo, la pantalla lo volvería a
   ocupar en su siguiente consulta. Propongo que, después de «Liberar cupo», la tableta diga «Esta pantalla dejó su
   cupo de cocina» y solo vuelva a ocuparlo cuando alguien toque «Usar esta pantalla» en ella. Así otra pantalla puede
   tomarlo.
6. **(C)** **«El volumen y el sonido se pueden desactivar»**: propongo «Sonido» (activado o desactivado) y «Volumen»
   (bajo, medio o alto) en Terminales, con el sonido activado por omisión en las pantallas nuevas. Hoy
   `kitchen_sound_alert` es falso por omisión y es del modo Cocina del escritorio: no se reutiliza.
7. **(C)** **Con «Hora del equipo incorrecta»** la L1 bloquea las marcas de cocina (L1-D14). Propongo lo mismo en la
   tableta: sigue mostrando los pedidos y dice que no se puede marcar hasta corregir la hora.
8. **(C)** **Límites de color**: la §6 los pide generales (10 y 20 minutos) y el escritorio usa los de cada estación.
   Quedan los dos: los generales solo para las pantallas Android.
9. **(C)** **Pantallas compartidas**: la §1 enumera emparejamiento, TLS, cliente, canal y estado de conexión. Para no
   tener dos copias, propongo mover también el tema, la paleta, los íconos, los botones, la barra con el indicador y
   las pantallas de buscar el servidor y del código (`android-core:ui`). Sin eso, la aplicación de cocina repetiría la
   paleta y las pantallas del emparejamiento.
10. **(C)** **Acuse KDS de la 9F** (`kitchen/tickets/{id}/ack`, `kitchen_kds_receipts.user_id NOT NULL`): la §4 no lo
    pide. Propongo no usarlo en la K1. Comandas seguirá mostrando «recepción pendiente» en las estaciones `kds` y
    `both`, como hoy sin pantalla.
11. **(C)** **Los estados de la barra del escritorio**: con la tolerancia, «Reconectando» (ámbar) solo tendría sentido
    para el canal de eventos caído más de 20 s mientras Go responde. Propongo conservarlo con ese significado. La otra
    opción es dejar solo «En línea» y «Sin conexión», como Android.
12. **(C)** **Cambio de mesa o de nombre de la cuenta**: la pantalla no recibe los eventos de `restaurant` (pide
    `accounts.read`). La tarjeta se actualiza con el siguiente evento de cocina o al volver a leer. Propongo aceptarlo,
    o dar al principal un filtro de eventos solo de las cuentas con tarjeta visible.
13. **(C)** **La 0029 en las fases**: la Fase A pone todo lo de Go en su sección. Las fases C a E solo agregan una
    sección si les falta algo, con su comentario.
14. **(P, informativa)** **El arreglo del «Reconectando» no toca Go**: el canal seguirá renovándose cada 30 s, sin que
    se note. Si el propietario prefiere un canal de vida larga, habría que probar Android y la aplicación de cocina de
    nuevo (§7.5).

### Decisiones propuestas

1. **Sin código del producto en la Fase 0.** La prueba de concepto y la reproducción viven fuera del repositorio
   (`$SP/k1/spike/`, `$SP/k1/repro/`).
2. **Estructura**:
   - `android-core/` como build de Gradle propio, incluido con `includeBuild` desde `android/` y `cocina/`;
   - un solo catálogo (`android-core/gradle/libs.versions.toml`);
   - bloqueos y verificación por build raíz;
   - módulos `model`, `network` y, con la Duda 9, `ui`; paquetes `com.adelpos.core.*`;
   - `AppDatabase`, `CatalogDatabase`, el trabajador de la cola y los componentes del manifiesto se quedan en
     `android/` con sus nombres.
     Probado: sección 7.
3. **Tipo de dispositivo `kitchen`** (el reservado de la 0023) con `X-Client-Kind: kitchen`,
   `min_client_versions.kitchen = 0.1.0`, `matchClientKind` que lo exige, capacidad `kitchen-display-v1` y la etiqueta
   «Pantalla de cocina».
4. **Principal de dispositivo sin usuario ni sesión** para la pantalla:
   - punto único en `dispatch`, después de autenticar el dispositivo y antes de cualquier ruta de sesión, PIN, envío,
     agente o avisos;
   - lista cerrada de rutas y de permisos en código;
   - punto de entrada `kitchen.HandleDisplay` que reutiliza la lógica de la 9F;
   - marcas idempotentes por estado y versión, sin `restaurant_operations`.
5. **Tarjeta = la cuenta en las estaciones de la pantalla**, calculada en Go sin tabla nueva:
   - una ronda nueva empieza cuando llega una tarea y todo lo anterior ya estaba listo o cancelado (con funciones de
     ventana sobre `released_at`, `ready_at` y la cancelación);
   - «NUEVO» = una tarea de otra liberación que la primera de la ronda;
   - «CANCELADO» = cancelada dentro de la ronda;
   - la tarjeta se cierra sola cuando todo lo vivo está listo;
   - «Recientes» = lo marcado listo en los últimos 30 minutos en sus estaciones;
   - recuperar regresa las tareas a su estado anterior (`preparing` si ya había empezado, si no `released`) y la ronda
     reaparece.
6. **Cupo de cocina ligado al dispositivo**:
   - se ocupa con `POST kitchen-display/connect` con el mismo `claim` y el lock 7820148311;
   - la 0029 admite `session_id` vacío para `kitchen_display`;
   - cuenta mientras el dispositivo no esté revocado;
   - se libera al revocar o con «Liberar cupo», no por la red;
   - las lecturas de la pantalla son `RowKitchen` (sin activar no muestran pedidos).
7. **Eventos**: se reutilizan los de cocina por estación. Hay que agregar uno para «cambió la configuración de esta
   pantalla» (estaciones, letra, tema, sonido, límites de color) con un ámbito que pase el filtro de
   `events/store.go:112-118` para ese dispositivo. Ningún evento lleva contenido.
8. **Recuperar borra los `ready_notices`** de esas unidades; volver a marcar crea uno nuevo; se conserva
   `UNIQUE (unit_id, version_id)`.
9. **Versión instalada**: `authenticateDevice` guarda `X-Client-Version` cuando cambia, para todos los tipos;
   Terminales dice «Versión instalada».
10. **«Reconectando»**:
    - en Rust, el fin normal del canal se reabre en silencio y sin esperar;
    - un solo estado de conexión en React con la tolerancia de la A3;
    - `pool_idle_timeout(30 s)` en `reqwest`;
    - `native-driver.ts` con las mismas reglas;
    - Go sin cambios.
11. **Envío por estación**: se usa `kitchen_stations.reception` tal como está; la pantalla Android muestra solo `kds` y
    `both`.
12. **Contraseña del administrador** con `supervisorAuthorizer` y `devices.manage`, sin sesión.
13. **Locks**: ninguno nuevo. Las escrituras de la pantalla toman 7820148305 exclusivo, como cocina; el cupo,
    7820148311 al final y sin publicar eventos con él tomado. Si una fase necesita uno, el siguiente número es 7820148312.
14. **Exclusiones de formato e infraestructura** que hace la Fase B (no las hice): `.prettierignore` y `.gitignore`
    para `cocina/` y `android-core/` (sección 7), `dev.sh android-*` para las tres carpetas y la suite `kitchen` de
    `scripts/android-e2e.sh`.

### Pruebas ejecutadas en la Fase 0

| Qué                                                                                                   | Resultado                                                |
| ----------------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `ISO_TAG=P0 ISO_GO_PORT=8201 ISO_NET_PORT=8461 iso.sh <árbol> server-up` y asistente por `POST setup` | Correcto (esquema 28, `test_mode`).                      |
| Canal de eventos con `curl -N`, local y por la red (`$SP/k1/repro/sse.sh`)                            | 30,0 s cada canal, HTTP/1.1, 41 mensajes.                |
| Ciclo de reconexión imitado, 4 vueltas por la red (`$SP/k1/repro/loop.sh`, salida en `loop-red.txt`)  | Hueco de ~0,62 s cada 30 s.                              |
| Prueba de concepto de Gradle, pruebas 1, 2, 2b, 3, A1, A2, B, C, D, D2 y E (`$SP/k1/spike-*.log`)     | Como en la tabla de la sección 7: el mecanismo funciona. |
| `md5` de `docs/etapas/*.md` antes (`$SP/k1/etapas.md5`) y después                                     | Sin cambios.                                             |

**No verificado**: el «Reconectando» en la aplicación nativa (lo deduje del código y de las mediciones con `curl`); el
riesgo de las conexiones libres de `reqwest`; Room, KSP y Compose dentro de un build incluido; la firma de la aplicación
de cocina; nada en emuladores (esta fase no los usa).

### Decisiones del coordinador al cerrar la Fase 0

Las «Decisiones propuestas» y las dudas de arriba quedan así. Mandan sobre el resto del documento.

- **K1-D1. Fase 0 sin código del producto.** La prueba de concepto y la reproducción viven fuera del repositorio
  (`$SP/k1/spike/` y `$SP/k1/repro/`).
- **K1-D2. Estructura del proyecto** (Decisión propuesta 2 y Duda 9, aceptadas):
  - `android-core/` es un build de Gradle propio, incluido con `includeBuild("../android-core")` desde `android/` y
    `cocina/`, con `group = "com.adelpos.core"`;
  - **un solo catálogo de versiones**, `android-core/gradle/libs.versions.toml`, que `android/` y `cocina/` importan con
    `from(files(...))`;
  - bloqueos por build y verificación de dependencias por build raíz (`android/`, `cocina/` y `android-core/` cuando se
    compila solo), con el orden para regenerarlos en `android-core/CLAUDE.md`;
  - módulos `model` (Kotlin/JVM), `network` (biblioteca Android) y `ui` (Compose: tema, paleta, íconos, botones, barra
    con el indicador y las pantallas de buscar el servidor, del código y de estado), con paquetes `com.adelpos.core.*`;
  - lo de meseros se queda en `android/` con sus paquetes: `AppDatabase`, `CatalogDatabase`, `OrderSendWorker` y sus
    nombres únicos, los componentes del manifiesto y los canales de notificación;
  - **para instalar la 0.4.1 encima de la 0.4.0 sin desemparejar**, lo que se guarda en el dispositivo se fija como
    constante y nunca se deriva del paquete: el alias del Keystore `adel-pos-secrets-v1` y el formato del sobre, los
    propósitos `device-credential`, `pin-verifier:<id>` y `offline-credential:<id>`, los archivos `adel-pos.db` y
    `catalog.db`, los tipos NSD y el tipo de cliente de cada aplicación. Se agrega la prueba de actualizar encima en un
    emulador.
- **K1-D3. Tipo de dispositivo `kitchen`** (el reservado de la 0023), `X-Client-Kind: kitchen`,
  `min_client_versions.kitchen = "0.1.0"`, `matchClientKind` que exige `kitchen` a un dispositivo `kitchen`, capacidad
  `kitchen-display-v1` y la etiqueta «Pantalla de cocina» en Terminales.
- **K1-D4. Principal de dispositivo sin usuario ni sesión** (Decisión propuesta 4): un punto único en `dispatch`,
  después de `authenticateDevice` y `matchClientKind` y antes de toda ruta de sesión, PIN, envío, agente o avisos. Un
  dispositivo `kitchen` solo llega a su lista cerrada de rutas (todo lo demás, `403 KITCHEN_DISPLAY_ONLY`, auditado),
  y `kitchen-display/*` con otro tipo responde `403 KITCHEN_DISPLAY_REQUIRED`. Permisos en una lista cerrada en código
  (`KitchenDisplayPermissions`), nunca de un rol. La lógica, con un punto de entrada `kitchen.HandleDisplay` que
  reutiliza la de la 9F sin copiarla. Marcas idempotentes por estado y `version_id`.
- **K1-D5. La tarjeta es la cuenta en las estaciones de la pantalla**, calculada en Go sin tabla nueva (Decisión
  propuesta 5 y Duda 1): por rondas; «NUEVO» = tarea de una liberación posterior a la primera de la ronda; «CANCELADO» =
  cancelada dentro de la ronda; cierre solo cuando todo lo vivo está listo. **Mesero de la tarjeta** = el responsable
  actual de la cuenta (el que recibe el aviso) y, si no tiene, quien envió (Duda 4).
- **K1-D6. Cupo de cocina ligado al dispositivo** (Decisión propuesta 6): se ocupa con `POST kitchen-display/connect`
  con el mismo `claim` y el lock 7820148311; la 0029 admite `session_id` vacío para `kitchen_display`; cuenta mientras
  el dispositivo no esté revocado; se libera al revocar o con «Liberar cupo», nunca por la red. Las lecturas de la
  pantalla siguen la fila de cocina de la tabla de la licencia: «Vencida» y «Revocada» muestran y marcan; «Sin activar»
  no muestra pedidos. **Después de «Liberar cupo»** (Duda 5, la decide el coordinador por ser la consecuencia directa
  de la §2 y la §6): la tableta dice «Esta pantalla dejó su cupo de cocina.» y **no lo vuelve a ocupar sola**; lo pide
  otra vez solo cuando alguien toca «Usar esta pantalla» en ella (si no hay cupo, el mensaje de la §6). Así «Liberar
  cupo» sirve para que otra pantalla lo tome. Se avisará al propietario en la entrega.
- **K1-D7. Eventos** (Decisión propuesta 7 y Duda 12): se reutilizan los de cocina por estación y `license.changed`;
  se agrega uno sin contenido para «cambió la configuración de esta pantalla». Además, la pantalla recibe **sin
  contenido** los cambios de cuenta que cambian el encabezado de una tarjeta (mesa, nombre del cliente, combinación,
  transferencia, cancelación de la cuenta) **solo de las cuentas con tareas en sus estaciones**, con el filtro en Go; la
  tableta vuelve a leer las tarjetas. Ningún evento lleva datos.
- **K1-D8. Recuperar** (Decisión propuesta 8 y Duda 3): solo por la ruta de la pantalla Android, dentro de los 30
  minutos de «Recientes»; regresa la tarea a su estado anterior (`preparing` si ya había empezado, si no `released`) y
  **borra sus `ready_notices`**; volver a marcar crea uno nuevo; se conserva `UNIQUE (unit_id, version_id)`. El modo
  Cocina del escritorio sigue sin poder retroceder un «Listo».
- **K1-D9. Versión instalada** (Decisión propuesta 9): `authenticateDevice` guarda `X-Client-Version` cuando cambia,
  para todos los tipos; Terminales dice «Versión instalada».
- **K1-D10. «Reconectando» del escritorio** (Decisión propuesta 10, Dudas 11 y 14):
  - **La causa** es que Rust trata el fin normal del canal (Go lo renueva cada 30 s a propósito) como una falla. Se
    corrige ahí: el fin normal de un canal que respondió 200 se vuelve a abrir al momento desde el mismo cursor, sin
    `reconnecting`, sin esperar y sin un `connected` nuevo que recargue todas las pantallas.
  - **Go no se toca** (Duda 14): los 30 s siguen, así Android y la aplicación de cocina no cambian (§7.5). Se le
    informará al propietario en la entrega.
  - Un solo estado de conexión en React con la tolerancia de la A3 (3 fallas seguidas y 20 s sin éxito, o el sistema
    sin red; «En línea» con el primer éxito), que leen la barra, la capa «Sin conexión con el servidor» y los avisos de
    cada pantalla.
  - «Reconectando» (ámbar) se queda **solo** para el canal de eventos caído más de la tolerancia mientras Go sí responde
    (Duda 11).
  - `pool_idle_timeout(30 s)` en los clientes `reqwest`, y `native-driver.ts` con las mismas reglas que Rust.
- **K1-D11. Envío por estación**: `kitchen_stations.reception` (`printer`, `kds`, `both`) ya existe desde la 0005 y se
  verifica con pruebas; la impresión no cambia. La pantalla Android muestra solo las estaciones `kds` y `both`, y
  Terminales ofrece solo esas.
- **K1-D12. Acuse de recepción** (Duda 10, decidida distinto de lo propuesto): con una pantalla Android real, Comandas
  no debe seguir diciendo «Recepción en pantalla de cocina pendiente» de una comanda que la pantalla ya muestra. La
  pantalla confirma la recepción de las comandas de sus estaciones cuando las muestra (una ruta suya, idempotente), con
  el `device_id` y sin usuario; la 0029 permite `kitchen_kds_receipts.user_id` vacío **solo** con un dispositivo
  `kitchen`. Es la función de la 9F, no una nueva.
- **K1-D13. Contraseña de un administrador en la tableta** (Decisión propuesta 12): `supervisorAuthorizer`, que valida
  sin crear sesiones y comparte el límite de intentos, con `devices.manage`; responde solo `{authorized: true}`, audita
  `kitchen_display.settings_opened` y la tableta no recibe ningún token.
- **K1-D14. Estaciones «por pedido»** (Duda 2): se respeta la regla de Go. En esas estaciones tocar un platillo no lo
  marca: la tarjeta solo ofrece el botón del pedido, con un texto que lo explique.
- **K1-D15. Sonido** (Duda 6): en Terminales, «Sonido» (activado o desactivado) y «Volumen» (bajo, medio o alto),
  activado por omisión en las pantallas nuevas. Es aparte de `kitchen_sound_alert` del modo Cocina del escritorio.
- **K1-D16. Hora del equipo incorrecta** (Duda 7): como la L1 (L1-D14), la tableta sigue mostrando los pedidos y dice
  que no se puede marcar hasta corregir la hora de la computadora principal.
- **K1-D17. Límites de color** (Duda 8): los generales (10 y 20 minutos por omisión, en la configuración general de
  cocina) son solo para las pantallas Android; el modo Cocina del escritorio sigue con los de cada estación.
- **K1-D18. La 0029** (Duda 13): una sola migración; la Fase A pone todo lo de Go en su sección y las fases C a E solo
  agregan una sección propia si les falta algo.
- **K1-D19. Locks** (Decisión propuesta 13): ninguno nuevo. Las escrituras de la pantalla toman 7820148305 exclusivo,
  como cocina; el cupo, 7820148311 al final y sin publicar eventos con él tomado. Si hiciera falta uno, el siguiente es 7820148312.
- **K1-D20. Infraestructura en la Fase B** (Decisión propuesta 14): exclusiones de `.prettierignore` y `.gitignore`
  para `cocina/` y `android-core/`, `dev.sh android-*` sobre las tres carpetas y la suite `kitchen` de
  `scripts/android-e2e.sh`.
- **K1-D21. Playwright**: las especificaciones nuevas se llaman `workflow-stagez-k1-*.spec.ts`, para ordenarse después de
  `workflow-stagel1-*` y antes de `workflow-zz-test-data.spec.ts`.
- **K1-D22. Reparto**: el plan de la sección 9. El agente S hace la Fase A y después la E; el agente M hace la B en
  paralelo con la A y después la C y la D. La revisión independiente R1 (emparejamiento como pantalla de cocina y la
  autorización de sus rutas) corre al terminar la A, mientras M sigue; sus correcciones las hace S. La Fase F, un agente
  para la regresión y el coordinador para la documentación, los entregables y la fusión.

## Fase A · Servidor: dispositivo de cocina, estaciones, API y eventos

Agente S, 24/11/2026, en `$SP/wt/k1-a` (rama `etapa-k1-a`, desde `etapa-k1` `45fa9b2`), `ISO_TAG=A`. Contrato para
la aplicación de cocina en `$SP/k1/api-k1.md` (lo usa el agente M; dos cambios anotados al final). Las rutas son del
árbol; los números de línea, los del último commit de la fase.

### Qué se hizo

- **Migración 0029** (`backend/migrations/0029_pantalla_de_cocina.sql`, sección «Fase A»; esquema 29): límites de color
  generales en `kitchen_settings` (`display_warning_minutes` 10 y `display_late_minutes` 20, con sus `CHECK`); ajustes
  de cada pantalla en `devices` (`kitchen_display_text_size`, `_theme`, `_sound` activado por omisión, `_volume` y
  `kitchen_display_seat_released_at`, el «cupo liberado» de K1-D6, solo en un `kitchen`); el cupo `kitchen_display`
  sin sesión y `kitchen_kds_receipts.user_id` vacío **solo para un dispositivo `kitchen`** (un disparador por tabla que
  responde 23514 como un `CHECK`); la tabla `kitchen_display_dismissals` (ver la decisión propuesta 1) y dos índices
  (`kitchen_tasks_ready_idx` para «Recientes» y `kitchen_tickets_release_idx` para los acuses pendientes). No cambia
  ninguna fila existente. `config/product.json`: `schema_version` 29 (lo exigen las pruebas de subida: la última
  migración debe coincidir con el esquema del producto) y `min_client_versions.kitchen` `0.1.0`; `version` sigue en
  0.15.0 (lo sube la Fase F).
- **Tipo `kitchen`** (K1-D3): `httpapi.ClientKinds` con `kitchen` (`httpapi/clientversion.go:55`), capacidad
  `kitchen-display-v1` (`httpapi/router.go`), `matchClientKind` exige `kitchen` a un dispositivo `kitchen`
  (`administration/service.go:524`) y emparejar con `device_kind: kitchen` exige `X-Client-Kind: kitchen` y al revés
  (`administration/pairing.go`, 400 `CLIENT_KIND_MISMATCH`). `GET devices` ya informa `kind`; Terminales pone la
  etiqueta en la Fase E.
- **Principal de la pantalla** (K1-D4): `authenticateDevice` devuelve el tipo (`service.go:467`); justo después de
  `matchClientKind` y **antes** de la puerta de la licencia y de toda ruta de sesión, PIN, envío, agente o avisos, un
  dispositivo `kitchen` va a `service.kitchenDisplay` (`service.go:390`, `administration/kitchen_display.go:67`), que
  solo atiende su lista cerrada (`kitchen_display.go:44`: las ocho `kitchen-display/*`, `events/snapshot` y
  `license/status`). Todo lo demás: **403 `KITCHEN_DISPLAY_ONLY`**, auditado como `access.denied` con el dispositivo
  como autor; `authenticateSession` nunca corre, aunque llegue un `Authorization` de otra sesión. `kitchen-display/*`
  con otro tipo: **403 `KITCHEN_DISPLAY_REQUIRED`** (`service.go:394`), también auditado. Permisos en una lista cerrada
  en código (`kitchen.DisplayPermissions`: `kitchen.states.read`, `kitchen.states.manage`, `events.read`).
- **Rutas `kitchen-display/*`** (sección 9 del avance, ajustadas): `connect`, `orders`, `items/ready`, `orders/ready`,
  `recent`, `recover`, `receipts` y `settings-access`. La lógica de cocina está en `kitchen/display.go` y reutiliza la de
  la 9F sin copiar reglas:
  - `advanceTasks` y `taskStatesChanged`, extraídos de `saveStates` (`kitchen/states.go`): mismas validaciones, mismos
    eventos `kitchen.state.changed` por estación, la preparación de la cuenta y, con ella, el aviso de platillos listos
    (`recordReadyNotices`);
  - `recordReceipt`, extraído de `ackTicket` (`kitchen/reads.go`), para el acuse sin usuario (K1-D12);
  - `documents.KitchenPartDetails`, extraído de `renderKitchenParts` (`documents/render_kitchen.go`): los renglones de
    cada parte en la pantalla son los mismos de la comanda impresa (la impresión no cambia: las pruebas de plantillas
    pasan);
  - `stationscope.Load` con la pantalla como terminal y sin usuario, restringido con `Scope.Within` a las estaciones
    `kds`/`both` (K1-D11).
- **Tarjetas** (K1-D5, `kitchen/display.go:317` y `:342`): la cuenta en las estaciones de la pantalla por rondas;
  «NUEVO» = llegó después del primer envío de la ronda; «CANCELADO» dentro de la ronda; cierre solo cuando lo último que
  pasó fue marcar listo; mesa o «Para llevar» con el cliente (`title`/`subtitle`), mesero = responsable de la cuenta o
  quien envió; platillos agrupados por partida, estación, envío, versión y estado, con modificadores, notas,
  `pending_change` y `tracking`. Las estaciones «por pedido» (K1-D14): `items/ready` responde 400 y `orders/ready`
  marca el pedido completo de la estación.
- **Marcas** (K1-D4): `{tasks: [{id, version_id}]}`, idempotentes por estado y versión, sin `operation_id`; lo ya
  listo, cancelado o cambiado va en `skipped` (`already_ready`, `cancelled`, `changed`). Historial y auditoría sin
  usuario y con el dispositivo (`kitchen_history` admite `actor_id` y `operation_id` vacíos: `kitchen/context.go`).
- **Aviso al mesero** (K1-D8): marcar desde la pantalla registra el aviso de la A2 igual que hoy; `recover` regresa la
  tarea a «En preparación» si había empezado antes de marcarla (`started_at < ready_at`) o a «Por preparar», y **borra
  sus `ready_notices`**; volver a marcar crea uno nuevo (otra secuencia). Fuera de 30 minutos, 409
  `KITCHEN_NOT_RECOVERABLE`.
- **Eventos** (K1-D7): `kitchen.DisplayEvents` (`kitchen/display.go:1153`) con `events.ReadWith` (extraído de
  `events.Read`, mismo cursor y ventana): `license.changed`, `kitchen_display.configuration.changed` (de esa pantalla
  o sin agregado), los de cocina de sus estaciones (`ticket.created`, `state.changed`, `change.pending`,
  `kds.received`) y los de encabezado de las cuentas con tareas en sus estaciones (`restaurant.account.updated`,
  `.move`, `.assign`, `.merged`, `.split`, `.cancelled`, `restaurant.items.transferred`), por sus unidades o por sus
  liberaciones (así llega también el de la cuenta de origen de una combinación o transferencia). El evento de
  configuración se publica en `PATCH devices/{id}` de una pantalla, en «Liberar cupo» de una pantalla y en las
  escrituras de cocina de estaciones, formatos, tiempos y acceso (`kitchen/handler.go`). Revocar: `interrupted` con
  `DEVICE_REVOKED` en menos de un segundo (lo comprueba la prueba por SSE real).
- **Estado completo**: `orders` lee el cursor (`events.Head`) **antes** que las tarjetas en la misma transacción
  (`kitchen_display.go:221`).
- **Cupo y licencia** (K1-D6, K1-D16): `connect` ocupa `kitchen_displays` con el mismo `claim` y el lock 7820148311
  (`licenseseats.ConnectKitchenDisplay`), fila sin sesión que cuenta mientras el dispositivo `kitchen` no esté revocado
  (`licenseseats/store.go`, `inUseCondition`). Sin cupo, 409 `LICENSE_SEAT_UNAVAILABLE` con el mensaje de la §6.
  «Liberar cupo» deja la pantalla en «cupo liberado» y le avisa; `connect` responde `KITCHEN_DISPLAY_SEAT_RELEASED`
  hasta `take_seat: true` («Usar esta pantalla», auditado). Las lecturas y marcas exigen el cupo
  (`KITCHEN_DISPLAY_SEAT_RELEASED` o `KITCHEN_DISPLAY_NOT_CONNECTED`). `kitchen-display/*` es la fila F5 en la tabla de
  la licencia (`licensing/operations.go`, `classifyKitchenDisplay`): Vencida y Revocada muestran y marcan, Sin activar no
  muestra, con la hora incorrecta se lee pero no se marca; `settings-access` es «siempre».
- **Contraseña del administrador** (K1-D13, `kitchen_display.go:292`): `supervisorAuthorizer` con `devices.manage`, sin
  sesión ni token, auditado `kitchen_display.settings_opened` (o `settings_denied`).
- **Versión instalada** (K1-D9): `touchDevice` (`service.go:494`) guarda `X-Client-Version` válida en la misma
  sentencia de `last_seen_at`, para los tres tipos; también por el canal de eventos y el de avisos (que ahora pasan la
  cabecera).
- **Terminales (lado Go)**: `PATCH devices/{id}` con letra, tema, sonido y volumen (solo `kitchen`); `GET devices` y
  la configuración con `kitchen_display`, `app_version`, `online`, `seats` y `kitchen_display_station_options`;
  `PUT kitchen/access` solo con estaciones `kds`/`both` para una pantalla; límites de color en `PUT kitchen/timing` (y
  en `PUT kitchen/settings`, porque el escritorio guarda lo que leyó; ver la decisión propuesta 6).
- **Contrato**: ocho operaciones nuevas en `openapi.json` (con sus esquemas `KitchenDisplay*`, `X-Client-Kind` con
  `kitchen` y la descripción del canal de la pantalla), la sección «Pantalla de cocina» de `docs/API_V1.md`, la sección
  K1 de `API_CONTRACTS.md` y el paso `contractKitchenDisplay` de `TestContractV1` (empareja por la API como `kitchen`).

### Decisiones propuestas

1. **La tarjeta cerrada por una cancelación espera a que se marque** (§4: lo cancelado no desaparece «hasta que se
   marque el pedido»). Con K1-D5 literal, cancelar lo último que faltaba cerraría la tarjeta al momento y la cocina no
   vería la cancelación. Por eso, si lo último que pasó en la ronda fue una cancelación (posterior a todo «Listo»), la
   tarjeta queda con `status: "cancelled"` hasta que alguien toca el botón del pedido (`orders/ready` la da por vista) o
   pasan 30 minutos. Necesita guardar «vista»: tabla nueva `kitchen_display_dismissals` (una fila por tarea cancelada;
   no se tocó `kitchen_tasks` para no cambiar la forma del tablero de la 9F). Es una tabla, no la tarjeta: K1-D5 («sin
   tabla nueva») se mantiene para el modelo de la tarjeta.
2. **`orders` no ocupa el cupo** (solo `connect`): sin cupo responde `KITCHEN_DISPLAY_SEAT_RELEASED` o el código nuevo
   **`KITCHEN_DISPLAY_NOT_CONNECTED`** (409, volver a `connect`). Así una lectura nunca toma el lock de los cupos y
   «Liberar cupo» no se deshace con la siguiente lectura.
3. **La regla de la tableta es «con cualquier evento, volver a leer `orders`»**: `orders` trae también la
   configuración y la licencia, y su error decide la pantalla (sin cupo, cupo liberado, sin licencia). El contrato es
   más simple para M que reaccionar por tipo de evento.
4. **Orden de los locks de la pantalla**: para una pantalla, `authenticateDevice` no toca la fila del dispositivo;
   `touchDevice` va después de tomar 7820148305 exclusivo en las escrituras (el mismo orden que `PUT kitchen/access`:
   7820148305 → fila del dispositivo) y en `connect` antes de 7820148311. Las lecturas no toman 7820148305: leer el
   cursor antes que las tarjetas basta para no perder nada (los productores publican con 7820148306 hasta el commit).
5. **`kitchen.ticket.created` por estación es el evento de «pedido nuevo»** (también de agregado, modificación y
   cancelación); `kitchen.released` (sin estación) no llega a la pantalla, como a todo usuario sin
   `kitchen.stations.all`. Los cambios de mesa y de mesero se llaman `restaurant.account.move` y `.assign`.
6. **`PUT kitchen/settings` acepta los límites de color** (si cambian, exige `kitchen.timing.manage`): el escritorio
   actual guarda el formulario con todo lo que leyó de `GET kitchen/settings`, que ahora los trae; sin esto, Cocina ›
   Comandas y tiempos fallaría con 400. Los límites viven en `SettingsView`, no en `Settings`, para no cambiar la forma de
   las comandas guardadas.
7. **Emparejar**: `device_kind: kitchen` ⇔ `X-Client-Kind: kitchen` (400 `CLIENT_KIND_MISMATCH` si uno sí y el otro no;
   sin cabecera, herramientas locales, no se compara). Un `kitchen` emparejado antes de la K1 (no debería existir: el
   escritorio siempre fue `desktop`) ahora es pantalla y ya no puede usar la aplicación de escritorio.
8. **La versión instalada** se escribe en la misma sentencia de `last_seen_at` (no hay escritura extra); en
   `ready-alerts` se sigue escribiendo como mucho una vez por minuto, salvo que cambie la versión.
9. **Acuses pendientes** (`receipts_pending`): solo de las comandas de las tarjetas que se muestran y con la regla de
   Comandas («ningún dispositivo la confirmó»); una comanda de una ronda ya cerrada sigue «pendiente» en Comandas, como
   hoy sin pantalla.
10. **La pantalla muestra también las estaciones inactivas** (como el tablero de la 9F), para que lo pendiente no
    desaparezca al desactivar una estación a media operación.
11. **Límite del cuerpo** de `kitchen-display/*`: 2 MiB, como `kitchen/*` (una marca admite 1000 tareas).

### Archivos

- Nuevos: `backend/migrations/0029_pantalla_de_cocina.sql`, `backend/internal/kitchen/display.go` (y
  `display_test.go`), `backend/internal/administration/kitchen_display.go` (y `kitchen_display_test.go`),
  `backend/internal/database/stagek1_upgrade_test.go`, `backend/internal/operations/stagek1_kitchen_display_test.go`.
- Cambiados: `administration/service.go` (punto único, `authenticateDevice`, `touchDevice`, `matchClientKind`),
  `devices.go`, `license_seats.go` («Liberar cupo» de una pantalla), `pairing.go`, `http.go`, `events.go`,
  `ready_alerts.go`; `kitchen/states.go`, `reads.go`, `settings.go`, `stations.go`, `handler.go`, `context.go`;
  `documents/render_kitchen.go`; `events/store.go`; `stationscope/scope.go`; `licenseseats/store.go` y
  `operations.go`; `licensing/operations.go`; `httpapi/clientversion.go` y `router.go`; `apicontract/openapi.json` y
  `contract_integration_test.go`; `docs/API_V1.md`; `API_CONTRACTS.md`; `config/product.json` (y sus copias de `sync`);
  `database/stage9h_upgrade_test.go` (excluye la tabla nueva del conteo, como con las de la A1, A2 y L1).
- No toqué `android/`, `cocina/`, `android-core/`, `desktop/` (salvo la copia generada de `product.json`), `docs/etapas/`
  ni `contracts/`.

### Pruebas ejecutadas

| Qué                                                                                                                                                                                                                                                                                     | Resultado                                                                                                                                             |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `iso.sh go-unit` (vet normal e `integration`, `test -race` de todo, `GOOS=windows` build y vet)                                                                                                                                                                                         | Aprobado (al final de la fase).                                                                                                                       |
| `go-int ./internal/database` (TestStageK1Upgrade 28 → 29 y todas las de subida anteriores hasta la 29)                                                                                                                                                                                  | Aprobado.                                                                                                                                             |
| `go-int ./internal/kitchen`, `./internal/licenseseats`, `./internal/licensing`                                                                                                                                                                                                          | Aprobado.                                                                                                                                             |
| `go-int ./internal/administration`                                                                                                                                                                                                                                                      | Aprobado (286 s).                                                                                                                                     |
| `go-int ./internal/operations` completo (9F, A1, A2, L1 y la K1)                                                                                                                                                                                                                        | Aprobado (877 s).                                                                                                                                     |
| `go-int ./internal/operations -run TestStageK1KitchenDisplay` (con el emparejamiento por la API)                                                                                                                                                                                        | Aprobado.                                                                                                                                             |
| `go-int ./internal/apicontract` (TestContractV1 con `contractKitchenDisplay` y la de la licencia)                                                                                                                                                                                       | Aprobado.                                                                                                                                             |
| `go-int ./cmd/ad-el-pos-server` (con el candado `$SP/k1/candado-cmd`)                                                                                                                                                                                                                   | Aprobado.                                                                                                                                             |
| `format`, `fmt-check`, `sync-check` (lo hace `format`/`sync`) y `md5` de `docs/etapas/*.md`                                                                                                                                                                                             | Correcto; `docs/etapas` sin cambios.                                                                                                                  |
| Playwright Chromium `restaurant.spec` y `workflow-stage9f-kitchen-stations` (Cocina › Comandas y tiempos guarda con lo que leyó, ya con los límites nuevos; estaciones y terminales de cocina), con `ISO_GO_PORT=8203`, `ISO_PREVIEW_PORT=1433` e `ISO_LICENSE_TRUST` de `vectors.json` | 2 de 2 aprobadas (1,8 min). Un primer intento sin `ISO_LICENSE_TRUST` no pudo activar la licencia de prueba (configuración del arnés, no del código). |

`TestStageK1KitchenDisplay` (`internal/operations/stagek1_kitchen_display_test.go`) cubre lo de la §8 «Servidor»:
emparejar como pantalla por la API (y los tipos de cliente equivocados); la lista cerrada (33 familias de rutas con el
`Authorization` del administrador: sesión, PIN, captura, cobros, cocina del escritorio, catálogo, administración,
mantenimiento, licencia, reportes, agente, avisos, inventario…; su auditoría; `KITCHEN_DISPLAY_REQUIRED`;
`CLIENT_KIND_MISMATCH`); conectar y la configuración; el filtrado por estación (`PUT kitchen/access`) y por recepción
(impresora, pantalla y ambas) con la impresión de hoy (trabajos solo de las estaciones con impresora); tarjetas de mesa y
para llevar, mesero, modificadores y notas, «NUEVO», «CANCELADO», la tarjeta cerrada por una cancelación y su
cancelación vista desde otra pantalla; marcar platillo y pedido (idempotente, versión distinta, «por pedido», 403 y 404
sin marcar nada); el aviso al mesero al marcar, que recuperar no avise y borre el aviso, y que volver a marcar sí
avise; recientes y recuperar (a «Por preparar» y a «En preparación», y fuera de 30 minutos); el acuse y Comandas; los
eventos de pedido nuevo, agregado, cancelación, marca desde otra pantalla, cambio de mesa, configuración y licencia (y los
que no le tocan); la reconexión con el estado completo sin duplicados ni pendientes; la versión instalada de los tres
tipos; la contraseña de los ajustes; el cupo («Liberar cupo», «Usar esta pantalla», sin cupo con el mensaje de la §6,
sin licencia válida); revocar por SSE real; y Sin activar, Vigente, Vencida, Revocada y la hora del equipo incorrecta.

### No verificado o pendiente

- Nada con emuladores ni tabletas (la Fase A no los usa). La conexión por la escucha de red (HTTPS) de la pantalla la
  probará M con el Go temporal.
- WebKit y el resto de Playwright (la fase no cambió el escritorio; solo agregó campos a respuestas que usa y
  `PUT kitchen/settings` los acepta de vuelta).
- La revisión independiente R1 (emparejamiento como pantalla y la autorización de sus rutas) viene después.

### Decisiones del coordinador al cerrar la Fase A

- **K1-D23. Se aceptan las decisiones propuestas 1 a 11 de la Fase A.** La tarjeta que cerró una cancelación espera a
  que se marque el pedido (o 30 minutos), como pide la §4; `kitchen_display_dismissals` guarda solo «cancelación vista»
  y no cambia el modelo de la tarjeta de K1-D5, que sigue calculado en Go.
- **K1-D24. Solo `connect` ocupa el cupo.** `orders` y las marcas nunca toman el lock 7820148311; sin cupo responden
  `KITCHEN_DISPLAY_NOT_CONNECTED` (volver a `connect`) o `KITCHEN_DISPLAY_SEAT_RELEASED` (K1-D6).
- **K1-D25. La regla de la tableta**: con cualquier evento, volver a leer `orders`, que trae la configuración y la
  licencia y cuyo error decide la pantalla. La tableta agrupa los eventos seguidos en una sola lectura (una en curso y
  como mucho una pendiente), para no leer una vez por evento en una ráfaga.
