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

## Fase B · Módulo compartido `android-core/` y proyecto `cocina/`

Agente M, 24/11/2026, en `$SP/wt/k1-b` (rama `etapa-k1-b`, desde `etapa-k1` `45fa9b2`), `ISO_TAG=B`, Go temporal
8211/8471, emuladores `Pixel_Tablet` (Android 17, consola 5580) y `ADEL_A1_Tablet_API26` (Android 8.0, 5582), en
horizontal y de solo lectura. Guiones de la fase (fuera del repositorio) en `$SP/k1/B/`; resultados en
`$SP/k1/base-B/` (antes del movimiento) y `$SP/k1/after-B/` (después).

### Resumen

- **`android-core/`** es un build de Gradle propio (K1-D2) con el **único catálogo de versiones**, incluido con
  `includeBuild("../android-core")` desde `android/` y `cocina/`. Módulos `model` (Kotlin/JVM), `network` (biblioteca
  Android), `ui` (Compose) y `testing` (ayudantes de las pruebas contra Go), paquetes `com.adelpos.core.*`.
- Lo compartido se **movió con `git mv`** (84 archivos, con su historia) **sin cambiar su comportamiento**: las mismas
  413 pruebas JVM y las mismas 168 instrumentadas en cada tableta, con el mismo resultado, antes y después; `access` y
  `alerts` contra Go aprobadas en los dos emuladores antes y después.
- **Meseros 0.4.1** (`versionCode` 5) se instala encima de la 0.4.0 sin emparejar de nuevo: el pedido que dejó la
  0.4.0 en la cola sale solo, una vez, con la credencial de su mesero (prueba en `Pixel_Tablet`).
- **`cocina/`**: proyecto propio (`com.adelpos.cocina` 0.1.0/1, «AD-EL POS Cocina», ícono propio, la misma llave),
  solo en horizontal y a pantalla completa, base Room propia, emparejamiento como pantalla de cocina
  (`device_kind`/`X-Client-Kind: kitchen`) con las pantallas compartidas, conexión (`kitchen-display/connect`) y la
  pantalla de pedidos vacía («No hay pedidos»). Con la Fase A traída a la rama, la suite `kitchen` (emparejar y
  conectar) aprueba en los dos emuladores.
- **Infraestructura** (K1-D20): `.gitignore`, `.prettierignore`, `dev.sh android-*` sobre las tres carpetas y
  `android-e2e.sh` con la aplicación de cocina y la suite `kitchen` (`k1` emparejar, `k2` conectar).

### 1. Línea base, antes de mover nada

Con el código de `etapa-k1` tal cual, en una copia `git archive 45fa9b2` (`$SP/k1/base-B-src`), para poder editar el
árbol mientras corrían las pruebas:

- JVM, Robolectric, Lint y ktfmt de `android/` (con `--rerun`): **62 XML, 413 pruebas, 0 fallas**; Lint «No issues
  found». La lista de cada prueba con su resultado queda en `$SP/k1/base-B/jvm-casos.txt`.
- Instrumentadas (`connectedDebugAndroidTest`, aplicación y paquetes de prueba desinstalados antes, XML contados y
  copiados): `Pixel_Tablet` **168** (82 corridas, 86 omitidas, 0 fallas); `ADEL_A1_Tablet_API26` **168** (80
  corridas, 88 omitidas, 0 fallas). Las mismas cifras que la regresión de la L1.
- `scripts/android-e2e.sh --suite access` (pasos 1–5 y 7) y `--suite alerts` (r1–r10), cada una con un Go temporal
  nuevo y su licencia de prueba: **6/6 y 11/11 en los dos emuladores**. En Android 8, con `adb install -r -t` y
  `--no-install` (A3-D33), igual antes y después.

### 2. `android-core/`: el build y el movimiento

**Build** (`android-core/settings.gradle.kts`, `build.gradle.kts`, `gradle.properties`, el mismo wrapper de Gradle
9.9.1, bloqueos y `gradle/verification-metadata.xml` propios). `group = "com.adelpos.core"`: `android/` y `cocina/`
piden `com.adelpos.core:<módulo>` y Gradle lo sustituye por el proyecto incluido. El catálogo
`android/gradle/libs.versions.toml` pasó con `git mv` a `android-core/gradle/libs.versions.toml`; `android/` y
`cocina/` lo importan con `versionCatalogs { create("libs") { from(files(...)) } }`.

**Qué se movió** (con `git mv` y el guion `$SP/k1/B/move.py`, que cambia el paquete y corrige las importaciones de todo
`android/`; lista en `$SP/k1/B/moves.txt`):

| Módulo                                 | Desde                        | Qué                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------------------------------------- | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `model` (`com.adelpos.core.model`)     | `core:domain`                | `connection/` (`ServerLink`, `ServerAnswers`, `ServerLinkTracker`, `LinkState`, `ConnectionSummary`, `DefaultNetworkTracker`), `error/` (`AppError`, `LicenseRefusal`, `LicenseMessages`), `license/`, `pairing/`, `server/` (dirección, anuncio NSD, cambio de IP, `ServerCompatibility`), `realtime/`, `security/SecretSealer`, `text/Hex`, y 7 clases de prueba                                                                                                                                                                        |
| `network` (`com.adelpos.core.network`) | `core:data`                  | `api/` (cliente con la huella fijada y su punto único de respuestas, `ClientIdentity`, `ServerApi`, `ApiErrors`, `CallAwait`, `ApiJson`, `VersionInfo`, DTO del emparejamiento), `security/` (huella fijada, primer certificado, `KeystoreSecretSealer`), `discovery/`, `pairing/` (`PairingRepository`, `PairedConnection`), `realtime/` (`RealtimeClient`), `session/`, `connection/` (`ServerLinkMonitor`, `ConnectionMonitor`), `wipe/`, `license/`, el manifiesto con los permisos de red, 5 clases de prueba JVM y 2 instrumentadas |
| `ui` (`com.adelpos.core.ui`)           | `core:ui` y `feature:access` | el tema, la paleta, la tipografía y las medidas; `AdelIcons`; los botones; `AdelTopBar` con `ConnectionIndicator`; `UserActivity`; el logotipo; `AccessTexts`, «Buscar el servidor» y el código (`findserver/`), las pantallas de estado y el permiso de red local (`status/`); sus 96 textos; `PaletteContrastTest`, `ColorsOnlyInPaletteTest`, `PlainLanguageTest` y 3 instrumentadas                                                                                                                                                   |
| `testing` (`com.adelpos.core.testing`) | pruebas de `app`             | `MainComputer`, `GoTestServer` y `SystemDialogs` (ayudantes de las pruebas contra el Go temporal), para las dos aplicaciones                                                                                                                                                                                                                                                                                                                                                                                                              |

**Lo que se separó a mano** para que el núcleo no dependa de meseros (cada cambio deja el comportamiento igual):

- `ClientIdentity(version, kind)`: el tipo de cliente es un parámetro, con las constantes `ClientKinds.ANDROID` y
  `KITCHEN`; sin valor por omisión, para que la de cocina nunca se presente como `android` por descuido.
- `ServerCompatibility.check(…, requirements)` con `ServerRequirements` (versión mínima y capacidades): las de meseros
  en `WaiterServerRequirements` (`android/core:domain`, las mismas 0.13.0 y 4 capacidades; `ServerCompatibilityTest`
  se quedó en meseros con ellas), las de cocina en `KitchenServerRequirements`.
- `PairingRepository` entrega el emparejamiento a un `PairingStore` (`PairedDevice`); `AccessRepository` lo implementa
  con el mismo registro de `paired_server` (y el bloqueo por inactividad de fábrica, que antes ponía el
  emparejamiento). `PairedDevice.CREDENTIAL_PURPOSE = "device-credential"` es el único lugar del propósito; el
  `AccessRepository.DEVICE_PURPOSE` de meseros lo toma de ahí. La fecha de Go, en `GoTime`.
- La entrada con PIN se quedó en meseros: sus DTO (`PinDtos.kt`, separados de `AccessDtos.kt`) y sus llamadas
  (`PinCalls.kt`, extensiones sobre `PairedConnection.call`, que se hizo público).
- `RepairReason`, `NetworkPermissions` y `UnsentOrder` (los usan las pantallas de estado) pasaron a
  `model/access/AccessStates.kt`; `AppForeground` (lo usa el estado de conexión) a `network/connection`.
- `FindServerRoute(pairing, …, wallClock, defaultDeviceName)` recibe el emparejamiento en lugar de las dependencias de
  meseros; con `defaultDeviceName` la de cocina propone «Cocina 1» (sin él, «Tableta 1» o «Teléfono 1», como antes).
- Visibilidad: `RequestSent`, `Call.awaitResponse`, `ProblemNotice` y `AdelIcons.icon`/`circle` pasaron de `internal` a
  públicos, porque los usa código de meseros desde otro build.
- Textos: los 92 de las pantallas compartidas y los 4 títulos comunes de la barra pasaron a
  `android-core/ui/src/main/res/values/strings.xml`; los dos archivos de meseros que usan uno de ellos lo piden con
  `com.adelpos.core.ui.R as CoreR` (`AccessGate`, `PinEntryScreen`).
- `PlainLanguageTest` y `ColorsOnlyInPaletteTest` viven en `android-core:ui` y recorren **las tres carpetas**
  (`adelpos.repositoryRoot`): la regla de textos y la paleta única valen para las dos aplicaciones.

**Lo que se guarda en el dispositivo no cambió** (K1-D2): el alias del Keystore `adel-pos-secrets-v1` y el formato del
sobre, los propósitos `device-credential`, `pin-verifier:<id>` y `offline-credential:<id>`, `adel-pos.db` y
`catalog.db` con `AppDatabase` y `CatalogDatabase` (se quedaron en meseros, con sus esquemas), `OrderSendWorker` y sus
nombres únicos, los componentes del manifiesto, los canales de notificación, los tipos NSD (ahora también
`ServerAnnouncements.TEST_SERVICE_TYPE`) y `X-Client-Kind: android`. Ninguno se deriva del paquete.

**Dependencias**: ninguna biblioteca ni versión nueva. `network` usa `lifecycle-process` (para `AppForeground`) y
`testing` OkHttp, kotlinx-serialization, JUnit y el ejecutor de AndroidX, todas del catálogo. Los bloqueos de las
bibliotecas de meseros cambiaron porque ahora les llega `android-core` (por ejemplo, `core:data` ve `lifecycle-process`
2.11.0 en su classpath de ejecución), pero el de la aplicación no: `:app:assembleRelease` pasó con el mismo
`app/gradle.lockfile` de la 0.4.0, así que el APK lleva las mismas versiones. La verificación de `android/` sumó solo
los POM y módulos que Gradle consulta al resolver (`collection-jvm` 1.4.2, `animation` 1.10.5, `window-core` 1.5.0,
`kotlinx-coroutines-android` 1.6.4); `android-core` y `cocina` tienen la suya.

### 3. Meseros 0.4.1

`android/gradle.properties`: `adelpos.versionName=0.4.1`, `adelpos.versionCode=5` (`config/product.json` no cambia).
Es la única diferencia de la aplicación de meseros además del movimiento: misma `applicationId`, la misma llave de
desarrollo (huella `fb6a4bac…7c86`, comprobada con `apksigner` en el APK 0.4.0 entregado y en la 0.4.1 release) y el
mismo bloqueo de dependencias de la aplicación.

### 4. `cocina/`

Proyecto Gradle propio en la raíz (`cocina/settings.gradle.kts` con `includeBuild("../android-core")` y el catálogo
único; un módulo `app`), sin nada de la aplicación de meseros:

- `com.adelpos.cocina`, `minSdk` 26, `targetSdk` 36, `compileSdk` 37, versión 0.1.0 (`versionCode` 1) en
  `cocina/gradle.properties`. Nombre «AD-EL POS Cocina» y **ícono propio** (`mipmap-*/ic_launcher.png`): el del
  escritorio con fondo oscuro, la parte negra del logotipo en claro y el azul en naranja (el de meseros es blanco y
  azul). Se generó con un guion sin bibliotecas (`$SP/k1/B/icon-cocina.py`) y se redujo con `sips`, como el de meseros;
  es PNG y no un ícono adaptable porque `ColorsOnlyInPaletteTest` no permite colores en los recursos XML.
- **Firma**: la misma llave de desarrollo y la misma comprobación de «fuera del repositorio» que `android/` (la función
  de `app/build.gradle.kts`, 30 líneas, repetida: Gradle no deja compartirla sin un complemento propio). El APK release
  de cocina lleva la huella `fb6a4bac…7c86`.
- **Horizontal y pantalla completa**: `sensorLandscape` con `PROPERTY_COMPAT_ALLOW_RESTRICTED_RESIZABILITY` (Android 16
  ignora la orientación fija en pantallas grandes con `targetSdk` 36) y `tools:ignore="DiscouragedApi"` con su porqué;
  `MainActivity` esconde las barras del sistema (vuelven un momento al deslizar). Tema oscuro por omisión.
- **Base Room propia** `cocina.db` (`KitchenDatabase` versión 1, `paired_server` de una fila, la credencial cifrada con
  el Keystore), esquema exportado en `cocina/app/schemas/`.
- **`KitchenAccess`** (implementa `PairingStore`): lo guardado, la conexión emparejada con la huella fijada
  (`PairedConnection`), la comprobación de versiones con `KitchenServerRequirements` (capacidad `kitchen-display-v1`),
  la sonda del estado único de conexión y lo que hace cada error de Go: `DEVICE_REVOKED` borra todo y vuelve a «Buscar
  el servidor» (§2), `DEVICE_*`, otra huella y `CLIENT_KIND_MISMATCH` piden emparejar de nuevo, 426 pide actualizar la
  aplicación y `CLIENT_KIND_INVALID` (servidor sin el tipo `kitchen`) pide actualizar el servidor, como dice el
  contrato de la Fase A (`$SP/k1/api-k1.md`, sección 15).
- **Conexión** (paso 2 del orden de arranque del contrato): con el servidor compatible, `POST kitchen-display/connect`
  con `{}`; el resultado (`DisplayConnection`: conectando, conectada o rechazada por la licencia, el cupo o la hora con
  el texto de `LicenseMessages`) lo muestra la pantalla de pedidos.
- **Pantallas**: la barra común con el indicador de conexión y, según el estado, las compartidas de `android-core`
  (cargando, «Buscar el servidor» con el código, emparejar de nuevo, actualizar la aplicación o el servidor) o la
  **pantalla de pedidos**: «No hay pedidos» (conectada), «Conectando con la computadora principal…» o el texto de la
  licencia. Sin textos provisionales (`PlainLanguageTest` la recorre).
- **Pruebas**: `CocinaTestRunner` (el tipo NSD de pruebas y `clientVersion`), `KitchenScreensTest`,
  `KitchenAppStartTest` y `KitchenOnGoTest` (`k1` emparejar, `k2` conectar); en la JVM, `KitchenAccessTest`.
- `cocina/CLAUDE.md` con sus reglas, estructura, comandos, firma y pruebas.

### 5. Infraestructura (K1-D20)

- `.gitignore` y `.prettierignore`: compilaciones, cachés, `local.properties` y propiedades de firma de `android-core/` y
  `cocina/`, y los esquemas de Room de cocina.
- `scripts/dev.sh`: `android-format`, `android-format-check`, `android-build` (las dos aplicaciones), `android-test` y
  `android-connected` recorren `android-core`, `android` y `cocina`; `android_gradle` recibe la carpeta.
- `scripts/android-e2e.sh`: `select_app` elige la aplicación por suite (meseros o, en `kitchen`, `com.adelpos.cocina`
  con `com.adelpos.cocina.test`, `CocinaTestRunner` y el proyecto `cocina/`); la suite `kitchen` con `k1` (emparejar
  como pantalla de cocina) y `k2` (conectar y la pantalla de pedidos vacía) y sus capturas. Fuera de `all`.
- ktfmt y Lint de los tres builds (cada uno en su carpeta: las tareas de `android-core` no corren desde las
  aplicaciones).
- `iso.sh` ya admitía `ISO_GRADLE_DIR`; `sync-check` no cambia.

### Decisiones propuestas

1. **Alcance del movimiento** (K1-D2): además de lo que enumera la §1, se movieron las piezas que esas necesitan para no
   dejar dos copias: `UnsentOrder`, `RepairReason` y `NetworkPermissions` (las pantallas de estado), `AppForeground`
   (el estado de conexión), los textos de las pantallas compartidas y los 4 títulos comunes de la barra, y los
   ayudantes de las pruebas contra Go (`android-core:testing`). Lo que mezclaba meseros con lo compartido se separó: la
   entrada con PIN (`PinDtos.kt`, `PinCalls.kt`) y lo que meseros exige al servidor (`WaiterServerRequirements`) se
   quedaron en `android/`.
2. **Lo que cambia por aplicación entra como parámetro**, nunca como valor por omisión del núcleo: el tipo de cliente
   (`ClientIdentity(version, kind)`, `ClientKinds`), los requisitos del servidor (`ServerRequirements`), dónde se
   guarda el emparejamiento (`PairingStore`/`PairedDevice`) y el nombre que se propone al emparejar.
3. **Requisitos del servidor de la aplicación de cocina**: la capacidad `kitchen-display-v1` y la versión **0.15.0**
   como mínima (no 0.16.0): `config/product.json` sigue en 0.15.0 hasta la Fase F y con 0.16.0 la tableta diría
   «Actualiza el servidor» contra el Go de las fases B a E. La capacidad es la que decide; la Fase F puede subir la
   mínima a 0.16.0 si el coordinador lo prefiere.
4. **`PlainLanguageTest` y `ColorsOnlyInPaletteTest` en `android-core:ui`**, recorriendo `android/`, `android-core/` y
   `cocina/` (`adelpos.repositoryRoot`): una sola guardia de la regla de textos y de la paleta para las dos
   aplicaciones.
5. **La función de la firma se repite** en `cocina/app/build.gradle.kts` (las mismas 30 líneas y la misma regla de
   «fuera del repositorio»): compartirla exigiría un complemento de Gradle propio. Es la opción que dejó abierta la
   Fase 0.
6. **Visibilidad**: cuatro declaraciones `internal` pasaron a públicas porque las usa código de meseros desde otro
   build (`RequestSent`, `Call.awaitResponse`, `ProblemNotice`, `AdelIcons.icon`/`circle`).
7. **Cocina, al revocarla** (`DEVICE_REVOKED`): borra todo y vuelve directo a «Buscar el servidor» (§2), sin la
   pantalla de «dispositivo retirado» de meseros (no hay pedidos sin enviar que mostrar). `CLIENT_KIND_MISMATCH` pide
   emparejar de nuevo y `CLIENT_KIND_INVALID` pide actualizar el servidor, como la tabla de la sección 15 del contrato
   de la Fase A.
8. **Horizontal en Android 16+**: `sensorLandscape` con `PROPERTY_COMPAT_ALLOW_RESTRICTED_RESIZABILITY` (funciona con
   `targetSdk` 36; al subir a 37 Android ya no la respeta en pantallas grandes) y `tools:ignore="DiscouragedApi"`.
   Pantalla completa escondiendo las barras del sistema.
9. **Ícono de cocina en PNG** (no adaptable): `ColorsOnlyInPaletteTest` no permite colores escritos en recursos XML,
   que un ícono adaptable necesita para su fondo.
10. **Prueba de actualizar encima en dos partes** (ver «Pruebas»): los datos con las compilaciones debug y el ejecutor de
    pruebas, y la firma con el APK 0.4.0 entregado y la 0.4.1 release **sin abrirla**: abrir la aplicación normal sin
    emparejar empieza la búsqueda automática en la red real, que las reglas de la etapa prohíben.

### Pruebas ejecutadas en la Fase B

«Antes» es la copia de `etapa-k1` (`45fa9b2`); «después», el árbol de la fase. Las listas de cada prueba con su resultado
están en `$SP/k1/base-B/*casos.txt` y `$SP/k1/after-B/*casos.txt` y se compararon con `diff`.

| Prueba                                                                                                                   | Antes                                       | Después                                                                                                                                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM + Robolectric (`--rerun` antes) de meseros y, después, también de `android-core`                                     | `android/`: 62 XML, **413**, 0 fallas       | `android/` 47 XML (327) + `android-core` 15 XML (86) = 62 XML, **413**, 0 fallas; **los mismos 413 casos** (`diff` vacío)                                                                                                                                                                                                                                                                      |
| Lint                                                                                                                     | `android/`: sin problemas                   | `android/`, `android-core/` y `cocina/`: sin problemas (en cocina, `DiscouragedApi` de la orientación, ignorado con su porqué)                                                                                                                                                                                                                                                                 |
| ktfmt                                                                                                                    | correcto                                    | correcto en los tres builds                                                                                                                                                                                                                                                                                                                                                                    |
| Instrumentadas, `Pixel_Tablet` (5580)                                                                                    | **168**: 82 corridas, 86 omitidas, 0 fallas | `android/` 158 + `android-core` 10 = **168**: 82/86, 0 fallas; **los mismos casos**                                                                                                                                                                                                                                                                                                            |
| Instrumentadas, `ADEL_A1_Tablet_API26` (5582)                                                                            | **168**: 80 corridas, 88 omitidas, 0 fallas | 158 + 10 = **168**: 80/88, 0 fallas; **los mismos casos**                                                                                                                                                                                                                                                                                                                                      |
| e2e `access` (1–5 y 7), 5580 / 5582                                                                                      | 6/6 · 6/6                                   | 6/6 · 6/6                                                                                                                                                                                                                                                                                                                                                                                      |
| e2e `alerts` (r1–r10), 5580 / 5582                                                                                       | 11/11 · 11/11                               | 11/11 · 11/11                                                                                                                                                                                                                                                                                                                                                                                  |
| `fmt-check` del repositorio y `md5` de `docs/etapas/*.md`                                                                | —                                           | correcto; especificaciones sin cambios                                                                                                                                                                                                                                                                                                                                                         |
| Prueba de actualizar encima, parte 1 (datos), `Pixel_Tablet`                                                             | —                                           | 0.4.0 debug: `waiter` w1 y w3 **aprobadas** (emparejada, Ana entró, un pedido «Por enviar»); 0.4.1 instalada **encima** (`adb install -r`, el mismo `firstInstallTime`); w4 **aprobada**: sin volver a entrar, WorkManager envía el pedido de la 0.4.0 con la credencial de envío de Ana y entra una sola vez (`$SP/k1/after-B/upgrade-5580/`)                                                 |
| Prueba de actualizar encima, parte 2 (firma), `Pixel_Tablet`                                                             | —                                           | APK 0.4.0 entregado y 0.4.1 release con la misma huella `fb6a4bac…7c86`; `adb install -r` de la 0.4.1 sobre la entregada: **Success**, `versionCode` 4 → 5 con el mismo `firstInstallTime` (sin abrir la aplicación)                                                                                                                                                                           |
| `android-core` solo: JVM, Lint, ktfmt e instrumentadas, con bloqueos y verificación regenerados                          | —                                           | 15 XML (86) JVM; instrumentadas 10/10 en 5580 y en 5582                                                                                                                                                                                                                                                                                                                                        |
| `android/` regenerado (bloqueos y verificación con `resolveAllDependencies`, `assembleRelease` e instrumentadas en 5580) | —                                           | sin cambios en bloqueos ni verificación; instrumentadas 158 (72 corridas, 86 omitidas, 0 fallas)                                                                                                                                                                                                                                                                                               |
| `cocina`: JVM (`KitchenAccessTest`), Lint, ktfmt                                                                         | —                                           | 7/7, sin problemas                                                                                                                                                                                                                                                                                                                                                                             |
| `cocina`: instrumentadas en 5580 y 5582 (sin argumentos de Go)                                                           | —                                           | 5 en cada una: 3 aprobadas (`KitchenScreensTest` ×2, `KitchenAppStartTest`) y 2 omitidas (`KitchenOnGoTest` sin Go); la primera corrida falló por dos errores de las pruebas (dos `setContent` en una regla y `k2` sin omitirse sin argumentos) y la comprobación de pantalla completa por insets en Android 8 (ahí se comprueba la marca `SYSTEM_UI_FLAG_FULLSCREEN`); corregidas y repetidas |
| `android-e2e.sh --suite kitchen` contra el Go de la Fase A (`88b4d8d` traído a la rama)                                  | —                                           | **2/2 en 5580 y 2/2 en 5582**: `k1` empareja como pantalla de cocina (Go la registra con `kind: kitchen`, `app_version` 0.1.0) y `k2` se conecta (`kitchen-display/connect`) y muestra «No hay pedidos»                                                                                                                                                                                        |
| APK release de cocina                                                                                                    | —                                           | `com.adelpos.cocina` 0.1.0 (1), `minSdk` 26, `targetSdk` 36, «AD-EL POS Cocina», huella `fb6a4bac…7c86`                                                                                                                                                                                                                                                                                        |

Capturas (en horizontal, tema oscuro) en `$SP/k1/capturas-B/`: `pixel-tablet/` y `tablet-api26/` (b0 la aplicación real
al abrirla sin emparejar: horizontal, pantalla completa, «Cocina 1»; b1 pedidos vacía y «Conectando…»; b2 buscar el
servidor; b3 emparejar de nuevo; b4 actualizar el servidor) y `e2e-5580-kitchen/`, `e2e-5582-kitchen/` (k1 el código,
k2 la pantalla de pedidos conectada). Revisadas: sin textos provisionales ni identificadores; en Android 17, la primera
vez que la aplicación esconde las barras, Android muestra su aviso «Viewing full screen» (del sistema, una sola vez).

### No verificado

- Las suites `access` y `alerts` después del movimiento corrieron **antes** de traer la Fase A (con el Go de
  `45fa9b2`, como la línea base); la prueba de actualizar encima y la suite `kitchen`, con el Go de la Fase A. La
  regresión completa (`--suite all`) es de la Fase F.
- La prueba de actualizar encima se hizo en `Pixel_Tablet`; en Android 8 no.
- La parte 2 no abre la aplicación release (regla de la etapa): que la release 0.4.1 abra con lo emparejado por la
  0.4.0 release se infiere de la parte 1 (mismo código y mismos datos con la compilación debug) y de la firma.
- Sin conexión, la pantalla de cocina sigue «Conectando…» hasta que Go responde; el canal de eventos, las tarjetas, los
  textos propios de cupo y licencia de la §6 («No hay cupo de pantalla de cocina libre (N de N en uso)…», «Esta
  pantalla dejó su cupo…» y «Usar esta pantalla») y la pantalla siempre encendida son de las fases C, D y E.

### Archivos principales

- `android-core/` (`settings.gradle.kts`, `build.gradle.kts`, `gradle/libs.versions.toml`, `CLAUDE.md`, módulos
  `model`, `network`, `ui`, `testing`); nuevos: `model/…/access/AccessStates.kt`, `model/…/text/GoTime.kt`,
  `network/…/api/ClientIdentity.kt` (con `ClientKinds`), `network/…/pairing/PairingStore.kt`,
  `network/…/connection/AppForeground.kt`, `ui/src/main/res/values/strings.xml`.
- `android/`: `settings.gradle.kts` (`includeBuild` y catálogo), `gradle.properties` (0.4.1/5), `CLAUDE.md`,
  `core/domain/…/server/WaiterServerRequirements.kt`, `core/data/…/api/PinDtos.kt`, `core/data/…/access/PinCalls.kt`,
  `core/data/…/access/AccessRepository.kt` (`PairingStore`), `feature/access/…/AccessGate.kt`.
- `cocina/` completo: `app/src/main/kotlin/com/adelpos/cocina/` (`CocinaContainer`, `access/KitchenAccess.kt`,
  `access/KitchenStatus.kt`, `db/KitchenDatabase.kt`, `ui/CocinaApp.kt`, `ui/OrdersScreen.kt`), `AndroidManifest.xml`,
  `CLAUDE.md`, pruebas en `app/src/test` y `app/src/androidTest`.
- `scripts/dev.sh`, `scripts/android-e2e.sh`, `.gitignore`, `.prettierignore`.

### Commits (rama `etapa-k1-b`)

| Commit    | Qué                                                                                   |
| --------- | ------------------------------------------------------------------------------------- |
| `b5ef209` | `android-core` como build propio y lo compartido movido sin cambiar el comportamiento |
| `a74e32d` | `android-core:testing` con los ayudantes de las pruebas contra Go                     |
| `1eadd72` | Fusión de `etapa-k1` (Fase A)                                                         |
| `b4c642e` | Meseros 0.4.1 y `android-core/CLAUDE.md` y `android/CLAUDE.md`                        |
| `0504ba7` | Infraestructura de los tres proyectos (K1-D20)                                        |
| `68840a9` | Verificación y bloqueo de settings de `android-core` solo                             |
| `9828104` | Aplicación de cocina (`cocina/`)                                                      |
| `fa3c7b7` | `android/CLAUDE.md`: la prueba de actualizar encima                                   |

## Revisión independiente R1 · Seguridad de la pantalla de cocina

Revisor R1, 24/11/2026, en `$SP/wt/k1-r1` (rama `etapa-k1-r1`, desde `etapa-k1` `88b4d8d`), `ISO_TAG=R`. No toqué el
código del producto: solo agregué la prueba de reproducción
`backend/internal/operations/stagek1_r1_test.go` (`TestStageK1R1KitchenDisplaySecurity`). No hizo falta el Go temporal
(8221/8481) ni `go-int ./cmd/ad-el-pos-server`: todo se probó contra PostgreSQL con el servicio real y, delante, el
enrutador completo (`httpapi.NewWithOptions`, con la versión mínima por tipo de cliente), por la escucha local y por la
de red (contexto `lanlistener.WithNetwork` y estado TLS).

Revisé `git diff 45fa9b2..88b4d8d -- backend/`, sobre todo: `administration/service.go` (`dispatch`,
`authenticateDevice`, `touchDevice`, `matchClientKind`), `administration/kitchen_display.go`, `pairing.go`,
`devices.go`, `license_seats.go`, `supervisor.go`, `events.go`, `http.go`, `licenseseats/store.go`,
`licensing/operations.go`, `kitchen/display.go`, `kitchen/states.go`, `kitchen/reads.go`, `kitchen/stations.go`,
`events/store.go`, `stationscope/scope.go`, `httpapi/router.go` y `clientversion.go`, y la `0029`.

**Resultado:** no encontré ningún defecto de gravedad alta ni media. El punto único de `dispatch` es sólido (852
combinaciones de ruta y método rechazadas a la pantalla, también con el `Authorization` del administrador y por la red),
el alcance por estación se aplica en todas las lecturas, marcas, recuperaciones, acuses y eventos, y el cupo es atómico.
Hay dos hallazgos de gravedad baja.

### Hallazgos

#### R1-1 (baja). `settings-access` responde códigos que el contrato no tiene, uno con el texto de otra función

- **Dónde:** `backend/internal/administration/kitchen_display.go:300` y `:303`, que llama a
  `backend/internal/administration/supervisor.go:19-22`.
- **Qué pasa:**
  - un usuario de más de 40 caracteres o una contraseña de más de 256 responde **403 `AUTHORIZATION_REQUIRED`** con
    «Indica el usuario y la contraseña del supervisor que autoriza.» (el texto de las autorizaciones de cobros);
  - el usuario o la contraseña vacíos responden **400 `VALIDATION_ERROR`**.
- **El contrato:** `docs/API_V1.md:944-947` y `$SP/k1/api-k1.md` §13 solo dicen 200, 401 `AUTHORIZATION_INVALID`, 429
  `AUTHORIZATION_THROTTLED` y 403 `PERMISSION_DENIED`. `openapi.json` (`checkKitchenDisplaySettingsAccess`) solo trae
  `200` y `default`.
- **Riesgo:** no es de seguridad. Esos intentos no se validan contra nadie y no cuentan en el límite. Pero la tableta
  recibe un 403 con un código que no conoce. Si decide por estado y no por código, puede decir «Ese usuario no puede
  abrir los ajustes» a quien solo escribió de más.
- **Cómo se reproduce:** `testR1SettingsAccess` (`stagek1_r1_test.go:697`; el registro de la línea 768 muestra
  `403 {"code":"AUTHORIZATION_REQUIRED","message":"Indica el usuario y la contraseña del supervisor que autoriza."}`).
- **Corrección sugerida:**
  - en `kitchenDisplaySettingsAccess`, validar antes la longitud (como `login`: 40 y 256) y responder 401
    `AUTHORIZATION_INVALID` sin llamar a `supervisorAuthorizer`, o un 400 con un texto de la pantalla;
  - documentar el 400 de los campos vacíos en `API_V1.md`, en `api-k1.md` y en las respuestas de `openapi.json`, para
    que M lo trate.

#### R1-2 (baja). Una petición que ya pasó `authenticateDevice` sigue aunque la pantalla se revoque mientras espera

- **Dónde:**
  - `backend/internal/administration/service.go:467-487`: `authenticateDevice` lee `revoked_at` sin bloquear la fila;
  - `backend/internal/administration/kitchen_display.go:81`: `touchDevice` (`service.go:494`) actualiza la fila sin
    condición `revoked_at IS NULL`, y nada más vuelve a mirar la revocación.
- **Qué pasa:** si la revocación se confirma entre `authenticateDevice` y el trabajo (la petición espera la fila del
  dispositivo o el lock 7820148305), la petición termina como si la pantalla siguiera autorizada:
  - **`settings-access`** responde `200 {"authorized": true}` a una pantalla ya revocada y audita
    `kitchen_display.settings_opened`;
  - **`connect`** responde 200 con la configuración (nombre, estaciones, establecimiento y licencia) e inserta una fila
    de cupo del dispositivo revocado. Esa fila no cuenta (`inUseCondition` exige `revoked_at IS NULL`) y la borra el
    siguiente `claim`;
  - **el canal de eventos** entrega un lote más antes de `interrupted`;
  - **las marcas, recuperar, el acuse, `orders` y `recent` no pasan**, pero solo de rebote: revocar borra el cupo y
    `requireKitchenDisplaySeat` (`kitchen_display.go:94`) responde **409 `KITCHEN_DISPLAY_NOT_CONNECTED`**, no 401
    `DEVICE_REVOKED`. La tableta vuelve a `connect` y ahí recibe el 401.
- **Riesgo:** bajo. Nada de esto da un token, una sesión, pedidos ni marcas. Pero la seguridad de las escrituras depende
  hoy de la comprobación del cupo, no de la revocación, y la §2 pide que revocar desconecte «de inmediato».
- **Cómo se reproduce:**
  - `testR1RevokedSettingsInFlight` (`stagek1_r1_test.go:961`): otra transacción bloquea la fila de la pantalla,
    Terminales revoca (queda primero en la cola) y después llega `settings-access`. El registro muestra
    `settings-access en curso de una pantalla revocada (R1-2): 200 {"authorized":true}`;
  - `testR1RevokedInFlight` (`:906`): una marca que espera 7820148305 mientras se revoca la pantalla no se aplica, pero
    responde `409 KITCHEN_DISPLAY_NOT_CONNECTED`;
  - las dos aceptan también la respuesta corregida (401 `DEVICE_REVOKED`), así que siguen en verde después de corregir.
- **Corrección sugerida:**
  - en `kitchenDisplay`, para la pantalla, usar `UPDATE app.devices SET … WHERE id=$1 AND revoked_at IS NULL` y, con 0
    filas, responder `unauthorized("DEVICE_REVOKED", …)`. El `UPDATE` espera la fila y, en READ COMMITTED, vuelve a
    evaluar la condición sobre la versión confirmada;
  - como `touchDevice` ya va antes de toda ruta de la lista y, en las escrituras, después de 7820148305, eso cierra la
    ventana en todas sin otro lock;
  - el patrón es el mismo en los demás tipos de dispositivo y en las sesiones (anterior a la K1). Lo dejo fuera del
    alcance.

### Lo que se comprobó y está bien

**Emparejamiento como `kitchen`** (`testR1Pairing`, y la subprueba de emparejamiento de la Fase A):

- **Cómo nace un `kitchen`:** solo `claimPairing` (`pairing.go:554`) inserta un dispositivo con otro tipo que no sea el
  de omisión. `enroll` y `setup` crean siempre `desktop`. El tipo sale de la fila de la solicitud que aprobó un
  administrador con el código de 6 dígitos en la computadora principal (la aprobación no acepta otro tipo,
  `approvePairing`).
- **El tipo no cambia:**
  - ningún código del servidor hace `UPDATE … kind`;
  - `PATCH devices/{id}` con `kind` responde 400 (`DisallowUnknownFields`);
  - reclamar una solicitud de escritorio con `X-Client-Kind: kitchen` sigue creando un escritorio.
- **Un solo reclamo:** el segundo reclamo, con cualquier `X-Client-Kind`, responde 409 `PAIRING_ALREADY_CLAIMED` y no
  crea otra terminal. Antes de aprobar, 409 `PAIRING_NOT_APPROVED`.
- **`X-Client-Kind`:**
  - la credencial de un `kitchen` con `X-Client-Kind` `desktop` o `android` responde 400 `CLIENT_KIND_MISMATCH`;
  - la de un escritorio o un teléfono con `kitchen`, igual;
  - sin la cabecera, el tipo del dispositivo decide igual: un `kitchen` sigue en su lista cerrada.
- **Versión mínima por tipo** (`testR1ClientVersion`), por la red:
  - `kitchen` 0.0.9 o sin versión: 426 `CLIENT_UPDATE_REQUIRED`;
  - `KITCHEN`: 400 `CLIENT_KIND_INVALID`;
  - `0.1`: 400 `CLIENT_VERSION_INVALID`;
  - 0.1.0: 200;
  - un escritorio que dice ser `kitchen` para usar la mínima 0.1.0: 400 `CLIENT_KIND_MISMATCH`.

**Punto único de `dispatch`** (`testR1ClosedList`, `testR1OtherKinds`):

- **Orden en `service.go`:**
  - el desvío de la pantalla (`:390`) va justo después de `authenticateDevice` y `matchClientKind`, antes de la puerta
    de la licencia, la entrada con contraseña o PIN, la credencial de envío, el agente, los avisos y
    `authenticateSession`;
  - lo único anterior son las rutas públicas (`setup/status`, `setup`, `devices/enroll` solo local, `pairing/*`), que
    no dan nada que no tenga cualquiera sin credencial.
- **Lo que se probó con la pantalla:** 71 rutas de todas las familias × GET, POST, PUT, PATCH, DELETE y HEAD, por la
  escucha local y por la de red, con el `Authorization` del administrador. Todas responden 403 `KITCHEN_DISPLAY_ONLY`
  (852 combinaciones). Las familias:
  - sesión, `auth/*` y PIN;
  - `restaurant/*`, `takeaway/*`, `payments/*`, `inventory/*`, `reports/*` y `maintenance/*`;
  - `devices/*` (revocar, liberar cupo, configuración), el emparejamiento administrado y `license/*` (salvo `GET
license/status`);
  - `settings/*`, `print-agent/*`, `ready-alerts` y su canal, y `kitchen/*` del escritorio;
  - catálogo, usuarios, roles y auditoría;
  - variantes de la propia lista: mayúsculas, barra final, otro método y prefijos parecidos.
- **Sin efectos:** la sesión del administrador no se tocó (`last_seen_at` igual: `authenticateSession` nunca corrió) y
  no se abrió ninguna cuenta ni sesión.
- **Rutas raras:**
  - `%2F`, `%61uth`, barras dobles, `.` y `..`: el enrutador limpia (301) o decodifica a la misma ruta antes de
    comparar. `kitchen-display%2Forders` es la misma ruta de la lista y no abre nada más;
  - OPTIONS lo responde el enrutador sin llegar a la aplicación;
  - la credencial de envío del mesero en `restaurant/offline-orders` responde 403 `KITCHEN_DISPLAY_ONLY`.
- **Al revés:** las 8 rutas `kitchen-display/*` (y barra final, HEAD, DELETE), por las dos escuchas, responden 403
  `KITCHEN_DISPLAY_REQUIRED` a un escritorio (con sesión, sin sesión y sin `X-Client-Kind`) y a un teléfono de meseros
  (sin sesión y con el `Authorization` de otra sesión). Sin credencial, 401 `DEVICE_REQUIRED`. Ninguna llegó a la lógica
  de la pantalla (ninguna auditoría `kitchen_display.*`). `kitchen.HandleDisplay` solo se llama desde
  `runKitchenDisplay`, y `kitchen/` (`routes.go:25`) no atrapa `kitchen-display/`.

**Alcance por estación** (`testR1StationScope`):

- **Qué muestra:** `loadDisplayScope` usa `stationscope.Load` con la pantalla como terminal y `Within` las estaciones
  `kds`/`both`, recalculado en cada petición: una estación que pasa a «Solo impresora» desaparece al momento (la
  pantalla queda sin estaciones y `recover` responde 403). Las tarjetas, los recientes y los acuses pendientes solo traen
  sus estaciones, y una cuenta sin nada de sus estaciones no aparece.
- **Marcar, recuperar y confirmar:**
  - una tarea de otra estación (o de Postres, solo impresora) en el lote responde 403 `STATION_SCOPE_DENIED` sin marcar
    ni la propia;
  - recuperar una tarea de Barra marcada por la otra pantalla, igual;
  - una versión distinta va en `skipped: changed`;
  - un identificador en mayúsculas no es otra tarea;
  - ningún acuse de comandas de otras estaciones se guardó.
- **Eventos:** solo los tipos de la lista; ninguno de otra estación; nada de la cuenta sin Cocina (aunque cambie su
  mesero) ni de la configuración de la otra pantalla. Sí llega el cambio de mesero de su cuenta.
- **Datos:** los eventos no llevan datos (`events.Change`). `DisplayItem` no expone `Instructions.Price` y
  `KitchenPartDetails` no imprime precios.

**La contraseña del administrador** (`testR1SettingsAccess`):

- **Respuesta:** exactamente `{"authorized":true}`. No crea, cierra ni reemplaza sesiones y no devuelve token ni
  permisos.
- **No vale el `Authorization`:** con el de la sesión del administrador y una contraseña equivocada, 401.
- **`devices.manage`:** un usuario válido sin el permiso recibe 403 `PERMISSION_DENIED`; uno desactivado, 401.
- **Mismo límite que la entrada:**
  - `app.login_attempts` por terminal y usuario: cinco fallos y después ni la contraseña correcta pasa (429
    `AUTHORIZATION_THROTTLED`);
  - el bloqueo queda en la misma fila que leería `login`;
  - la otra pantalla conserva su propio contador.
- **No se registra:** la contraseña no está en `audit_events`, `kitchen_history` ni `login_attempts`, y el registro HTTP
  solo anota método, estado y duración.
- **Observación** (no es un hallazgo): un 403 `PERMISSION_DENIED` dice que la contraseña de ese usuario es correcta y
  reinicia su contador. Es lo mismo que ya revela `auth/login` desde cualquier terminal con el mismo límite.

**El cupo `kitchen_displays`** (`testR1Seats` y la subprueba de cupos de la Fase A):

- **Asignación atómica:**
  - con un solo cupo, 8 pantallas que conectan a la vez (5 rondas): exactamente una lo ocupa y las demás reciben 409;
  - una pantalla y el modo Cocina del escritorio a la vez (5 rondas): uno solo;
  - `claim` cuenta y ocupa en la misma transacción después de `pg_advisory_xact_lock(7820148311)`.
- **Orden de los locks:**
  - en `connect`, 7820148311 es el último lock y después solo hay lecturas (`kitchenDisplayView`);
  - «Liberar cupo» publica `kitchen_display.configuration.changed` antes de `ReleaseDevice`;
  - revocar no publica nada;
  - las escrituras de la pantalla toman 7820148305 antes de la fila del dispositivo, como `PUT kitchen/access`;
  - no encontré ningún ciclo con «Liberar cupo» (fila → 7820148306 → 7820148311) ni con `PATCH devices/{id}`.
- **Una desconexión no lo libera:** la fila sin sesión cuenta mientras el dispositivo no esté revocado, sin hora. Tres
  días simulados sin comunicarse: la otra pantalla sigue sin cupo y la primera sigue leyendo.
- **«Liberar cupo» frente a `connect` de la misma pantalla a la vez (15 rondas):** nunca queda «cupo liberado» con el
  cupo ocupado. La fila del dispositivo (que `touchDevice` bloquea antes de leer `kitchen_display_seat_released_at`)
  ordena las dos. Después, ni `connect {}`, ni `take_seat: false`, ni `orders` lo vuelven a ocupar.
- **Revocar sí lo libera:** la otra pantalla lo toma.
- **Licencia y hora:**
  - sin licencia válida no se ocupa nada;
  - Vencida y Revocada muestran y marcan; Sin activar no muestra pedidos;
  - con la hora incorrecta se lee pero no se marca, recupera ni confirma (clasificación en
    `licensing/operations.go`, F5 con escrituras «con fecha»; probado en la Fase A, que volví a correr).
- **Nota de diseño** (K1-D6, no es un defecto): el servidor no distingue si «Usar esta pantalla» (`take_seat: true`) lo
  tocó una persona. Una aplicación modificada podría volver a pedir el cupo sola. Nunca pasa del límite, pero «Liberar
  cupo» solo sirve si otra pantalla lo toma antes. Lo definitivo es «Revocar». Conviene decirlo así en
  `docs/COCINA_INSTALAR.md` o en Terminales.

**Revocar corta el canal:** la subprueba de la Fase A (SSE real) da `interrupted` con `DEVICE_REVOKED` en menos de un
segundo, y la volví a correr. El canal revalida dispositivo y tipo cada 750 ms (`events.go:133`).

### Observaciones fuera del alcance (anteriores a la K1)

- **Emparejar sin coincidencia entre escritorio y meseros:** al emparejar solo se exige la coincidencia de `kitchen`
  (`pairing.go:236`). `device_kind: android` con `X-Client-Kind: desktop` (o al revés) se acepta y después falla en cada
  petición con `CLIENT_KIND_MISMATCH`. No da más permisos (el administrador aprueba el tipo que ve).
- **Pantalla sin `X-Client-Kind` por la red:** toma la versión mínima general (0.10.0), así que la aplicación de cocina
  0.1.0 sin la cabecera recibiría 426. La aplicación siempre la manda (`api-k1.md`).

### Lo que no se pudo verificar

- **El certificado fijado y el filtro de pares de la red:** la escucha de red se simuló con su marca de contexto y un
  estado TLS, sin un apretón TLS real ni `lanlistener.Listen`. El Go temporal y la tableta los prueba M en las fases
  B a D.
- **La aplicación de cocina** (que trate cada código como dice el contrato, incluidos los de R1-1 y R1-2): no es de esta
  revisión.
- **Interbloqueos bajo carga real:** solo hay análisis del código y las pruebas de concurrencia del cupo.
- **Lo demás del servidor:** no corrí `go-unit` completo ni la `go-int` completa de los paquetes. No toqué el código
  del producto.

### Pruebas ejecutadas

| Qué                                                                                                                                                               | Resultado                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `ISO_TAG=R ISO_RUN='TestStageK1R1KitchenDisplaySecurity\|TestStageK1KitchenDisplay$' iso.sh go-int ./internal/operations` en `9316ea2` (la mía y la de la Fase A) | Aprobado (45 s).                                                                         |
| `$SP/k1/r1tools/govtest.sh` (lo mismo con `-v` en una base `adel_pos_test_r_*` propia, para ver los registros)                                                    | Aprobado. 852 combinaciones rechazadas; R1-1 y R1-2 registrados como se describe arriba. |
| `go test -run 'KitchenDisplay\|MatchClientKind\|InstalledVersion\|ForOperation\|Classify\|Within'` en administration, licenseseats, licensing y stationscope      | Aprobado.                                                                                |
| `go vet -tags=integration ./internal/operations`, `gofmt -l` y `iso.sh fmt-check`                                                                                 | Correcto.                                                                                |
| `md5` de `docs/etapas/*.md` antes y después                                                                                                                       | Sin cambios.                                                                             |

Los primeros intentos fallaron por errores de la propia prueba, que corregí:

- por la red, el escritorio sin `X-Client-Version` recibe 426 antes de llegar a la aplicación;
- el mesero quedó desactivado para la subprueba siguiente.

### Commits en `etapa-k1-r1`

- `559621c` etapa K1 revisión R1: TestStageK1R1KitchenDisplaySecurity con el enrutador completo.
- `f71b7cb` una marca que esperaba el lock de cocina no se aplica si la pantalla se revoca mientras.
- `2da5983` OPTIONS lo responde el enrutador sin llegar a la aplicación.
- `9316ea2` reproducción de R1-2 (settings-access en curso de una pantalla revocada).

Solo agregan `backend/internal/operations/stagek1_r1_test.go`, que siempre pasa (acepta también la respuesta corregida
de R1-2). El coordinador decide si se queda.

### Para el coordinador

- **R1-1 y R1-2:** las dos son de gravedad baja y las corrige S en la Fase E, si se aceptan. Ninguna obliga a cambiar
  una decisión.
- **Nota de diseño de K1-D6** (que «Liberar cupo» no es definitivo): es para el texto de la documentación o de
  Terminales. No hace falta código.

### Decisiones del coordinador al cerrar la Fase B y la revisión R1

- **K1-D26. Se aceptan las decisiones propuestas 1 a 10 de la Fase B.**
- **K1-D27. Versión mínima del servidor para la aplicación de cocina:** queda en 0.15.0 con la capacidad
  `kitchen-display-v1` mientras `config/product.json` siga en 0.15.0. En la Fase F, junto con la 0.16.0 del producto,
  `KitchenServerRequirements` sube a **0.16.0** (la capacidad sigue).
- **K1-D28. La prueba de actualizar encima en dos partes se acepta**: los datos con las compilaciones debug y la firma
  con la release sin abrirla, porque abrir la release sin emparejar empieza la búsqueda automática en la red real. En la
  Fase C se repiten las dos partes en `ADEL_A1_Tablet_API26` (Android 8), el sistema de las tabletas baratas.
- **K1-D29. Hallazgos de la R1:** se aceptan los dos, y S los corrige al empezar la Fase E.
  - **R1-1:** `settings-access` valida la longitud antes de `supervisorAuthorizer`, como `login`, y responde 401
    `AUTHORIZATION_INVALID`. Los campos vacíos (400 `VALIDATION_ERROR`) se documentan en `API_V1.md`, en `openapi.json` y
    en el contrato para la tableta.
  - **R1-2:** para la pantalla, `touchDevice` actualiza solo con `revoked_at IS NULL` y, si no cambia ninguna fila,
    responde 401 `DEVICE_REVOKED`.
  - La prueba de la R1 (`stagek1_r1_test.go`) se queda, y la de R1-2 exige solo la respuesta corregida.
  - Lo que la R1 dejó fuera del alcance también se corrige: al emparejar, la coincidencia entre `device_kind` y
    `X-Client-Kind` vale para los tres tipos cuando llega la cabecera.
- **K1-D30. «Liberar cupo» no es definitivo:** la tableta puede pedir el cupo otra vez con «Usar esta pantalla», y lo
  definitivo es «Revocar». Lo dicen Terminales, junto a «Liberar cupo» (Fase E), y `docs/COCINA_INSTALAR.md` (Fase D).

## Fase C · La pantalla de pedidos de la aplicación de cocina (§4)

Agente M, 24/11/2026, en `$SP/wt/k1-b` (rama `etapa-k1-b`, con `etapa-k1` traída hasta `f690bce`: la Fase B, la R1 y
sus correcciones), `ISO_TAG=B`, Go temporal 8211/8471, emuladores `Pixel_Tablet` (Android 17, consola 5580) y
`ADEL_A1_Tablet_API26` (Android 8.0, 5582), en horizontal y de solo lectura. Guiones de la fase (fuera del
repositorio) en `$SP/k1/B/`; resultados en `$SP/k1/after-C/`; capturas en `$SP/k1/capturas-C/`.

### Resumen

- La tableta ya es una **pantalla de cocina**: se conecta sola, muestra las tarjetas del pedido más antiguo al más
  nuevo y se actualiza **sin tocar nada** con el canal de eventos (con varios eventos seguidos, una sola lectura).
- **Tarjetas**:
  - encabezado con la mesa o «Para llevar» y el cliente, el mesero, la hora de envío y la espera en vivo;
  - platillos en letra grande con sus cambios y notas;
  - «NUEVO», y «CANCELADO» tachado hasta que se marca el pedido;
  - color por tiempo de espera con los límites generales.
- **Marcar**:
  - tocar un platillo lo marca: se atenúa con una palomita al momento;
  - el botón grande «Pedido listo» marca la tarjeta completa, que desaparece;
  - la tarjeta se cierra sola cuando todo lo vivo está listo;
  - en las estaciones «por pedido», tocar el platillo no lo marca y un texto lo explica;
  - con la hora del servidor incorrecta, los pedidos se siguen viendo pero no se pueden marcar, y un aviso lo dice.
- **«Recientes»** muestra lo marcado en los últimos 30 minutos, con «Recuperar».
- **Muchos pedidos**: columnas con desplazamiento y un aviso de cuántos quedan fuera de la vista.
- **Letra y tema**: la letra (normal o grande) y el tema (oscuro o claro) que se cambian en Terminales llegan al
  momento.
- **Licencia y cupo**: sin cupo, cupo liberado con «Usar esta pantalla», y licencia sin activar (§6).
- **Contra Go**, la suite `kitchen` tiene seis pasos con pedidos reales y **aprueba en los dos emuladores**. Incluye
  el aviso al mesero que deja marcar (`ready_notices`) y que se quita al recuperar.
- **K1-D28**: actualizar la 0.4.0 a la 0.4.1 encima aprueba en `ADEL_A1_Tablet_API26`, en sus dos partes.

### 1. Lo que se hizo

**Código** (`cocina/app/src/main/kotlin/com/adelpos/cocina/`):

- `board/KitchenDisplayDtos.kt`: el contrato de `api-k1.md` §5–§9.
- `board/Board.kt`: el modelo de la pantalla y `BoardMapper`.
- `board/BoardRules.kt`: las reglas puras:
  - orden de las tarjetas;
  - espera y su texto;
  - nivel de color;
  - qué se puede tocar;
  - botón del pedido;
  - pedidos fuera de la vista;
  - avisos de licencia;
  - qué pantalla corresponde a cada error.
- `board/KitchenDisplayService.kt`: las siete rutas de `kitchen-display/…`.
- `board/KitchenBoard.kt`: el ciclo (K1-D25):
  - `connect`, después `orders` con su cursor, después el canal;
  - lecturas coalescidas;
  - acuse de comandas (K1-D12);
  - marcas e idempotencia por versión;
  - Recientes;
  - cupo.
- `ui/OrdersScreen.kt`, `ui/OrderCardView.kt` y `ui/RecentDialog.kt`: la pantalla.
- `access/KitchenAccess.kt`: da la credencial al canal y a las rutas de la pantalla.
- `CocinaContainer` y `CocinaApplication`: arrancan el tablero y el `RealtimeClient` de `android-core` (el mismo de
  meseros), con la conexión de la barra según el canal.

**`android-core/`**:

- `LicenseMessages`: los textos de la pantalla de cocina (§6: sin cupo con `in_use`/`limit`, cupo liberado, sin
  activar, hora incorrecta, vencida y revocada) con su prueba.
- `LicenseSummaryDto.toSummary()`: ahora público, para leer la licencia de `orders` igual que meseros.
- `testing/SystemDialogs.dismissFullScreenNotice`: cierra el aviso de Android «Viewing full screen», que tapaba las
  capturas la primera vez que se abre la aplicación de cocina.

**Pruebas y guiones**:

- JVM: `BoardRulesTest` y `KitchenBoardTest`.
- Instrumentadas: `OrdersScreenTest`, `KitchenScreensTest` y `KitchenOnGoTest` (k1–k6).
- `scripts/android-e2e.sh`: suite `kitchen` con los pasos k1–k6, más el conteo de `ready_notices` en la base temporal.

**Commits** en `etapa-k1-b`:

- `adb0320`: textos y licencia en `android-core`.
- `31835aa`: la pantalla de pedidos.
- `1fe6827`: corrección de una prueba.
- `63017c8`: el aviso de pantalla completa en las capturas.
- `90eca7d`: letra grande pareja, el aviso de pedidos fuera de la vista y el texto de la tarjeta mezclada.
- `f6d5e4f`: lo marcado sin parpadeo.
- `104a50e`: la suite `kitchen` con pedidos reales.

### 2. Cómo se resolvió cada punto de la §4

| Punto                    | Cómo                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Ciclo (K1-D25)           | `connect` → `orders` → canal `GET events` con la credencial de la pantalla. Cualquier evento con cambios (o `reset`) pide una lectura. Un canal `CONFLATED` deja una lectura en curso y **a lo más una pendiente**. El canal se abre desde el principio (también sin cupo) para recibir `license.changed` y los cambios de configuración.                                                                          |
| Acuse (K1-D12)           | Después de pintar una lectura, `receipts` con sus `receipts_pending`. Si falla, la siguiente lectura las vuelve a traer; la tableta no avisa nada.                                                                                                                                                                                                                                                                 |
| Espera en vivo           | `elapsed_seconds` de la lectura más el reloj monotónico de la tableta desde que llegó (`SystemClock.elapsedRealtime`). Nunca la hora de la tableta. Avanza cada segundo.                                                                                                                                                                                                                                           |
| Color (K1-D17)           | Normal, «Atención: lleva tiempo esperando» (ámbar) y «Retrasado» (rojo), con `display_warning_minutes` y `display_late_minutes`. El borde y el encabezado usan los tokens de la paleta única.                                                                                                                                                                                                                      |
| CANCELADO (K1-D23)       | Tachado en rojo con su etiqueta. Una tarjeta cerrada por cancelación queda con el botón «Quitar de la pantalla» (`orders/ready`).                                                                                                                                                                                                                                                                                  |
| NUEVO                    | Etiqueta azul cuando `is_new`.                                                                                                                                                                                                                                                                                                                                                                                     |
| Marcar                   | Al tocar, el platillo se ve listo al momento (atenuado y con palomita) y la tarjeta del botón se esconde. Manda la siguiente lectura de Go. `skipped` no es error: la lectura trae el estado real.                                                                                                                                                                                                                 |
| Por pedido (K1-D14)      | Tocar el platillo no lo marca. Si toda la tarjeta es «por pedido», un texto al pie lo explica. En una tarjeta **mezclada** (platillos que se marcan al tocarlos y otros por pedido), cada platillo por pedido dice «Se marca con el botón «Pedido listo».».                                                                                                                                                        |
| Hora incorrecta (K1-D16) | Aviso fijo arriba; platillos y botones deshabilitados; las lecturas siguen.                                                                                                                                                                                                                                                                                                                                        |
| Recientes                | Botón «Recientes» en la barra: diálogo a pantalla completa con «Recuperar» por platillo y «Recuperar el pedido» si hay más de uno listo. Con `KITCHEN_NOT_RECOVERABLE` muestra el mensaje de Go y vuelve a leer `recent`.                                                                                                                                                                                          |
| Muchos pedidos           | Columnas adaptables (`LazyVerticalStaggeredGrid`, 300 dp mínimo) con desplazamiento. El aviso «Hay N pedidos más: desliza para verlos» va **debajo** de las columnas, para no tapar ningún botón.                                                                                                                                                                                                                  |
| Letra y tema             | Vienen de `display` en cada lectura. La letra grande es ×1.25 **pareja**, sobre la escala del sistema. El tema usa `AdelTheme` claro u oscuro.                                                                                                                                                                                                                                                                     |
| Cupo y licencia (§6)     | Sin cupo: el texto con `in_use`/`limit`, y se vuelve a intentar `connect` cada 30 s además de con los eventos. Cupo liberado: «Usar esta pantalla» (`connect` con `take_seat: true`). `KITCHEN_DISPLAY_NOT_CONNECTED`: `connect` otra vez. Sin activar: no se muestran pedidos. Vencida o revocada: aviso y se sigue trabajando. `DEVICE_REVOKED` y `CLIENT_KIND_MISMATCH`: lo maneja la entrada, como en meseros. |
| Textos                   | `PlainLanguageTest` (de `android-core`) ya recorre `cocina/`. Las capturas se revisaron: sin textos provisionales ni identificadores.                                                                                                                                                                                                                                                                              |

**Puntos de enganche para la D:**

- `KitchenBoard.retryLater` es hoy un reintento silencioso cada 5 s: ahí van la conexión tolerante y el aviso «Sin
  conexión…».
- Las marcas en camino (`dishesInFlight`/`cardsInFlight`) son el lugar del reintento silencioso de una marca.
- Los `id` estables de las tarjetas sirven para saber cuál es nueva (sonido y animación).
- `DisplaySettings` ya trae `soundEnabled` y `soundVolume`.

### 3. Pruebas reales

Todas en este árbol, con el código final salvo donde se dice. Resultados y listas de casos en `$SP/k1/after-C/`.

| Prueba                                                                                          | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ----------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| JVM y Robolectric de los tres builds (`--rerun`), `jvm-C.sh`                                    | **65 XML, 439 pruebas, 0 fallas**: `android/` 327, `android-core` 87 y `cocina` 25 (`BoardRulesTest` 8, `KitchenBoardTest` 10 y `KitchenAccessTest` 7). Lista de casos en `after-C/jvm/casos.txt`.                                                                                                                                                                                                                                             |
| Lint de los tres builds                                                                         | Sin problemas. Solo las 2 sugerencias `AutoboxingStateCreation` de siempre en `android/feature/access`, que esta fase no toca.                                                                                                                                                                                                                                                                                                                 |
| ktfmt de los tres builds y `fmt-check` del repositorio                                          | Correctos.                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `md5` de `docs/etapas/*.md`                                                                     | Igual que al empezar la Fase B; `docs/`, `contracts/`, `backend/`, `desktop/` y `config/` sin cambios.                                                                                                                                                                                                                                                                                                                                         |
| Instrumentadas de cocina, `Pixel_Tablet`                                                        | **17**: 11 corridas y 6 omitidas (las de `KitchenOnGoTest`, que solo corren con el guion), 0 fallas. Son `OrdersScreenTest` (c1–c6 y c8), `KitchenScreensTest` y `KitchenAppStartTest`.                                                                                                                                                                                                                                                        |
| Instrumentadas de cocina, `ADEL_A1_Tablet_API26`                                                | **17**: 11 corridas y 6 omitidas, 0 fallas.                                                                                                                                                                                                                                                                                                                                                                                                    |
| Instrumentadas de meseros y de `android-core`, `Pixel_Tablet` (por el cambio en `android-core`) | `android/` **158** (72 corridas, 86 omitidas) y `android-core` **10**; 0 fallas. Las mismas 168 de la línea base de la B.                                                                                                                                                                                                                                                                                                                      |
| Instrumentadas de meseros y de `android-core`, `ADEL_A1_Tablet_API26`                           | `android/` **158** (70 corridas, 88 omitidas) y `android-core` **10**; 0 fallas. Las mismas 168 de la línea base.                                                                                                                                                                                                                                                                                                                              |
| Suite `kitchen` contra el Go temporal, `Pixel_Tablet`                                           | **8/8 aprobadas** (`k1`–`k6` y los dos conteos de `ready_notices`), en 37 s.                                                                                                                                                                                                                                                                                                                                                                   |
| Suite `kitchen` contra el Go temporal, `ADEL_A1_Tablet_API26`                                   | **8/8 aprobadas**, en 28 s. Se instala con `adb install --no-streaming -r -t` y `--no-install` (A3-D33): en Android 8 la instalación en flujo llegó a colgarse.                                                                                                                                                                                                                                                                                |
| K1-D28, actualizar 0.4.0 → 0.4.1 encima, `ADEL_A1_Tablet_API26`, parte 1 (datos)                | Con la 0.4.0 debug, `w1` (emparejar y entrar) y `w3` (pedido sin conexión en la cola) aprobadas. Se instala encima la 0.4.1 sin desinstalar y `w4` aprueba: el pedido sale una sola vez, sin volver a entrar. La base, copiada antes y después, conserva el servidor emparejado, la mesera Ana y el pedido. El pedido ya estaba `received` al instalar encima, sin abrir la aplicación: lo mandó el trabajo en segundo plano al reprogramarse. |
| K1-D28, parte 2 (firma)                                                                         | El APK 0.4.0 entregado y la 0.4.1 release firmada tienen el mismo certificado (`SHA-256 fb6a4bac…`). `adb install -r` encima funciona: `versionCode` 4 → 5, `firstInstallTime` sin cambio. La aplicación normal nunca se abrió.                                                                                                                                                                                                                |

Detalle de la suite `kitchen` (cada paso es su propia instrumentación, con un Go temporal nuevo, su licencia de
prueba y el ejecutor de pruebas, que nunca busca en la red real):

- **`k1`** arma el restaurante por la API de la principal:
  - estaciones «Cocina K» (por platillo) y «Barra K» (por pedido), las dos en pantalla;
  - un ingrediente que se puede quitar, productos y rutas;
  - una mesa y un mesero.

  Después empareja la tableta como pantalla de cocina escribiendo la dirección.

- **`k2`**: conecta sola y muestra «No hay pedidos».
- **`k3`**: con la tableta abierta y sin tocarla, la principal manda dos pedidos y la tableta los muestra:
  - mesa: 3 tacos sin cebolla, con la nota «Bien dorados»; un agua; 2 limonadas de la barra;
  - para llevar, a nombre de Luisa Martínez: 2 hamburguesas.

  Después se agrega un flan, que llega con «NUEVO», y se cancela el agua, que queda tachada con «CANCELADO». En la
  tarjeta mezclada, la limonada dice que se marca con el botón.

- **`k4`**:
  - tocar los tacos → `kitchen/board` de Go los tiene `ready`;
  - tocar lo único del pedido para llevar → la tarjeta se cierra sola;
  - el botón de la mesa → el flan y las limonadas quedan listos y la tarjeta desaparece;
  - queda «No hay pedidos».

  El guion cuenta en la base temporal `app.ready_notices`: **6 avisos de la mesa y 2 del pedido para llevar**.

- **`k5`**: «Recientes» → «Recuperar» la hamburguesa. En Go deja de estar lista, la tarjeta vuelve a la pantalla y
  los avisos del pedido para llevar bajan de 2 a 0.
- **`k6`**: `PATCH devices/{id}` con letra grande y tema claro llega al momento, y se comprueba en lo que se ve: el
  alto del platillo pasa de 60 a 75 px en `Pixel_Tablet` y de 30 a 38 px en Android 8, y el fondo es claro en la
  captura.

Capturas en `$SP/k1/capturas-C/`:

- `pixel-tablet/` y `tablet-api26/`, todas las pantallas nuevas:
  - `c1` tarjetas;
  - `c2` colores;
  - `c3` marcado;
  - `c4` hora incorrecta;
  - `c5` Recientes;
  - `c6` muchos pedidos;
  - `c7` sin cupo, cupo liberado y sin activar;
  - `c8` los dos tamaños por los dos temas;
  - `b*`, las de la entrada.
- `e2e-5580-kitchen/` y `e2e-5582-kitchen/`, las de los pedidos reales.

Se revisaron una por una: sin textos provisionales, sin identificadores y sin nada en inglés de la aplicación. El
único texto en inglés que apareció fue el aviso del sistema «Viewing full screen», que ahora se cierra antes de
capturar.

### 4. Decisiones propuestas

1. **Un solo tablero por proceso.** `KitchenBoard` vive en el contenedor y no en la pantalla. Usa el `RealtimeClient`
   de `android-core` con la credencial de la pantalla, sin sesión. Las lecturas se piden por un canal conflado: una
   en curso y como mucho una pendiente. El canal de eventos se abre antes de `connect` y también sin cupo.
2. **Espera en vivo**: `elapsed_seconds` de Go más el reloj monotónico de la tableta desde que llegó la lectura.
   Nunca la hora de la tableta.
3. **Marca optimista sin parpadeo.**
   - Lo tocado se ve listo al momento.
   - Si Go responde, sigue así hasta la primera lectura **empezada después** de la respuesta. Una lectura que ya iba
     en camino lo hacía parpadear (listo → pendiente → listo).
   - Lo encontré revisando el código por una captura de `k4` que no mostraba la marca. Esa captura resultó ser solo de
     antes del cuadro con el cambio, pero la carrera era real: la cubre una prueba JVM, que falla sin el arreglo.
   - Con un rechazo o sin respuesta, la marca se quita y manda lo que diga Go.
4. **Tarjeta cerrada por cancelación** (K1-D23): su botón dice «Quitar de la pantalla» en vez de «Pedido listo».
5. **Tarjeta mezclada** (estaciones por platillo y por pedido en la misma cuenta, K1-D14): debajo de cada platillo por
   pedido, «Se marca con el botón «Pedido listo».». El texto del pie queda solo cuando toda la tarjeta es por pedido.
6. **El aviso de pedidos fuera de la vista** va debajo de las columnas, no encima, porque tapaba los botones de la
   última fila. Cuenta las tarjetas que no se ven **ni en parte**.
7. **Letra grande**:
   - es la tipografía ×1.25 de las tarjetas y avisos de la pantalla de pedidos, encima del tamaño de letra del sistema;
   - no se cambia `fontScale`: Compose escala la letra grande de forma no lineal y los platillos crecían solo 8 %;
   - la barra y «Recientes» quedan con letra normal;
   - el tema claro u oscuro se aplica a toda la aplicación.
8. **Recientes**:
   - es un diálogo a pantalla completa desde la barra;
   - «Recuperar» por platillo, y «Recuperar el pedido» cuando hay más de un platillo listo;
   - con `KITCHEN_NOT_RECOVERABLE` muestra el mensaje de Go y vuelve a leer `recent`.
9. **Sin cupo**: además de los eventos, la tableta vuelve a pedir `connect` cada 30 s. Sin respuesta de Go, lo
   intenta otra vez en silencio cada 5 s; la tolerancia y el aviso son de la D.
10. **`ready_notices` en la suite**: Go no los da por la API a la principal, así que el guion los cuenta en la base
    temporal, con las mismas guardas que `test_sql` (solo `adel_pos_test_…`, nunca 55432, y la terminal principal
    de ese servidor). Sin `ADELPOS_E2E_DATABASE_URL`, esos dos pasos quedan omitidos.
11. **Capturas**: `SystemDialogs.dismissFullScreenNotice` cierra el aviso «Viewing full screen». Android lo muestra
    una vez por instalación y tapaba las capturas de la aplicación de cocina.

### 5. Hallazgos y notas

- **Nada pendiente en Go** (no hizo falta `pedidos-M.md`). El contrato alcanzó tal cual, incluidos los cambios
  de la R1, que para la tableta no cambian nada.
- **Orden de los platillos** de una tarjeta: es el de Go (`items`). En un mismo envío, lo de la barra salió antes que
  lo de la cocina, y lo cancelado conserva su lugar.
- **K1-D28 en Android 8**: el pedido en cola salió al instalar encima, antes de abrir la 0.4.1, por el trabajo en
  segundo plano. `w4` confirma que llegó una sola vez.
- `upgrade.sh` ya no lee la base con `run-as … sqlite3`, porque Android 8 no lo permite. Ahora la copia con
  `exec-out run-as … cat`, junto con su `-wal`, y la lee en la Mac.
- `k1/B/e2e.sh` exporta `ADELPOS_E2E_DATABASE_URL` y `ADELPOS_E2E_PSQL` de la base temporal de B.
- **Para la D**:
  - En los emuladores, una captura tomada 0.4 s después de un cambio de estado puede salir sin el cambio. Una captura
    de la pantalla desde la Mac, 5 s después, ya lo mostraba.
  - Por eso `k4` espera a ver la palomita del platillo y `k6` espera a ver el alto del texto y el color del fondo
    antes de capturar.
  - La bandeja del sistema y la navegación quedan escondidas (pantalla completa de la B). El aviso de Android sale
    una vez por instalación: en una tableta real, el dueño lo cierra una vez.

### Decisiones del coordinador al cerrar la Fase C

- **K1-D31. Se aceptan las decisiones propuestas 1 a 11 de la Fase C.** Las tarjetas siguen el orden de Go (del envío
  más antiguo al más nuevo). Go manda como mucho 200 tarjetas, y con más de 200 pedidos abiertos el indicador no cuenta
  las que faltan. Queda como limitación.
- **K1-D32. «Volver a emparejar» en la tableta suelta el cupo.** El cupo está ligado al dispositivo (K1-D6). Si la
  tableta solo borrara lo suyo, la pantalla vieja seguiría ocupando el cupo en Terminales hasta que alguien la
  revocara, y con una licencia de una pantalla la nueva diría «No hay cupo». Por eso hay una ruta nueva de la lista
  cerrada, `POST kitchen-display/unpair`:
  - pide la contraseña del administrador como `settings-access` (mismas validaciones, mismo límite de intentos y
    `devices.manage`);
  - revoca el propio dispositivo igual que Terminales (suelta el cupo y corta el canal) y lo audita con el
    administrador como autor;
  - la tableta guarda la contraseña solo en memoria mientras los ajustes estén abiertos.

  Si la computadora principal no responde o ya no reconoce la pantalla, la tableta borra lo suyo de todos modos y dice
  que hay que revocarla en Terminales.

## Revisión R1 (correcciones) y Fase E · Terminales, licencia y el «Reconectando» del escritorio

Agente S, 24/11/2026, en `$SP/wt/k1-a` (rama `etapa-k1-a`, con `etapa-k1` `70ee1a1` y `etapa-k1-r1` fusionadas en
`f545c4d`, y después `etapa-k1` `efaaba7` en `5f1d291`), `ISO_TAG=A`. En Go solo cambiaron las correcciones de la R1, una
línea de `kitchen/access` y la ruta de K1-D32 que pidió el coordinador (abajo); los 30 s del canal siguen. No toqué
`android/`, `cocina/`, `android-core/`, `docs/etapas/` ni `contracts/`.

### Commits

| Commit    | Tema                                                                                                                                                               |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `f690bce` | Correcciones de la R1 (K1-D29). Ya unidas a `etapa-k1` por el coordinador.                                                                                         |
| `f3830bf` | Terminales: la pantalla de cocina, la versión instalada de los tres tipos, los límites de color, `kitchen/access` sin pantallas.                                   |
| `66c09f5` | Rust: el canal de eventos (`realtime.rs`) y `pool_idle_timeout(30 s)`.                                                                                             |
| `ab244fd` | React: el estado único de conexión (`navigation/serverLink.ts`).                                                                                                   |
| `85e79e1` | React: la pantalla de inicio de sesión o de alta ya cargada con la misma tolerancia (`useSession.ts`).                                                             |
| `924ce67` | React: Mesas y Preparación sin el aviso de conexión propio que `ab244fd` había vuelto a conectar (ver abajo).                                                      |
| `5f1d291` | Fusión de `etapa-k1` `efaaba7` (Fase C de M), pedida por el coordinador para K1-D32.                                                                               |
| `895efa2` | K1-D32: `POST kitchen-display/unpair` («Volver a emparejar»), pedido por el coordinador a media fase.                                                              |
| `9d7dd13` | Playwright: `native-driver.ts` con las reglas de Rust, las dos especificaciones nuevas, `pairing.ts`, los plazos de cinco y el texto «versión instalada» de la 9G. |

### Qué se hizo

**1. Correcciones de la R1 (K1-D29, `f690bce`).**

- R1-1: `kitchen-display/settings-access` valida la longitud antes de consultar al supervisor, como `auth/login` (usuario
  hasta 40 y contraseña hasta 256, sin contar un intento): 401 `AUTHORIZATION_INVALID` «El usuario o la contraseña no
  son correctos.». Vacíos, 400 `VALIDATION_ERROR`, documentado en `docs/API_V1.md`, `openapi.json` y `api-k1.md` (nota
  con hora al final).
- R1-2: para una pantalla, la marca de «visto» es `UPDATE … WHERE id=$1 AND revoked_at IS NULL` en la misma
  transacción; si no cambia ninguna fila, 401 `DEVICE_REVOKED` (`administration/kitchen_display.go`,
  `touchKitchenDisplay`). La prueba de R1-2 exige solo la respuesta corregida.
- Emparejar: `device_kind` y `X-Client-Kind` deben coincidir en los tres tipos cuando llega la cabecera (400
  `CLIENT_KIND_MISMATCH`; `administration/pairing.go`). Antes de cambiarlo revisé que el escritorio manda `desktop` y la
  aplicación de meseros `android` al emparejar (sus pruebas siguen pasando).

**2. Terminales y licencia (§6, `f3830bf`).**

- Sistema › Terminales: una pantalla se ve como «Pantalla de cocina». «Ver configuración» muestra su conexión, la
  versión instalada, el cupo («Ocupa un cupo de pantalla de cocina», o liberado: «En la tableta se puede pedir otra vez
  con «Usar esta pantalla».»), sus estaciones, letra, tema, sonido y volumen. «Editar» deja cambiar las estaciones (solo
  las que reciben comandas en pantalla: `kds` y `both`; «Mostrar todas…» o una lista), «Tamaño de letra de la
  pantalla», «Tema de la pantalla», «Sonido de pedidos» y «Volumen» (`DevicesPage.tsx`, `KitchenDisplayEditor`); guarda
  con `PATCH devices/{id}` y `PUT kitchen/access`, que ya publican `kitchen_display.configuration.changed` (Fase A).
- «Liberar cupo» de una pantalla dice el texto de K1-D30: «…En la tableta se puede pedir el cupo otra vez con «Usar
  esta pantalla». Para que deje de usarse, revócala.». «Revocar»: «La pantalla deja de mostrar pedidos al momento y
  libera su cupo de cocina…».
- La versión instalada (K1-D9) se ve en la fila de todas las terminales: «Android 14 · versión instalada 0.1.1»,
  «Esta computadora · versión instalada 0.15.0».
- Cocina › Comandas y tiempos, sección «Colores de las pantallas de cocina»: «Color de atención desde (minutos de
  espera)» y «Color de retraso desde (minutos de espera)» (10 y 20), con `kitchen.timing.manage` y la misma validación
  que Go (`KitchenSettingsForm.tsx`, `displayLimitsProblem`).
- Todo lo anterior se esconde sin la capacidad `kitchen-display-v1` (`serverCapabilities.ts`).
- Cocina › Estaciones › Terminales de cocina ya no lista las pantallas (`kitchen/reads.go`: `kind NOT IN
('android','kitchen')`): sus estaciones se eligen en Terminales.
- Los cambios llegan a la tableta al momento: lo comprueba la especificación de Playwright desde la interfaz (el evento
  de configuración llega a `events/snapshot` de la pantalla y `kitchen-display/orders` trae lo nuevo).

**3. El «Reconectando» falso del escritorio (§7, K1-D10).**

_Para el propietario._ Go cierra a propósito el canal de avisos del escritorio cada 30 segundos y la aplicación lo abre
otra vez; así el servidor vuelve a revisar la sesión, los permisos y las estaciones. Hasta la 0.15.0, la parte de Rust
del escritorio tomaba ese cierre normal como si se hubiera caído la conexión: avisaba «reconectando», esperaba medio
segundo, volvía a abrir y avisaba «conectado». Por eso la barra mostraba «Reconectando» uno o dos segundos cada 30
segundos aunque todo estuviera bien, y además todas las pantallas volvían a consultar al servidor sin necesidad. Ahora
ese cierre normal se vuelve a abrir al momento, sin avisar nada, y solo cuenta como falla lo que de verdad lo es (el
servidor no contesta, se corta a la mitad, se queda mudo 10 segundos o responde con un error). Además, el escritorio usa
la misma tolerancia que la aplicación de meseros desde la A3: un corte breve no se nota; «Sin conexión» solo aparece si
el servidor no contesta durante unos 20 segundos seguidos (o si la terminal por la red se queda sin red), y vuelve a «En
línea» con la primera respuesta.

_Rust (`66c09f5`, `desktop/src-tauri/src/realtime.rs`)._ El bucle es `follow(after, open, sink, active, timing)` con
`ChannelLink` (cuándo mandar `connected` y `reconnecting`):

- el fin normal de un canal que respondió 200 (cuerpo terminado sin error, después de 5 s o más) se vuelve a abrir al
  momento desde el mismo cursor, sin `reconnecting`, sin esperar y sin otro `connected`;
- falla real: no conectar, sin cabecera de respuesta en 10 s, cuerpo cortado, canal mudo 10 s (Go manda un mensaje cada
  750 ms), un 5xx, `interrupted` de Go o un fin antes de 5 s. Entonces `reconnecting` y espera de 0,5 a 15 s; al
  volver, un `connected`;
- 401 y 403: `unauthorized` y el canal se detiene;
- `pool_idle_timeout(30 s)` en los dos clientes `reqwest` (`main.rs` y `pairing.rs`): una conexión guardada más tiempo
  que el de Go ya no se reutiliza cerrada.

_React (`ab244fd`, `desktop/src/navigation/serverLink.ts`)._ Un solo estado con reglas puras (`ServerLinkTracker`, reloj
inyectable) y una tienda para toda la ventana (`serverLink`, `useServerLink`):

- lo alimentan cada respuesta de `services/api.ts` (cualquier respuesta HTTP es éxito; solo no llegar es falla), la
  consulta de la sesión (que pasa por `api`) y el canal (`useRealtime`: `reconnecting` es falla; `connected` o un mensaje,
  canal vivo);
- «Sin conexión»: 3 fallas seguidas y 20 s desde la primera, o sin red del sistema en una terminal por la red; «En
  línea» con el primer éxito; «Reconectando» (ámbar) solo con el canal caído más de esa tolerancia mientras Go responde;
- lo leen la barra (`serverState`), la capa «Sin conexión con el servidor» (su «Reintentar» es el de Mesas y
  Preparación desde la 9G, que bloquea esas pantallas sin desmontarlas), el aviso persistente y la sesión: con una sesión
  abierta, `App.tsx` deriva el error de conexión del estado único; en la pantalla de inicio de sesión o de alta ya
  cargada, `useSession.ts` solo pasa a «Revisa la conexión» con «Sin conexión» (`85e79e1`). Al arrancar sin servidor
  sigue siendo inmediato. Tras un «Sin conexión», la vuelta recarga todo como antes (`announceReconnection`).
- La Fase 0 nombraba el «Reintentar» de Mesas y de Preparación como lector aparte. Desde la 9G (`04a1f98`) `App.tsx` ya
  no les pasa `connectionLost`: la capa lo sustituyó. En `ab244fd` los volví a conectar y quedaba un segundo
  «Reintentar» inerte detrás de la capa; lo detectó la especificación de la 9D y lo quité en `924ce67`.

_Driver de Playwright (`e2e/native-driver.ts`)._ `streamEvents` imita a Rust (renovación silenciosa, cabecera 10 s,
canal mudo 10 s, fin antes de 5 s o `interrupted` como falla, espera creciente, `connected` una vez por falla) y las
peticiones tienen los plazos de Rust (12 s; reportes 45, licencia 80, mantenimiento 130): sin ellos, con Go pausado,
una petición del driver esperaba para siempre y la tolerancia nunca se cruzaba.

**4. K1-D32, pedido por el coordinador a media fase (`895efa2`).** `POST kitchen-display/unpair` `{username, password}`
en la lista cerrada de la pantalla:

- las validaciones de `settings-access`, ahora en una sola función (`kitchenDisplayAdministrator`): vacíos 400; más de
  40 o 256 caracteres, 401 `AUTHORIZATION_INVALID` sin contar un intento; `supervisorAuthorizer` con `devices.manage` y
  el mismo contador de intentos (por pantalla y usuario, compartido entre las dos rutas). Sin el permiso, 403
  `PERMISSION_DENIED` con la auditoría `kitchen_display.unpair_denied`;
- revoca la propia pantalla con el mismo código que Terminales › «Revocar», extraído a `revokeDevice`
  (`administration/devices.go`): fila del dispositivo, sesiones, credenciales de envío y al final el lock de cupos
  (`seats.ReleaseDevice`). El canal se corta con `interrupted` `DEVICE_REVOKED` en su siguiente lectura;
- 200 `{}`; auditoría `kitchen_display.unpaired` con el administrador como autor y la pantalla como dispositivo;
- licencia «siempre» (`RowAlways`, sin fecha); no exige cupo;
- contrato: `openapi.json` (`unpairKitchenDisplay`), `docs/API_V1.md`, `TestContractV1` y `$SP/k1/api-k1.md` (sección
  13 bis y nota de las 22:15); nombres de las dos auditorías en `desktop/src/services/auditActions.ts`.

### Decisiones propuestas

1. **Los mensajes del canal no reinician la racha de fallas de las peticiones.** «Sin conexión» lo deciden las
   peticiones a la API (la sesión se consulta cada 5 s); el canal vivo solo apaga «Reconectando». Si no, una terminal con
   el canal abierto y la API sin responder (como simula `__adelOffline` en las especificaciones de la 9D) nunca diría
   «Sin conexión». Las fallas del canal sí cuentan para «Sin conexión».
2. **Plazos del canal en Rust: 10 s para la cabecera y 10 s de canal mudo.** Con la computadora principal detenida, la
   conexión TCP puede abrirse y la respuesta no llegar nunca; Go manda un mensaje cada 750 ms, así que 10 s mudo es una
   caída clara y queda dentro de los 20 s de la tolerancia.
3. **Un fin «normal» antes de 5 s cuenta como falla**, para que un servidor que cerrara el canal enseguida no produzca un
   bucle sin espera.
4. **«Sin red del sistema» solo en una terminal por la red.** La computadora principal habla con su propio servidor por
   127.0.0.1 aunque no haya red; seguir `navigator.onLine` ahí la bloquearía sin motivo.
5. **La pantalla de acceso ya cargada (inicio de sesión, alta) usa la misma tolerancia**; al arrancar sin servidor,
   «Revisa la conexión» sigue siendo inmediato.
6. **Cocina › Estaciones › Terminales de cocina ya no lista las pantallas de cocina**: sus estaciones se eligen en
   Terminales (un solo lugar; `GET kitchen/access` excluye `kitchen` como ya excluía `android`).
7. **Plazos de 35 s en cinco especificaciones** (9d-colors, 9d-topbar-tables en dos lugares, 9f-kitchen-stations,
   9f-topbar-map y 9g-two-terminals) que esperaban «Sin conexión» en 15 s: con la tolerancia, «Sin conexión» tarda unos
   20 a 30 s (K1-D10).
8. **K1-D32: sin `devices.manage`, la auditoría es `kitchen_display.unpair_denied`** (como `settings_denied` en los
   ajustes), y el texto del 403 es «Ese usuario no puede volver a emparejar esta pantalla.».
9. **Los errores de carga de una pantalla (por ejemplo el estado «No se pudo comunicar…» del mapa de Mesas) no pasan
   por el estado único**: son la respuesta a una consulta que sí falló. Con el canal corregido ya no hay recargas cada
   30 s, y durante un corte breve no se consulta nada (no llegan eventos), así que no aparecen.

### Pruebas ejecutadas

Todas con `ISO_TAG=A`; Playwright con Go temporal 8203, vista previa 1433, simulador de licencias 8491 con
`ISO_LICENSE_TRUST` (`vectors.json`) y Go sin activar 8205 (`$SP/k1/E/pw-all.sh`, registros en `$SP/k1/E/pw-*.log`).

| Qué                                                                                                                                                                                                                                     | Resultado                                                                                                                                                                                                  |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1: `go-int ./internal/administration`, `./internal/operations -run TestStageK1`, `./internal/apicontract`; `go-unit`                                                                                                                   | Aprobado (antes de `f690bce`).                                                                                                                                                                             |
| `go-int ./internal/kitchen` (`kitchen/access` sin pantallas)                                                                                                                                                                            | Aprobado (6 s).                                                                                                                                                                                            |
| `go-int ./internal/operations` con `TestStageK1KitchenDisplay`, `TestStageK1R1KitchenDisplaySecurity`, `TestStage9FKitchenStations` y `TestRestaurantAcceptance` (usan `kitchen/access`)                                                | Aprobado (328 s).                                                                                                                                                                                          |
| `go-int ./internal/administration -run TestStageTwoAcceptance` (Terminales e inicio de la terminal)                                                                                                                                     | Aprobado (106 s).                                                                                                                                                                                          |
| K1-D32: `go-int ./internal/operations -run TestStageK1` (con la subprueba nueva y las llamadas de licencia), `./internal/apicontract`, `./internal/licensing`, `./internal/administration` completo                                     | Aprobado (64, 53, 12 y 286 s).                                                                                                                                                                             |
| `go-unit` (vet normal e `integration`, `test -race` de todo, `GOOS=windows` build y vet), después de K1-D32                                                                                                                             | Aprobado.                                                                                                                                                                                                  |
| Vitest completo con `tsc` (`iso.sh vitest`)                                                                                                                                                                                             | 107 archivos, 1014 pruebas aprobadas.                                                                                                                                                                      |
| `npm run build`                                                                                                                                                                                                                         | Correcto (también lo hace cada corrida de Playwright).                                                                                                                                                     |
| `cargo test --locked --offline` (7 pruebas nuevas del canal con un servidor HTTP en 127.0.0.1, más la ignorada de 10 minutos)                                                                                                           | 122 aprobadas, 4 ignoradas (entre ellas la de 10 minutos).                                                                                                                                                 |
| Prueba de Rust de 10 minutos sin uso contra el Go temporal, por la escucha local (`http://127.0.0.1:8203`) y por la de red (`https://127.0.0.1:8463`), a la vez, 21:11 a 21:21                                                          | Las dos aprobadas: «600 s: 816 mensajes de cambios, 1 connected, 0 reconnecting» y «600 s: 818 mensajes de cambios, 1 connected, 0 reconnecting».                                                          |
| El mismo script de la Fase F (`$SP/k1/E/idle-fase-f.sh`) con 20 s, sobre un Go temporal recién levantado                                                                                                                                | Las dos: «1 connected, 0 reconnecting».                                                                                                                                                                    |
| Playwright `workflow-stagez-k1-reconnect` en Chromium y WebKit (cada uno: más de un minuto sin hacer nada; pausas de 1, 3 y 5 s; pausa de 42 s; otra de 3 s después; lo mismo como terminal por la red, con 35 s sin hacer nada)        | 2 de 2 en cada motor (2,8 y 2,1 min). Ninguna lectura distinta de «En línea» ni capa en los tramos sin hacer nada y en las pausas breves; en la larga, «Sin conexión» y la capa, y «En línea» al reanudar. |
| Playwright `workflow-stagez-k1-terminals` en Chromium y WebKit                                                                                                                                                                          | 1 de 1 en cada motor (unos 10 s). La última corrida de la K1 en Chromium (3 de 3, 5,3 min) ya con K1-D32.                                                                                                  |
| Playwright de lo afectado, Chromium: `restaurant.spec`, 9c-navigation, 9d-colors, 9d-topbar-tables, 9f-kitchen-stations, 9f-topbar-map, 9g-terminals (2), 9g-two-terminals (2), 9g-modality, a1-pin, a2-terminals, l1-notices, l1-seats | 15 de 15 (15,3 min).                                                                                                                                                                                       |
| Lo mismo en WebKit                                                                                                                                                                                                                      | 15 de 15 (14,9 min).                                                                                                                                                                                       |
| `fmt-check` y `md5` de `docs/etapas/*.md`                                                                                                                                                                                               | Correcto; `docs/etapas` sin cambios.                                                                                                                                                                       |

Fallas encontradas por el camino, ya corregidas: un segundo «Reintentar» inerte detrás de la capa (la 9D, `924ce67`);
cuatro esperas de «Sin conexión» de 15 s más una que se me había pasado (9f-topbar-map); el texto «versión instalada»
en la 9G; en las especificaciones nuevas, el indicador fuera de la navegación principal y una consulta antes de que
terminara «Liberar cupo». Cuando una especificación falla, las siguientes de la misma corrida fallan con «La aceptación
debe comenzar con una base sin configurar» (el puente pierde la terminal preparada al reiniciarse el proceso de
Playwright): no son fallas propias.

Capturas en `$SP/k1/shots-E/` (`chromium-` y `webkit-`): `k1-terminales-configuracion`, `k1-terminales-editar`,
`k1-cocina-colores`, `k1-terminales-liberar-cupo`, `k1-reconexion-sin-conexion` y `k1-reconexion-terminal-red`.

### Prueba de 10 minutos para la Fase F

Con un Go temporal recién levantado (sin configurar) y sus dos escuchas:

```
ISO_TAG=<tag> ISO_GO_PORT=<local> ISO_NET_PORT=<red> $SP/iso.sh <árbol> server-up
$SP/k1/E/idle-fase-f.sh <árbol> <tag> <local> <red>        # 600 s por omisión; un 5.º argumento cambia los segundos
ISO_TAG=<tag> $SP/iso.sh - server-down
```

El script configura el Go temporal con su token, entra como admin, toma la huella del certificado y corre a la vez
`cargo test --locked --offline realtime::tests::idle_channel_against_go -- --ignored --nocapture` por
`http://127.0.0.1:<local>` y por `https://127.0.0.1:<red>` (con `ADEL_IDLE_FINGERPRINT`). Cada una debe terminar con
«600 s: … mensajes de cambios, 1 connected, 0 reconnecting» y `test result: ok`. La prueba rechaza 8181 y 8443.

### No verificado o pendiente

- Nada en la aplicación de escritorio instalada (.pkg): lo hará la Fase F. El «Reconectando» real lo cubren la prueba
  de Rust de 10 minutos contra Go (local y por la red) y Playwright con el puente que imita a Rust.
- El ámbar «Reconectando» (canal caído más de la tolerancia con Go respondiendo) solo está probado en Vitest
  (`serverLink.test.ts`, `navigation.test.tsx`): con Go pausado se caen las dos cosas a la vez.
- En Windows no corrí nada (`GOOS=windows` build y vet de Go sí).
- K1-D32: la pantalla de la tableta («Volver a emparejar») es de M en la Fase D.

### Decisiones del coordinador al cerrar la Fase E

- **K1-D33. Margen cuando Android dice «sin red», solo en la aplicación de cocina** (lo pidió el agente M durante la
  Fase D).
  - **El problema:** el estado de conexión de la A3 declara «Sin conexión» al instante si Android dice que no hay red.
    En una tableta, una falla de la wifi de 1 s ya mostraría el aviso grande, y el propietario pidió que una
    desconexión de un segundo no muestre nada (§5.1).
  - **La regla:** `ServerLinkTracker` recibe un margen opcional, contado desde el aviso de Android.
    - En la aplicación de cocina es de 10 s: «Sin conexión» solo si la falta de red dura más que eso.
    - Mientras dura el margen, las marcas se reintentan en silencio.
    - En la de meseros el margen es 0, así que no cambia.
  - **Por qué 10 s y no 5:** después de quitar el modo avión, la wifi tarda unos segundos en volver. Un corte de 5 s
    puede verse en Android como 7 a 10 s sin red.
- **K1-D34. Se aceptan las decisiones propuestas 1 a 9 de la Fase E.**
- **K1-D35. Dos retoques de Terminales**, vistos en las capturas:
  - «Sonido» se elige con el mismo control de dos opciones que la letra y el tema («Activado» o «Desactivado»);
  - el texto de los cupos dice que una tableta de cocina ocupa su cupo mientras esté emparejada, y que se libera con
    «Liberar cupo» o «Revocar».

## Fase D · Conexión tolerante, alertas, pantalla, ajustes y `docs/COCINA_INSTALAR.md` (§5)

Agente M, 24/11/2026, en `$SP/wt/k1-b` (rama `etapa-k1-b`, con `etapa-k1` traída hasta `895efa2`, que trae
`kitchen-display/unpair`), `ISO_TAG=B`, Go temporal 8211/8471, emuladores `Pixel_Tablet` (Android 17, consola 5580) y
`ADEL_A1_Tablet_API26` (Android 8.0, 5582), en horizontal y de solo lectura. Guiones de la fase (fuera del
repositorio) en `$SP/k1/B/`; resultados en `$SP/k1/after-D/`; capturas en `$SP/k1/capturas-D/`.

### Resumen

- **Conexión tolerante**:
  - un corte de la computadora de 1 a 5 s no muestra nada (`kill -STOP` del Go temporal);
  - un corte de red de 1, 3 y 5 s tampoco: Android ve 1,3 a 6,0 s sin red, dentro del margen de 10 s de K1-D33;
  - una marca hecha durante un corte se repite sola y Go la registra una vez;
  - un corte largo muestra arriba «Sin conexión con la computadora principal. Reconectando…», no deja marcar y se
    quita solo.
- **Alertas**:
  - un sonido con cada pedido nuevo y otro distinto con cada cancelación o cambio, generados en la aplicación;
  - salen por el canal de notificaciones, con el «Sonido» y el «Volumen» de Terminales;
  - nunca en la primera lectura, y una sola vez al volver de «Sin conexión»;
  - la tarjeta nueva late en azul.
- **Pantalla**: siempre encendida, y abre directo en Pedidos.
- **Ajustes en la tableta**:
  - se abren con una pulsación larga en el logotipo y la contraseña de un administrador, que se guarda solo en memoria;
  - muestran la dirección, la conexión y la versión;
  - «Volver a emparejar» (`kitchen-display/unpair`) suelta el cupo.
- **`docs/COCINA_INSTALAR.md`**: la guía para el dueño.
- **La prueba de punta a punta (§8) aprueba en los dos emuladores**: Ana envía a cocina desde la aplicación de
  meseros, la pantalla de cocina lo marca listo y le llega «Mesa R4 · 2 platillos listos».

### 1. Lo que se hizo

**`android-core/`**

- `model/.../connection/ServerLinkTracker.kt`: el **margen sin red** opcional (K1-D33,
  `networkLossGraceMillis`):
  - con 0, como siempre: la falta de red de Android es «Sin conexión» al instante;
  - con un margen, «Sin conexión» solo si la falta de red dura más que el margen, contado desde el aviso de Android;
  - si pasa del margen, al volver la red sigue «Sin conexión» hasta el primer éxito;
  - `nextDecisionIn` avisa cuándo vence el margen, para que el monitor vuelva a publicar el estado;
  - la tolerancia de las fallas (3 en 20 s) no cambia.
- `network/.../ServerLinkMonitor.kt`: pasa el margen al tracker. Meseros queda en 0 (sin ningún cambio); la pantalla de
  cocina usa 10 s.
- `ui/.../AdelTopBar.kt`: `onLogoLongPress`. Una pulsación larga en el logotipo abre los ajustes, también con su
  acción para lectores de pantalla («Abrir los ajustes»); un toque normal no hace nada.

**`cocina/`**

- **Conexión tolerante** (`board/KitchenBoard.kt`):
  - la pantalla sigue el estado único de conexión, el mismo de la barra (`serverLink.state`);
  - dentro de la tolerancia (y del margen) no se ve nada;
  - una marca sin respuesta queda en una cola y se repite sola cada 2 s, con su palomita a la vista;
  - con «Sin conexión», las marcas pendientes esperan y se repiten al volver; las descarta el rechazo de Go
    (`skipped` cuenta como éxito);
  - una respuesta mal formada se toma como «Go respondió»: manda la lectura;
  - al volver de «Sin conexión», lectura completa;
  - con «Sin conexión» (`BoardState.offline`):
    - un aviso grande arriba (`ui/OrdersScreen.kt`, «Sin conexión con la computadora principal. Reconectando…» y «No
      se puede marcar nada…»);
    - `markingAllowed` en falso: platillos y botones desactivados.
- **Alertas**:
  - `board/BoardAlerts.kt` compara cada lectura con la anterior por **tareas**:
    - lo nuevo pendiente → sonido de pedido nuevo;
    - lo cancelado ahora o un `pending_change` nuevo → sonido de cambio; una modificación suena solo como cambio;
    - nada en la primera lectura, por lo que recupera esta pantalla (60 s) ni por lo que otra marca listo;
    - a lo más un sonido de cada tipo por lectura.
  - `alerts/Tones.kt`: los tonos se generan en la aplicación, en PCM con subida y bajada suaves:
    - pedido nuevo: sol, si, mi, que suben;
    - cambio: dos pares de notas que bajan, más graves.
  - `alerts/ToneAlertPlayer.kt`:
    - reproduce con `AudioTrack` en un hilo propio, con uso `USAGE_NOTIFICATION_EVENT`;
    - aplica el volumen de Terminales (bajo 0,35, medio 0,65 y alto 1,0) y solo suena si «Sonido» está activado.
  - `ui/OrderCardView.kt`: el borde de la tarjeta nueva late tres veces en azul (menos de 2 s); `freshCards` guarda
    desde cuándo es nueva.
- **Pantalla** (`MainActivity.kt`):
  - `FLAG_KEEP_SCREEN_ON` en la ventana;
  - el arranque directo en Pedidos ya venía de la Fase B (lo guardado en Room).
- **Ajustes** (`settings/KitchenSettings.kt` y `ui/SettingsDialog.kt`):
  - se abren con una pulsación larga en el logotipo, que ahora se ve siempre en la barra y abre los ajustes solo con
    la pantalla lista;
  - piden el usuario y la contraseña de un administrador, que valida `settings-access`;
  - la contraseña vive **solo en memoria** y se borra al cerrarlos, al volver a emparejar, si la pantalla deja de
    estar lista y a los 2 minutos sin tocar nada;
  - muestran la dirección, la conexión y la versión;
  - «Volver a emparejar» pide confirmación y llama a `kitchen-display/unpair`:
    - con 200, se borra todo;
    - sin respuesta, o si Go ya no reconoce la pantalla (401), se borra de todos modos y se dice que hay que revocarla
      en Terminales;
    - con un 401, 403 o 429 de la contraseña, se queda abierto con el mensaje.
  - Los textos de cada error van en palabras claras: 401, 403, 429, 400 y sin conexión.
- **`docs/COCINA_INSTALAR.md`**: la guía para el dueño.

**Pruebas y guiones**

- JVM:
  - `ServerLinkTrackerTest`: 6 pruebas nuevas del margen;
  - `BoardAlertsTest` (10), `TonesTest` (3) y `KitchenSettingsTest` (9);
  - `KitchenBoardTest`: 9 pruebas nuevas, de la tolerancia (con el `ServerLinkMonitor` real y un reloj simulado), el
    reintento, «Sin conexión», los sonidos y lo recuperado.
- Instrumentadas:
  - `ConnectionAndSettingsScreensTest`: «Sin conexión», la animación, la pulsación larga, la contraseña, los ajustes y
    «Volver a emparejar»;
  - `KitchenAppStartTest` comprueba la pantalla siempre encendida.
- Contra Go:
  - `KitchenOnGoTest` k7–k13;
  - `NetworkEpisodes`: la falta de red como la ve Android;
  - `AlertsOnGoTest` e1: el lado del mesero de la prueba de punta a punta.
- `scripts/android-e2e.sh`:
  - pasos k7–k13, con los cortes en segundo plano;
  - suite `endtoend`, con las dos aplicaciones en el mismo dispositivo.

### 2. Pruebas reales

Resultados en `$SP/k1/after-D/`. «A» es `Pixel_Tablet` (Android 17) y «8» es `ADEL_A1_Tablet_API26` (Android 8).

**Comprobaciones de los tres builds**

| Prueba                                          | Resultado                                                                                     |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------- |
| JVM y Robolectric (`--rerun`, `jvm-D.sh`)       | **68 XML, 476 pruebas, 0 fallas**: `android/` 327, `android-core` 93 (model 60) y `cocina` 56 |
| Lint                                            | Sin problemas; solo las 2 sugerencias de siempre en `android/feature/access`                  |
| ktfmt, `fmt-check`, `md5` de `docs/etapas/*.md` | Correctos; `backend/`, `desktop/`, `config/` y `contracts/` sin cambios                       |

**Instrumentadas, en cada emulador**

| Prueba                                           | Resultado                                                                                                                    |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| Cocina (`connectedDebugAndroidTest`)             | 15 corridas, 0 fallas. Las de Go se omiten aquí: corren con su guion                                                         |
| Meseros y `android-core` (`android-core` cambió) | **A**: 159 (87 omitidas) + 10. **8**: 158 (88 omitidas) + 10. 0 fallas, los mismos casos de la línea base más `e1` (omitida) |

**Suite `kitchen` contra el Go temporal, en los dos emuladores**

| Paso                     | Resultado                                                                                                                                                                                                                     |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| k1–k6 (los de la Fase C) | Aprobados en las dos corridas completas, A y 8                                                                                                                                                                                |
| k7                       | Aprobado en A y 8. Cinco pausas del Go de 1 a 5 s con 5 s entre ellas, ningún aviso, «sin respuesta 0»                                                                                                                        |
| k8                       | Aprobado en A y 8 (corrida `k1`–`k8`). Corte de 18 s; la marca se queda sin respuesta una vez, se repite y Go la registra una sola vez; ningún aviso                                                                          |
| k9                       | Aprobado en A y 8. Pausa de 45 s: el aviso aparece, el botón está desactivado, tocar no marca y se recupera solo. El aviso se vio 4 s: con el Go pausado, la primera falla llega a los 15 s y el aviso a los 41 s (sección 5) |
| k10                      | Aprobado en A (modo avión) y en 8 (wifi y datos con root). Medidas en la sección 3                                                                                                                                            |
| k11                      | Aprobado en A y 8. Contraseña equivocada con «El usuario o la contraseña no son correctos.»; con la del administrador, dirección `10.0.2.2:8471`, «En línea» y `0.1.0`; al cerrar se borra la contraseña                      |
| k13                      | Aprobado en A y 8. Vuelve a «Buscar el servidor»; en Go, la pantalla queda revocada (`revoked_at`) y `devices/sessions-summary` pasa de 1 a 0 pantallas de cocina en uso                                                      |

**Suite `endtoend` (§8)**

| Prueba              | Resultado                                                                                                                                                                                                                                                           |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `endtoend` en A y 8 | **4/4 en los dos**: r1 (meseros) prepara y entra Ana; c1 empareja la cocina; e1 Ana envía 2 Filete R desde la aplicación y cierra; k12 (cocina, en paralelo) ve la tarjeta «Mesa R4» y la marca lista; a Ana le llega «Mesa R4 · 2 platillos listos / 2 × Filete R» |

**Capturas** (`capturas-D.sh`, 14 pruebas en cada emulador, más las de las suites), en `$SP/k1/capturas-D/`:

- `d1` «Sin conexión»;
- `d2` la tarjeta nueva a media animación;
- `d3` la contraseña y el error sin permiso;
- `d4` los ajustes;
- `d5` «Volver a emparejar»;
- `d6` «revócala en Terminales»;
- de las suites:
  - `k8` el platillo marcado durante el corte;
  - `k9` «Sin conexión»;
  - `k11` los ajustes y la contraseña equivocada;
  - `k13` la confirmación y «Buscar el servidor».

Todas se revisaron: ningún texto provisional ni identificador.

**Lo que falló y se corrigió**:

- en la primera corrida de la suite, k8 (cómo se leía el historial de Go), k10 en los dos emuladores (el guion) y k11 y
  k13 en A, que fallaron porque el modo avión quedó encendido;
- las dos primeras corridas de `endtoend`: la mesa R8 fuera de la vista, y después el salón que agregaba k1.

Los detalles están en la sección 5.

### 3. Medidas del modo avión y del corte de red (K1-D33)

`k10` corta la red del emulador desde la Mac:

- `Pixel_Tablet`: modo avión real (`cmd connectivity airplane-mode`);
- `ADEL_A1_Tablet_API26`: Android 8 no tiene esa orden, así que se cortan los datos y después la wifi con `adb root`
  (`svc`), y se devuelven al revés.

La prueba anota cuándo Android pierde y recupera la red predeterminada (el mismo aviso que usa la aplicación) y cuándo
se ve «Sin conexión». El guion anota además cuánto tardó Android en tener red otra vez, según
`dumpsys connectivity`.

| Corte | `Pixel_Tablet` (modo avión): sin red para Android | `ADEL_A1_Tablet_API26` (wifi y datos): sin red para Android | Aviso                                                                 |
| ----- | ------------------------------------------------- | ----------------------------------------------------------- | --------------------------------------------------------------------- |
| 1 s   | 2,9 s                                             | 1,3 s                                                       | ninguno en los dos                                                    |
| 3 s   | 4,0 s                                             | 3,3 s                                                       | ninguno en los dos                                                    |
| 5 s   | 6,0 s                                             | 5,3 s                                                       | ninguno en los dos                                                    |
| 35 s  | 36,1 s                                            | 35,3 s                                                      | el aviso aparece al pasar el margen de 10 s y se quita solo al volver |

Con el modo avión, después de quitarlo la wifi tarda 1 a 2 s en volver. Ningún corte de 5 s pasó de los 10 s del
margen, así que no hizo falta cambiarlo.

La primera corrida del guion tenía el modo avión al revés («enable» cortaba la red) y midió unos 21 s sin red en cada
corte. Era un error del guion, no del emulador: se corrigió y se volvió a medir.

### 4. Decisiones propuestas

1. **El margen sin red** (K1-D33, del coordinador) es un parámetro del tracker compartido, con 0 por omisión. Meseros
   no cambia.
2. **Reintento de marcas**:
   - cada 2 s mientras haya «En línea», también dentro de la tolerancia;
   - con «Sin conexión», esperan y se repiten al volver;
   - no hay número máximo de intentos: es idempotente por versión y la palomita se queda hasta que Go responde.
3. **El aviso grande** se ve en cualquier pantalla de pedidos (también «Conectando» o sin cupo) mientras el estado único
   de conexión diga «Sin conexión». La barra también lo dice.
4. **Sonidos**:
   - a lo más uno de cada tipo por lectura;
   - **nunca en la primera lectura** (al abrir la aplicación, al emparejar o al salir de una pantalla sin pedidos);
   - **al volver de «Sin conexión», una sola vez** por lo que llegó mientras tanto, comparado con lo último que se vio;
   - lo que esta pantalla recupera no suena durante 60 s;
   - lo que se recupera desde otra pantalla o desde el escritorio sí suena como pedido nuevo, porque la tableta no lo
     distingue.
5. **El canal de audio** es el de notificaciones (`USAGE_NOTIFICATION_EVENT`):
   - suena aunque el volumen de multimedia esté bajo;
   - respeta «No molestar»;
   - el volumen que manda es el de notificaciones. Lo dice la guía.
6. **La animación** marca las tarjetas nuevas y las cambiadas: el borde azul late tres veces.
7. **Los ajustes**:
   - el logotipo se ve siempre en la barra de cocina; la pulsación larga solo abre los ajustes con la pantalla lista;
   - el usuario se escribe, no hay un «admin» fijo;
   - se cierran solos a los 2 minutos sin tocar nada.
8. **«Volver a emparejar» sin respuesta** borra lo de la tableta y lo dice: «La computadora principal no respondió. Esta
   pantalla ya no está emparejada aquí, pero puede seguir en Terminales ocupando su cupo de cocina: revócala ahí.».
9. **Pruebas contra Go**: la marca que se repite se prueba con el Go pausado 18 s. Con cortes de 1 a 5 s la petición
   solo espera (el cliente espera 15 s), así que nada falla y no hay qué repetir. Con 18 s sí se queda sin respuesta, y
   no llega al aviso: hacen falta 3 fallas y 20 s.

### 5. Hallazgos y pendientes

- **Ninguna ruta nueva de Go**: con `unpair` (`895efa2`) alcanzó. No hizo falta `pedidos-M.md`.
- **El canal de eventos se renueva cada 30 s sin contar como falla.**
  - En k7 la pantalla estuvo abierta más de 45 s con cinco pausas del Go (más de una renovación del canal).
  - Terminó con «sin respuesta 0» (`serverLink.noAnswers`) y ningún aviso.
  - Lo mismo en k8, k11 y k13 (minutos con la aplicación abierta).
- **Diálogos y barras del sistema**: con un diálogo abierto (ajustes o «Recientes»), Android muestra un momento las
  barras del sistema encima de la pantalla completa. Se esconden al cerrarlo. Es solo de aspecto.
- **La marca que se repite y el historial de Go**:
  - Go registra la marca una sola vez: un solo `states.changed` de esa pantalla desde la recuperación de k5;
  - la petición original, que llega cuando el Go se reanuda, la marca;
  - la repetición vuelve con `already_ready`.
- **Primera corrida de la suite**: falló en partes del guion y de las pruebas, no de la aplicación:
  - el modo avión al revés en el guion;
  - los datos y la wifi cortados en dos pasos en Android 8;
  - el historial de cocina leído como objeto, y contando también la marca de k4;
  - la mesa R8 fuera de la vista;
  - el salón «Salón K» que agregaba k1: el mesero veía ese salón primero. En `endtoend`, k1 corre con
    `-e restaurant existing` y no arma nada.

  Se corrigió y se volvió a correr; los resultados de la sección 2 son los de la última corrida.

- **El aviso con la computadora colgada tarda más que con la red cortada.** Con el Go pausado, la computadora acepta
  la conexión pero no responde, así que cada falla tarda lo que el plazo de la petición:
  - el canal da la conexión por muerta a los 10 s;
  - la primera falla que cuenta llega hacia los 15 s;
  - el aviso sale a los ~41 s (hacen falta 3 fallas en 20 s).

  Con la red cortada, Android lo dice y el aviso sale a los 10 s del margen. Sigue la regla de la A3, que esta fase no
  cambia.

- **Me equivoqué una vez con el guion**: cambié una línea de `scripts/android-e2e.sh` mientras corría la suite
  `endtoend` en 8. Esa corrida terminó con un error de sintaxis al final (después de sus pasos) y se repitió entera.
- **Para la Fase F** (o el coordinador): `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md`
  quedan por actualizar con la etapa. Esta fase solo agregó `docs/COCINA_INSTALAR.md`.

### Decisiones del coordinador al cerrar la Fase D

- **K1-D36. Se aceptan las decisiones propuestas 1 a 9 de la Fase D.** Las medidas confirman el margen de 10 s de
  K1-D33: ningún corte de 5 s pasó de 6 s sin red para Android.
- **K1-D37. Limitación conocida: con la computadora principal colgada, el aviso tarda unos 41 s.**
  - Pasa cuando la computadora acepta la conexión pero no responde. Cada falla dura lo que el plazo de la petición
    (15 s), y la regla de la A3 pide 3 fallas en 20 s.
  - Con la red cortada, el aviso sale a los 10 s del margen.
  - No se cambia la regla de la A3, que las dos aplicaciones comparten. Mientras tanto la tableta sigue mostrando los
    pedidos y reintenta en silencio.

---

## Fase F · Versiones, regresión final y entregables (§8 y §9)

Agente F, 24 y 25/11/2026, en `$SP/wt/k1-f` (rama `etapa-k1-f`, desde `etapa-k1` `e6c03a4`). Usó estos recursos:

- `ISO_TAG=F`, con el Go temporal en 8231/8501;
- Playwright con la vista previa 1443, el simulador de licencias 8511 y el Go sin activar 8233;
- un Go limpio (`ISO_TAG=FC`, 8232/8502) para las cuatro pruebas de «instalación limpia»;
- los emuladores `Pixel_Tablet` (Android 17, consola 5580), `ADEL_A1_Tablet_API26` (Android 8.0, 5582) y el teléfono
  `Pixel_8` (Android 17, 5586), sin ventana y de solo lectura.

Los guiones y registros (fuera del repositorio) están en `$SP/k1/F/`. El detalle completo, con los entregables, está en
[VERIFICATION_STAGEK1.md](VERIFICATION_STAGEK1.md).

### Resumen

- **Versiones** (`4debb8c`):
  - servidor **0.16.0**, con el esquema 29 y `min_client_versions` sin cambio;
  - la aplicación de cocina exige el servidor 0.16.0 con `kitchen-display-v1` (K1-D27);
  - meseros queda en 0.4.1 (5) y cocina en 0.1.0 (1).
- **Regresión**: el servidor y el escritorio, sobre una copia exacta de `4debb8c`; Android, sobre una de `43b2bd4`, con
  la corrección del texto compartido. Todo aprobó, con tres salvedades explicadas:
  - `TestOperationalReports` corrió a las 00:00 y falló en su franja conocida de 00:00 a 05:00 (§6 de `CLAUDE.md`). El
    coordinador la repitió después de las 05:00 (K1-D40);
  - Playwright tuvo que repetirse, sin ningún cambio en la aplicación (K1-D39):
    - en Chromium, por la medianoche (`workflow-stagel1-notices` y su cascada);
    - en WebKit, por una carrera de `workflow-stage9d-accounts`;
    - al final, 53 de 53 en cada motor;
  - `takeaway t6` en Android 8, el pendiente conocido A3-D32.
- **El «Reconectando»**: la prueba de 10 minutos sin uso, por la escucha local y por la de red a la vez, terminó con
  **0 `reconnecting`** en las dos.
- **Dos correcciones**:
  - `dd532f8`: una prueba de Vitest de la 9F con una carrera;
  - `43b2bd4`: el texto «Actualiza el servidor del restaurante» de la tableta de cocina, que nombraba a meseros y su
    versión.
- **Entregables**: el APK de meseros 0.4.1, el de cocina 0.1.0 y el `.pkg` 0.16.0, con sus comprobaciones.
  - Se compilaron sobre una copia exacta de `9b9f472`; después solo cambió Markdown.
  - El `.pkg` no se instaló.
- **Documentos**:
  - `cocina/CLAUDE.md`, `android-core/CLAUDE.md` y `android/CLAUDE.md`, al día con las fases C y D;
  - `CLAUDE.md`, sin nada «en curso» de la K1;
  - `docs/COCINA_INSTALAR.md`, con el nombre del APK;
  - `docs/licencias`, que ahora dice que la 0.16.0 también es de desarrollo;
  - y `docs/VERIFICATION_STAGEK1.md`, nuevo.

### Commits (rama `etapa-k1-f`)

- `4debb8c`: versión 0.16.0 del servidor; la aplicación de cocina exige el servidor 0.16.0.
- `dd532f8`: la prueba del primer toque de Preparación espera a que el efecto lo escuche.
- `43b2bd4`: «Actualiza el servidor del restaurante» con la versión que pide cada aplicación.
- `9b9f472`: `cocina/CLAUDE.md`, `android-core/CLAUDE.md` y `android/CLAUDE.md`, al día con las fases C y D.
- `2e81ff1`: `CLAUDE.md` y `docs/VERIFICATION_STAGEK1.md`, con la regresión final y los entregables.

### Pruebas reales

| Qué                                                                                                    | Commit                | Resultado                                                                                                                                                                                                                                                                          |
| ------------------------------------------------------------------------------------------------------ | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fmt-check`, `sync-check` y `md5` de `docs/etapas`, `contracts/` y del contrato y la guía de licencias | todos                 | Aprobados; los protegidos, sin cambios                                                                                                                                                                                                                                             |
| `go-unit` (vet, `-race`, `GOOS=windows` build y vet) y `go vet` con `legacydocker`                     | `4debb8c`             | Aprobado (39 paquetes `ok`)                                                                                                                                                                                                                                                        |
| `go-int` de los 16 paquetes de `scripts/database.py` y `./internal/kitchen`                            | `4debb8c`             | 16 de 17 (`operations` 886 s, `administration` 325 s, `cmd` 70 s con el candado); `reports/operational`: fallaron las 4 subpruebas de siempre de la franja de 00:00 a 05:00 (corrió a las 00:00)                                                                                   |
| `go-int ./internal/reports/operational`, repetida por el coordinador después de las 05:00              | `2e81ff1`             | **Aprobado** a las 05:02 (42 s), con las 4 subpruebas de la franja de 00:00 a 05:00                                                                                                                                                                                                |
| Vitest con `tsc`                                                                                       | `4debb8c` y `9b9f472` | 1 014 de 1 015 (la de la 9F, corregida en `dd532f8`); sobre `9b9f472`, **1 015 de 1 015** con la Mac cargada (carga 20)                                                                                                                                                            |
| `npm run build` y `cargo test`                                                                         | `4debb8c`             | Aprobado; 122 y 4 ignoradas                                                                                                                                                                                                                                                        |
| Scripts del instalador sin root y `bash -n`                                                            | `4debb8c`             | 31 comprobaciones aprobadas                                                                                                                                                                                                                                                        |
| Playwright Chromium completo (53)                                                                      | `4debb8c`             | 47 + las 6 repetidas: **53 de 53**                                                                                                                                                                                                                                                 |
| Playwright WebKit completo (53)                                                                        | `4debb8c`             | Segunda corrida: **53 de 53**                                                                                                                                                                                                                                                      |
| La prueba de Rust de 10 minutos, local y por la red                                                    | `4debb8c`             | 817 y 818 mensajes, **1 connected, 0 reconnecting** en las dos                                                                                                                                                                                                                     |
| JVM, Robolectric, Lint y ktfmt de los tres builds                                                      | `4debb8c` y `43b2bd4` | 68 XML, **476 pruebas**, 0 fallas; Lint y ktfmt aprobados                                                                                                                                                                                                                          |
| Instrumentadas: `android-core` / meseros / cocina                                                      | `43b2bd4`             | `Pixel_Tablet` 10 / 159 / 28; `ADEL_A1_Tablet_API26` 10 / 159 / 28; `Pixel_8` 10 / 159; 0 fallas                                                                                                                                                                                   |
| Capturas de cocina (14 por tableta)                                                                    | `43b2bd4`             | Revisadas; el defecto de `b4` se corrigió en `43b2bd4`                                                                                                                                                                                                                             |
| `--suite all` de meseros                                                                               | `43b2bd4`             | **84/84** en `Pixel_Tablet` y en `Pixel_8`; **83/84** en Android 8, por partes (`takeaway t6`, A3-D32; repetida, 8/9 otra vez)                                                                                                                                                     |
| `--suite kitchen` (con `kill -STOP` solo al Go temporal)                                               | `43b2bd4`             | **14/14** en las dos tabletas; `k10`: 2,9/4,0/6,1/36,1 s y 1,3/3,3/5,3/35,3 s sin red, aviso solo en el largo                                                                                                                                                                      |
| `--suite endtoend`                                                                                     | `43b2bd4`             | **4/4** en las dos tabletas                                                                                                                                                                                                                                                        |
| Actualizar 0.4.0 → 0.4.1 encima, datos y firma                                                         | `43b2bd4`             | Aprobada en `Pixel_Tablet`                                                                                                                                                                                                                                                         |
| APK y `.pkg` de entrega                                                                                | `9b9f472`             | `apksigner` (v2, `fb6a4bac…7c86` en los dos), `aapt2` (`minSdk` 26, `targetSdk` 36, nombres e íconos), `pkgutil --expand-full` (0.16.0 en los tres paquetes, esquema 29 y la `0029` embebida), `codesign --verify` y sin llaves ni archivos de prueba; los anteriores, sin cambios |

### Lo corregido

- **`dd532f8`** (solo una prueba):
  - el problema: la prueba de Vitest del primer toque de Preparación (revisión 9F) tocaba antes de que React corriera
    el efecto que lo escucha, y con la Mac cargada falló una vez;
  - la corrección: ahora toca hasta que el efecto escucha;
  - la comprobación: con el efecto retrasado a propósito, la prueba anterior falla y la nueva aprueba.
- **`43b2bd4`** (regla de textos, `android-core`):
  - el problema: en la tableta de cocina, «Actualiza el servidor del restaurante» decía «…la aplicación de meseros…
    (necesita la versión 0.13.0 o posterior)»;
  - la corrección: el texto ya no nombra ninguna aplicación y recibe la versión mínima de cada una (meseros 0.13.0,
    cocina 0.16.0); `KitchenScreensTest` lo comprueba;
  - como toca código compartido de Android, toda la regresión de Android corrió sobre `43b2bd4`.

### Entregables

| Archivo                                                                  | Bytes      | SHA-256                                                            |
| ------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------ |
| `desktop/src-tauri/target/release/bundle/macos-pkg/AD-EL POS_0.16.0.pkg` | 23 957 122 | `add1e9e7e9fb46cdc6649cfc3edc7e692ecabea989d66339a775faf5307d3f47` |
| `android/app/build/outputs/apk/release/AD-EL-POS-Android-0.4.1.apk`      | 2 674 583  | `d783157306448bcde7368285e9fa6121fadea01e1720e3c75464099d1c9e8cfd` |
| `cocina/app/build/outputs/apk/release/AD-EL-POS-Cocina-0.1.0.apk`        | 1 878 017  | `36bb9822660cda64a2fc37f0941d79d7f68ce4cfae97bcc8eb556b91f44ae039` |

Hay copias en `$SP/`. El coordinador volvió a comprobar los tres SHA-256 en las dos ubicaciones, más:

- la firma de los dos APK (`fb6a4bac…7c86`);
- `minSdk` 26 y `targetSdk` 36;
- los paquetes `com.adelpos.meseros` 0.4.1 (5) y `com.adelpos.cocina` 0.1.0 (1), con los nombres «AD-EL POS» y «AD-EL POS
  Cocina».

El `.pkg` va sin firma de instalador, como el 0.15.0 (es de desarrollo). Los entregables anteriores no cambiaron.

### No verificado

- El `.pkg` instalado, que es la parada del propietario.
- Dispositivos físicos, wifi real, la aplicación real de Tauri, Windows y Docker.
- `ADEL_A1_Phone_API33` (del propietario), la suite `connection` y la aplicación de cocina en un teléfono.
- `takeaway t6` en Android 8 (A3-D32).

### Decisiones del coordinador al cerrar la Fase F y la etapa

Fusionada en `etapa-k1` con avance rápido (`2e81ff1`).

- **K1-D38. Se aceptan las dos correcciones de la Fase F.**
  - `dd532f8` solo cambia una prueba.
  - `43b2bd4` corrige la regla de textos del código compartido, con la versión mínima de cada aplicación (K1-D27).
  - Ninguna cambia una decisión de la especificación ni del avance.
- **K1-D39. Las repeticiones de Playwright son fragilidad de las pruebas, no de la aplicación.**
  - **Chromium**: `workflow-stagel1-notices` cerró el aviso diario a las 23:59:58 y volvió a entrar a las 00:00:05. Ya
    era otro día y el aviso volvió a salir, como debe.
  - **WebKit**: `workflow-stage9d-accounts` vuelve a entrar por la API con la misma terminal justo después de entrar por
    la página.
    - Una petición de la página que ya iba en camino con la sesión anterior recibe «sesión reemplazada», que es lo
      correcto.
    - La aplicación no tiene ese segundo inicio de sesión.
  - Las dos aprobaron al repetirse.
  - Quedan anotadas para una etapa siguiente: que la de la L1 no dependa de la medianoche, y que la de la 9D espere a
    que la página termine sus peticiones antes de volver a entrar.
- **K1-D40. `TestOperationalReports` aprobó fuera de su franja.** El coordinador repitió
  `go-int ./internal/reports/operational` a las 05:02 del 25/11/2026 sobre `2e81ff1`, con el mismo código que el
  final, y aprobó entero (42 s), con las 4 subpruebas que habían fallado a las 00:00. Con eso la integración queda en
  17 de 17. La K1 no tocó `internal/reports`.
- **K1-D41. `takeaway t6` en Android 8 sigue como el pendiente conocido** (A3-D32, L1-D63).
  - Falló igual que en la A3 y la L1, sin rechazos de licencia, cupo u hora.
  - No lo causa la K1 y no detiene la etapa.
- **K1-D42. Se acepta el Go limpio en 8232/8502** (`ISO_TAG=FC`), un puerto que no estaba en el encargo. No choca con el
  servicio real (8181, 8443) ni con otro agente.
- **K1-D43. La etapa K1 queda terminada**: se fusiona con `main` y se sube.
  - **Lo que hace el propietario**:
    - instala el `.pkg` 0.16.0 con «Servidor del restaurante» marcado. El `host` respalda y aplica la `0029`, y las
      terminales y tabletas emparejadas siguen emparejadas;
    - instala la 0.4.1 encima de la 0.4.0 en los teléfonos y tabletas de meseros, sin desemparejarlos;
    - instala la aplicación de cocina en las tabletas de la cocina, con `docs/COCINA_INSTALAR.md`.
  - **Avisos para el propietario**:
    - después de «Liberar cupo», la tableta no vuelve a tomar su cupo sola: hay que tocar «Usar esta pantalla» en ella,
      y lo definitivo es «Revocar» (K1-D6, K1-D30);
    - Go no se tocó para el «Reconectando» y la renovación del canal cada 30 s sigue (K1-D10).
  - **Quedan para etapas siguientes**:
    - las limitaciones conocidas: el tope de 200 tarjetas (K1-D31) y los unos 41 s del aviso con la computadora
      principal colgada (K1-D37);
    - lo anotado en K1-D39.
