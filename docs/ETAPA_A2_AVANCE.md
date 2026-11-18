# Etapa A2 (Android) · Avance

Segunda etapa de la aplicación Android de meseros: identificador definitivo `com.adelpos.meseros`, modo Para llevar,
imprimir la precuenta desde el dispositivo y aviso de platillos listos con la pantalla apagada o la aplicación cerrada.
Especificación: [docs/etapas/ETAPA_A2_ANDROID.md](etapas/ETAPA_A2_ANDROID.md). Rama `etapa-a2-android`, creada desde
`main` (servidor 0.12.0, esquema 25, Android 0.1.0, con la A1 fusionada; el servicio de la Mac ya está en 0.12.0).
Coordinador y como máximo dos agentes a la vez, cada fase en su árbol de trabajo, probada en copias aisladas (clúster
temporal 55499, Go temporal en modo de pruebas con su escucha de red en loopback y emuladores sin ventana en modo de
solo lectura). **Ninguna prueba usa el servicio real de la Mac** (8181, 8443, 55432 ni su carpeta); se aplica la
protección que dejó la A1 tras el incidente de su Fase F (tipo de anuncio de pruebas `_adelpostest._tcp`, wifi del
emulador apagada si ve el anuncio real y parada si una lista muestra un servidor real).

**Dispositivos**: los cuatro emuladores de la A1: `Pixel_8` (teléfono) y `Pixel_Tablet` (tableta) con Android 17 del
propietario, abiertos siempre de solo lectura, y `ADEL_A1_Phone_API33` (Android 13) y `ADEL_A1_Tablet_API26`
(Android 8.0, 1280×800). Sin dispositivo físico.

**Punto de partida del repositorio (18/11/2026)**: el 17/11/2026 el propietario reescribió la historia del remoto para
quitar las líneas `Co-Authored-By` de todos los commits y actualizó `main` en GitHub (`be6115a`, con los mismos
árboles que la historia anterior, `68beea2`). La rama de la etapa se creó desde ese `origin/main`; el `main` local de
la Mac todavía apunta a la historia anterior (mismo contenido) y se alinea al fusionar, en la Fase F. Ningún commit de
la etapa lleva líneas de atribución (regla del propietario, en `CLAUDE.md` §0).

---

## Fase 0 · Verificación

Rama `etapa-a2-0`, un solo agente, 18 de noviembre de 2026. `$SP` es el scratchpad de la sesión del coordinador.

Verificación de §1 de `docs/etapas/ETAPA_A2_ANDROID.md` sobre `etapa-a2-android` (`1198148`): servidor 0.12.0, esquema
25 (`config/product.json`), Android 0.1.0 (`android/gradle.properties`). No se cambió nada del repositorio. La prueba
de humo del servicio en primer plano vive fuera, en `$SP/spike-a2/` (desechable). Rutas `ruta:línea` relativas a la
raíz del repositorio (`backend/internal/…` abreviado como `internal/…`).

**Resultado.** No hay ninguna contradicción que obligue a cambiar una regla de negocio. Todo lo que pide la etapa se
puede hacer con cambios compatibles del contrato v1, una migración `0026` (secciones C y D) y permisos nuevos en
`WaiterPermissions`. Hay **siete decisiones para el coordinador** (§0.7.5); ninguna es una parada. Tipo de servicio
recomendado: **`specialUse`**, comprobado en Android 17 y Android 8.

### 0.1 Para llevar en la API v1

En Go, un pedido para llevar es **una cuenta con `mode: "takeaway"` y sin mesa**
(`internal/operations/accounts_write.go:17-26`: `takeaway.manage` y «Los pedidos para llevar no utilizan mesas»). No
hay mesa ficticia, como pide §0-B. La caja usa la misma creación diferida que las mesas.

| Necesidad                                      | Qué existe                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Qué falta                                                                                                                                                                                                                                       | Cambio propuesto (fase)                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Crear con creación diferida                    | `openAccount` (`POST restaurant/accounts`) con `mode: "takeaway"` e `items` crea la cuenta y su borrador en una sola operación idempotente (`accounts_write.go:152-196`). Exige `accounts.open` (`handler.go:205`), `takeaway.manage` (`handler.go:124-128`) y, con productos, `orders.capture`, `catalog.read` y `catalog.prices.read` (`handler.go:131-134`). Sin nombre, Go pone «Pedido para llevar» (`account_names.go:45-56`). El escritorio manda `origin: "Para llevar"` (`desktop/src/hooks/useDraftSync.ts:36-49`). `seen_account_ids` no aplica (sin mesa, `accounts_write.go:207-209`).               | Solo permisos: `takeaway.*` no está en `WaiterPermissions` (`internal/administration/waiter_scope.go:14-36`).                                                                                                                                   | Agregar `takeaway.read` y `takeaway.manage` a `WaiterPermissions` (B).                                                                                                                                                                                                                                                                                                                                                                                                                |
| Capturar, guardar y «Enviar a cocina» en línea | `saveDraft`, `previewDraft` y `confirmDraft` valen para cualquier cuenta. En una cuenta para llevar, toda escritura exige `takeaway.manage` y toda lectura `takeaway.read` (`queries.go:139-148`, `authorizeAccount`).                                                                                                                                                                                                                                                                                                                                                                                            | Permisos (fila anterior).                                                                                                                                                                                                                       | —                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Pedido sin conexión                            | `POST restaurant/offline-orders` acepta `destination.account_id` de una cuenta para llevar ya creada (`offline_orders.go:321-333`, con `authorizeAccount(account, true)` → `takeaway.manage`).                                                                                                                                                                                                                                                                                                                                                                                                                    | **Un pedido nuevo para llevar capturado sin conexión no tiene destino**: `destination` es `account_id` o `table_id`, uno de los dos (`offline_orders.go:117-123`, `220-222`). La cuenta nueva siempre nace `dine_in` (`offline_orders.go:827`). | Destino nuevo, opcional y compatible: `destination.takeaway: {customer_name, phone}` (exclusivo con `account_id` y `table_id`). Abre una cuenta para llevar con `origin` «Para llevar», responsable = quien envía, y exige `accounts.open` + `takeaway.manage`. Misma clave, mismas reglas de A1-D6, A1-D18 y A1-D19. Pruebas en `TestContractV1` y en `stagea1_*`/`stagea2_*_test.go` (B).                                                                                           |
| Nombre y teléfono en cualquier momento         | Al abrir: `customer_name` (100) y `phone` (40) en `AccountInput` (`models.go:66-79`, validación en `accounts_write.go:10-15`). Para editarlos después: `PATCH restaurant/accounts/{id}` (`updateAccount`, `accounts_write.go:228-270`), que exige **`accounts.manage`** (`handler.go:223`), la **revisión vigente** (`ensureOpen`) y el cuerpo completo.                                                                                                                                                                                                                                                          | `accounts.manage` permite además renombrar cualquier cuenta y cambiar observaciones y programación. La revisión obligatoria choca con la cola sin conexión: el borrador sube la revisión en cada guardado.                                      | **Decisión 1.** Propuesta: una ruta estrecha `POST restaurant/accounts/{id}/customer` `{operation_id, customer_name, phone}` con `takeaway.manage`, sin revisión (cambia solo esos dos campos), con evento `restaurant.account.updated` e idempotente. Sin conexión, el dato va en el destino `takeaway` o queda en la cola como operación propia. Alternativa: dar `accounts.manage` al mesero y usar `PATCH` con su revisión (B).                                                   |
| Listar pendientes                              | `getRecoveryState` ya devuelve las cuentas para llevar abiertas con `takeaway.read` (`recovery.go:52-54`). `listAccounts?mode=takeaway&state=open` (`accounts_read.go:101-143`) también. Cada cuenta trae `folio`, `name`, `customer_name`, `phone`, `elapsed_seconds`, `urgency`, `preparation_state`, `payment_state`, `delivery_state` y `assigned_user_id`/`assigned_name` (`models.go:99-145`), calculados por Go (`accounts_read.go:38-72`). Los cambios llegan por el canal con ámbito **`takeaway`** (`queries.go:85-88`, `kitchen/preparation.go:104-107`; `events/store.go:116` exige `takeaway.read`). | El esquema `Account` del contrato no documenta `customer_name`, `phone` ni `origin`, aunque Go ya los entrega. `RealtimeScopes.TAKEAWAY` ya existe en Android.                                                                                  | Documentar esos campos en `openapi.json` y `docs/API_V1.md` (B). Correspondencia en el dispositivo (solo se muestra lo que dice Go): pendiente = `released`/`pending_release`/`scheduled`; en preparación = `preparing`; listo = `ready` (`kitchen/states.go:165-210`); pago `not_recorded`/`partial`/`paid` (`payments/finance.go:482-490`).                                                                                                                                         |
| La entrega exige pago                          | `POST payments/accounts/{id}/deliver` con `takeaway.deliver` (`payments/routes_extended.go:26-27`). Responde 409 **`BALANCE_PENDING`** («Cóbralo antes de marcarlo como entregado») si no está liquidado y 409 `ALREADY_DELIVERED` (`payments/delivery.go:23-58`). Un pedido pagado y entregado se cierra solo (`payments/collect.go:636-664`; uno pagado sin entregar sigue abierto, `:644`). La caja puede cobrar y entregar en una sola operación (`deliver: true`, `collect.go:478-482`, `596-601`).                                                                                                          | `takeaway.deliver` no está en `WaiterPermissions`; la ruta no está en el contrato.                                                                                                                                                              | **Decisión 2.** La regla ya la cumple Go, así que el dispositivo puede ofrecer «Entregado» sin cambiarla. Propuesta: agregar `takeaway.deliver` (opcional) a `WaiterPermissions` y documentar `deliverTakeaway` en el contrato. El dispositivo ofrece «Entregado» solo con `payment_state == "paid"` y el permiso. Sin pagar muestra «Cobrar en caja antes de entregar». Sin permiso, solo el estado. Un `BALANCE_PENDING` (otra caja dejó saldo) se muestra con ese mismo texto (B). |

**Permisos.** Lista final de `WaiterPermissions` para la A2 (se repite en el contrato `pinLogin`, `docs/API_V1.md` y
`API_CONTRACTS.md`; lo comprueba `waiter_scope_test.go`): **+ `takeaway.read`, `takeaway.manage`, `payments.tickets.print`**,
y **`takeaway.deliver`** si se acepta la decisión 2 (más `accounts.manage` si se elige la alternativa de la decisión 1).
Para el rol Mesero, la guía agrega estas filas: «Consultar tablero de pedidos para llevar y ventanilla» y «Administrar
datos de pedidos para llevar» (`internal/operations/context.go:81-82`); «Imprimir tickets y precuentas»
(`payments/context.go:47`); y, de forma opcional, «Registrar entrega de pedidos para llevar» (`payments/context.go:52`).
El rol Mesero de una instalación nueva solo trae `waiter_devices.use` (`administration/setup.go:109-116`). Los demás
permisos los marca el administrador, como en la A1 (`docs/ANDROID_INSTALAR.md:98-118`).

### 0.2 Impresión de la precuenta

**Cómo se pide hoy.** La ruta es `POST payments/accounts/{id}/precheck/print` `{operation_id}`, con
`payments.tickets.print` (`payments/routes_extended.go:13-14`). Exige además `accounts.read`, y `takeaway.read` si es
para llevar (`payments/handler.go:146-157`). Es idempotente con `restaurant_operations` (`payments/handler.go:110-136`).
Cada toque arma una precuenta **nueva** con el estado vigente de la cuenta (el escritorio hace lo mismo:
`desktop/src/pages/restaurant/documents/PrecheckPanel.tsx:76-86`). Responde
`{requested, status, job_id, ticket_id, message}` (`payments/documents.go:235-287`, `320-339`) y publica
`payments.document.printed` en el ámbito de la cuenta.

- **Ya salió en papel → copia idéntica.** `documents/reprint` `{ticket_id, reason}` con `payments.tickets.reprint` y un
  motivo de 5 a 500 caracteres (regla D3 de la 9F, `documents.go:341-386`, `document_printing.go:279-285`).
- **Nunca salió → reintentar.** `documents/retry` `{ticket_id, job_id?}` sin motivo, con `payments.tickets.print`
  (`document_printing.go:232-273`). Sin trabajo fallido responde 409 `PRINT_NOT_FAILED`.
- **Sin impresora válida → rechazo antes de encolar.** 409 `PRINTER_NOT_CONFIGURED` o `PRINTER_UNAVAILABLE`, con
  `details.document/problem/printer_name` (`document_printing.go:116-124`).

**Qué impresora usa.** `receiptPrinter` (`document_printing.go:88-114`) elige la impresora de la **caja de la terminal
que pide** (`devices.register_id` → `register_receipt_printers`, 9G D10, `migrations/0023…sql:436-447`) o, si no hay, la
impresora de tickets general de Cobros (`payment_settings.receipt_printer_id`). Reimprimir y reintentar usan la misma
resolución (`document_printing.go:298-309`). Un dispositivo `android` hoy no tiene caja. **Hoy imprimiría en la general
sin avisar**, y el modelo de terminales **no admite** una impresora por dispositivo: `devices` solo tiene `register_id`.
`GET devices/{id}/configuration` lista las impresoras _conectadas a_ la terminal, no la que _usa_ para documentos
(`administration/devices.go:173-214`). Es lo que la A1 dejó para la A2.

**Cambio mínimo propuesto (fase C):**

1. **Migración.** `0026` sección C: `ALTER TABLE app.devices ADD COLUMN precheck_printer_id uuid REFERENCES
app.kitchen_printers(id)`. Es NULL por omisión. Go solo lo acepta en `kind = 'android'`.
2. **Terminales.** `PATCH devices/{id}` acepta `precheck_printer_id`. Es opcional, como `home_screen` e
   `idle_lock_seconds` (`devices.go:109-160`). Hace falta poder borrarlo, así que el campo necesita distinguir
   «no enviado» de `null`. Go valida que la impresora esté activa y que `kitchen.PrinterProblem` sea vacío
   (`kitchen/commercial.go:106-116`). Así se ofrecen las mismas impresoras que en Cobros › Políticas: las de red
   (`escpos_tcp` o la cola `cups` del servidor) y las USB de una terminal con agente (`agent` + `agent_device_id`,
   `0023…sql:398-408`), con el nombre de su terminal. `devices/{id}/configuration` la devuelve con su nombre y
   `getIdentity.terminal` también, para que el dispositivo diga a dónde se envió.
3. **Resolución.** `receiptPrinter` ahora mira primero si quien pide es un dispositivo `android`. **Decisión 3.**
   Propuesta: un `android` imprime solo en su impresora asignada, sin pasar a la general («en la impresora asignada a
   **ese** dispositivo», §0-B). Sin ella responde 409 `PRINTER_NOT_CONFIGURED` con un detalle nuevo `scope: "device"`.
   El dispositivo dice «Este dispositivo no tiene impresora asignada. Pídele al administrador que la elija en
   Terminales». Reintentar y reimprimir usan la misma resolución.
4. **Respuesta y seguimiento.** La respuesta de `precheck/print` y de `documents/retry` agrega `problem`,
   `printer_name` y `terminal_name`. Es compatible: hoy el estado sale de la cola y casi siempre es `pending`. Además,
   una lectura estrecha: `GET payments/accounts/{id}/documents/status?ticket_id=…` con `payments.tickets.print`
   (no `payments.read`, que expone el cobro completo). Devuelve `job_state`, `job_problem` (`JobProblemSQL`,
   `kitchen/print_problems.go:61-66`), `printer_name` y `terminal_name` (`JobTerminalSQL`, `:85-87`). La ruta va con
   query porque el puente admite un solo id por ruta (`payments/handler.go:60-61`). El escritorio hoy lo sigue con
   `payments_account` (`payments.read`, `CheckoutPanel.tsx:88`).
5. **Permisos y contrato.** `payments.tickets.print` en `WaiterPermissions` (el comentario de `waiter_scope.go:10-11`
   ya lo prevé para la A2). En el contrato: `printPrecheck`, `retryDocument` y `getDocumentStatus`. Capacidad
   `device-precheck-printer-v1`, que el escritorio usa para mostrar «Impresora de precuentas» en Terminales ›
   Editar de un `android` (`desktop/src/pages/DevicesPage.tsx:368`, `486`, junto a lo del PIN).

**Mensajes en el dispositivo:**

| Mensaje                                                         | Qué lo produce                                                                                                                                                                                                                                                                                                                   |
| --------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| «Enviada a Caja 1»                                              | Encolado sin problema, con `printer_name` (o `terminal_name` si la impresora es USB de una terminal).                                                                                                                                                                                                                            |
| «La impresora no responde»                                      | El trabajador intenta cada segundo como mínimo (`kitchen/worker.go:31`, `47`), con 4 s para conectar (`transport.go:246`). El problema `unreachable` llega en unos 5 s. El dispositivo consulta el estado unos 15 s.                                                                                                             |
| «La Caja 2 está desconectada»                                   | Problema `agent_offline`: el agente de esa terminal no consulta desde hace más de 30 s (`print_problems.go:33-37`, `77-81`). Go lo calcula al leer, **ya al encolar**, y el trabajo espera y sale cuando la terminal vuelve. No hay resultado síncrono del papel: los mensajes salen de la respuesta y de la consulta de estado. |
| «Reintentar»                                                    | Solo si el trabajo nunca salió (`error`, `intervention`): `documents/retry` sin motivo. Con `uncertain` se pide revisar el papel, como en la caja.                                                                                                                                                                               |
| «Para imprimir necesitas conexión con la computadora principal» | Lo decide el dispositivo: imprimir nunca entra en la cola sin conexión.                                                                                                                                                                                                                                                          |

**Nombre visible (9F).** La lista cerrada `PrecheckNames` es `precheck`/«Precuenta», `sales_ticket`/«Ticket de venta»
y `bill`/«Cuenta» (`payments/document_printing.go:45-49`), guardada en `payment_settings.precheck_name`. El escritorio
la lee de la sesión: `auth/me` → `establishment.precheck_name` (`administration/routes.go:308-316`,
`desktop/src/App.tsx:229-234`). **El dispositivo ya la puede leer**: `getIdentity` está en el contrato con
`establishment.precheck_name` obligatorio (esquema `Identity`). El encabezado impreso lo pone Go
(`printedPrecheckTitle`, `document_printing.go:73-79`). El botón dice «Imprimir precuenta», «Imprimir ticket de venta»
o «Imprimir cuenta».

**«Atendió: <mesero>».** La precuenta es la misma de la caja: `buildDocument` con la plantilla activa e
`internal/documents` (`payments/documents.go:88-130`). Su campo `cashier` (etiqueta «Atendió:»,
`internal/documents/defaults.go:85`) es el `display_name` **de quien pide la impresión** (`payments/documents.go:92-94`).
Impresa desde el dispositivo dice «Atendió: <mesero que la imprime>» **sin cambios**. Si otro mesero imprime la
cuenta de un compañero, sale su nombre. Que dijera el mesero asignado cambiaría también la precuenta de la caja, una
regla de negocio, así que no se propone.

**Bytes ESC/POS en pruebas.** Hay tres caminos:

1. **Agente simulado (recomendado para la Fase E).** Una terminal de escritorio de prueba se enrola por la escucha
   local y se le asigna una impresora `agent` en el Go temporal. La prueba hace de su agente:
   `POST print-agent/jobs/claim` con la credencial de esa terminal recibe **los bytes ESC/POS ya preparados por Go**
   (`kitchen/agent.go:19-33`). Sirve también para «La Caja 2 está desconectada»: basta con no reclamar durante 30 s.
   El escritorio ya lo prueba en `internal/operations/stage9g_print_agent_test.go`.
2. **Impresora de red falsa.** Un oyente TCP en `127.0.0.1` de la Mac como `escpos_tcp`. El puerto debe estar entre 9100
   y 9109 (`kitchen/printers.go:33-36`), así que dos agentes chocarían. Es lo que usa Go en
   `operations/cash_printing_test.go`.
3. **Contrato.** Pruebas de integración de Go (`stagea2_print_test.go`) con los dos casos anteriores y el texto
   `content` del documento.

En el repositorio no existe `ADEL_E2E_SIN_IMPRESORA`: las pruebas de la A1 usan «cocina en pantalla sin impresora»
(`FieldOnGoTest.kt:1206`).

### 0.3 Evento `restaurant.account.preparation_changed` (A1-D11)

**Carga exacta.** `events.Append(ctx, tx, "restaurant.account.preparation_changed", scope, &accountID, nil)`
(`internal/kitchen/preparation.go:28`, `101-111`). Se publica uno por cuenta y por transacción, en la misma transacción
que el cambio de cocina. Lo publican: el avance a «En preparación» o «Listo» (`kitchen/states.go:111-131`),
liberaciones, programación, reintentos de avisos y las operaciones de la cuenta. Llega como `events.Change`
`{sequence, id, version, kind, scope, station_id, aggregate_id, created_at}` (`internal/events/store.go:16-25`).
**No trae contenido**: el cliente vuelve a leer la cuenta.

| ¿Trae lo necesario para el aviso?                |                                                                                                                                                                                                                                                                  |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cuenta                                           | Sí (`aggregate_id`).                                                                                                                                                                                                                                             |
| Mesa o folio, nombre del cliente                 | No (hay que leer la cuenta).                                                                                                                                                                                                                                     |
| Mesero asignado                                  | No.                                                                                                                                                                                                                                                              |
| Platillos y cantidades que **pasaron** a «Listo» | No: ni siquiera dice qué cambió. «Listo» es por tarea (unidad × estación) en `kitchen_tasks.state/ready_at` (`migrations/0005…sql:127-142`). Una unidad está lista cuando todas sus tareas lo están; la cuenta, cuando todas sus unidades (`states.go:165-210`). |

**Filtrado del canal de hoy.** `GET /api/v1/events` (`internal/administration/events.go:56-144`):

- Exige **sesión** de usuario: `events/snapshot` pasa por `authenticateSession` (`administration/service.go:358`) y
  `events.read`.
- Filtra **por permisos y ámbito** (`events/store.go:105-129`) y por estaciones visibles en la terminal (`:90-99`).
  **No filtra por dispositivo ni por mesero.**
- Se revalida en cada lectura. Consulta cada **750 ms** (`events.go:116`) y cierra a los **30 s** (`:118`); el cliente
  se reconecta. Admite **dos canales por dispositivo** (`:85-88`, 429 `EVENT_CONNECTION_LIMIT`).
- **Reanudación**: `Last-Event-ID` → `after` (`:64-66`). Si falla la revalidación manda `event: interrupted` con
  `id: 0` (`:131-136`).
- La ventana de reenvío son **los últimos 5000 eventos**: más atrás, `reset: true` (`store.go:69-72`). En un día con
  mucho movimiento se pasa de 5000. `realtime_events` no se poda en ningún lado (no hay `DELETE` en Go). Tras un `reset`
  la A1 vuelve a leer `restaurant/state` (`android/CLAUDE.md`, «Tiempo real»).

**Conclusión.** El evento sirve para refrescar pantallas, **no basta para el aviso**:

1. No dice qué pasó a «Listo» ni para quién.
2. Con un `reset` no se puede saber qué «listos» se perdieron.
3. Exige sesión, y §0-B pide el aviso **en «¿Quién eres?»** y para meseros sin sesión en ningún dispositivo.
4. Su filtrado por permisos daría a cualquier mesero los eventos de todas las cuentas (sin contenido, pero no es el
   destinatario).

«Ningún aviso se pierde ni se repite» necesita un registro propio, consultable desde un cursor.

**Propuesta (fase D, compatible, el evento actual no cambia):**

1. **Registro de «listos».** `0026` sección D: `app.ready_notices` con `sequence bigint GENERATED ALWAYS AS IDENTITY`
   como clave y cursor, `unit_id`, `version_id`, `account_id`, `recipient_user_id` (el `assigned_user_id` de la cuenta
   **en el momento del «Listo»**), `ready_at` y la restricción **`UNIQUE (unit_id, version_id)`**. Se escribe en
   `kitchen` en la misma transacción que `preparation_changed`, cuando la última tarea viva de una unidad pasa a
   `ready`. «Listo» no retrocede (`states.go:109-111`), y si una modificación crea otra versión, es otra preparación y
   otro aviso. Va en el mismo lugar de `notePreparationUnits` (`preparation.go:40-65`). El nombre y la cantidad se
   arman al leer (agrupado por cuenta y producto). Nunca se guardan ni se envían precios.
2. **Ruta con la credencial del dispositivo**, sin sesión, como el agente de impresión (`service.go:353-357`), que
   «imprime aunque nadie haya iniciado sesión»:
   - `GET ready-alerts?after=<sequence>` es la lectura de recuperación.
   - `GET ready-alerts/stream` es SSE con `id: <sequence>` y `Last-Event-ID`, el mismo patrón de 750 ms y 30 s. Tiene
     **su propio contador**, un canal por dispositivo, para no competir con los dos de eventos.
   - Solo dispositivos `android` no revocados, con `ready_alerts_enabled`.
   - Responde `{cursor, active, day_ends_at, settings: {enabled, sound, vibration}, alerts: [{id, account_id, mode,
table_name, folio, customer_name, waiter_user_id, waiter_short_name, items: [{product_name, quantity}],
ready_at}]}`. Son solo los datos del aviso, sin precios ni importes.
   - `active` y `day_ends_at` le dicen al servicio cuándo detenerse (§0.5).
   - Capacidad `ready-alerts-v1`.
3. **Filtrado por destinatario en Go.** Así no se exponen datos a dispositivos que no deben verlos. Un aviso llega a un
   dispositivo D si el destinatario M **entró en D durante el día comercial en curso** (sesión con
   `opened_with_pin = true` en D con `created_at ≥` inicio del día, `businessday.Config.DayOf`,
   `internal/businessday/day.go:82-90`) y además:
   - M tiene una sesión con PIN vigente en D (`revoked_at IS NULL AND expires_at > now()`), o
   - M no tiene ninguna vigente en ningún dispositivo `android`.

   **Decisión 4** (detalle de §4.2). Desde la 9H puede haber varias sesiones vigentes de M en dispositivos distintos
   (`sessions_one_active_user_device_idx`, `migrations/0024…sql:16-17`). Propuesta: la de `last_seen_at` más reciente;
   alternativa más simple: todas las vigentes. «Día comercial» no se cierra con ninguna acción en el sistema: termina en
   la hora de corte (`administration/business_day.go:12-18`, `inventory_settings.operating_day_cutoff`). Por eso el
   servicio se detiene en `day_ends_at`.

4. **Sin pérdidas ni repeticiones.**
   - La ventana de `ready_notices` es el día comercial (sin `reset` dentro del día; se poda pasados 2 días).
   - El dispositivo guarda en Room el cursor y los `id` ya avisados del día, y **descarta un `id` repetido** aunque
     vuelva a llegar (cursor atrasado, dos procesos, reintento).
   - Al reconectar pide `after=<cursor>`.
   - Así se cumplen «cada partida lista una sola vez por dispositivo» y «ningún aviso se pierde por un corte de red».
     Sin este cambio no se cumplen.
   - La prueba de humo comprobó el mecanismo SSE + `Last-Event-ID` con cortes reales (§0.5.6).
5. **Configuración por dispositivo.** `0026` sección D: `devices.ready_alerts_enabled`, `ready_alerts_sound` y
   `ready_alerts_vibration` (`boolean NOT NULL DEFAULT true`). Se editan en `PATCH devices/{id}`, como en el punto 2
   de §0.2, y se leen en `devices/{id}/configuration`, `getIdentity.terminal` y la respuesta de `ready-alerts`.

**Qué credencial usa el servicio sin sesión.**

| Credencial                               | Ventajas                                                                                                                              | Riesgos y problemas                                                                                                                                                                                                        | Veredicto                                            |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| Sesión del mesero                        | Ya existe el canal                                                                                                                    | Solo vive en memoria (se pierde al cerrar la app, `android/CLAUDE.md`, «Entrada al dispositivo»); en «¿Quién eres?» no hay; vence a las 12 h (`administration/auth.go:139`)                                                | No sirve                                             |
| Credencial de envío del mesero (A1-D1)   | Por mesero y dispositivo, sobrevive al cierre                                                                                         | Hoy **solo** vale para `offline-orders` y se rechaza en cualquier otra ruta (`service.go:296-302`, revisión R3 B1). Ampliarla cambia una decisión de seguridad de la A1. Un dispositivo con 5 meseros tendría 5 conexiones | No se recomienda                                     |
| Credencial del dispositivo (recomendada) | Una sola, cifrada con el Keystore. Ya la usa el agente de impresión sin sesión. Se corta al retirar el dispositivo (`DEVICE_REVOKED`) | Quien la robe vería mesa o folio, nombre del cliente, platillo y cantidad de los meseros que entraron ese día en _ese_ dispositivo. Sin precios ni acceso a cuentas. Es lo que §0-B permite mostrar en «¿Quién eres?»      | **Sí**, con el filtrado en Go y solo datos del aviso |

### 0.4 Mesero asignado

- **Cuentas de mesa y para llevar.** El responsable es `restaurant_accounts.assigned_user_id` (`NOT NULL`,
  `migrations/0004…sql:55`). Sin indicarlo, al abrir es **quien abre**: `accounts_write.go:66-72`, en `openAccount` y
  en el pedido sin conexión (que también pasa por `validateAccountInput`, `offline_orders.go:827-833`). Junto está
  `opened_by` (`accounts_write.go:133-136`). Abrir a nombre de otro exige `accounts.transfer` (`:73-84`). Un pedido para
  llevar es igual: responsable = quien lo captura (§4.2 «el mesero que capturó o tiene asignado el pedido» coincide).
- **Cambiar de mesero** existe aparte de cambiar de mesa:
  - `POST restaurant/accounts/{id}/assign` `{assigned_user_id}` con `accounts.transfer` («Transferir la responsabilidad
    de cuentas», `handler.go:246`, `accounts_write.go:281-337`), o `PATCH` con otro `assigned_user_id`.
  - Publica `restaurant.account.assign` en el ámbito de la cuenta (`queries.go:66-95`).
  - «Mover» (`accounts.move`) solo cambia la mesa.
  - Al dividir o transferir unidades a una cuenta nueva, esta hereda el responsable de la de origen
    (`transfers.go:83`). Al combinar, las unidades pasan a la cuenta destino con su responsable (`merges.go:61-71`).
  - Como `ready_notices` toma el responsable de la cuenta actual de la unidad en el momento del «Listo» (la misma regla
    de `notePreparationUnits`), **transferir cambia el destinatario** de lo que aún no está listo. Lo ya avisado no se
    repite.
- **«Sesión abierta en un dispositivo».** Desde la 9H hay **una sesión vigente por usuario y por dispositivo**
  (`issueSession`, `auth.go:110-140`; índice de `0024`). Tiene `created_at`, `last_seen_at` (se actualiza en cada
  petición, `service.go:462`), `expires_at` (12 h), `revoked_at` y `opened_with_pin`. Go sabe en qué dispositivos tiene
  M una sesión vigente. Terminales ya muestra el «usuario actual» por dispositivo (`devices.go:24-32`).
- **Ojo:** la app de la A1 no cierra la sesión al cambiar de mesero ni al cerrarse, porque el token vive en memoria.
  Para Go, la sesión «abierta» sigue vigente hasta 12 h aunque nadie la use. Por eso la decisión 4 propone la sesión
  de `last_seen_at` más reciente. Una entrada **sin conexión** no crea sesión en Go: ese mesero no cuenta como «entró en
  el dispositivo» hasta su primera entrada con conexión. Es una limitación aceptable que la guía debe decir.

### 0.5 Servicio en primer plano con `targetSdk` 36

#### 0.5.1 Tipo de servicio (documentación oficial vigente)

| Tipo              | Permiso                                                                                                     | Límite y restricciones                                                                                                                                                                                                                                                                                                                                                                                                                                                             | ¿Sirve?                                                                                                              |
| ----------------- | ----------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `dataSync`        | `FOREGROUND_SERVICE_DATA_SYNC`                                                                              | Android 15+: **6 h por cada 24** (compartidas por todos los `dataSync` de la app). Después llega `onTimeout()` y no se puede volver a iniciar («Time limit already exhausted») hasta que la app pase a primer plano. **Prohibido iniciarlo desde `BOOT_COMPLETED`.** ([fgs/timeout](https://developer.android.com/develop/background-work/services/fgs/timeout), [15/behavior-changes-15](https://developer.android.com/about/versions/15/behavior-changes-15#fgs-boot-completed)) | No: un turno pasa de 6 h, y **comprobado** (timeout y arranque tras reiniciar rechazados)                            |
| `specialUse`      | `FOREGROUND_SERVICE_SPECIAL_USE` + `<property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE">` | Sin límite de tiempo y sin requisitos en tiempo de ejecución. «Covers any valid foreground service use cases that aren't covered by the other foreground service types»; el subtipo es libre y lo revisa Google Play ([fgs/service-types](https://developer.android.com/develop/background-work/services/fgs/service-types#special-use)). No está en la lista prohibida de `BOOT_COMPLETED`.                                                                                       | **Sí (recomendado)**                                                                                                 |
| `remoteMessaging` | `FOREGROUND_SERVICE_REMOTE_MESSAGING`                                                                       | Sin límite, pero su uso es «Transfer text messages from one device to another. Assists with continuity of a user's messaging tasks when they switch devices».                                                                                                                                                                                                                                                                                                                      | Encaje dudoso: un aviso de cocina no es continuidad de mensajería                                                    |
| `connectedDevice` | `FOREGROUND_SERVICE_CONNECTED_DEVICE` + p. ej. `CHANGE_NETWORK_STATE`                                       | Sin límite: «Interactions with external devices that require a Bluetooth, NFC, IR, USB, or network connection»                                                                                                                                                                                                                                                                                                                                                                     | Técnicamente posible (la computadora principal por la red), pero su intención son periféricos; Play podría objetarlo |
| `shortService`    | ninguno                                                                                                     | Unos 3 minutos y sin `START_STICKY`                                                                                                                                                                                                                                                                                                                                                                                                                                                | No                                                                                                                   |

**Google Play (si algún día se publica).** Con `targetSdk` 34+, cada tipo se declara en Play Console › Contenido de la
app: descripción, impacto de interrumpirlo, **video** y caso de uso. `specialUse` es para «limited scenarios» y «all
foreground service types are subject to review»
([Play Console Help 13392821](https://support.google.com/googleplay/android-developer/answer/13392821)). Fuera de Play
(APK con la llave de desarrollo) no hay revisión. Si se publica: declarar `specialUse` con su subtipo y su video. Si la
revisión lo rechazara, la alternativa sería `connectedDevice`, cambiando solo el manifiesto y el tipo que se pasa a
`startForeground`.

**Recomendación: `specialUse`.** Es el único tipo sin límite de horas que describe bien el uso: recibir de la
computadora del restaurante, en la red local, el aviso de que un platillo está listo, con la pantalla apagada.
`dataSync` queda descartado por las 6 h y por `BOOT_COMPLETED`, ambos comprobados (§0.5.6).

#### 0.5.2 Restricciones de arranque y de vida

- **Desde segundo plano (Android 12+).** `ForegroundServiceStartNotAllowedException`, salvo excepciones: transición
  desde una actividad visible, toque en una notificación, `BOOT_COMPLETED`/`LOCKED_BOOT_COMPLETED`/
  `MY_PACKAGE_REPLACED` y **«The user turns off battery optimizations for your app»**, entre otras
  ([restrictions-bg-start](https://developer.android.com/develop/background-work/services/fgs/restrictions-bg-start)).
  Comprobado en Android 17 (rechazado desde un `broadcast` sin exención). En Android 8 se acepta.
- **Tras reiniciar el equipo.** `specialUse` desde `BOOT_COMPLETED` se acepta y `dataSync` se rechaza («FGS type dataSync
  not allowed to start from BOOT_COMPLETED!»), ambos comprobados en Android 17. Hace falta `RECEIVE_BOOT_COMPLETED`.
  Llega después del primer desbloqueo, salvo con `directBootAware`, que no se propone.
- **Muerte del proceso.** Con `START_STICKY` el sistema lo reinicia y `onStartCommand` llega con `intent == null`.
  `startForeground` se acepta aunque la app esté en segundo plano, comprobado en ambos (Android 17 en unos 1,9 s,
  Android 8 al instante). Los reinicios repetidos tardan más (`restartDelay` crece).
- **Deslizar de recientes.** Llega `onTaskRemoved`, el proceso y el servicio siguen y los avisos también, comprobado en
  ambos. Algunos fabricantes matan el proceso igual: es la limitación de «dispositivo físico».
- **`am force-stop`** (Ajustes › Forzar detención) no es «cerrar»: mata todo, no reinicia y no llegan ni
  `BOOT_COMPLETED` ni alarmas hasta que el usuario abra la app, comprobado. En Android 15+, al salir del estado
  «detenido» el sistema entrega `BOOT_COMPLETED` (visto en Android 17 al abrir la app). Si la app no está en primer
  plano en ese momento, también se rechaza el arranque, comprobado: no es una exención.
- **Detener desde el Administrador de tareas (Android 13+).** Se cierra la app entera sin callbacks y sin reinicio
  ([handle-user-stopping](https://developer.android.com/develop/background-work/services/fgs/handle-user-stopping)).
  Al volver se ve `ApplicationExitInfo.REASON_USER_REQUESTED` y la app debe decir que los avisos se detuvieron.
- **`am kill`** no mata un proceso con servicio en primer plano, comprobado.
- **Android 16** (todas las apps): los trabajos de JobScheduler/WorkManager que corren **con un servicio en primer
  plano activo ya respetan la cuota** del bucket
  ([16/behavior-changes-all](https://developer.android.com/about/versions/16/behavior-changes-all#job-quota-opt)).
  No afecta a la cola, que hace envíos cortos.
- **Android 17:** límites de memoria por app y **endurecimiento del audio en segundo plano**
  ([17/behavior-changes-all](https://developer.android.com/about/versions/17/behavior-changes-all)). La app no debe
  reproducir el sonido del aviso ella misma: lo hace el **canal** de notificaciones del sistema. Ninguna regla nueva de
  servicios en primer plano ni de `BOOT_COMPLETED`. `ACCESS_LOCAL_NETWORK` solo con `targetSdk` 37 (`targetSdk` sigue
  en 36, A1-D7).

#### 0.5.3 Notificaciones (Android 13+) y canales (Android 8)

- **Permiso.** `POST_NOTIFICATIONS` es de tiempo de ejecución desde la API 33, y en una instalación nueva viene
  **apagado** ([notification-permission](https://developer.android.com/develop/ui/views/notifications/notification-permission)).
  «Apps don't need to request the `POST_NOTIFICATIONS` permission in order to launch a foreground service». Sin él,
  la notificación del servicio solo se ve en el Administrador de tareas. Comprobado: sin el permiso el servicio corre
  (`isForeground=true`), `notify()` no falla y los avisos **no se ven** (`areNotificationsEnabled()=false`). Propuesta:
  pedirlo con una pantalla en español antes del diálogo, la primera vez que el servicio va a arrancar. Si se niega, la
  app sigue y la pantalla de ayuda lo dice, con un botón a Ajustes.
- **Canales.** Existen desde la API 26, que es el `minSdk`, así que no hace falta otro camino. «After you create a
  notification channel, you can't change the notification behaviors. The user has complete control at that point»
  ([channels](https://developer.android.com/develop/ui/views/notifications/channels)).
  - «AD-EL POS conectado»: `IMPORTANCE_LOW`, sin sonido ni globo.
  - «Platillos listos»: `IMPORTANCE_HIGH`, con globo, sonido y vibración. El globo emergente se vio en Android 8 en la
    prueba.
- **Sonido y vibración apagados desde Terminales.** Decisión propuesta, porque un canal no se puede cambiar después: un
  canal **por combinación**, creado solo cuando se usa (`listos_sonido_vibracion`, `listos_sonido`, `listos_vibracion`,
  `listos_silencio`, todos con `IMPORTANCE_HIGH`). El servicio publica en el que corresponde a la configuración que
  llega en `ready-alerts`. El usuario puede además ajustar el canal en Ajustes de Android, como dice §4.4.
  - Alternativa más corta: dos canales («con sonido y vibración» y «en silencio»). No permite «solo vibración» ni
    «solo sonido».
  - No sirve `setSilent()` por notificación: apaga sonido y vibración juntos.
- **Agrupar.** Propuesta: una notificación **por cuenta** (etiqueta = `account_id`) que se reemplaza con el acumulado
  («Mesa 4: 3 platillos listos»), con `setGroup` y un resumen cuando hay varias cuentas. Al tocarla, un `PendingIntent`
  a `MainActivity` con la cuenta: «¿Quién eres?» y después la cuenta. Tocar una notificación es una acción del usuario
  y permite abrir la actividad.

#### 0.5.4 Batería, Doze, buckets y convivencia con la cola

- **Doze.** «Suspends network access… Ignores wake locks… Doesn't let JobScheduler run (WorkManager tasks don't run)»
  ([doze-standby](https://developer.android.com/training/monitoring-device-state/doze-standby)). Pero una app con
  servicio en primer plano tiene **«Network: No restrictions»**
  ([power-details](https://developer.android.com/topic/performance/power/power-details#app-state)). Comprobado: con
  `force-idle` (estado `IDLE`) y la pantalla apagada, **todos los eventos llegaron** en Android 17 y en Android 8, a
  menos de 1 s, **sin** la exclusión de batería.
- **App Standby y buckets.** Con un servicio en primer plano la app no entra en espera: el bucket fue `10` (ACTIVE) en
  Android 17.
- **Exclusión de la optimización de batería.** `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` +
  `ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`. Google Play solo la admite si Doze rompe la función principal y no se
  puede usar FCM. Aquí no se puede: red local, la wifi del restaurante puede no tener internet, y sin servidores de
  Google. Además la app no está en Play. Sirve para tres cosas:
  1. Es una **exención para arrancar el servicio desde segundo plano**, por ejemplo cuando la app vuelve tras morir sin
     sesión pegajosa.
  2. Sostiene los _wake locks_ en Doze.
  3. Atenúa los gestores de batería de algunos fabricantes.

  Si el usuario la niega, la app sigue y la ayuda lo indica (§4.3).

- **Convivencia con `OrderSendWorker`.** El servicio **no envía pedidos** ni abre la cola. Solo pide
  `OrderSendWork` (trabajo único, `ExistingWorkPolicy.REPLACE`, `core/data/.../waiter/OrderSendWork.kt:104`) al
  reconectar, igual que los disparadores de hoy. La clave de operación ya impide duplicados en Go. En Doze, los trabajos
  esperan a la ventana de mantenimiento, también con el servicio. Eso no cambia la regla 18 ni la 19 de
  `android/CLAUDE.md`.
- **Un solo canal por proceso.** El canal de **eventos** (`CatalogRealtimeContainer`) sigue siendo uno, solo en primer
  plano y con sesión, y Go le permite dos por dispositivo. El servicio sostiene **otro tipo de canal**, el de **avisos**
  (`ready-alerts/stream`), con su propio límite en Go: uno por dispositivo. Propuesta: reescribir la regla 18 de
  «Catálogo y tiempo real» como «un solo canal de eventos y un solo canal de avisos por proceso». El servicio no abre el
  canal de eventos. Unificar los dos haría depender el catálogo y las mesas de la credencial del dispositivo, que no
  tiene permisos de mesero.

#### 0.5.5 Cuándo arranca y cuándo se detiene (§4.3)

- **Arranca** (siempre desde primer plano o una exención):
  - al entrar un mesero **con conexión** (de ahí sale «entró en el día»);
  - al abrir la app, si el último `ready-alerts` dijo `active: true` y aún no pasa `day_ends_at`;
  - desde `BOOT_COMPLETED` y `MY_PACKAGE_REPLACED`, con la misma condición guardada en Room.
- **Activo** mientras Go diga `active: true`, es decir, si este dispositivo tiene destinatarios posibles: un mesero con
  sesión o que entró en el día comercial.
- **Se detiene** cuando:
  - llega `day_ends_at`;
  - `active: false` (nadie entró en el día, o «Avisar platillos listos» se apagó);
  - el dispositivo se retira (`DEVICE_REVOKED` → `stopSelf` y borra el cursor en `deviceDataWipe`);
  - se elige «Emparejar de nuevo».
- **Reconecta** con espera creciente (2, 4, 8, 16 y 30 s), como en la prueba, y al volver pide `after=<cursor>`.
- **Sin destinatarios en el día, no arranca**, así que no hay notificación permanente.

#### 0.5.6 Prueba de humo real

Proyecto en `$SP/spike-a2/fgs/` con AGP 9.5.1, Kotlin 2.4.21, OkHttp 5.5.1 y `okhttp-sse`. Son tres archivos de
Kotlin: `ReadyService.kt` (`specialUse|dataSync`, notificación «AD-EL POS conectado», SSE con `Last-Event-ID` y espera
creciente, aviso en «Platillos listos», `onTimeout`, `onTaskRemoved`), `BootReceiver.kt` y `SpikeActivity.kt`. El
servidor es `$SP/spike-a2/server/sse_server.py`, en `127.0.0.1:18471` (el emulador lo ve en `10.0.2.2`). Emite un evento
cada 20 s y reenvía lo perdido desde `Last-Event-ID`. No se usaron ni el Go temporal ni la red real. Registros:
`$SP/spike-a2/spike-log-pixel8.txt`, `spike-log-api26.txt` y `server-log-*.txt`.

| Prueba                                                    | `Pixel_8` (Android 17, API 37, imagen `user`)                                                                             | `ADEL_A1_Tablet_API26` (Android 8.0)                                                                                                   |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Arranque desde la actividad                               | `specialUse` aceptado (`types=0x40000000`), `procState` 4 (servicio en primer plano), bucket 10                           | Aceptado, `procState` 4                                                                                                                |
| Sin `POST_NOTIFICATIONS`                                  | El servicio corre; avisos no visibles                                                                                     | No aplica                                                                                                                              |
| Pantalla apagada + `force-idle` (`IDLE`)                  | 08:56:30–08:58:40: los 7 eventos llegaron (≈1 s después de producirse), sin reconexiones                                  | 09:04:58–09:06:25: todos llegaron. A los 34 s, «Software caused connection abort»: reconectó en 2 s desde el 33, sin perder ni repetir |
| Quitar de recientes                                       | `onTaskRemoved` 08:59:58; mismo pid y siguieron los eventos                                                               | `onTaskRemoved` 09:06:53; mismo pid y siguieron                                                                                        |
| `kill -9` del proceso (`run-as`)                          | Reinicio pegajoso en 1,9 s (`intent=null`), `startForeground` aceptado, siguió desde el 17                                | Reinicio inmediato, siguió desde el 39                                                                                                 |
| `am kill`                                                 | No mata el proceso                                                                                                        | No probado                                                                                                                             |
| `am force-stop`                                           | No reinicia (45 s); al abrir la app, se reenviaron 19–21 una vez                                                          | No reinicia (25 s); 40 se reenvió al volver                                                                                            |
| Arranque desde segundo plano (`broadcast`)                | `ForegroundServiceStartNotAllowedException`                                                                               | Aceptado                                                                                                                               |
| `dataSync` con límite de 60 s (`device_config`)           | `onTimeout` a los 62 s («FGS (dataSync) timed out»); el servicio se destruye                                              | No aplica (Android 15+)                                                                                                                |
| Reinicio real del emulador                                | `BOOT_COMPLETED` → `startForegroundService` aceptado; **`dataSync` rechazado**; `specialUse` aceptado; siguió desde el 26 | `BOOT_COMPLETED` → aceptado (los dos tipos)                                                                                            |
| Corte de red (`adb root`, `svc wifi`/`data disable` 45 s) | No probado (imagen sin root)                                                                                              | Detectado a los 20 s (lectura sin datos); reintentos 2/4/8/16 s; reconectó y recibió 45 y 46 una vez                                   |

**Batería (`dumpsys batterystats`).** Es referencia de método para la Fase E, **no una medición real**: el perfil de
energía del emulador es ficticio.

| Equipo     | Tiempo                                     | Consumo informado                                                                                                                                           |
| ---------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Android 17 | 2 min 21 s con servicio, casi todo en Doze | Wi-Fi 1,61 KB recibidos y 1,13 KB enviados (29/29 paquetes); CPU 81 + 39 ms; _wake lock_ de publicar notificaciones 1,1 s (7 veces); 0,000035 mAh estimados |
| Android 8  | 1 min 29 s                                 | Wi-Fi 864 B recibidos y 741 B enviados; CPU 25 + 5 ms; vibrador 3,5 s (10 veces); 0,0000123 mAh                                                             |

Método para la Fase E: `dumpsys batterystats --reset`, `dumpsys battery unplug`, una hora con la pantalla apagada y
`dumpsys batterystats <paquete>` (secciones `u0aNNN` y «Estimated power use»).

No verificado: un dispositivo físico (Doze real, CPU en reposo y despertar por paquete de wifi), gestores de batería de
fabricantes, la wifi real del restaurante y el Administrador de tareas de Android 13+.

### 0.6 Cambio de identificador (`local.adel.pos.android` → `com.adelpos.meseros`)

En el repositorio, `local.adel.pos.android` aparece solo en Android (237 archivos `.kt`, todos con paquete
`local.adel.pos.android…`), en `scripts/android-e2e.sh` y en la documentación. **Go y el escritorio no dependen del
nombre del paquete**: no hay ninguna lista ni guarda. El tipo de cliente es la constante `android`
(`ClientIdentity.kt:9-16`, `X-Client-Kind`) y el dispositivo se identifica por su `device_id`.

| Pieza                                                                      | Qué implica                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Room (`adel-pos.db` v5, `catalog.db` v1)                                   | Las bases son por aplicación: la 0.2.0 empieza vacía y nada se migra. Al cambiar el espacio de nombres hay que **mover las carpetas de esquemas** `core/data/schemas/local.adel.pos.android.core.data.db.AppDatabase` y `…catalog.db.CatalogDatabase`, porque llevan el nombre completo de la clase y `MigrationTestHelper` las busca así. La A2 sube de todos modos a v6 (§0.7.2).                                |
| Keystore (`adel-pos-secrets-v1`, `KeystoreSecretSealer.kt:170`)            | Las llaves son del UID de cada app. La nueva crea las suyas y las de la 0.1.0 se borran al desinstalarla.                                                                                                                                                                                                                                                                                                          |
| Emparejamiento                                                             | La 0.2.0 es **otro dispositivo** para Go: pide código de nuevo. La 0.1.0 se revoca en Terminales («Retirar»), lo que también revoca sus credenciales de envío (`devices.go:93-105`).                                                                                                                                                                                                                               |
| Ejecutor de pruebas                                                        | `testInstrumentationRunner = "…AdelTestRunner"` (`app/build.gradle.kts:67`); paquete de pruebas `<applicationId>.test`; `testApplicationId` de las bibliotecas sale del `namespace`.                                                                                                                                                                                                                               |
| `scripts/android-e2e.sh`                                                   | `uninstall` (`:268-269`), `-e class "local.adel.pos.android.$TEST_CLASS#…"` y el componente `local.adel.pos.android.test/local.adel.pos.android.AdelTestRunner` (`:294-299`), `/sdcard/Android/data/local.adel.pos.android/files/screens` (`:459`). Conviene una variable `APP_ID` y otra `TEST_PACKAGE`. Hay que desinstalar **también** los paquetes viejos antes de las pruebas (regla de AGP 9.5.1 del brief). |
| `namespace` de cada módulo                                                 | `app`, `core:data`, `core:ui`, `feature:access`, `feature:waiter` (`*/build.gradle.kts`); `core:domain` es JVM y no tiene.                                                                                                                                                                                                                                                                                         |
| Llave de firma de desarrollo                                               | La misma, la de `~/.android/adel-pos-dev/`: Android solo exige la misma firma para _actualizar el mismo paquete_. Un `applicationId` nuevo no pide otra llave.                                                                                                                                                                                                                                                     |
| `network_security_config` (debug), `data_extraction_rules`, `backup_rules` | No dependen del paquete.                                                                                                                                                                                                                                                                                                                                                                                           |
| Recursos                                                                   | `app_name` «AD-EL POS» no cambia. `R`/`BuildConfig` cambian de paquete con el `namespace`.                                                                                                                                                                                                                                                                                                                         |
| WorkManager, canales                                                       | Nombres sin paquete (`SEND_WORK`). Los canales nuevos de la A2 nacen en la 0.2.0.                                                                                                                                                                                                                                                                                                                                  |
| `X-Client-Kind`, `ClientIdentity`, versiones                               | Sin cambio. `X-Client-Version` 0.2.0 sale de `gradle.properties`.                                                                                                                                                                                                                                                                                                                                                  |

**Convención (decisión 5).** Propuesta: `applicationId` **y** espacios de nombres a `com.adelpos.meseros`, con módulos
`com.adelpos.meseros.core.data`, `.core.ui`, `.feature.access`, `.feature.waiter` y `.feature.takeaway`. Los paquetes de
Kotlin siguen el mismo prefijo (`com.adelpos.meseros.core.domain…`). Es un solo commit mecánico en la Fase A
(`git mv` de los árboles `kotlin/local/adel/pos/android/**` y de las carpetas de esquemas, más sustituir el prefijo),
**antes** de B–D, porque toca todos los archivos. Así un nombre nunca contradice el otro. Alternativa: cambiar solo el
`applicationId`. Es menos ruido, pero deja `local.adel.pos.android` en el código para siempre.

**Pasos de la guía (0.1.0 → 0.2.0).**

1. En cada dispositivo con la 0.1.0, revisar «Pedidos pendientes» y **enviar todo lo que esté «Por enviar»** con
   conexión. El servidor 0.13.0 no debe subir `min_client_versions.android`, para que la 0.1.0 pueda vaciar su cola.
2. Desinstalar la 0.1.0 (Ajustes › Aplicaciones).
3. En la computadora, **Terminales › Retirar** ese dispositivo.
4. Instalar la 0.2.0 y emparejar con el código.
5. Dar los permisos de notificaciones y de batería que pida.

Hoy no hay dispositivos reales instalados (§2 de la especificación).

### 0.7 Plan para el coordinador

#### 0.7.1 Cambios del servidor por fase (servidor 0.13.0, esquema 26)

| Fase | Rutas y contrato                                                                                                                                                                                                                                                                                                                                 | `0026_…sql`                                                                                                        | Eventos                                                          | Permisos                                                                        | Capacidad                    |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------------------------------------------------------- | ---------------------------- |
| A    | —                                                                                                                                                                                                                                                                                                                                                | —                                                                                                                  | —                                                                | —                                                                               | —                            |
| B    | `offline-orders` con `destination.takeaway`; `POST restaurant/accounts/{id}/customer` (decisión 1); contrato y `API_V1.md`: `customer_name`, `phone` y `origin` en `Account`, `listAccounts?mode=takeaway`, `deliverTakeaway` (decisión 2)                                                                                                       | — (no hace falta)                                                                                                  | Los de siempre (`restaurant.account.updated`, ámbito `takeaway`) | `WaiterPermissions` + `takeaway.read`, `takeaway.manage` (+ `takeaway.deliver`) | `takeaway-v1`                |
| C    | `PATCH devices/{id}` + `precheck_printer_id`; `devices/{id}/configuration` y `getIdentity.terminal` con la impresora; `receiptPrinter` por dispositivo (decisión 3); `problem`, `printer_name` y `terminal_name` en la respuesta; `GET payments/accounts/{id}/documents/status`; contrato: `printPrecheck`, `retryDocument`, `getDocumentStatus` | Sección C: `devices.precheck_printer_id`                                                                           | `payments.document.printed` (ya existe)                          | + `payments.tickets.print`                                                      | `device-precheck-printer-v1` |
| D    | `GET ready-alerts` y `GET ready-alerts/stream` (credencial del dispositivo); `PATCH devices/{id}` + los tres booleanos de aviso; configuración y `getIdentity.terminal`                                                                                                                                                                          | Sección D: `app.ready_notices` (`UNIQUE (unit_id, version_id)`) + `devices.ready_alerts_enabled/_sound/_vibration` | `preparation_changed` sin cambios; registro propio de «listos»   | — (credencial del dispositivo, filtrada por destinatario)                       | `ready-alerts-v1`            |

**Versiones (decisión 6).**

- `min_client_versions.android` se queda en **0.1.0**, por el paso 1 de la guía. La de escritorio no cambia.
- La 0.2.0 exige servidor **0.13.0** con las tres capacidades: `ServerCompatibility.check`, como la A1 con
  `waiter-pin-v1`. Esto evita probar combinaciones de capacidades sueltas.
- El escritorio esconde los campos nuevos de Terminales por capacidad.
- El `.pkg` 0.13.0 es la parada de §0 de la especificación.

**Escritorio.** Terminales › Editar de un `android` agrega «Impresora de precuentas» (C) y «Avisar platillos listos»,
«Sonido» y «Vibración» (D), en `desktop/src/pages/DevicesPage.tsx`, con Vitest y Playwright
`workflow-stagea2-*.spec.ts`.

#### 0.7.2 Estructura en Android

| Qué                      | Dónde                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Captura compartida       | **`core:capture`** (biblioteca Android nueva; depende de `core:ui` y `core:domain`). Pasan ahí de `feature:waiter` `capture/*` (`CaptureScreen`, `OrderPanel`, `SentLineDialogs`, `CaptureViewModel`, ≈1470 líneas) y lo reutilizable de `pending/*`. `feature:waiter` y `feature:takeaway` solo aportan la entrada (mesa o «Nuevo pedido») y su lista. Ningún módulo `feature` depende de otro (`android/CLAUDE.md`, «Estructura»). |
| Para llevar              | `feature:takeaway` (nuevo): pendientes con folio, nombre, tiempo, preparación, pago y entrega; «Nuevo pedido» → captura; datos del cliente; «Entregado» o «Cobrar en caja antes de entregar». Navegación principal «Mesas                                                                                                                                                                                                            | Para llevar» en `app` (`navigation/AdelPosApp.kt`), visible según los permisos de Go. |
| Dominio y datos          | `core:domain/waiter`: destino `Takeaway` en la captura y la cola (`OrderFactory`/`OfflineOrderPlan`, `OrderTransitions` sin cambios de estados); `core:data/waiter`: `WaiterApi` (deliver, customer, precheck, estado del documento), `ready/` (cliente de `ready-alerts`, cursor y deduplicación)                                                                                                                                   |
| Room `AppDatabase` v6    | `captures` (hoy `table_id` `NOT NULL`, `WaiterEntities.kt:21-43`): modalidad y datos del cliente (migración 5 → 6 con caso en `AppDatabaseMigrationTest`); `ready_alerts_state` (cursor, día) y `ready_alerts_seen` (ids avisados del día). Todo registrado en `deviceDataWipe`.                                                                                                                                                     |
| Servicio en primer plano | `app` (`ReadyAlertsService` + `BootReceiver`, manifiesto con `specialUse`, `FOREGROUND_SERVICE_SPECIAL_USE`, `RECEIVE_BOOT_COMPLETED`, `POST_NOTIFICATIONS`, `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`); la lógica pura (agrupar, destinatario visible, deduplicar y espera creciente) en `core:domain/ready` con pruebas JVM. Avisos dentro de la app: un `SharedFlow` que consume la pantalla de la cuenta abierta.                   |
| Precuenta                | Botón en `core:capture` (cuenta de mesa y pedido para llevar), con el nombre visible de `getIdentity`.                                                                                                                                                                                                                                                                                                                               |

#### 0.7.3 Dependencias y paralelismo (dos agentes)

1. **A primero y sola.** Cambia el paquete de los 237 archivos y pisaría todo. Es corta: un commit mecánico más
   guion, ejecutor y documentación.
2. **Después, en paralelo:**
   - **Agente 1: B en Android** (`core:capture`, `feature:takeaway`, Room v6, cola).
   - **Agente 2: servidor y escritorio** de B, C y D, en ese orden: Go, `0026`, contrato, `API_V1.md` y
     `DevicesPage.tsx`.

   Archivos compartidos que chocarían: `waiter_scope.go` + `openapi.json` + `docs/API_V1.md` + `API_CONTRACTS.md` (los
   toca solo el agente 2); `android/core/data/.../WaiterApi.kt` y `WaiterDtos.kt` (solo el agente 1, contra el contrato
   que publica el agente 2); `httpapi/router.go` (capacidades, agente 2).

3. **Luego C y D en Android** (un agente cada uno o seguidos). D toca `app` (servicio y manifiesto) y C toca
   `core:capture` (botón). Comparten `AppContainer.kt`, `WaiterApi.kt` y la migración de Room: **una sola v6**, que
   escribe el primero que llegue con todas sus tablas. Los cambios de D se acuerdan antes con el revisor independiente
   que pide §0.
4. **E y F**, al final, con un agente.

#### 0.7.4 Estrategia de pruebas de la Fase E

- **Cocina.** El paso hace de computadora principal por la escucha local (`MainComputer`, como en `FieldOnGoTest`):
  configura estación y ruta y marca «En preparación» y «Listo» por la API de cocina.
- **Pantalla apagada y app cerrada.**
  - `UiAutomation.executeShellCommand("input keyevent KEYCODE_SLEEP")`, `dumpsys battery unplug` y `dumpsys deviceidle
force-idle` desde la prueba (con `unforce` y `battery reset` en `finally`).
  - «App cerrada»: `am kill` no sirve, porque no mata un proceso con servicio. Se cierra la tarea con el gesto o
    `cmd activity stop-app` (Administrador de tareas), y aparte el reinicio pegajoso con `run-as … kill -9`.
  - La comprobación se hace con `NotificationManager.getActiveNotifications()` desde la prueba o con
    `dumpsys notification --noredact`, buscando el texto «Mesa N: …».
- **Corte de red durante el «Listo».**
  - En Android 11+: `cmd connectivity airplane-mode enable` (ya lo usa la A1).
  - En Android 8: `adb root` + `svc wifi/data disable` desde la Mac. La imagen es `google_apis` y en la prueba se
    reconectó desde el cursor.
  - Mientras dura el corte, la principal marca «Listo». Al volver, el aviso llega **una sola vez**: se comprueba por
    conteo de notificaciones y por la tabla de vistos.
- **Transferir la cuenta.** `POST restaurant/accounts/{id}/assign` desde la principal y «Listo» de otra partida: debe
  llegar al otro mesero en el otro dispositivo, o en el mismo tras cambiar de mesero, y no al primero.
- **Retirar el dispositivo.** El servicio se detiene: `dumpsys activity services <paquete>` vacío y sin la notificación
  permanente.
- **Sonido y vibración.** Se comprueba el canal de la notificación publicada (`NotificationChannel.getSound()` y
  `shouldVibrate()`) tras cambiarlo en Terminales.
- **ESC/POS de la precuenta.** Una terminal de prueba con impresora `agent`. El paso reclama el trabajo con
  `print-agent/jobs/claim` y revisa en los bytes el título según el nombre visible, «Atendió: <mesero>», folio y
  partidas. «Desconectada»: no reclamar durante 30 s → `agent_offline` → «La Caja 2 está desconectada».
- **Batería.** El método de §0.5.6, durante una hora.
- **Matriz.** Los cuatro AVD de siempre (`connectedDebugAndroidTest` + `android-e2e.sh --suite all` con suites nuevas
  `takeaway`, `precheck` y `alerts`). En Android 17, el emulador ve el anuncio real: se usa el ejecutor de pruebas y se
  apaga la wifi si hace falta.

#### 0.7.5 Decisiones para el coordinador (ninguna es parada)

1. **Nombre y teléfono después de abrir.** Ruta estrecha `…/customer` con `takeaway.manage` (propuesta) o
   `accounts.manage` + `PATCH` con revisión.
2. **«Entregado» en el dispositivo** con `takeaway.deliver` (opcional en la guía) y solo con `payment_state == "paid"`
   (propuesta), o solo mostrar el estado.
3. **Sin impresora asignada**, el dispositivo **no** usa la general de Cobros (propuesta, por §0-B). Hoy sí la usaría.
4. **Destinatario con varias sesiones vigentes:** la de `last_seen_at` más reciente (propuesta) o todas las vigentes.
5. **Convención de nombres:** `applicationId` y espacios de nombres a `com.adelpos.meseros` en un solo commit al
   empezar A (propuesta), o solo el `applicationId`.
6. **Versiones:** la 0.2.0 exige servidor 0.13.0; `min_client_versions.android` se queda en 0.1.0.
7. **Canales de aviso** por combinación de sonido y vibración (cuatro, creados a pedido), o dos canales sin «solo
   vibración» ni «solo sonido».

Ninguna contradice §0-B. Hay un matiz de §0-B («en el dispositivo donde tenga sesión abierta») que conviene decir: la
A1 no cierra la sesión de Go al cambiar de mesero ni al cerrarse la app. Por eso la decisión 4 usa la actividad más
reciente. Una entrada **sin conexión** no le dice a Go que el mesero entró: ese mesero no recibe avisos en ese
dispositivo hasta su primera entrada con conexión. Esto se documenta como limitación.

### Pruebas ejecutadas

| Qué                                                                                                                                                                                                 | Dónde                                                 | Resultado                                                                                                                                                  |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Compilación de la prueba de humo (`assembleDebug --offline`)                                                                                                                                        | `$SP/spike-a2/fgs`, JDK de Android Studio             | Aprobada (sin dependencias nuevas: las de la caché del proyecto)                                                                                           |
| Servicio `specialUse` + SSE: arranque, Doze forzado con pantalla apagada, recientes, `kill -9`, `am kill`, `force-stop`, arranque desde segundo plano, `dataSync` con límite de 60 s, reinicio real | `Pixel_8` (`emulator-5580`, Android 17)               | Lo de la tabla de §0.5.6: todos los eventos con el servicio vivo, sin pérdidas ni repeticiones al reconectar; `dataSync` vence y se rechaza tras reiniciar |
| Lo mismo (sin `dataSync`/`am kill`) + corte de red real con `adb root`                                                                                                                              | `ADEL_A1_Tablet_API26` (`emulator-5582`, Android 8.0) | Lo de la tabla de §0.5.6                                                                                                                                   |
| `dumpsys batterystats` en ambos                                                                                                                                                                     | Los dos emuladores                                    | Referencia (§0.5.6); no es una medición real                                                                                                               |
| Lectura del código, migraciones y contrato                                                                                                                                                          | `etapa-a2-0` (`1198148`)                              | Evidencia `ruta:línea` arriba                                                                                                                              |
| Go, escritorio, Android del repositorio                                                                                                                                                             | —                                                     | No se ejecutaron: no hubo cambios (Fase 0)                                                                                                                 |

No se usaron el Go temporal (`iso.sh server-up`) ni la red real. Los emuladores se apagaron y no quedó `force-idle`, ni
`device_config` (se borró `data_sync_fgs_timeout_duration`), ni el servidor de prueba.

### Decisiones del coordinador al cerrar la Fase 0

Ninguna de las siete decisiones de §0.7.5 contradice §0-B ni cambia una regla de negocio, así que se toman según la
regla de autonomía de `CLAUDE.md` §0 (sin parada). Se numeran **A2-Dn** y las fases las aplican tal cual.

| Decisión                                   | Qué se decide                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Por qué                                                                                                                                                                                                                                                                                                     |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A2-D1** Nombre y teléfono                | Ruta estrecha `POST restaurant/accounts/{id}/customer` `{operation_id, customer_name, phone}` con `takeaway.manage`, **solo para cuentas para llevar** (una cuenta de mesa responde 409/400), sin revisión, idempotente y con `restaurant.account.updated` en el ámbito `takeaway`. Al capturar sin conexión un pedido nuevo, el nombre y el teléfono viajan en `destination.takeaway`; editados sin conexión en un pedido que ya está en Go, se guardan en el dispositivo y se envían al volver la conexión (la última edición gana; nunca se pierden).                                                                                                                                            | `accounts.manage` daría al mesero renombrar cualquier cuenta y cambiar observaciones y programación, y su revisión obligatoria choca con el borrador, que la sube en cada guardado. La lista cerrada de `WaiterPermissions` (A1-D17) solo crece con funciones de la app.                                    |
| **A2-D2** «Entregado» en el dispositivo    | Se ofrece con `takeaway.deliver` (opcional en la guía) **solo** cuando Go dice `payment_state == "paid"`; sin pagar, «Cobrar en caja antes de entregar»; sin el permiso, solo el estado. Un 409 `BALANCE_PENDING` se muestra con el mismo texto.                                                                                                                                                                                                                                                                                                                                                                                                                                                    | La regla «no se entrega sin pagar» ya la impone Go (`payments/delivery.go`); el dispositivo no decide nada. §3.3 de la especificación lo pide así cuando el servidor lo permite.                                                                                                                            |
| **A2-D3** Sin impresora asignada           | Un dispositivo `android` imprime **solo** en su «Impresora de precuentas»; sin ella, 409 `PRINTER_NOT_CONFIGURED` con `details.scope: "device"` y «Este dispositivo no tiene impresora asignada» (con el aviso de pedírsela al administrador). Nunca pasa a la general de Cobros. Las cajas y el escritorio siguen igual.                                                                                                                                                                                                                                                                                                                                                                           | §0-B: «en la impresora asignada a **ese** dispositivo». Imprimir en otra en silencio confundiría al mesero.                                                                                                                                                                                                 |
| **A2-D4** Destinatario con varias sesiones | Solo cuentan las sesiones con PIN de dispositivos `android`. Con sesiones vigentes en varios, el aviso va al de `last_seen_at` más reciente; sin ninguna vigente, a todos los `android` donde entró con PIN en el día comercial en curso. Si el cierre de sesión explícito de la app no cierra la sesión en Go, la Fase D lo hace (`auth/logout`), para que «sesión abierta» signifique algo.                                                                                                                                                                                                                                                                                                       | Desde la 9H puede haber una sesión vigente por dispositivo y la A1 no cierra la de Go al cambiar de mesero: la actividad más reciente es la lectura más fiel de «el dispositivo donde tenga sesión abierta».                                                                                                |
| **A2-D5** Identificador y código           | `applicationId` **y** espacios de nombres a `com.adelpos.meseros` (`…core.data`, `…core.ui`, `…core.capture`, `…feature.access`, `…feature.waiter`, `…feature.takeaway`; los paquetes de Kotlin con el mismo prefijo, también `core:domain`), en un commit mecánico al empezar la Fase A, antes de B–D.                                                                                                                                                                                                                                                                                                                                                                                             | Una sola convención, sin que un nombre contradiga al otro; hacerlo antes evita que pise el trabajo de las fases siguientes.                                                                                                                                                                                 |
| **A2-D6** Versiones                        | Servidor **0.13.0**, esquema **26**; `min_client_versions.android` se queda en **0.1.0** (la 0.1.0 debe poder vaciar su cola antes de desinstalarse) y la del escritorio no cambia. La app 0.2.0 exige servidor 0.13.0 con las capacidades de la etapa (`takeaway-v1`, `device-precheck-printer-v1`, `ready-alerts-v1`), que cada fase agrega a `ServerCompatibility.check` al usarlas. El escritorio esconde lo nuevo de Terminales por capacidad.                                                                                                                                                                                                                                                 | El paso 1 de la guía (enviar lo pendiente con la 0.1.0) necesita que el servidor nuevo la siga aceptando.                                                                                                                                                                                                   |
| **A2-D7** Canales de aviso                 | Un canal «AD-EL POS conectado» (`IMPORTANCE_LOW`) y **un solo canal visible «Platillos listos»** (`IMPORTANCE_HIGH`), cuyo identificador depende de la combinación de sonido y vibración de Terminales (`listos_sonido_vibracion`, `listos_sonido`, `listos_vibracion`, `listos_silencio`); al cambiar la combinación se crea el nuevo y se borra el anterior, así que en Ajustes de Android siempre hay uno solo con ese nombre (si se vuelve a una combinación, Android recupera los ajustes que el usuario le haya hecho).                                                                                                                                                                       | Un canal no se puede cambiar después de creado y `setSilent()` apaga sonido y vibración juntos; con un solo canal visible se cumple «un canal propio “Platillos listos”» (§4.4).                                                                                                                            |
| **A2-D8** Servicio en primer plano         | Tipo **`specialUse`** (subtipo descriptivo en el manifiesto), con `FOREGROUND_SERVICE_SPECIAL_USE`, `RECEIVE_BOOT_COMPLETED` y `POST_NOTIFICATIONS`; se pide la exclusión de la optimización de batería con su explicación y, si se niega, la app sigue y la ayuda lo dice. Arranca desde primer plano (entrada con conexión, apertura de la app) o una exención (`BOOT_COMPLETED`, `MY_PACKAGE_REPLACED`, exclusión de batería) y se detiene con `day_ends_at`, `active: false`, el retiro del dispositivo o «Emparejar de nuevo».                                                                                                                                                                 | `dataSync` vence a las 6 h (un turno dura más) y se rechaza tras reiniciar, ambos comprobados en Android 17; los demás tipos no describen el uso. Con el servicio, Doze no corta la red (comprobado en Android 17 y 8).                                                                                     |
| **A2-D9** Canal de avisos                  | Rutas `GET ready-alerts?after=` y `GET ready-alerts/stream` (SSE con `Last-Event-ID`) con **solo la credencial del dispositivo**, como el agente de impresión, para `android` no revocados; Go filtra por destinatario (A2-D4) y entrega solo los datos del aviso (mesa o folio y nombre del cliente, mesero, platillo y cantidad; nunca precios ni importes). Registro propio `app.ready_notices` con `UNIQUE (unit_id, version_id)` escrito en la misma transacción que el «Listo»; el dispositivo guarda cursor e ids avisados del día y descarta repetidos. Un canal de avisos por dispositivo, aparte del canal de eventos («un solo canal de eventos y un solo canal de avisos por proceso»). | §0-B permite ver el aviso en «¿Quién eres?», sin sesión; la sesión del mesero solo vive en memoria y la credencial de envío está restringida a `offline-orders` por la revisión R3 de la A1. El canal de eventos no dice qué pasó a «Listo» ni para quién, y con un `reset` no permite saber qué se perdió. |

**Limitaciones que la guía debe decir**: un mesero que entró **sin conexión** no recibe avisos en ese dispositivo hasta
su primera entrada con conexión; el día comercial no se «cierra» con ninguna acción: termina a la hora de corte y ahí
se detiene el servicio; «Detener» del Administrador de tareas (Android 13+) o «Forzar detención» paran los avisos hasta
abrir la app.

### Plan de las fases siguientes

| Paso | Agentes (máximo dos a la vez)                          | Contenido                                                                                                                                                                                                                                                                                                                                                   |
| ---- | ------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Fase A** (Android) ‖ **Servidor y escritorio**       | A: identificador y espacios de nombres (A2-D5), 0.2.0 / `versionCode` 2, ejecutor de pruebas, `scripts/android-e2e.sh`, guía del paso 0.1.0 → 0.2.0. En paralelo, sin tocar `android/`: lo del servidor y el escritorio de B, C y D en ese orden (0.13.0, `0026` con secciones C y D, contrato, `docs/API_V1.md`, `API_CONTRACTS.md`, Terminales › Editar). |
| 2    | **Fase B** (Android) ‖ servidor y escritorio (sigue)   | B: `core:capture` (la captura compartida sale de `feature:waiter`), `feature:takeaway`, Room v6, cola con destino para llevar, datos del cliente, pendientes y entrega.                                                                                                                                                                                     |
| 3    | **Fase C** (Android) ‖ **Fase D** (Android) o seguidas | C: botón «Imprimir [nombre visible]» en `core:capture` con sus mensajes. D: `ReadyAlertsService`, avisos, canales, permisos y ayuda. Una sola versión de Room para la etapa (la que abre B).                                                                                                                                                                |
| 4    | **Revisión independiente** de §4                       | Servicio en primer plano, reconexión y entrega sin duplicados ni pérdidas (servidor y dispositivo). Sus correcciones vuelven a quien hizo la parte.                                                                                                                                                                                                         |
| 5    | **Fase E**                                             | Pruebas de punta a punta en los cuatro emuladores (suites nuevas `takeaway`, `precheck` y `alerts` de `scripts/android-e2e.sh`), batería en una hora como referencia.                                                                                                                                                                                       |
| 6    | **Fase F**                                             | Regresión completa (un agente), documentación, APK 0.2.0 con la llave de desarrollo, `.pkg` 0.13.0 (parada: lo instala el propietario), fusión en `main` y push.                                                                                                                                                                                            |

## Fase A · Identificador definitivo

Árbol `$SP/wt/a`, rama `etapa-a2-a` desde `etapa-a2-android` (`9c0f59c`). Solo se tocaron `android/`,
`scripts/android-e2e.sh`, `android/CLAUDE.md` y `docs/ANDROID_INSTALAR.md` (nada de `backend/`, `desktop/`,
`docs/API_V1.md`, `API_CONTRACTS.md`, `config/product.json` ni `CLAUDE.md` raíz). El servidor no cambió en esta fase.

**Resultado.** La aplicación es `com.adelpos.meseros` 0.2.0 (`versionCode` 2) con la misma llave de desarrollo que la
0.1.0. Instala, empareja, entra con PIN y envía pedidos (en línea y sin conexión) contra un Go temporal en Android 17
y Android 8; las 256 pruebas de la JVM y las 94 instrumentadas por emulador pasan como al cerrar la A1.

### Qué se hizo

| Commit               | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `4091429` (mecánico) | `applicationId` y `namespace` de `app` = `com.adelpos.meseros`; `namespace` de `core:data`, `core:ui`, `feature:access` y `feature:waiter` = `com.adelpos.meseros.<módulo>`; el prefijo `local.adel.pos.android` → `com.adelpos.meseros` en los 237 archivos `.kt` (fuentes, pruebas JVM, `androidTest`, también `core:domain`) y en los 5 `build.gradle.kts` (con `testInstrumentationRunner = "com.adelpos.meseros.AdelTestRunner"`). Los 14 árboles `src/<conjunto>/kotlin/local/adel/pos/android/` se movieron con `git mv` a `…/kotlin/com/adelpos/meseros/` y las dos carpetas de esquemas de Room a `core/data/schemas/com.adelpos.meseros.core.data.db.AppDatabase/` y `….catalog.db.CatalogDatabase/` (los 6 JSON sin cambios). `ktfmtFormat` solo reordenó los imports por el prefijo nuevo. **243 renombres** detectados por git (237 `.kt` + 6 JSON; similitud mínima 53 %, así que `git log --follow` conserva la historia) y 1 821 líneas cambiadas (las mismas que tenían el nombre viejo). |
| `825d1f1`            | `android/gradle.properties`: `adelpos.versionName=0.2.0`, `adelpos.versionCode=2`. `app/build.gradle.kts`: el comentario del `applicationId` ya no dice «provisional A1-D9». `scripts/android-e2e.sh`: variables `APP_ID`, `TEST_PACKAGE`, `TEST_RUNNER` y `LEGACY_APP_ID` (`scripts/android-e2e.sh:149-155`); la clase de prueba (`-e class "$APP_ID.$TEST_CLASS#…"`), el componente de `am instrument` y la carpeta de capturas (`/sdcard/Android/data/$APP_ID/files/screens`) salen de ellas; `install_app` desinstala, antes de instalar, `com.adelpos.meseros`, su `.test`, `local.adel.pos.android` y su `.test` (`:279-285`). `ServerCompatibility` **no cambió** (A2-D6: cada fase agrega su capacidad).                                                                                                                                                                                                                                                                                           |
| `387441c`            | `android/CLAUDE.md`: sección nueva «Identificador y nombres (A2-D5)» con la convención, fila de `app` en «Estructura», ejecutor y paquete de pruebas, `APP_ID` del guion, la lista de paquetes que se desinstalan antes de las instrumentadas, versión 0.2.0 y la llave de desarrollo con su huella. `docs/ANDROID_INSTALAR.md`: «Pasar de la 0.1.0 a la 0.2.0» al final de la sección 1 y dos avisos que llevan ahí («Instalar una versión nueva» y «Actualiza esta aplicación», que dicen «instálala encima»).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |

`scripts/dev.sh` no nombra el paquete en ninguna orden `android-*`: no cambió.

`AdelTestRunner` solo cambió de paquete (`android/app/src/androidTest/kotlin/com/adelpos/meseros/AdelTestRunner.kt`):
sigue buscando `_adelpostest._tcp`.

Lo que queda con el nombre viejo, a propósito: `scripts/android-e2e.sh:80,155` (para desinstalar la 0.1.0),
`android/app/build.gradle.kts:59` (el comentario que dice que la 0.1.0 es otra aplicación) y `android/CLAUDE.md` (la
explicación y la lista de paquetes a desinstalar). `grep -rn 'local\.adel\.pos\.android\|local/adel/pos/android'
android/ scripts/ --exclude-dir=build` no encuentra nada más.

### Convención (A2-D5, documentada en `android/CLAUDE.md`)

Un solo prefijo, `com.adelpos.meseros`, para el `applicationId`, el `namespace` de cada módulo
(`com.adelpos.meseros`, `….core.data`, `….core.ui`, `….feature.access`, `….feature.waiter`; los de la A2:
`….core.capture`, `….feature.takeaway`) y los paquetes de Kotlin (`core:domain`, sin `namespace`, usa
`com.adelpos.meseros.core.domain…`). Carpeta = paquete (`kotlin/com/adelpos/meseros/…`). Paquete de pruebas
`com.adelpos.meseros.test` y de las bibliotecas `<namespace>.test`. Esquemas de Room con el nombre completo de la clase.

### Llave de firma

`assembleRelease` con `ADELPOS_SIGNING_PROPERTIES=~/.android/adel-pos-dev/signing.properties` (solo la variable; la
llave y sus contraseñas no se leyeron, copiaron ni anotaron). `apksigner verify --print-certs -v` (build-tools 36.0.0):

| APK                                                           | Paquete                            | Esquema         | Certificado                                          | SHA-256 del certificado                                            |
| ------------------------------------------------------------- | ---------------------------------- | --------------- | ---------------------------------------------------- | ------------------------------------------------------------------ |
| `app-release.apk` nuevo (2 417 755 bytes)                     | `com.adelpos.meseros` 0.2.0 (2)    | v2, un firmante | CN=AD-EL POS desarrollo - APK de prueba, O=AD-EL POS | `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86` |
| `AD-EL-POS-Android-0.1.0.apk` (solo lectura, 2 417 755 bytes) | `local.adel.pos.android` 0.1.0 (1) | v2, un firmante | el mismo                                             | `fb6a4bac93052ca9051342ea122ed90469340a3685de14243bdab288d71c7c86` |

**Mismo certificado.** (Que los dos APK midan lo mismo es coincidencia: el SHA-256 de los archivos es distinto,
`0dac3394…` y `3071539e…`.) El APK firmado se borró del árbol después de comprobarlo: el de entrega lo compila la Fase F.

### Pruebas

Todo en el árbol `$SP/wt/a` con el arnés (`ISO_TAG=A`), JDK de Android Studio. Tiempos con la caché de Gradle del
usuario caliente (la compilación de los 5 módulos y sus pruebas, después del cambio mecánico: 35 s).

| Qué                                                                                                                                                                       | Dónde                                                       | Resultado                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ktfmtCheck`                                                                                                                                                              | Mac, después de cada commit de Kotlin                       | Aprobado (4 s)                                                                                                                                                                                                                                                                                                                                                                                          |
| `:core:domain:test`                                                                                                                                                       | Mac                                                         | **103 pruebas, 0 fallas** (18 XML; 2 s)                                                                                                                                                                                                                                                                                                                                                                 |
| `testDebugUnitTest` (JVM + Robolectric SDK 36)                                                                                                                            | Mac                                                         | **153 pruebas, 0 fallas**: `core:data` 133, `core:ui` 9, `feature:access` 7, `feature:waiter` 4 (`app` sin pruebas JVM); con el dominio, **256**, las mismas de la A1 (17 s)                                                                                                                                                                                                                            |
| `lintDebug`                                                                                                                                                               | Mac                                                         | Sin errores ni avisos (3 sugerencias informativas de antes: `AutoboxingStateCreation` ×2 y el `lint.jar` de lifecycle; 33 s)                                                                                                                                                                                                                                                                            |
| `assembleDebug`                                                                                                                                                           | Mac                                                         | `app-debug.apk` 15 162 806 bytes, `com.adelpos.meseros` 0.2.0 (2), `minSdk` 26, `targetSdk` 36 (`aapt2 dump badging`)                                                                                                                                                                                                                                                                                   |
| `assembleRelease` sin firmar                                                                                                                                              | Mac                                                         | `app-release-unsigned.apk` 2 405 467 bytes (35 s)                                                                                                                                                                                                                                                                                                                                                       |
| `assembleRelease` firmado (punto 4)                                                                                                                                       | Mac                                                         | `app-release.apk` 2 417 755 bytes, mismo certificado que la 0.1.0 (arriba; 23 s)                                                                                                                                                                                                                                                                                                                        |
| Manifiestos                                                                                                                                                               | `aapt2 dump xmltree`                                        | Aplicación `com.adelpos.meseros.AdelPosApplication`, actividad `….MainActivity`, proveedor `com.adelpos.meseros.androidx-startup`; APK de pruebas `com.adelpos.meseros.test` con `instrumentation` `com.adelpos.meseros.AdelTestRunner` → `targetPackage` `com.adelpos.meseros`                                                                                                                         |
| `connectedDebugAndroidTest` (antes: desinstalar los 8 paquetes nuevos y viejos, también los `.test` de las bibliotecas)                                                   | `Pixel_8` (`emulator-5580`, Android 17)                     | **94 pruebas, 0 fallas**, 44 omitidas a propósito (las de punta a punta sin argumentos): `app` 62 (42 omitidas), `core:data` 19 (2), `core:ui` 13 — 3 XML contados (217 s)                                                                                                                                                                                                                              |
| Ídem                                                                                                                                                                      | `ADEL_A1_Tablet_API26` (`emulator-5582`, Android 8.0)       | **94 pruebas, 0 fallas**, 46 omitidas a propósito: `app` 62 (44), `core:data` 19 (2), `core:ui` 13 — 3 XML (142 s)                                                                                                                                                                                                                                                                                      |
| `scripts/android-e2e.sh --suite access --steps "1 2 3 4 5"` contra un Go temporal (`server-up`, 8201/8461), con la **0.1.0 instalada antes** en el emulador (sin abrirla) | `Pixel_8`                                                   | **5 de 5 aprobados** (emparejar con el código, PIN incorrecto y bloqueo, entrada sin conexión, PIN cambiado, 426); clase `com.adelpos.meseros.AccessOnGoTest`. El guion desinstaló la 0.1.0: después, `pm list packages` muestra solo `com.adelpos.meseros` y `com.adelpos.meseros.test`; `dumpsys package` dice `versionName=0.2.0`, `versionCode=2`                                                   |
| `scripts/android-e2e.sh --suite waiter` (w1–w13c) contra un Go temporal nuevo, con la 0.1.0 instalada antes                                                               | `ADEL_A1_Tablet_API26` (red cortada con `adb root` + `svc`) | **19 de 19 aprobados** (160 s) en la segunda corrida, con el emulador recién encendido y otro Go temporal. El guion desinstaló la 0.1.0 (después solo están `com.adelpos.meseros` 0.2.0 (2) y su `.test`). Primera corrida: `w1` aprobado y `w2` **colgado** más de 10 minutos antes de abrir la actividad (ver abajo); se detuvo; `w2` solo, en el mismo servidor y con `--no-install`, pasó en 11.8 s |
| `iso.sh format` y `fmt-check` (Prettier, gofmt, datos del sistema anterior) y `ktfmtCheck`                                                                                | Mac, antes del commit de documentación                      | Aprobados; `md5` de `docs/etapas/*.md` igual antes y después                                                                                                                                                                                                                                                                                                                                            |

Antes de las instrumentadas, ninguno de los dos emuladores tenía `local.adel.pos.android` ni `com.adelpos.meseros`
(`pm list packages`); AGP los desinstaló al terminar. Para probar que el guion quita la app vieja, se instaló el APK
0.1.0 (solo lectura, sin abrirlo: `stopped=true notLaunched=true`) antes de cada suite de punta a punta. Ninguna prueba
usó la red real ni el servicio de la Mac; la lista de servidores nunca mostró uno real (el ejecutor busca
`_adelpostest._tcp`).

**El cuelgue de `w2` en la primera corrida.** La instrumentación arrancó (`INSTRUMENTATION_STATUS_CODE: 1`) y en 10
minutos no se abrió ninguna actividad: ni una línea `START u0` en el registro, el lanzador con el foco, el hilo
principal de la app esperando en `epoll` (sin trabajo) y el de la prueba (`.AdelTestRunner`) esperando en un
`futex`. No se pudo sacar la pila de Java (`jdb` no se conectó y ART dejó «Timed out waiting for threads to
suspend»; `/data/anr` pide root, y `adb root` habría cortado la instrumentación). Se detuvieron el guion y la app, se
repitió `w2` solo (aprobado) y después la suite completa con el emulador recién encendido (19 de 19). Mientras tanto
la Mac corría también las pruebas de Go del otro agente. No se repitió en ninguna otra corrida de esta fase y nada
del cambio toca ese camino (la misma prueba pasó dos veces con el mismo APK), así que se anota como falla del
entorno **sin causa confirmada**; si reaparece en la Fase E o F, sacar la pila antes de detenerlo (`adb root` y
`kill -3`, perdiendo ese paso).

### No verificado

- Dispositivo físico (no hay).
- Instalar la 0.2.0 encima de una 0.1.0 **emparejada** con datos reales: no hay dispositivos reales con la 0.1.0. En el
  emulador la 0.1.0 estaba instalada sin abrir, así que no se vio en pantalla el caso de «dos AD-EL POS» que describe
  la guía (es comportamiento estándar de Android: paquetes distintos).
- Las suites `catalog` y `field`, `access` 6–7 y `Pixel_Tablet`/`ADEL_A1_Phone_API33`: la regresión completa es de la
  Fase F.

### Para la Fase F y las fases siguientes

- `docs/ANDROID_INSTALAR.md` todavía dice «versión 0.1.0» en la introducción, `AD-EL-POS-Android-0.1.0.apk` en la
  sección 1 y «0.12.0 o posterior» en las secciones 2 y 6: cambiarlos cuando la fase que agregue la primera capacidad
  de la A2 a `ServerCompatibility.check` (B) suba la versión mínima del servidor, y el nombre del APK en la Fase F. El
  paso 5 de «Pasar de la 0.1.0 a la 0.2.0» anuncia los permisos de notificaciones y batería que explicará la Fase D.
- `android/CLAUDE.md` «Pruebas» y `scripts/android-e2e.sh`: las suites nuevas de la Fase E (`takeaway`, `precheck`,
  `alerts`) usan `APP_ID`/`TEST_RUNNER` igual que las demás.

### Para CLAUDE.md

- §0/encabezado («Última revisión», línea 5): APK de prueba `AD-EL-POS-Android-0.1.0.apk` → el de la 0.2.0 al cerrar
  la etapa; «Android 0.1.0» → 0.2.0 (`com.adelpos.meseros`, `versionCode` 2).
- Árbol del repositorio (línea 202, `android/`): mencionar que el código vive en `kotlin/com/adelpos/meseros/…` con el
  identificador `com.adelpos.meseros` (A2-D5) y el ejecutor `com.adelpos.meseros.AdelTestRunner`.
- Regla «Nunca la red real» (línea 321): `scripts/android-e2e.sh` desinstala también la 0.1.0 (`local.adel.pos.android`)
  antes de instalar.
- Decisiones de la A1 (línea 684): «A1-D9 identificador provisional `local.adel.pos.android`» → agregar «reemplazado en
  la A2 por `com.adelpos.meseros` (A2-D5; otra aplicación para Android: se desinstala la 0.1.0, se revoca en
  Terminales y se empareja de nuevo)».
- La llave de desarrollo es la misma (`~/.android/adel-pos-dev/`, certificado SHA-256 `fb6a4bac…1c7c86`).

## Servidor · Parte B: Para llevar (servidor 0.13.0, sin migración)

Rama `etapa-a2-srv` (desde `etapa-a2-android` en `9c0f59c`), árbol `$SP/wt/srv`, un agente, 18/11/2026, en paralelo
con la Fase A en `android/` (que no se tocó). Pruebas en copias aisladas (`ISO_TAG=S`): clúster temporal 55499 con
bases `adel_pos_test_s_*`; no se usó el Go temporal ni el escritorio, ni el servicio real de la Mac. Aplica §3 de la
especificación con A2-D1, A2-D2 y A2-D6. Esquema **25** (la parte B no necesita migración; `0026` empieza en la C).

Commits: `cb9b77f` (versión 0.13.0 con metadatos sincronizados) y `5f52e73` (Para llevar en el servidor, contrato,
documentación y pruebas).

### Qué cambió

- **Permisos de mesero** (`backend/internal/administration/waiter_scope.go`): `WaiterPermissions` + `takeaway.read`,
  `takeaway.manage` y `takeaway.deliver` (A2-D2), con su comentario. `waiter_scope_test.go` ahora admite esas tres (y
  exige que estén) como únicas excepciones a los prefijos prohibidos; `pin_acceptance_test.go` (A1B_10) comprueba con
  `payments/methods` lo que la sesión con PIN sigue sin poder (cobros) y que `restaurant/accounts?mode=takeaway` ya
  responde 200.
- **Pedido para llevar sin conexión** (`backend/internal/operations/offline_orders.go`): `OfflineDestination.Takeaway`
  (`destination.takeaway: {customer_name?, phone?}`), exclusivo con `account_id` y `table_id`
  (`validateOfflineInput`); `seen_account_ids` y `name` solo con `table_id` y `replaces_draft_items` solo con
  `account_id` (como antes). `diagnoseOfflineTakeaway` exige `accounts.open` + `takeaway.manage` y responde
  `items_in_account` si las partidas ya están en un pedido para llevar abierto; `tableAccountsWithItems` pasó a
  `openAccountsWithItems(tableID *string, …)` (la misma consulta para la mesa y para llevar, sin duplicarla).
  `offlineAccountInput` abre la cuenta `mode: takeaway` con `origin` «Para llevar», el cliente y el teléfono; el
  responsable sigue siendo quien envía (`validateAccountInput`) y el resto de `acceptOffline` no cambia.
- **Nombre y teléfono** (`backend/internal/operations/takeaway.go`, nuevo): `POST restaurant/accounts/{id}/customer`
  (`updateTakeawayCustomer`), con `"customer": "takeaway.manage"` en `routePermission` (`handler.go`) y su caso en
  `accounts` (`accounts_read.go`). `TakeawayOrigin`, `validCustomerFields` y los límites 100/40 viven ahí y los usa
  también `validateAccountInput` (`accounts_write.go`), para que las tres entradas validen igual.
- **«Entregado»** (`backend/internal/payments/delivery.go`): sin cambio de regla; la respuesta lleva `finance` solo con
  `payments.read` (si no, `null`).
- **Capacidad** `takeaway-v1` (`backend/internal/httpapi/router.go`, comprobada en `clientversion_test.go`).
- **Versión 0.13.0** en `config/product.json` + `sync` (`metadata.json`, `desktop/src/generated/product.json`,
  `tauri.conf.json`, `package.json`, `Cargo.toml`) y a mano `package-lock.json` y `Cargo.lock` (como la A1);
  `schema_version` 25 y `min_client_versions` sin cambio (`desktop` 0.10.0, `android` 0.1.0; A2-D6).
- **Contrato** (`backend/internal/apicontract/openapi.json`): `Account` con `customer_name`, `phone` y `origin`
  (obligatorios: Go siempre los entrega) y los valores de `payment_state`/`delivery_state`; `listAccounts` y
  `getRecoveryState`/`RecoveryState` con los pedidos para llevar; `destination.takeaway` en `OfflineOrderRequest` y la
  descripción de `submitOfflineOrder`; operaciones nuevas `updateTakeawayCustomer` y `deliverTakeaway` (etiqueta
  «Cobros», nueva); `pinLogin` con la lista de permisos nueva; los 409 de la entrega en la respuesta `Error`.
  `TestContractV1` agrega `contractTakeaway` (34 operaciones, antes 32).
- **Documentación**: `docs/API_V1.md` (sección nueva «Para llevar (etapa A2, parte B)», tabla de operaciones,
  `takeaway-v1`, permisos de mesero, `destination` del pedido sin conexión, A1-D18 punto 4 y los dos códigos de error) y
  `API_CONTRACTS.md` (sección «Etapa A2 · Para llevar en el servidor» y una nota en la de la A1).

Sin locks consultivos nuevos: la ruta del cliente es una escritura de `restaurant/*` y toma 7820148305 exclusivo como
todas (`operations.Handle`); el pedido sin conexión y la entrega, los de siempre.

### Decisiones (con su porqué)

Ninguna cambia una regla de negocio ni la especificación. Son detalles que A2-D1 y A2-D2 no fijaban:

1. **Decisión propuesta · La ruta del cliente no pide ni sube la revisión.** A2-D1 dice «sin revisión». Si la subiera,
   el siguiente `saveDraft` del mismo teléfono (o de la caja que tiene la cuenta abierta) con la revisión que tenía
   chocaría (`ACCOUNT_CONFLICT`) cada vez que se escribe el nombre; y una edición hecha sin conexión llegaría «vieja».
   Los dos datos no intervienen en nada de lo que la revisión protege (partidas, borrador, importes, mesa,
   programación). Consecuencia aceptada: si alguien edita a la vez «Datos de la cuenta» en la caja (`PATCH` con su
   revisión), gana la última escritura de esos dos campos, que es la regla de A2-D1. Se prueba que el borrador se sigue
   guardando con la revisión anterior.
2. **Decisión propuesta · `customer_name` y `phone` son obligatorios en la ruta** (vacío = quitar; sin uno, 400). Así
   un cliente que solo manda uno no borra el otro sin querer. Un campo de más (`revision`) es 400, como en todas las
   rutas de `restaurant/*` (`decode` con `DisallowUnknownFields`).
3. **Decisión propuesta · Cuenta de salón en la ruta del cliente: 400 `OPERATION_VALIDATION`** (A2-D1 dejaba «409/400»;
   es un dato inválido para esa cuenta, no un conflicto). Pedido que ya no está abierto: el 400 de `ensureOpen` con
   `details.reason: account_not_open`, el mismo que la app ya maneja. Sin cambios reales, la ruta no escribe historia,
   evento ni auditoría (sí guarda la clave, como toda escritura).
4. **Decisión propuesta · `finance` de `deliverTakeaway` solo con `payments.read`.** Con `takeaway.deliver` en la lista
   de mesero, la respuesta existente habría entregado al teléfono el estado financiero completo de la cuenta (pagos con
   método, referencia, caja, turno y quién cobró), que la sesión con PIN no puede consultar (A1-D17: sin
   `payments.read`). Ahora llega `null` a quien no tiene ese permiso; la caja sigue igual y el escritorio no usa ese
   campo (`TakeawayDelivery.tsx` solo ejecuta la operación). Cambio compatible: la ruta no estaba en el contrato.
5. **Decisión propuesta · `items_in_account` para llevar mira solo pedidos para llevar abiertos**, igual que la mesa
   mira solo sus cuentas abiertas (A1-D18): es la red de seguridad para la apertura en línea sin respuesta; el
   dispositivo resuelve antes (A1-D19, repite `openAccount` con su clave). La misma clave que un `confirmDraft` con
   `destination.takeaway` sigue siendo `IDEMPOTENCY_CONFLICT`, como con una mesa: un `confirmDraft` exige que el
   dispositivo ya conozca la cuenta, así que ese pedido de la cola debe ir con `account_id`.
6. **Decisión propuesta · `Account.customer_name`, `phone` y `origin` quedan como obligatorios en el contrato**: Go los
   entrega siempre (columnas `NOT NULL`, vacías en salón), y la app puede confiar en ellos.

### Forma final para el agente de Android

Todo con la credencial del dispositivo, `X-Client-Version` y `X-Client-Kind: android`; con la sesión con PIN salvo
`submitOfflineOrder`, que también acepta la credencial de envío. Exigir **`takeaway-v1`** en `ServerCompatibility`
(servidor 0.13.0). Mostrar Para llevar según `user.permissions` de `getRecoveryState`/`getIdentity` (`takeaway.read`
para verlo; `takeaway.manage` para capturar y editar el cliente; `takeaway.deliver` para «Entregado»).

**1. «Nuevo pedido» con conexión** (sin cambios en Go): `POST /api/v1/restaurant/accounts`

```json
{
  "operation_id": "…",
  "mode": "takeaway",
  "origin": "Para llevar",
  "customer_name": "",
  "phone": "",
  "items": [
    {
      "id": "…",
      "product_id": "…",
      "variant_id": null,
      "quantity": 1,
      "customization": {}
    }
  ]
}
```

(los demás campos de `AccountInput` son opcionales; sin `table_id` ni `seen_account_ids`; un pedido para llevar con
`table_id`: 400 «Los pedidos para llevar no utilizan mesas.») → 200 `Bill` (`account.mode: "takeaway"`, `folio`,
`name: "Pedido para llevar"`). Sin `takeaway.manage`: 403 `PERMISSION_DENIED`. Después `saveDraft`, `previewDraft` y
`confirmDraft` como en una mesa.

**2. Sin conexión**: `POST /api/v1/restaurant/offline-orders` con `"destination": {"takeaway": {"customer_name":
"Juan", "phone": "5551234567"}}` (los dos opcionales, se recortan; `{}` vale). Reglas:

- 400 `OPERATION_VALIDATION` (sin `details`): dos destinos o ninguno («Indica el destino del pedido: una cuenta abierta,
  una mesa o un pedido para llevar nuevo.»); `seen_account_ids` o `name` con `takeaway` («Las cuentas vistas y el nombre
  solo aplican a una cuenta nueva en una mesa.»); `replaces_draft_items` con `takeaway`; cliente de más de 100 o
  teléfono de más de 40 («Revisa el nombre del cliente (100) y el teléfono (40 caracteres).»); un campo desconocido.
- 403 `PERMISSION_DENIED` («No tienes permiso para esta operación del restaurante.») sin `accounts.open` o sin
  `takeaway.manage` (antes de revisar el pedido; con la credencial de envío, los permisos de mesero vigentes).
- 200 `OfflineOrderResult`: `account_id` es el pedido nuevo; `bill.account` con `mode: "takeaway"`,
  `table_id: null`, `origin: "Para llevar"`, `customer_name`, `phone`, `folio`, `assigned_user_id` = el mesero de la
  sesión o de la credencial. Los cambios de precio y de catálogo, igual que en una mesa (`ORDER_REQUIRES_CONFIRMATION`
  y reenvío con `confirmation` y **la misma `operation_id`**).
- Misma clave y mismo contenido (con sesión o con credencial): la misma respuesta 200. Misma clave con otro contenido,
  del mismo mesero y dispositivo: 409 `OFFLINE_ORDER_ALREADY_ACCEPTED` con `details.origin: "offline_order"` y
  `details.result` (el pedido que entró; marcar «Recibido» con ese resultado). De otro mesero o dispositivo: 409
  `IDEMPOTENCY_CONFLICT`.
- **Apertura en línea sin respuesta**: 409 `ORDER_REQUIRES_CONFIRMATION`, `details.requires_changes: true`, un cambio
  `{"kind": "items_in_account", "item_id": null, "product_id": null, "product_name": "", "before": null, "now": "folio
12", "message": "Lo que capturaste ya está en el pedido para llevar de Juan (folio 12). Envía el pedido a esa cuenta.",
"acceptable": false, "account_id": "<pedido>"}` (sin cliente: «…en el pedido para llevar (folio 12)…»). No se acepta
  con `confirmation`: reenviar **con la misma `operation_id`** a `"destination": {"account_id": "<pedido>"}` (ahí
  aplican `replaces_draft_items`, el último guardado propio y `draft_item_sent` de A1-D18).

**3. Nombre y teléfono** (A2-D1): `POST /api/v1/restaurant/accounts/{id}/customer`

```json
{ "operation_id": "…", "customer_name": "Juan Pérez", "phone": "" }
```

→ 200 `Bill` con los datos nuevos y **la misma `revision`** (no la pide ni la sube). Los dos campos siempre (vacío
quita). Errores: 400 `OPERATION_VALIDATION` sin uno de los dos («Indica el nombre del cliente y el teléfono (vacíos para
quitarlos).»), con más de 100/40 («Revisa el nombre del cliente (100) y el teléfono (40 caracteres).»), con un campo de
más, o en una cuenta de salón («El nombre del cliente y el teléfono se cambian aquí solo en los pedidos para llevar.»);
400 `OPERATION_VALIDATION` con `details: {"reason": "account_not_open", "account_id", "account_state", "merged_into_id"}`
si ya no está abierto; 403 `PERMISSION_DENIED` sin `takeaway.manage`; 409 `IDEMPOTENCY_CONFLICT` con la misma clave y
otro contenido. Editado sin conexión: guardarlo y mandarlo al volver con su propia `operation_id` (la última edición
gana). El evento es `restaurant.account.updated` en el ámbito `takeaway`.

**4. Pendientes**: `GET /api/v1/restaurant/accounts?mode=takeaway&state=open` (`page_size` hasta 100; 403 sin
`takeaway.read`) o `accounts` de `GET /api/v1/restaurant/state` (que solo incluye los para llevar con `takeaway.read`).
Por cuenta: `folio`, `customer_name`, `phone`, `origin`, `elapsed_seconds`, `urgency`, `created_at`,
`preparation_state` (pendiente = `pending_release`/`scheduled`/`released`; en preparación = `preparing`; listo =
`ready`), `payment_state` (`not_recorded` sin pagar, `partial`, `paid` pagado) y `delivery_state` (`not_delivered`,
`delivered`). Ordenados del más antiguo al más reciente. Tiempo real: `streamEvents` en el ámbito `takeaway` (lo da
`takeaway.read`) con la cuenta como agregado (`restaurant.account.opened`, `restaurant.order.confirmed`,
`restaurant.draft.saved`, `restaurant.account.updated`, `restaurant.account.preparation_changed`, `takeaway.delivered`,
`restaurant.account.closed`…): volver a leer la cuenta. Un pedido pagado y entregado se cierra y deja de estar en la
lista.

**5. «Entregado»** (A2-D2): `POST /api/v1/payments/accounts/{id}/deliver` `{"operation_id": "…"}` (permisos
`takeaway.deliver`, `accounts.read`, `takeaway.read`):

- 200 `{"delivered": true, "closed": true, "finance": null}` (al mesero `finance` siempre es `null`). `closed` es
  `true` cuando el pedido queda pagado, entregado y sin nada por enviar en su borrador (se cierra y sale de la lista);
  con algo en el borrador es `false` y el pedido sigue abierto, ya entregado. El reintento con la misma clave devuelve
  lo mismo.
- 409 `BALANCE_PENDING` («El pedido tiene saldo pendiente. Cóbralo antes de marcarlo como entregado.») → mostrar
  «Cobrar en caja antes de entregar»; 409 `ALREADY_DELIVERED` («El pedido ya fue entregado.») → volver a leer; 403
  `PERMISSION_DENIED` sin `takeaway.deliver`; 400 en una cuenta de salón; 404 si no existe.
- Ofrecer el botón solo con el permiso y `payment_state == "paid"`.

### Para la guía (`docs/ANDROID_INSTALAR.md`, «Permisos del rol Mesero»)

Filas nuevas para Para llevar (los nombres son los de **Administración › Personal › Roles › Editar permisos**):

| Para que el mesero pueda…                                          | Marca                                                   |
| ------------------------------------------------------------------ | ------------------------------------------------------- |
| Ver los pedidos para llevar y sus estados (y la opción en el menú) | «Consultar tablero de pedidos para llevar y ventanilla» |
| Tomar pedidos para llevar y anotar el nombre y el teléfono         | «Administrar datos de pedidos para llevar»              |
| Marcar «Entregado» un pedido ya pagado (opcional)                  | «Registrar entrega de pedidos para llevar»              |

Las dos primeras van juntas (sin la segunda solo ve la lista). Para capturar hacen falta además los de la A1 («Abrir
cuentas», «Capturar y editar borradores de pedidos», «Confirmar pedidos», catálogo y precios). Sin la tercera, el
mesero ve si el pedido está pagado y entregado, pero la entrega se marca en la caja. El mesero **nunca cobra** y no se
entrega un pedido sin pagar: si no está pagado, el teléfono dice «Cobrar en caja antes de entregar». Se necesita la
computadora principal en la versión **0.13.0** (el `.pkg` de esta etapa).

### Pruebas ejecutadas

| Qué                                                                                                                                            | Dónde                                                  | Resultado                                                |
| ---------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------- |
| `TestStageA2Takeaway` (5 subpruebas, `-v`: pedido sin conexión, `items_in_account`, permisos y forma, nombre y teléfono, pendientes y entrega) | `./internal/operations`, base `_s_`                    | **PASS** las 5 (18 s); antes, una corrida sin `-v`: ok   |
| `go-int ./internal/apicontract` (`TestContractV1` con `contractTakeaway`, más las pruebas del documento)                                       | base `_s_`                                             | **ok** (36 s); 34 operaciones comprobadas (antes 32)     |
| `go-int ./internal/administration` completo (incluye `TestWaiterPermissionsAreClosedAndDocumented` y A1B_10)                                   | base `_s_`                                             | **ok** (215 s)                                           |
| `go-int ./internal/operations` completo                                                                                                        | base `_s_`                                             | **ok** (381 s), con `TestStageA2Takeaway` y las de la A1 |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                                                                 | `iso.sh go-unit`                                       | **aprobado** (35 paquetes ok)                            |
| `format`, `fmt-check`, `sync-check`                                                                                                            | árbol `srv`                                            | Correctos                                                |
| Pruebas del documento del contrato, `TestWaiter*` y `TestVersion*` después de los últimos retoques de redacción                                | `go test` (`apicontract`, `administration`, `httpapi`) | ok                                                       |
| md5 de `docs/etapas/*.md` antes y después de formatear                                                                                         | árbol `srv`                                            | Idénticos                                                |

### Para `CLAUDE.md` (no lo edité)

- §3, «Composición del backend»: etapa A2 → `restaurant/accounts/{id}/customer` en `operations/takeaway.go`;
  `destination.takeaway` en `operations/offline_orders.go`; `WaiterPermissions` con `takeaway.*`; `finance` de
  `payments/.../deliver` solo con `payments.read`.
- §6: prueba nueva `TestStageA2Takeaway` (`operations`) y `contractTakeaway` en `TestContractV1`.
- §15: A2-D1 y A2-D2 implementadas (con las decisiones propuestas 1 a 6 de arriba, si se aceptan).

### No verificado

- La aplicación Android contra estas rutas (agente de Android, Fase B en el dispositivo y Fase E).
- Vitest, `build`, `cargo` y Playwright: el escritorio no cambió (solo su número de versión, que `sync-check`
  comprueba). La regresión completa es de la Fase F.
- `go-int` de `payments` (sin pruebas de integración propias: la entrega se prueba en `operations`), `database`
  (sin migración) y los demás paquetes no tocados.

### Decisiones del coordinador sobre la parte B

Se aceptan las seis decisiones propuestas de la parte. La 4 cambia la respuesta de una ruta que ya existía y se
registra como **A2-D11**: `POST payments/accounts/{id}/deliver` devuelve `finance` solo a quien tiene `payments.read`
(si no, `null`). Con `takeaway.deliver` en la lista de mesero, el teléfono habría recibido el estado financiero completo
de la cuenta (pagos con método, referencia, caja, turno y quién cobró), que la sesión con PIN no puede consultar
(A1-D17); la caja lo sigue recibiendo y el escritorio no usa ese campo. La parte B se verificó sobre una copia exacta
de `5f52e73` (`git archive`): `fmt-check`, `sync-check`, `go-unit`, `go-int ./internal/apicontract`,
`TestStageA2Takeaway` (`operations`) y `TestStageA1WaiterPIN` (`administration`), todo aprobado. Fusión: la parte B
(`5f52e73`) y la Fase A (`387441c`) en `f90fd83`, sin archivos en común.

## Servidor · Parte C: impresora de precuentas por dispositivo (esquema 26)

Rama `etapa-a2-srv`, mismo árbol `$SP/wt/srv`, encima de `5f52e73` (parte B), un agente, 18/11/2026, en paralelo con
Android en su árbol (no se tocó `android/`, `scripts/android-e2e.sh` ni `docs/ANDROID_INSTALAR.md`). Pruebas aisladas
(`ISO_TAG=S`): clúster temporal 55499 con bases `adel_pos_test_s_*` y Playwright con el Go temporal del arnés
(`ISO_GO_PORT=8211`, `ISO_PREVIEW_PORT=1431`, detenido al terminar). Aplica §5 de la especificación con A2-D3.

Commits: `cc9bf57` (0026 sección C, esquema 26 y `TestStageA2Upgrade`), `8d2ebfd` (servidor, contrato, documentación y
pruebas) y `6c2fcdd` (escritorio, Vitest, Playwright y la firma de datos de prueba).

### Qué cambió

- **Migración `backend/migrations/0026_android_a2.sql`** (la única de la etapa; sección «Parte C»; la D se agregará
  ahí): `devices.precheck_printer_id` (uuid, NULL por omisión, FK `devices_precheck_printer_id_fkey` a
  `kitchen_printers`, índice parcial `devices_precheck_printer_idx` y `CHECK devices_precheck_printer_android_check`:
  solo un `android` la tiene). No cambia ninguna fila. Esquema **26** (`config/product.json` + `sync`).
  `TestStageA2Upgrade` (`internal/database/stagea2_upgrade_test.go`) sube 25 → 26 sobre datos existentes; las pruebas de
  subida anteriores siguen migrando hasta la última.
- **Terminales** (`internal/administration/devices.go` y `precheck_printer.go`, nuevo):
  - `PATCH devices/{id}` acepta `precheck_printer_id` con `optionalID` (distingue «no enviado» de `null`): sin el campo
    se conserva, `null` la quita, un id la asigna si el dispositivo es `android` y la impresora existe y puede imprimir
    (`kitchen.PrinterProblem`). Audita `device.updated` con antes y después.
  - `GET devices`, la respuesta del `PATCH` y la auditoría llevan `precheck_printer_id`, `precheck_printer_name` y
    `precheck_printer_terminal_name`.
  - `GET devices/{id}/configuration`: `precheck_printer` (la asignada con `problem`) y `precheck_printer_options` (todas
    las impresoras con su `problem`; vacía si no es `android`). Así quien edita terminales elige sin
    `kitchen.printers.manage` (`kitchen.DocumentPrinterOptions`/`DocumentPrinterChoice`, `internal/kitchen/commercial.go`).
  - `auth/me` y `restaurant/state` comparten `terminalIdentityQuery` con `terminal.precheck_printer_id` y
    `terminal.precheck_printer` (`{id, name, terminal_name}` o `null`).
- **Resolución y respuestas** (`internal/payments/document_printing.go`, `documents.go`, `finance.go`,
  `routes_extended.go`, `context.go`):
  - `receiptPrinter`: un `android` usa **solo** `devices.precheck_printer_id` (A2-D3); sin ella, `missing`. Las demás
    terminales, igual (caja → general). Lo usan precuenta, reintento, reimpresión, tickets y documentos de caja.
  - `printerUnavailableError` agrega `details.scope: "device"` en un `android`; `missingPrinterMessage` dice «Este
    dispositivo no tiene impresora asignada. Pide al administrador que la elija en Terminales.».
  - `addJobFeedback`: `precheck/print`, `documents/retry`, `documents/reprint` (y el `printing` de un ticket de pago
    encolado) agregan `problem` (`JobProblemSQL`, ya `agent_offline` al encolar), `printer_name` y `terminal_name`
    (`JobTerminalSQL`).
  - `GET payments/accounts/{id}/documents/status?ticket_id=…` (`documentStatus`, permiso `payments.tickets.print`, lock
    compartido): el `DocumentRecord` del documento, sin importes. `readDocuments` y `readDocument` comparten
    `documentRecordQuery`.
- **Permisos de mesero**: `WaiterPermissions` + `payments.tickets.print` (y `waiter_scope_test.go` lo admite como
  excepción de la etapa A2).
- **Capacidad** `device-precheck-printer-v1` (`internal/httpapi/router.go`, comprobada en `clientversion_test.go`).
- **«Quitar datos de prueba»** (`internal/maintenance/plan.go` y `apply.go`): la referencia `devices.precheck_printer_id`
  se maneja como configuración que se desvincula («Impresora de precuentas de «…»»), no como registro que impide borrar.
  Firma nueva `Precuentas A2 E2E` (`cups`, `ADEL_E2E_SIN_IMPRESORA`) en `signatures.go`, creada por
  `desktop/e2e/workflow-stagea2-terminals.spec.ts`.
- **Escritorio** (`desktop/src/pages/DevicesPage.tsx`, `services/models.ts`, `services/serverCapabilities.ts`):
  Terminales › Editar de un `android` → «Impresora de precuentas» («Sin impresora» y las impresoras con «· en <terminal>»;
  las que no sirven, deshabilitadas con el motivo, salvo la ya elegida), «Ver configuración» la informa («Sin impresora:
  este dispositivo no imprime precuentas»). Solo con `device-precheck-printer-v1`; sin ella el `PATCH` no lleva el
  campo. Sin operaciones nuevas en Rust (`device_configuration` y `update_device` ya existían).
- **Contrato** (`openapi.json`): `Terminal` con `precheck_printer_id` y `precheck_printer` (obligatorios: Go siempre
  los entrega); operaciones `printPrecheck`, `getDocumentStatus` y `retryDocument`; esquemas `PrintedDocument`,
  `PrinterRejection` y `DocumentStatus`; `pinLogin` con la lista nueva; descripciones de `getIdentity`,
  `getRecoveryState` y `Error`. `TestContractV1` agrega `contractPrecheck` (37 operaciones, antes 34; el cliente del
  recorrido lleva el pool para dejar un trabajo como fallido, porque ahí no corre el trabajador de la cola).
- **Documentación**: `docs/API_V1.md` («Imprimir la precuenta (etapa A2, parte C)», tabla, capacidad, permisos,
  `terminal` de la recuperación, códigos de error) y `API_CONTRACTS.md` («Etapa A2 · impresora de precuentas por
  dispositivo»).

Sin locks consultivos nuevos: el `PATCH` es de administración (transacción de autenticación) y las rutas de
`payments/*` toman 7820148305 como siempre (la de estado, compartido).

### Decisiones (con su porqué)

1. **Reimpresión (punto 5 del encargo): el dispositivo no necesita `reprintDocument` ni `payments.tickets.reprint`.**
   Comprobado en el escritorio: «Imprimir precuenta» (`PrecheckPanel.tsx`, `payments_print_precheck`) **siempre** arma
   una precuenta nueva con el estado vigente, sin motivo, aunque la cuenta no haya cambiado. La «copia idéntica con
   motivo» (`payments_reprint`, `payments.tickets.reprint`) solo existe en la lista de documentos de Cobros
   (`DocumentList.tsx`, que exige `payments.read`). La regla para el dispositivo es la misma del botón: volver a tocar
   «Imprimir …» = otra precuenta; «Reintentar» sin motivo si nunca salió (9F). No se agregó nada a `WaiterPermissions`
   por esto.
2. **Decisión propuesta · Las opciones del selector llegan en `devices/{id}/configuration`** (con `devices.read`) y no
   de `kitchen/printers` (que exige `kitchen.printers.manage`): quien administra terminales puede elegir sin permisos de
   cocina, y Go dice el motivo de cada impresora que no sirve (no se repite la regla en TypeScript).
3. **Decisión propuesta · `devices.precheck_printer_id` con `CHECK` de tipo `android`** además de la validación de Go,
   para que ningún camino deje una impresora de precuentas a una terminal de escritorio (A2-D3 la define solo para el
   dispositivo).
4. **Decisión propuesta · La resolución por dispositivo aplica a todos los documentos que pida un `android`** (también
   tickets de pago o de caja, si una sesión con contraseña en un teléfono llegara a pedirlos): «un android imprime solo
   en su impresora» sin excepciones. Las terminales de escritorio y de cocina no cambian.
5. **Decisión propuesta · `problem`, `printer_name` y `terminal_name` en todas las respuestas de impresión de
   documentos** (también la reimpresión y el ticket de pago), con la misma función: cambio compatible, el escritorio
   los ignora.
6. **Decisión propuesta · «Quitar datos de prueba» desvincula la impresora de precuentas** de un dispositivo (como la
   impresora propia de una caja): si no, la llave foránea nueva habría convertido una impresora de prueba asignada en
   «registro relacionado» y la habría dejado desactivada en lugar de eliminarla.
7. **«Atendió:»** sigue siendo quien pide la impresión (sin cambios, como dijo la Fase 0): impresa desde el teléfono
   dice el mesero que la imprime.

### Forma final para el agente de Android

Exigir **`device-precheck-printer-v1`** (servidor 0.13.0). Con la sesión con PIN (no con la credencial de envío) y
conexión; imprimir **nunca** entra en la cola sin conexión («Para imprimir necesitas conexión con la computadora
principal»). Mostrar el botón con `payments.tickets.print` en `user.permissions`.

- **Nombre del botón**: `establishment.precheck_name` de `getIdentity` → `precheck` «Imprimir precuenta»,
  `sales_ticket` «Imprimir ticket de venta», `bill` «Imprimir cuenta». El impreso lo titula Go igual.
- **A dónde va**: `terminal.precheck_printer` de `getIdentity`/`getRecoveryState`: `null` o
  `{"id", "name", "terminal_name"}` (`terminal_name` solo si es la USB de una terminal). Con `null` se puede avisar antes
  de tocar («Este dispositivo no tiene impresora asignada…»), pero la respuesta de Go manda.
- **Imprimir**: `POST /api/v1/payments/accounts/{id}/precheck/print` `{"operation_id": "<uuid>"}` (misma clave en los
  reintentos de red) → **200** `{"requested": true, "status": "pending", "job_id", "ticket_id", "message": "", "problem":
null | "agent_offline", "printer_name": "<impresora>", "terminal_name": null | "<terminal>"}`:
  - `problem == "agent_offline"` → «La <terminal_name> está desconectada» (el trabajo espera y saldrá cuando vuelva);
  - si no → «Enviada a <terminal_name o printer_name>».
  - **409 `PRINTER_NOT_CONFIGURED`**: `{"error": {"code": "PRINTER_NOT_CONFIGURED", "message": "Este dispositivo no
tiene impresora asignada. Pide al administrador que la elija en Terminales.", "details": {"document": "precheck",
"problem": "missing", "scope": "device"}}}` → «Este dispositivo no tiene impresora asignada» con el aviso de pedírsela
    al administrador.
  - **409 `PRINTER_UNAVAILABLE`**: `details: {"document": "precheck", "problem": "inactive" | "unsupported",
"printer_name", "scope": "device"}`, mensaje «La impresora <nombre> está desactivada.» o «Esta impresora todavía no
    puede usarse para imprimir.» → mostrar el mensaje de Go y pedir al administrador que la revise en Terminales.
  - 403 `PERMISSION_DENIED` sin el permiso; 409 `IDEMPOTENCY_CONFLICT` con la misma clave y otro contenido.
- **Seguir el trabajo** (unos 15 s, cada 2–3 s): `GET /api/v1/payments/accounts/{id}/documents/status?ticket_id=<ticket_id>`
  → `{"ticket_id", "kind": "precheck", "payment_id": null, "created_at", "job_id", "job_state", "job_problem",
"printer_name", "copies", "terminal_name"}`. `job_state` `submitted` = la impresora o su cola lo aceptó (fin: «Enviada a
  …»); `job_problem` `unreachable` → «La impresora no responde» (con «Reintentar»); `agent_offline` → «La
  <terminal_name> está desconectada»; `uncertain` → pedir revisar el papel (sin «Reintentar»); `not_found`, `setup`,
  `inactive`, `unsupported` → «La impresora necesita revisión: avisa al administrador». 404 si el ticket no es de esa
  cuenta; 400 si no es un uuid; 403 sin el permiso. Sin importes ni pagos.
- **Reintentar** (solo con `job_state` `error` o `intervention`): `POST /api/v1/payments/accounts/{id}/documents/retry`
  `{"operation_id": "<nueva>", "ticket_id": "<ticket_id>"}` → 200 `{"status", "job_id" (el mismo trabajo), "ticket_id",
"problem", "printer_name", "terminal_name"}`, en la impresora del dispositivo **de ese momento** (sin ella, el mismo 409
  `PRINTER_NOT_CONFIGURED` con `scope: "device"`). 409 `PRINT_NOT_FAILED` si ya no está fallido, `PRINT_UNCERTAIN` si no
  se sabe si salió, `PRINT_JOB_SUPERSEDED` si ya hay una impresión más reciente: volver a leer el estado.
- **Volver a imprimir** algo que ya salió: el mismo `precheck/print` con otra `operation_id` (precuenta nueva), como en
  la caja; no hay copia con motivo en el dispositivo.

### Para la guía (`docs/ANDROID_INSTALAR.md`)

- **Permiso del rol Mesero** (Administración › Personal › Roles › Editar permisos): «**Imprimir tickets y
  precuentas**» para que el mesero imprima la precuenta desde su teléfono. No hace falta «Reimprimir tickets»: volver a
  imprimir arma otra precuenta, como en la caja.
- **Impresora de precuentas por dispositivo**: Administración › Sistema › **Terminales** › **Editar** en la fila del
  teléfono o tableta → «**Impresora de precuentas**» → elige una impresora de red o la impresora USB de una caja («…
  · en Caja 1») → «Guardar cambios». «Ver configuración» la muestra. Sin impresora, ese dispositivo **no imprime**
  precuentas (nunca manda la precuenta a otra impresora); el mesero ve «Este dispositivo no tiene impresora asignada».
- La precuenta sale con el nombre visible elegido en Administración › Documentos (Precuenta, Ticket de venta o Cuenta) y
  con «Atendió: <el mesero que la imprime>».
- Si la impresora es la USB de una caja, esa caja debe tener AD-EL POS abierto: si no, el teléfono dice «La Caja 2
  está desconectada» y la precuenta sale cuando vuelva a abrirse.
- Requiere la computadora principal en la versión **0.13.0**.

### Pruebas ejecutadas

| Qué                                                                                                                                                                                                                                                       | Dónde                                 | Resultado                                  |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | ------------------------------------------ |
| `TestStageA2PrecheckPrinter` (5 subpruebas, `-v`: Terminales; sin impresora; USB de una terminal desconectada y bytes ESC/POS con PRECUENTA, TICKET DE VENTA y CUENTA, «Atendió:», folio y partidas; red que no responde y reintento; `documents/status`) | `./internal/operations`, base `_s_`   | **PASS** las 5 (18 s)                      |
| `TestStageA2Upgrade` y `TestStageA1Upgrade` (`-v`)                                                                                                                                                                                                        | `./internal/database`                 | **PASS** las 2                             |
| `TestStageTwoAcceptance` con «Quitar datos de prueba» y la impresora de precuentas de un teléfono (`-v`)                                                                                                                                                  | `./internal/administration`           | **PASS**                                   |
| `TestContractV1` (`-v`) con `contractPrecheck`                                                                                                                                                                                                            | `./internal/apicontract`              | **PASS**; 37 operaciones (antes 34)        |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                                                                                                                                                                            | `iso.sh go-unit`                      | **aprobado** (35 paquetes ok)              |
| `go-int` completos: `database`, `administration`, `operations`, `apicontract`                                                                                                                                                                             | `iso.sh go-int`, una base por paquete | **ok** (13 s, 221 s, 414 s, 35 s)          |
| Vitest completo (`tsc --noEmit` + `npm test`), con `stageA2Terminals.test.tsx` (5 casos)                                                                                                                                                                  | `iso.sh vitest`                       | **97 archivos, 921 pruebas aprobadas**     |
| `build` y `cargo test`                                                                                                                                                                                                                                    | `iso.sh build`, `iso.sh cargo`        | **ok** (cargo: 107 aprobadas, 3 ignoradas) |
| Playwright Chromium: `workflow-stagea2-terminals` sola                                                                                                                                                                                                    | `iso.sh browser chromium` (8211/1431) | **1 aprobada**                             |
| Playwright Chromium: `workflow-stage9g-terminals`, `workflow-stage9g-print-agent`, `workflow-stagea1-pin`, `workflow-stagea2-terminals` y `workflow-zz-test-data` en una corrida                                                                          | idem                                  | **6 aprobadas**                            |
| Playwright WebKit: `workflow-stage9g-terminals`, `workflow-stagea1-pin` y `workflow-stagea2-terminals`                                                                                                                                                    | `iso.sh browser webkit` (8211/1431)   | **4 aprobadas**                            |
| `format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                                                                                                                                                                                            | árbol `srv`                           | Correctos; idénticos                       |

### No verificado

- La aplicación Android contra estas rutas (agente de Android, fase C del dispositivo y Fase E con los bytes ESC/POS en
  el emulador).
- Papel real: las pruebas usan el agente simulado (`print-agent/jobs/claim`) y un adaptador controlado para «no
  responde»; ninguna impresora física.

### Decisiones del coordinador sobre la parte C

Se aceptan las siete decisiones de la parte. En particular, la reimpresión: «Imprimir precuenta» siempre arma una
precuenta nueva sin motivo (también en el escritorio), y la copia idéntica con motivo solo existe en la lista de
documentos de Cobros (`payments.read`); el dispositivo sigue esa misma regla, sin `payments.tickets.reprint`. La parte C
se verificó sobre una copia exacta de `6c2fcdd` (`git archive`): `fmt-check`, `sync-check`, `go-unit`, `go-int` de
`apicontract`, `database` (`TestStageA2Upgrade`) y `administration` completos, `TestStageA2PrecheckPrinter`
(`operations`), Vitest (97 archivos, 921 pruebas), `npm run build` y Playwright Chromium `workflow-stagea2-terminals`
(1 de 1, sola sobre una base nueva), todo aprobado.

## Fase B · Para llevar en el dispositivo

Árbol `$SP/wt/b`, rama `etapa-a2-b` desde `etapa-a2-android` (`8913ecc`: Fase A y servidor parte B), un agente,
18/11/2026, en paralelo con el agente del servidor y el escritorio (partes C y D), sin tocar `backend/`, `desktop/`,
`docs/API_V1.md`, `API_CONTRACTS.md`, `config/product.json` ni `CLAUDE.md` raíz. Aplica §3 de la especificación con
§0-B, A2-D1, A2-D2, A2-D5, A2-D6 y A2-D11 y usa tal cual la «Forma final para el agente de Android» de la parte B. **El
servidor no cambió en esta fase** (no hizo falta nada fuera de la parte B). Pruebas en copias aisladas (`ISO_TAG=B`):
Go temporal en 8221/8481 (modo de pruebas, base `adel_pos_test_b_*` del clúster 55499) y los emuladores `Pixel_Tablet`
(`emulator-5584`, Android 17) y `ADEL_A1_Phone_API33` (`emulator-5586`, Android 13), de solo lectura.

**Resultado.** El mesero cambia entre **Mesas** y **Para llevar** desde la navegación principal (solo con
`takeaway.read` de Go). «Nuevo pedido» entra directo al catálogo y el pedido nace con el primer producto (con conexión)
o al enviarse (sin conexión), con la misma captura, la misma cola, los mismos estados y las mismas claves de la A1. El
nombre y el teléfono se editan en cualquier momento (también sin conexión: se guardan y se envían al volver). La lista
muestra folio, nombre, tiempo, preparación y pago como los dice Go, al momento con conexión y, sin ella, lo último que
se vio con su hora; «Entregado» solo pagado y con permiso, y sin pagar «Cobrar en caja antes de entregar». La captura
vive ahora en `core:capture`, compartida, y el modo Mesero quedó igual para el mesero.

### Qué se hizo

| Commit    | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `0534dfc` | **Movimiento** (`git mv`, 6 renombres): `feature:waiter/capture/*` (`CaptureScreen`, `CaptureViewModel`, `OrderPanel`, `SentLineDialogs`) y `pending/*` (`PendingScreens`, `PendingViewModels`) pasan al módulo nuevo **`core:capture`** (`com.adelpos.meseros.core.capture`, biblioteca Android sobre `core:ui`), con sus textos (`core/capture/src/main/res/values/strings.xml`; `table_panel_products` de la fila de captura de «Pedidos pendientes» pasa a `capture_products`). `CaptureDependencies` (solo interfaces de `core:domain`) en `core:capture`; `WaiterDependencies` la contiene y agrega la vista de Mesas y el reloj. Sin cambios de comportamiento: las 256 pruebas de la JVM, `ktfmtCheck` y `lintDebug`, igual que antes.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `ed833c7` | **Compatibilidad** (A2-D6): `ServerCompatibility.check` exige servidor **0.13.0** con `waiter-pin-v1` **y `takeaway-v1`** (`REQUIRED_CAPABILITIES`, `ServerCompatibility.kt:60-63`), con el mismo «Actualiza el servidor del restaurante»; su texto ahora dice «le falta la entrada con PIN o Para llevar». El Go simulado de las pruebas de la JVM anuncia 0.13.0 con la capacidad.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `e767701` | **Dominio, datos, cola y Room v6.** Dominio (`core:domain/waiter/Takeaway.kt`, `TakeawayBoard.kt`): `OrderMode`, `TakeawayCustomer` (límites de Go en bytes del texto recortado: 100 y 40), `TakeawayStates` (preparación, pago, entrega y `deliveryOffer`, A2-D2), `CustomerEdit`/`CustomerTarget`, `DeliveryResult`, `TakeawayService` y `TakeawayBoard.entries` (la lista); `Capture`, `OrderDestination` y `UnresolvedSave` con la modalidad y el cliente (`tableId` opcional); `WaiterAction.VIEW_TAKEAWAY`/`MANAGE_TAKEAWAY`/`OPEN_TAKEAWAY`/`DELIVER_TAKEAWAY` y `WaiterPermissions.grants`; `CaptureService.openTakeaway`/`setCustomer`. Datos: `WaiterMapper.openBody` (`openAccount` con `mode: takeaway`, `table_id: null`, sin `seen_account_ids`, `origin` «Para llevar», `customer_name` y `phone`; el cuerpo de una mesa queda idéntico al de la A1), `destination.takeaway` en `OrderSender.body`, `updateTakeawayCustomer` y `deliverTakeaway` en `WaiterApi`, `AccountDto` con `customer_name`, `phone`, `origin`, `payment_state`, `delivery_state` y `created_at`; `TakeawayRepository` (lista, ediciones del cliente y «Entregado») y `CustomerEditSender` (envía las ediciones; lo llama el mismo `OrderSendWorker`, después de los pedidos); `RestaurantRepository` escucha también el ámbito `takeaway` y expone todas las cuentas abiertas con la hora de la lectura (`TakeawayRead`). **`AppDatabase` v6** (`AppDatabase.kt:183`, `WAITER_CHANGES_V6` en `WaiterEntities.kt`): `captures` con `table_id` opcional, `mode`, `customer_name` y `customer_phone` (la tabla se copia: Android 8 no cambia una columna a opcional) y la tabla nueva `takeaway_customer_edits`; `schemas/…/6.json`; caso 5 → 6 en `AppDatabaseMigrationTest`; `WaiterDao.eraseAll` (registrado en `deviceDataWipe`) borra también las ediciones y la bitácora del retiro las lista. Pruebas `TakeawayRulesTest` y `TakeawayQueueTest`. |
| `b434e5b` | **Pantallas.** Navegación principal «Mesas» / «Para llevar» (`NavigationBar` de Material 3, `AdelPosApp.kt:236-290`) solo con `takeaway.read` de Go; `feature:takeaway` (`TakeawayScreen`, `TakeawayViewModel`, `TakeawayDependencies`, sin depender de `feature:waiter`); en `core:capture`, el pedido para llevar en la captura (título «Para llevar», «Pedido nuevo: nace al guardar el primer producto.», el cliente y «Nombre y teléfono», `CustomerDialog`), y en «Pedidos pendientes» y «Requiere tu revisión» el nombre «Para llevar · Juan» y «Enviar como pedido nuevo para llevar»; en `core:ui`, los íconos `Table` y `Bag` (los trazos del escritorio) y `TakeawayPreparationBadge`/`TakeawayPaymentBadge`. Pruebas `TakeawayViewModelTest` y el cliente en `WaiterViewModelsTest`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `6671404` | **Pruebas en el dispositivo.** `TakeawayScreensTest` (Compose con datos fijos, en ambos temas), `AppNavigationTest.mesasYParaLlevarEnLaNavegacionPrincipal` y, contra el Go temporal, `TakeawayOnGoTest` (`t1`–`t8`) con la suite nueva **`takeaway`** de `scripts/android-e2e.sh` (`takeaway_step`; `t4` con la red cortada por el guion), también en `--suite all`. La prueba hace de cocina (estación y ruta, «En preparación» y «Listo» por `kitchen/states`, como `FieldOnGoTest`) y de caja (turno y pago total por `payments/…`) por la escucha local.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `8264d46` | **Documentación.** `docs/ANDROID_INSTALAR.md`: sección nueva **6 «Para llevar»** para el propietario (tomar un pedido, nombre y teléfono, la lista y entregar; las siguientes pasan a 7–9 con sus referencias), las tres filas de permisos del rol Mesero de «Para la guía» de la parte B con su explicación, la computadora principal **0.13.0** en las secciones 2 y 7 y «versión 0.2.0» en la introducción (el nombre del APK lo cambia la Fase F). `android/CLAUDE.md`: `core:capture` y `feature:takeaway` en «Estructura» y en las dependencias, reglas nuevas **20** (lo compartido vive en `core:capture`) y **21** (una edición posterior nunca cambia el pedido de la cola), sección «Para llevar (etapa A2, Fase B, para los demás módulos)», versiones (0.13.0 con `takeaway-v1`), `AppDatabase` v6 y la suite `takeaway`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |

### Cómo funciona (resumen para las fases siguientes)

- **Captura** (§3.2). `openTakeaway(user, null)` crea una captura sin mesa (`mode = TAKEAWAY`); salir sin productos la
  borra (`closeIfIdle`: el nombre solo no es trabajo pendiente), sin ninguna petición a Go. Con conexión, el primer
  guardado es `openAccount` para llevar (creación diferida de la A1); sin conexión, «Enviar a cocina» deja el pedido
  «Por enviar» con `destination.takeaway {customer_name, phone}` y la clave = id de la captura (A1-D19). Todo lo demás
  es el código de la A1: confirmación en línea, guardado sin respuesta repetido igual (ahora con la modalidad y el
  cliente en `UnresolvedSave`), `items_in_account` → el mismo pedido con la misma clave a `destination.account_id`,
  «Requiere tu revisión», descartes.
- **Nombre y teléfono** (A2-D1). Mientras nada salió a Go (`Capture.customerEditable`: sin cuenta y sin guardado en
  curso) viven en la captura y viajan al crear el pedido. Después, nunca se cambia el cuerpo de un pedido o de un
  guardado que puede repetirse con su clave: la edición va a `takeaway_customer_edits` (una fila por pedido, la
  última gana, clave de operación nueva en cada edición; `replaceCustomerEdit`). Si el pedido todavía no nace en Go,
  la fila espera su cuenta (la liga `acknowledge` de la captura o el `RECEIVED` de la cola: `bindCustomerEdit`).
  `CustomerEditSender` la envía con `updateTakeawayCustomer` y **la sesión del mesero que la hizo**; se borra solo si
  Go confirmó **esa** clave (`finishCustomerEdit` condicional: una edición más nueva hecha mientras tanto se queda y sale
  después) o si Go la rechaza para siempre.
- **Lista** (§3.3, `TakeawayBoard.entries`). Los pedidos para llevar abiertos de `restaurant/state` (la misma lectura y
  el mismo `restaurant_snapshot` de Mesas), y encima lo del mesero activo que todavía no nace en Go: los pedidos de la
  cola sin cuenta (o cuya cuenta ya no está en la lista) y las capturas nuevas con productos. Un pedido de Go con algo
  del mesero sin enviar lo dice («Tiene productos por enviar…»). Tocar un pedido de Go retoma o abre su captura (con
  `takeaway.manage`); uno «Requiere tu revisión» abre la revisión.
- **Entregado** (A2-D2). `TakeawayStates.deliveryOffer`; `deliverTakeaway` con la sesión del mesero, nunca en la cola;
  sin respuesta se repite una vez con la misma clave. Después se vuelve a leer la lista (y el canal avisa).

### Decisiones propuestas (con su porqué)

Ninguna cambia una regla de negocio ni la especificación; son detalles que §3, A2-D1 y A2-D2 no fijaban.

1. **Las ediciones del cliente posteriores a que el pedido sale nunca cambian el pedido de la cola** (regla nueva 21
   de `android/CLAUDE.md`). Si cambiaran el `destination.takeaway` de un pedido «Por enviar» que ya pudo haber entrado
   (respuesta perdida), el reintento con la misma clave y otro contenido daría `OFFLINE_ORDER_ALREADY_ACCEPTED` y la
   edición se perdería en silencio. Con la tabla aparte el reintento es idéntico y la edición llega después.
2. **Las ediciones salen con la sesión del mesero que las hizo, no con su credencial de envío.** La credencial de envío
   solo vale para `submitOfflineOrder` (revisión R3 de la A1) y `updateTakeawayCustomer` es una ruta de `restaurant/*`.
   Consecuencia (limitación para la guía, ya escrita): un nombre cambiado sin conexión se manda cuando ese mesero vuelve
   a entrar con su PIN **con conexión**; mientras, la lista lo muestra como «se guardará…». No se usa la sesión de otro
   mesero (regla M5 de la A1). Las ediciones no cuentan para `WaiterWork` (no bloquean cambiar de mesero ni cerrar
   sesión) porque nunca se pierden al hacerlo: esperan en Room.
3. **Rechazos definitivos de una edición** (el pedido ya no está abierto, no es para llevar, no cabe, sin permiso) **se
   descartan**: no hay a qué aplicarla y reintentarla para siempre no sirve. `IDEMPOTENCY_CONFLICT` (no debería pasar)
   se reintenta con otra clave; sin respuesta, 5xx o servidor ocupado, se reintenta (WorkManager, espera creciente).
   Una edición de un pedido que ya no está en el dispositivo (se descartó en «Requiere tu revisión») se borra.
4. **En la lista solo se detalla lo local del mesero activo**, como en «Pedidos pendientes» (revisión R6, B3): los
   pedidos sin enviar de otros meseros siguen en el contador del indicador, pero no se muestran. Lo que todavía no
   nace en Go va **arriba** de la lista (es lo que el mesero debe vigilar); después, los de Go en su orden.
5. **Correspondencia de la preparación** (§3.3 pide pendiente, en preparación y listo): `pending_release`, `scheduled`,
   `released`, `confirmed_pending_dispatch` y `change_pending` → «Pendiente»; `preparing` → «En preparación»; `ready` →
   «Listo»; además `without_items`/`draft` → «Sin enviar a cocina» y `cancelled` → «Cancelado» (Go los da y no son
   ninguno de los tres). Pago: `not_recorded` «Sin pagar», `partial` «Pago parcial», `paid` «Pagado»; con un pago
   parcial se dice «Cobrar en caja antes de entregar» (solo `paid` permite «Entregado»).
6. **«Para llevar» y «Entregado» solo con el permiso conocido** (`WaiterPermissions.grants`): con permisos que nunca se
   leyeron no se muestran (a diferencia de `allows` de la A1, que no deshabilita nada sin permisos conocidos). §3.1
   dice «solo aparece con los permisos del servidor»; en la práctica los permisos siempre se conocen, porque una
   entrada con conexión los guarda.
7. **Navegación**: barra inferior de Material 3 en todas las pantallas del mesero (también en la captura), solo cuando
   hay `takeaway.read`; sin el permiso no hay barra y el modo Mesero se ve exactamente como en la A1. Mesas es el
   inicio: cambiar de sección vuelve a la lista de esa sección (lo capturado queda en el dispositivo y se ve en su
   lista). Terminar una captura para llevar («Enviar a cocina» o descartar) vuelve a Para llevar; una de mesa, a Mesas.
   Si el mesero pierde el permiso estando en Para llevar, vuelve a Mesas.
8. **«Nuevo pedido» reutiliza una captura nueva que quedó vacía** (por ejemplo, si Android cerró la aplicación con ella
   abierta), con el cliente en blanco, en lugar de dejar filas vacías en Room.
9. **«Entregado» sin respuesta** se repite una vez con la misma clave (Go devuelve lo mismo); si sigue sin respuesta se
   dice «necesitas conexión» y se vuelve a leer la lista.
10. **La bitácora del retiro** también lista los nombres y teléfonos que no llegaron a Go («Para llevar: nombre y
    teléfono sin guardar…»), como los pedidos sin enviar.

### Pruebas

Todo en el árbol `$SP/wt/b` con el arnés (`ISO_TAG=B`), JDK de Android Studio. Lo de punta a punta contra el Go
temporal construido desde este mismo árbol (servidor 0.13.0 con la parte B: `GET version` anuncia `takeaway-v1`), uno
nuevo para cada corrida (`server-down` + `server-up`, 8221/8481).

| Qué                                                                                    | Dónde                                                                    | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `:core:domain:test`                                                                    | Mac                                                                      | **113 pruebas, 0 fallas** (19 XML): las 103 de antes, `TakeawayRulesTest` (9: preparación, pago y entrega; cuándo se ofrece «Entregado»; permisos; límites en bytes; la cola con el destino para llevar, `items_in_account` y «pedido nuevo para llevar»; el guardado sin respuesta; `WaiterWork` con capturas para llevar; la lista) y 1 más de `ServerCompatibilityTest`                                                                                                                                                                                                                                                                                                                                                                                  |
| `testDebugUnitTest` (JVM + Robolectric SDK 36)                                         | Mac                                                                      | **167 pruebas, 0 fallas**: `core:data` 141 (con `TakeawayQueueTest`, 8: nuevo pedido vacío que no deja nada ni pide nada a Go, nace con el primer producto con su cliente, sin conexión nace al enviarse con `destination.takeaway`, `items_in_account` con la misma clave, ediciones sin conexión —la última gana, esperan a la cuenta y a la sesión, la respuesta de una vieja no borra la nueva—, rechazo definitivo y sin respuesta, «Entregado» con `BALANCE_PENDING`/`ALREADY_DELIVERED`/sin respuesta, la lista de `restaurant/state` y sin conexión lo último con su hora), `core:ui` 9, `feature:access` 7, `feature:waiter` 5 (con el cliente en la captura), `feature:takeaway` 5 (`TakeawayViewModelTest`). Con el dominio, **280** (antes 256) |
| `ktfmtCheck`, `lintDebug`                                                              | Mac, al final con `--rerun-tasks`                                        | Aprobados; lint sin problemas en ningún módulo (también `core:capture` y `feature:takeaway`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `assembleDebug`, `assembleRelease` (sin firmar)                                        | Mac                                                                      | `app-debug.apk` 15 314 386 bytes; `app-release-unsigned.apk` 2 458 743 bytes (antes 2 405 467), `com.adelpos.meseros` 0.2.0 (2), `minSdk` 26, `targetSdk` 36 (`aapt2 dump badging`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Bloqueos de Gradle                                                                     | `resolveAllDependencies … --write-locks`                                 | Sin dependencias nuevas: `core/capture/gradle.lockfile` y `feature/takeaway/gradle.lockfile` iguales al de `feature:waiter` (salvo el encabezado); ningún otro bloqueo ni `verification-metadata.xml` cambió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `connectedDebugAndroidTest` (antes: desinstalar la app, sus `.test` y los de la 0.1.0) | `Pixel_Tablet` (`emulator-5584`, Android 17)                             | **108 pruebas, 0 fallas**, 52 omitidas a propósito (las de punta a punta sin argumentos, 8 más que antes por `TakeawayOnGoTest`): `app` 75 (50), `core:data` 20 (2; con el caso 5 → 6 de `AppDatabaseMigrationTest`), `core:ui` 13 — 3 XML contados (3 min 56 s). La primera corrida tuvo 2 fallas de la prueba nueva `TakeawayScreensTest` (las etiquetas dentro de un renglón tocable se fusionan con él: se buscan en el árbol sin fusionar; y «11 min» salía en tres renglones); corregidas, la clase sola pasó 4 de 4 y después la corrida completa de arriba                                                                                                                                                                                          |
| Ídem                                                                                   | `ADEL_A1_Phone_API33` (`emulator-5586`, Android 13)                      | **108 pruebas, 0 fallas**, 52 omitidas: `app` 75 (50), `core:data` 20 (2), `core:ui` 13 — 3 XML (3 min 8 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `scripts/android-e2e.sh --suite takeaway` (t1–t8)                                      | `Pixel_Tablet`                                                           | **8 de 8 aprobados**, dos veces: con la versión anterior de la prueba (que solo cambió después en un ayudante que la tableta no usa) y otra vez con la final, con otro Go temporal (pasos en ≈ 1 min 30 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Ídem                                                                                   | `ADEL_A1_Phone_API33` (red cortada con `cmd connectivity airplane-mode`) | **8 de 8 aprobados** en la segunda corrida (otro Go temporal). Primera corrida: `t1` y `t2` aprobados y `t3` falló en la prueba: después de escribir el nombre, el primer Atrás solo cerró el teclado y la cuenta seguía a la vista (el teléfono tiene un panel); `t4`–`t8` fallaron porque dependen de `t3`. Corrección: «Volver a Catálogo» se repite mientras la cuenta siga a la vista (`backToCatalog`)                                                                                                                                                                                                                                                                                                                                                |
| `scripts/android-e2e.sh --suite waiter` (w1–w13c, el modo Mesero de la A1)             | `ADEL_A1_Phone_API33`, otro Go temporal                                  | **19 de 19 aprobados** a la primera (pasos en ≈ 3 min): el modo Mesero sigue igual para el mesero                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `iso.sh format`, `fmt-check` (Prettier, gofmt, datos del sistema anterior)             | árbol `b`, antes del commit de la documentación                          | Aprobados; `md5` de `docs/etapas/*.md` igual antes y después                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |

Ninguna prueba usó la red real ni el servicio de la Mac (el ejecutor busca `_adelpostest._tcp` y se empareja por la
dirección del Go temporal; ninguna lista mostró un servidor real). Al terminar se apagaron los dos emuladores, se
detuvo el Go temporal (y se borró su base) y no quedó ninguna aplicación instalada en los emuladores.

### No verificado

- Dispositivo físico (no hay); `Pixel_8` y `ADEL_A1_Tablet_API26` (Android 8): la matriz de cuatro es de la Fase E.
  En particular, la migración 5 → 6 (copia de `captures`) no se corrió en Android 8, aunque usa el mismo
  procedimiento que la 2 → 3, probado ahí en la A1.
- La migración 5 → 6 con datos reales de una 0.2.0 anterior a esta fase: no existe ninguna instalada (la 0.2.0 es de
  esta etapa); el caso de `AppDatabaseMigrationTest` crea la v5 con capturas y pedidos y comprueba que se conservan.
- Las suites `access`, `catalog`, `field` y `--suite all`: la regresión completa es de la Fase F. `--suite all` ahora
  incluye `takeaway`; no se corrió completa.
- El escritorio y Go no cambiaron en esta fase y no se probaron aquí (la parte B se verificó en su propia parte).
- Que el teclado del teléfono quede abierto un instante después de «Guardar» en «Nombre y teléfono» se vio solo como
  causa de la primera falla de `t3`; no se revisó a mano si se nota al usarlo.

### Para el coordinador

- **`AppDatabase` v6 ya existe** (`captures` y `takeaway_customer_edits`; `schemas/…/6.json`, `MIGRATION_5_6` con
  `WAITER_CHANGES_V6` y su caso en `AppDatabaseMigrationTest`). Las fases C y D agregan sus tablas **a esta misma
  versión**: sus entidades en `@Database`, su SQL al final de la migración 5 → 6, el `6.json` regenerado y el caso de
  la migración ampliado (si se fusionan en paralelo, el `6.json` y la lista de la migración chocarán: conviene que una
  de las dos parta de la otra).
- **`core:capture` es donde va el botón «Imprimir [nombre visible]» de la Fase C** (`OrderPanel`, para mesa y para
  llevar: `capture.takeaway`), y `CaptureDependencies` es donde se le dan sus servicios.
- **`OrderSendWorker` corre ahora dos cosas por vuelta** (la cola y después `CustomerEditSender`); `OrderWorkerFactory`
  recibe `customers`. La Fase D (servicio en primer plano) debe convivir con eso igual que con la cola.
- **El servidor no cambió** y no hizo falta nada fuera de la parte B. Nada obligó a cambiar una regla de negocio.
- Decisiones propuestas 1–10 de arriba: ninguna es parada; la 2 (ediciones con la sesión del mesero que las hizo) deja
  una limitación que ya dice la guía.
- `docs/ANDROID_INSTALAR.md` todavía nombra el APK `AD-EL-POS-Android-0.1.0.apk` en la sección 1 (lo cambia la Fase F,
  como se acordó) y la sección 9 dice que la precuenta y el aviso llegan «más adelante en esta misma etapa» (C y D lo
  quitan al terminar).
- La navegación inferior ocupa la altura estándar de Material 3 (80 dp): con `takeaway.read` también se ve en la
  captura del teléfono en horizontal. Las pruebas de la A1 no la ven (sus meseros no tienen `takeaway.read`); la Fase
  E puede revisar con capturas si conviene ocultarla en la captura del teléfono horizontal.

### Para CLAUDE.md

- §0/encabezado: Android 0.2.0 con Para llevar (Fase B); módulos `core:capture` y `feature:takeaway`; `AppDatabase` v6
  (una sola para la etapa); la aplicación exige el servidor 0.13.0 con `takeaway-v1`.
- Árbol del repositorio (`android/`): `core/capture` (captura compartida de Mesas y Para llevar) y `feature/takeaway`.
- Pruebas (§6): `TakeawayRulesTest`, `TakeawayQueueTest`, `TakeawayViewModelTest`, `TakeawayScreensTest` y la suite
  `takeaway` de `scripts/android-e2e.sh` (`TakeawayOnGoTest`, `t1`–`t8`; también en `--suite all`).
- Reglas de Android nuevas en `android/CLAUDE.md`: 20 (lo compartido vive en `core:capture`) y 21 (una edición
  posterior nunca cambia el pedido de la cola).

### Decisiones del coordinador sobre la Fase B

Se aceptan las diez decisiones propuestas de la fase. Queda una limitación para la guía: una edición del nombre o del
teléfono hecha **sin conexión** sale con la sesión del mesero que la hizo (la credencial de envío solo sirve para
`offline-orders`, revisión R3 de la A1), así que llega a Go cuando ese mesero vuelve a tener conexión con su sesión o
vuelve a entrar con conexión; nunca se pierde. Fusión: `etapa-a2-b` (`8264d46`) en `bfa7d22`, sin archivos en común
con las partes B y C del servidor; `android/`, `scripts/android-e2e.sh` y `docs/ANDROID_INSTALAR.md` quedaron
idénticos a los de la rama de la fase, así que sus pruebas valen para la fusión. Las fases C y D agregan sus tablas a
la misma `AppDatabase` v6 (aún no entregada) en lugar de abrir una v7.

## Servidor · Parte D: registro y canal de platillos listos (esquema 26)

Rama `etapa-a2-srv`, mismo árbol `$SP/wt/srv`, encima de `6c2fcdd` (parte C), un agente, 18/11/2026, en paralelo con
Android en su árbol (no se tocó `android/`, `scripts/android-e2e.sh` ni `docs/ANDROID_INSTALAR.md`). Pruebas aisladas
(`ISO_TAG=S`): clúster temporal 55499 con bases `adel_pos_test_s_*` y Playwright con el Go temporal del arnés
(`ISO_GO_PORT=8211`, `ISO_PREVIEW_PORT=1431`, detenido al terminar). Aplica §4 de la especificación con A2-D4, A2-D9 y,
para el contenido, A2-D10 (el texto lo arma la app; Go entrega `product_name` y `quantity`).

Commits: `b17f56f` (0026 sección D y su comprobación en `TestStageA2Upgrade`; `TestStage9HUpgrade` excluye la tabla
nueva de su conteo, como ya excluía las de la 0025), `c988d16` (servidor, contrato, documentación y pruebas) y
`a2c2812` (escritorio, Vitest y Playwright).

### Qué cambió

- **Migración `0026_android_a2.sql`, sección «Parte D»**: `app.ready_notices` (`sequence` identidad como clave y
  cursor, `unit_id`, `version_id`, `account_id`, `recipient_user_id`, `recipient_device_id` opcional, `ready_at`;
  `UNIQUE (unit_id, version_id)`; índices `(recipient_user_id, sequence)` y `(ready_at)`), vacía al actualizar, y
  `devices.ready_alerts_enabled`, `ready_alerts_sound`, `ready_alerts_vibration` (`boolean NOT NULL DEFAULT true`).
  Ninguna fila cambia; `TestStageA2Upgrade` lo comprueba (también la sección C).
- **Escritura** (`internal/kitchen/preparation.go`, `recordReadyNotices` dentro de `publishPreparation`): en la misma
  transacción que publica `restaurant.account.preparation_changed`, cada unidad activa de las cuentas anotadas cuyas
  tareas vivas de su versión están todas en «Listo» y la última se marcó en esta transacción (`ready_at = now()`).
  Revisé los caminos: el único `UPDATE … state='ready'` es `kitchen.saveStates` (unidad, pedido de una estación con
  `release_id` y cuenta completa desde el KDS); los reintentos de avisos y las modificaciones no marcan «Listo» (una
  modificación de receta reinicia las tareas y su versión nueva se avisa al estar lista; una económica conserva las
  tareas listas y no se vuelve a avisar). Destinatario: el responsable de la cuenta en ese momento; dispositivo
  destinatario: el android no revocado de su sesión con PIN vigente con `last_seen_at` más reciente, o ninguno. Al
  registrar algo se poda lo de más de 2 días. Las instalaciones existentes empiezan sin avisos (nada viejo se avisa).
- **Lectura** (`internal/administration/ready_alerts.go`, nuevo; `service.go`, `http.go`):
  - `GET ready-alerts?after=` en `dispatch` después de `authenticateDevice` y `matchClientKind`, antes de la sesión
    (solo la credencial del dispositivo); solo `android` (si no, 403 `READY_ALERTS_DEVICE_NOT_ALLOWED`).
  - `GET ready-alerts/stream` (`serveReadyAlerts`): el patrón de `serveEvents` (750 ms, 30 s, `Last-Event-ID`,
    `interrupted`), con su contador `readyStreams`, **uno por dispositivo** (429 `READY_ALERTS_CONNECTION_LIMIT`).
  - Lock 7820148305 **compartido** al leer (cocina escribe con él en exclusiva): ningún aviso queda detrás del cursor.
  - Filtro (A2-D4, A2-D9): para el dispositivo D, avisos cuyo mesero entró con PIN en D desde el inicio del día
    comercial (`businessday`, «Hora de inicio del día») y con `recipient_device_id` = D o vacío; agrupados por la
    operación de cocina (cuenta, destinatario e instante de la transacción), hasta 50 por respuesta.
  - Contenido: `id`, `account_id`, `mode`, `table_name`, `folio`, `customer_name`, `waiter_user_id`,
    `waiter_short_name`, `items: [{product_name, quantity}]` (`product_name` = el nombre del producto y, si tiene
    presentación, «· Presentación», como en la comanda), `ready_at`. Nunca precios ni importes.
- **Terminales** (`internal/administration/devices.go`, `precheck_printer.go`): `PATCH devices/{id}` con
  `ready_alerts_enabled`, `ready_alerts_sound` y `ready_alerts_vibration` opcionales (sin ellos se conserva; en un
  dispositivo que no es `android`, 400); `GET devices`, la respuesta del `PATCH`, la configuración (`device`) y la
  terminal de `auth/me` y `restaurant/state` los informan (`waiterDeviceColumns`, que ya llevaba la impresora de
  precuentas; `requireAndroidDevice` compartido con la parte C).
- **Capacidad** `ready-alerts-v1` (`internal/httpapi/router.go`, `clientversion_test.go`).
- **Escritorio** (`desktop/src/pages/DevicesPage.tsx`, `services/models.ts`, `services/serverCapabilities.ts`):
  Terminales › Editar de un `android` → «Avisos de platillos listos» con los interruptores «Avisar platillos listos»
  (activado por omisión), «Sonido» y «Vibración» (`ToggleButton`; sonido y vibración deshabilitados con el aviso
  apagado); «Ver configuración» lo dice («Activado · con sonido y vibración», «Desactivado»…). Solo con
  `ready-alerts-v1`. Sin operaciones nuevas en Rust.
- **Contrato** (`openapi.json`): `getReadyAlerts` y `streamReadyAlerts` (seguridad: solo `deviceId` + `deviceToken`),
  esquemas `ReadyAlertsBatch` y `ReadyAlert`, `Terminal` con los tres booleanos (obligatorios), descripciones de
  `getIdentity`, `getRecoveryState` y `Error`. `TestContractV1` agrega `contractReadyAlerts` (39 operaciones).
- **Documentación**: `docs/API_V1.md` («Avisos de platillos listos (etapa A2, parte D)», tabla, capacidad, `terminal`,
  códigos de error) y `API_CONTRACTS.md` («Etapa A2 · registro y canal de platillos listos»).

Sin locks consultivos nuevos (para la tabla de `CLAUDE.md` §3: la lectura de `ready-alerts` toma 7820148305 compartido,
como `restaurant/state`; la escritura va dentro de las operaciones de cocina, que ya lo tienen en exclusiva).

### Decisiones (con su porqué)

1. **Decisión propuesta · El dispositivo destinatario se fija al «Listo» (`recipient_device_id`)**, una columna más de
   las que pedía el encargo. Si se decidiera al leer, un mesero que cambia de tableta justo cuando cocina marca «Listo»,
   o una tableta que se reconecta tarde con su cursor, podría hacer que el aviso no llegara a ninguna (la nueva ya pasó
   ese cursor y la vieja ya no es la «más reciente») o llegara a dos. Fijado al registrar, cada dispositivo lo recibe o
   no sin importar cuándo consulte: «ningún aviso se pierde ni se repite» (§4.1). «Entró en D durante el día comercial»
   sí se revisa al leer (solo crece durante el día).
2. **Decisión propuesta · Un aviso = una operación de cocina en una cuenta**, agrupado por platillo (`items` con
   `quantity`); su `id` es el último número del grupo y el cursor avanza por grupos completos. Es lo que la app
   necesita para «Mesa 4 · 2 platillos listos» (A2-D10) sin contar ella; varios avisos de la misma mesa los agrupa la
   app en su notificación (§4.4).
3. **Decisión propuesta · Se escribe en `publishPreparation` con la regla `ready_at = now()`** y no en cada camino:
   cubre cualquier forma de marcar «Listo» (también una futura) y nunca avisa algo que ya estaba listo.
4. **Decisión propuesta · `active`** = aviso activado **y** (algún mesero entró con PIN en D desde el inicio del día
   comercial **o** hay una sesión con PIN vigente en D). Es §4.3: «activo mientras haya un mesero con sesión en el
   dispositivo o un mesero que entró en él durante el día comercial en curso».
5. **Decisión propuesta · Apagado o inactivo, el cursor avanza** hasta el último aviso: al volver a activarse (o al
   entrar un mesero) no llegan avisos viejos de ese tiempo.
6. **Decisión propuesta · Un cursor mayor que el último aviso** (solo si la base volvió atrás, un respaldo restaurado)
   se trata como 0: se lee desde el inicio del día y el dispositivo descarta los ya avisados (por `key`, desde las
   correcciones de la revisión).
7. ~~**Un canal por dispositivo** (429 `READY_ALERTS_CONNECTION_LIMIT`)~~: **reemplazada** en las correcciones de la
   revisión (R12): un canal nuevo del mismo dispositivo cierra el anterior; ya no hay 429.
8. **Decisión propuesta · `product_name` con la presentación** («Filete · Grande», el mismo formato de la comanda);
   `quote_line.name` de la cuenta solo trae el producto.
9. **Decisión propuesta · 403 `READY_ALERTS_DEVICE_NOT_ALLOWED`** a un dispositivo que no es `android` y 400 al
   configurar los avisos de una terminal de escritorio.

### Forma final para el agente de Android

(Corregida con el protocolo acordado tras la revisión independiente; lo que cambió está en «Correcciones de la
revisión», al final.)

Exigir **`ready-alerts-v1`** (servidor 0.13.0). Todas las peticiones con `X-Device-ID`, `X-Device-Token`,
`X-Client-Version` y `X-Client-Kind: android`, **sin `Authorization`** ni credencial de envío.

- **Lectura**: `GET /api/v1/ready-alerts?after=<cursor>` → 200

  ```json
  {
    "cursor": 812,
    "active": true,
    "day_ends_at": "2026-11-19T11:00:00Z",
    "settings": { "enabled": true, "sound": true, "vibration": false },
    "alerts": [
      {
        "id": 812,
        "key": "5f0c2d6e-3b1a-4c8e-9f27-6d4b1a0e7c35",
        "account_id": "…",
        "mode": "dine_in",
        "table_name": "Mesa 4",
        "folio": 31,
        "customer_name": "",
        "waiter_user_id": "…",
        "waiter_short_name": "Ana",
        "items": [{ "product_name": "Filete · Grande", "quantity": 2 }],
        "ready_at": "2026-11-18T20:41:07.12Z"
      }
    ]
  }
  ```

  En un pedido para llevar: `"mode": "takeaway"`, `"table_name": null` y `customer_name` el capturado (o vacío).

- **Canal**: `GET /api/v1/ready-alerts/stream?after=<cursor>` (o `Last-Event-ID: <cursor>`): `text/event-stream`. El
  primer mensaje (`id: <cursor>`, `event: alerts`, `data: <el mismo JSON>`) llega enseguida; después **solo** llega
  otro cuando hay avisos nuevos o cambian `active`, `settings` o `day_ends_at`. En silencio, el comentario SSE
  `: ping` cada **15 s** (si pasan ~45 s sin nada, la conexión está muerta: reconectar). A los **5 minutos** el servidor
  cierra: reconectar enseguida con `Last-Event-ID` = el último `id`. **Un canal nuevo del mismo dispositivo reemplaza
  al anterior** (el viejo se cierra): tras un cambio de red, abrir uno nuevo sin esperar. `event: interrupted` llega
  **sin `id`** (el cursor no cambia) con `data: {"code": "<código>"}`: **solo `DEVICE_REVOKED` significa retirado**;
  con cualquier otro (`TEMPORARY`, …) reconectar con espera creciente.
- **Cursor**: guardarlo en Room al recibir cada mensaje (después de mostrar sus avisos). Sin cursor guardado (primera
  vez), `after=0`: llega lo del día comercial en curso que le corresponde, **solo desde que el mesero entró en este
  dispositivo**. Es el último aviso que Go revisó para el dispositivo, le correspondan o no. No calcularlo con los `id`
  de los avisos. Si la base volvió atrás (respaldo restaurado), Go lee desde el inicio del día: el cursor puede bajar.
- **`key` repetida**: descartarla. Un aviso puede volver a llegar (relectura desde un cursor anterior, dos procesos,
  reintento, respaldo restaurado): guardar en Room las `key` avisadas del día y no notificar dos veces. **`id` es solo
  el cursor**: tras restaurar un respaldo el mismo número puede ser otro aviso.
- **`account_id`** es la cuenta vigente (si se combinó, la de destino). Las unidades canceladas después del «Listo» no
  vienen en `items` y un aviso sin unidades vigentes no llega. Si al tocarlo la cuenta ya no está abierta, mostrar
  «Esta cuenta ya no está abierta».
- **`active: false`**: no hay avisos (no es un error). Detener el servicio en primer plano; volver a preguntar al
  entrar un mesero con PIN con conexión o al abrir la app (A2-D8). Pasa con el aviso apagado en Terminales o si ningún
  mesero tiene en el dispositivo una sesión con PIN creada hoy (desde el inicio del día comercial) o una de antes,
  vigente y usada en los últimos 30 minutos (A2-D12). **Puede pasar a `false` sola durante la noche**, sin que nadie
  haga nada, cuando la última sesión de antes del inicio del día cumple 30 minutos sin usarse: el canal lo manda en
  cuanto cambia.
- **`day_ends_at`**: programar ahí una nueva lectura (no detener a ciegas): si sigue `active` (un mesero con sesión
  vigente que se sigue usando), continuar; si no, detener. Al cambiar de día, borrar las `key` avisadas del día anterior (Go ya no
  entrega avisos de ese día; lo que no llegó antes de esa hora se pierde, regla del día comercial). Conservar el
  cursor.
- **`settings`**: `sound`/`vibration` eligen el canal «Platillos listos» (A2-D7); `enabled: false` llega con
  `active: false`. Pueden cambiar en cualquier mensaje. También están en `terminal.ready_alerts_enabled`,
  `ready_alerts_sound` y `ready_alerts_vibration` de `getIdentity`/`getRecoveryState`.
- **Texto** (A2-D10, lo arma la app): «Mesa 4 · 2 platillos listos» / «2 × Filete · Grande»; para llevar, con el folio
  y `customer_name`. Un aviso es lo que cocina marcó «Listo» en esa cuenta de una vez; varios de la misma cuenta se
  agrupan en la notificación. Al tocarlo: abrir `account_id` y, si el dispositivo está en «¿Quién eres?», pedir el PIN
  (`waiter_user_id` es el mesero del aviso).
- **Errores**:
  - **401 `DEVICE_REVOKED`** al abrir o leer, o `interrupted` con `DEVICE_REVOKED`: detener el servicio y hacer lo de
    siempre (credencial inválida). 401 `DEVICE_REQUIRED`: igual que en las demás rutas.
  - 403 `READY_ALERTS_DEVICE_NOT_ALLOWED`: no es un dispositivo `android` (no debería pasar).
  - 400: cursor inválido o un parámetro de más. 426: actualizar la app.
  - Sin red, 5xx o `interrupted` con otro código: espera creciente y reconectar desde el cursor; lo de mientras llega
    al volver (y `key` evita repetirlo).
  - Ya no hay 429 en este canal.
- **A quién llega** (lo decide Go; para entender lo que ve el mesero): el mesero «está» en el dispositivo si tiene en
  él una sesión con PIN creada desde el inicio del día comercial, o una de antes que sigue vigente y se usó (alguna
  petición con esa sesión) en los últimos 30 minutos (A2-D12). Al «Listo», el aviso se fija al dispositivo de su
  sesión vigente más reciente que cuente **y pueda recibirlo** (android, «Avisar platillos listos» activado y, si Go
  conoce la versión, app 0.2.0 o posterior); si ninguno sirve, va a todos los dispositivos donde está.
- **Cerrar sesión**: llamar `POST /api/v1/auth/logout` con la sesión del mesero al cerrar su sesión en la app (A2-D4):
  así los «Listo» siguientes ya no se fijan a ese dispositivo; van a su otra sesión vigente o, si no tiene ninguna, a
  todos los dispositivos donde entró hoy (este incluido, hasta el fin del día comercial).
- **Entrada sin conexión**: no crea sesión en Go; ese mesero no cuenta en el dispositivo hasta su primera entrada con
  conexión.

### Para la guía (`docs/ANDROID_INSTALAR.md`)

- **Terminales › Editar** del teléfono o tableta → «**Avisos de platillos listos**»: «Avisar platillos listos»
  (activado por omisión), «Sonido» y «Vibración» → «Guardar cambios». «Ver configuración» los muestra. Android también
  deja ajustar el canal «Platillos listos» en sus Ajustes.
- **Solo funciona si la cocina marca «Listo» en la pantalla de cocina (KDS)**. Si la cocina solo imprime comandas, no
  hay avisos.
- **A quién llega**: al mesero responsable de la cuenta (el que la abrió o a quien se transfirió), en el dispositivo
  donde tiene la sesión abierta más reciente con «Avisar platillos listos» activado; si no tiene ninguna así, en todos
  los dispositivos donde entró con su PIN durante el día (desde la «Hora de inicio del día»). Un dispositivo solo recibe
  lo que cocina terminó después de que el mesero entró en él. Transferir la cuenta cambia a quién llegan los platillos
  que aún no estaban listos.
- **Entrar sin conexión** no cuenta: el mesero recibe avisos en ese dispositivo desde su primera entrada con conexión.
- **Al terminar el día comercial** (la hora de inicio del día siguiente) dejan de contar los meseros que entraron ese
  día; el que sigue trabajando con su sesión abierta sigue recibiendo avisos, y el aviso se detiene a los 30 minutos
  de que nadie use el dispositivo. Lo que cocina terminó antes de esa hora y no llegó (por
  ejemplo, por un corte de red que cruza esa hora) ya no se avisa.
- No hace falta ningún permiso nuevo para el rol Mesero.
- Requiere la computadora principal en la versión **0.13.0**.

### Pruebas ejecutadas

| Qué                                                                                                                                                                                                                                                                                                                                                                         | Dónde                                 | Resultado                                                                                                                                                                     |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestStageA2ReadyAlerts` (8 subpruebas, `-v`: credencial y tipo; destinatario, agrupación y sin precios; transferir; varias sesiones, ninguna vigente y `auth/logout`; `after` recupera y nada se repite, con «Listo» por unidad y del pedido completo; canal SSE con `Last-Event-ID` y 429 del segundo canal; apagado, sonido y vibración; día comercial, poda y revocado) | `./internal/operations`, base `_s_`   | **PASS** las 8 (34 s)                                                                                                                                                         |
| `TestStageA2Upgrade` (`-v`), con las secciones C y D                                                                                                                                                                                                                                                                                                                        | `./internal/database`                 | **PASS**                                                                                                                                                                      |
| `TestContractV1` (`-v`) con `contractReadyAlerts`                                                                                                                                                                                                                                                                                                                           | `./internal/apicontract`              | **PASS**; 39 operaciones (antes 37)                                                                                                                                           |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                                                                                                                                                                                                                                                                                              | `iso.sh go-unit`                      | **aprobado**                                                                                                                                                                  |
| `go-int` completos: `administration`, `operations`, `apicontract`                                                                                                                                                                                                                                                                                                           | `iso.sh go-int`                       | **ok** (230 s, 465 s, 39 s)                                                                                                                                                   |
| `go-int ./internal/database`                                                                                                                                                                                                                                                                                                                                                | idem                                  | 1.ª corrida: **falló `TestStage9HUpgrade`** (su conteo de filas por tabla no excluía la tabla nueva `ready_notices`); corregido como la A1 con las suyas; 2.ª corrida: **ok** |
| Vitest completo, con `stageA2Terminals.test.tsx` (9 casos: 5 de la parte C y 4 de la D)                                                                                                                                                                                                                                                                                     | `iso.sh vitest`                       | **97 archivos, 925 pruebas aprobadas**                                                                                                                                        |
| `build` y `cargo test`                                                                                                                                                                                                                                                                                                                                                      | `iso.sh build`, `iso.sh cargo`        | **ok** (cargo: 107 aprobadas, 3 ignoradas)                                                                                                                                    |
| Playwright Chromium: `workflow-stagea2-terminals` (con los avisos), `workflow-stagea1-pin`, `workflow-stage9g-terminals` y `workflow-zz-test-data`                                                                                                                                                                                                                          | `iso.sh browser chromium` (8211/1431) | **5 aprobadas**                                                                                                                                                               |
| Playwright WebKit: `workflow-stagea2-terminals` y `workflow-stagea1-pin`                                                                                                                                                                                                                                                                                                    | `iso.sh browser webkit` (8211/1431)   | **2 aprobadas**                                                                                                                                                               |
| `format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                                                                                                                                                                                                                                                                                                              | árbol `srv`                           | Correctos; idénticos                                                                                                                                                          |

### No verificado

- La aplicación Android contra estas rutas (servicio en primer plano, notificación, dedup en Room) y la Fase E en los
  emuladores (pantalla apagada, app cerrada, corte de red).
- El canal con muchos dispositivos a la vez: cada uno lee cada 750 ms con el lock 7820148305 compartido (igual que el
  canal de eventos); no se midió con carga.

### Correcciones de la revisión

Hallazgos R1–R12 de la revisión independiente («Parte 1 · Servidor» de `$SP/doc-rev.md`), con el protocolo de
`ready-alerts` acordado con el agente de la Fase D de Android. Rama `etapa-a2-srv`: `git merge etapa-a2-android` (avance
rápido de `084f46a`, la corrección de la prueba de Terminales, a `394cab6`: solo Android y documentación), después
`git cherry-pick 712e869` (la prueba de la revisión, `82bf1a7`) y encima `70d3165` (0026, servidor, contrato y
pruebas) y `e3a3a88` (`docs/API_V1.md` y `API_CONTRACTS.md`). La `0026` se corrigió en su lugar (no se había
entregado); no hay migración nueva. El escritorio no cambió.

| #   | Qué cambió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1  | `recordReadyNotices` registra también la unidad que queda con todas sus tareas vivas listas porque **en esta transacción se canceló la que faltaba** (una tarea cancelada ahora sin haber estado lista: `state='cancelled'`, `ready_at IS NULL`, `updated_at=now()`). Revisé los tres caminos que escriben tareas: `states.go` (KDS), `changes.go` (modificaciones, cancelaciones y sus reintentos) y `releases.go` (solo crea pendientes); todos pasan por `publishPreparation`. Quitar una parte que ya estaba lista no vuelve a avisar (la cancelación conserva su `ready_at`). |
| R2  | «Entró en el dispositivo» = la regla de `active`: sesión con PIN en D **vigente o creada desde el inicio del día comercial** (con A2-D12, la vigente de antes del inicio del día solo si se usó en los últimos 30 minutos; ver abajo). Un mesero que entró antes de la hora de inicio y sigue trabajando con la sesión vigente recibe sus avisos.                                                                                                                                                                                                                                  |
| R3  | Columna `ready_notices.alert_key uuid NOT NULL` (`gen_random_uuid()` por cuenta y operación, compartida por las filas del grupo) y campo **`key`** en cada aviso: el dispositivo descarta repetidos por `key`; `id` es solo el cursor. Se conserva la decisión 6.                                                                                                                                                                                                                                                                                                                  |
| R4  | El canal revisa cada 750 ms pero **solo escribe** al abrir y con avisos nuevos o un cambio de `active`/`settings`/`day_ends_at`; en silencio `: ping` cada 15 s; dura 5 minutos. `authenticateDevice` actualiza `last_seen_at` de `GET ready-alerts` como mucho una vez por minuto (Terminales considera en línea lo de los últimos 2).                                                                                                                                                                                                                                            |
| R5  | Índices parciales en la `0026`: `sessions_pin_device_created_idx (device_id, created_at) WHERE opened_with_pin` y `sessions_pin_device_open_idx (device_id) WHERE opened_with_pin AND revoked_at IS NULL`; `active` son dos `EXISTS` y la presencia un `UNION ALL` de dos consultas, cada una con su índice. Medición abajo.                                                                                                                                                                                                                                                       |
| R6  | `event: interrupted` **sin `id`** y con `data: {"code": "…"}`: el código del rechazo si es 4xx (`DEVICE_REVOKED`, …) o `TEMPORARY` (5xx, la base no respondió a tiempo). Solo `DEVICE_REVOKED` significa retirado.                                                                                                                                                                                                                                                                                                                                                                 |
| R7  | El dispositivo fijado al «Listo» se elige solo entre `android` no revocados, con `ready_alerts_enabled` y, si `app_version` es una versión, 0.2.0 o posterior; si ninguno sirve, `recipient_device_id` nulo (para todos los dispositivos donde el mesero está).                                                                                                                                                                                                                                                                                                                    |
| R8  | `dispatch` toma 7820148305 compartido **antes** de `authenticateDevice` en `GET ready-alerts` (el orden de los escritores: lock y después la fila del dispositivo); además la fila casi nunca se toca (R4).                                                                                                                                                                                                                                                                                                                                                                        |
| R9  | Se queda como regla del día comercial y está en la guía (`docs/API_V1.md`, `day_ends_at`) y en «Para la guía»: lo marcado «Listo» antes de la hora de inicio que no llegó ya no se avisa después de esa hora.                                                                                                                                                                                                                                                                                                                                                                      |
| R10 | Al leer, `account_id` sigue `merged_into_id` hasta la cuenta vigente (recursivo, hasta 20 saltos) y `items` solo trae unidades activas; un aviso sin unidades activas no se manda pero el cursor avanza. Una cuenta cerrada o cancelada no se filtra: al tocar el aviso, la app muestra «Esta cuenta ya no está abierta».                                                                                                                                                                                                                                                          |
| R11 | Solo llegan los avisos con `ready_at` **desde la primera sesión con PIN que cuente** del mesero en D: un dispositivo nuevo o una lectura sin cursor no recibe lo de antes de que el mesero entrara en él.                                                                                                                                                                                                                                                                                                                                                                          |
| R12 | Un canal nuevo del mismo dispositivo **reemplaza** al anterior (`readyStreams` guarda el canal abierto y el nuevo cierra el viejo); ya no hay 429 `READY_ALERTS_CONNECTION_LIMIT` (se quitó del contrato y de la guía).                                                                                                                                                                                                                                                                                                                                                            |

**R5, medición** (Go temporal `iso.sh server-up` con 8211/8541, base desechable; 10 dispositivos android con 58 000
sesiones con PIN cada uno, 580 000 en total, una cada 9 minutos durante un año, todas cerradas salvo la última de cada
mesero; `EXPLAIN (ANALYZE, BUFFERS)` de las dos consultas de sesiones de cada lectura, segunda corrida; servidor
detenido y base borrada con `server-down`; scripts `$SP/srv-tools/r5-seed.sql` y `r5-explain.sql`):

| Consulta (un dispositivo)                                                 | Sin los índices parciales                                                  | Con los índices de la `0026`                     |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------ |
| `active`, con un mesero que entró hoy                                     | 0,04 ms (recorrido secuencial que acertó pronto: depende del orden físico) | 0,02 ms (`Index Only Scan` del índice por fecha) |
| `active`, nadie entró hoy pero hay una sesión vigente                     | 4,6 ms (58 000 filas descartadas)                                          | 0,01 ms                                          |
| Presencia (`readyAlertsQuery`), con un mesero que entró hoy (58 sesiones) | 4,9 ms (57 942 filas descartadas)                                          | 0,07 ms                                          |
| Presencia, nadie entró hoy                                                | 4,6 ms (58 000 filas descartadas)                                          | 0,03 ms                                          |

**Pruebas de las correcciones**

| Qué                                                                                                                                                                             | Dónde                               | Resultado                                    |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- | -------------------------------------------- |
| `TestStageA2ReadyReview` (`-v`, ajustada al protocolo: R1 se avisa, R2 llega, R3 `key` distinta, R4 sin mensajes en silencio e `interrupted` sin `id` con `DEVICE_REVOKED`)     | `./internal/operations`, base `_s_` | **PASS** las 4 (27,9 s)                      |
| `TestStageA2ReadyAlerts` (`-v`, 9 subpruebas: las 8 de la parte D ajustadas, con el reemplazo del canal y `key`, y la nueva de las correcciones: R11, R7, R10 y `last_seen_at`) | idem                                | **PASS** las 9 (28,9 s)                      |
| `TestContractV1` y `TestDocumentedInAPIV1` (`-v`; `contractReadyAlerts` exige `key`)                                                                                            | `./internal/apicontract`            | **PASS** (32,9 s)                            |
| `TestStageA2Upgrade` (con `alert_key` y los dos índices parciales) y `TestStage9HUpgrade` (`-v`)                                                                                | `./internal/database`               | **PASS**                                     |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)                                                                                                  | `iso.sh go-unit`                    | **aprobado**                                 |
| `go-int` completos: `database`, `administration`, `operations`, `kitchen`, `apicontract`                                                                                        | `iso.sh go-int`                     | **ok** los 5 (14 s, 225 s, 467 s, 6 s, 38 s) |
| `format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                                                                                                                  | árbol `srv`                         | Correctos; idénticos                         |
| Playwright                                                                                                                                                                      | —                                   | No se corrió: el escritorio no cambió        |

**No verificado (además de lo de arriba)**: la aplicación Android con el protocolo nuevo (la Fase D de Android); el
consumo real de batería y datos del canal con `: ping` (R4 midió el tráfico de antes; con el canal nuevo, en silencio
son 2 líneas cada 15 s y una reconexión cada 5 minutos); la restauración real de un respaldo (R3 se reproduce
simulándola en `ready_notices`, como la revisión).

#### A2-D12 · Sesiones de antes del inicio del día (revisión, parte 2: RD2)

Con la regla de R2 («vigente o creada en el día»), la última entrada de la noche (una sesión con PIN dura 12 h y la app
no la cierra al cambiar de mesero ni al bloquearse) mantenía `active: true` hasta la mañana y el servicio de las
tabletas no se detenía nunca. Decisión del coordinador **A2-D12**: una sesión con PIN creada **antes** del inicio del
día comercial en curso solo cuenta si sigue vigente **y** su `last_seen_at` es de los últimos **30 minutos**; las
creadas desde el inicio del día cuentan como antes. Commit `b91ae66` (encima de `e3a3a88`; la `0026` solo cambió en un comentario).

- **Constante** `kitchen.ReadyAlertsRecentUse = 30 * time.Minute` (`kitchen/preparation.go`, comentada), la misma para
  las tres reglas: `active` y la presencia («entró en el dispositivo», `readyPresenceSQL`) en
  `administration/ready_alerts.go`, y la elección del dispositivo fijado en `recordReadyNotices`, que ahora lee el día
  comercial (`businessday`) para no fijar un aviso a un dispositivo que no lo entregaría. Se pasa a SQL en segundos
  (`make_interval(secs => …)`); los índices parciales de R5 siguen sirviendo (el filtro nuevo va sobre las pocas
  sesiones abiertas del dispositivo).
- `last_seen_at` de la sesión lo actualiza cada petición con esa sesión (`authenticateSession`); con la tableta en
  «¿Quién eres?» deja de moverse y a los 30 minutos `active` pasa a `false` sola (el canal lo manda en cuanto cambia).
  **Depende de que la app no use la sesión del mesero mientras nadie trabaja**: cada lectura de `GET events` (cada
  750 ms) también mueve `last_seen_at`. Según su comentario, `RealtimeClient` solo lo abre en primer plano y con
  sesión; si en «¿Quién eres?» con la pantalla encendida sigue usando la sesión del último mesero, el servicio no se
  detendría. Lo debe confirmar el agente de Android (no lo revisé más allá de ese comentario).
- Un aviso fijado a un dispositivo cuya sesión de antes del inicio del día deja de contar antes de que lo lea (el
  dispositivo pasó más de 30 minutos sin red y el mesero no volvió a usar la sesión) ya no llega: es el mismo caso que
  un mesero que dejó de trabajar.
- **Pruebas**: `TestStageA2ReadyReview` R2 (sesión de antes del inicio usada hace 10 minutos → fijado a la tableta y
  llega; usada hace 31 minutos → `active: false`, el aviso queda para todos y no llega; al volver a entrar, `active`
  vuelve y lo de antes de su entrada no llega) y `TestStageA2ReadyAlerts` «día comercial, sesiones de antes del inicio
  del día (A2-D12), poda y dispositivo revocado» (31 minutos → `active: false`; 5 minutos → `true`). Documentación:
  `docs/API_V1.md` (a quién llega, `active`, `day_ends_at`), `openapi.json` (`getReadyAlerts` y
  `ReadyAlertsBatch.active`), `API_CONTRACTS.md` y la «Forma final» de arriba.

| Qué                                                                                         | Dónde                               | Resultado                            |
| ------------------------------------------------------------------------------------------- | ----------------------------------- | ------------------------------------ |
| `TestStageA2ReadyReview` y `TestStageA2ReadyAlerts` (`-v`)                                  | `./internal/operations`, base `_s_` | **PASS** 4/4 (27,3 s) y 9/9 (26,5 s) |
| `go-unit` (vet normal e integración, `test -race`, `GOOS=windows` build y vet)              | `iso.sh go-unit`                    | **aprobado**                         |
| `go-int` completos: `administration`, `apicontract`, `kitchen`; `TestStageA2Upgrade` (`-v`) | `iso.sh go-int`                     | **ok** (215 s, 40 s, 5 s); **PASS**  |
| `format`, `fmt-check`, `sync-check`; md5 de `docs/etapas/*.md`                              | árbol `srv`                         | Correctos; idénticos                 |

### Decisiones del coordinador sobre la parte D

Se aceptan las nueve decisiones de la parte; la 1 (guardar `recipient_device_id` en el momento del «Listo») precisa
A2-D4: el dispositivo se elige cuando el platillo queda listo, para que un mesero que cambia de tableta en ese momento o
una tableta que se reconecta tarde no pierda ni repita el aviso; que el mesero entró en ese dispositivo en el día se
revisa al leer. La 8 aplica A2-D10: `product_name` lleva la presentación con el formato de la comanda.

**Verificación sobre una copia exacta de `a2c2812`**: `fmt-check`, `sync-check`, `go-unit`, `go-int` de `apicontract`,
`database`, `administration` y `kitchen` completos, `TestStageA2ReadyAlerts` (`operations`), Vitest (97 archivos, 925
pruebas) y `npm run build` aprobados; **Playwright Chromium `workflow-stagea2-terminals` sola falló**: tras apagar
«Sonido», `ready-alerts` todavía decía `sound: true`. La causa era de la prueba: esperaba el aviso «Terminal
actualizada.» y el del guardado anterior seguía en pantalla, así que consultaba Go antes de que terminara el guardado
nuevo (con otras especificaciones antes, el tiempo cambiaba y pasaba). Corrección del coordinador en `084f46a`: los
pasos 3 y 4 esperan a que Go tenga el cambio (`expect.poll`). Repetida sola sobre una copia exacta de `084f46a`:
Chromium 3 de 3 y WebKit 1 de 1.

## Fase C · Imprimir la precuenta

Árbol `$SP/wt/c`, rama `etapa-a2-c` desde `etapa-a2-android` (`706dd71`: fases A y B y partes B y C del servidor), un
agente, 18/11/2026, en paralelo con el agente de la Fase D (avisos, en `app/` y `core:*/ready`). Aplica §5 de la
especificación con §0-B, A2-D3 y A2-D6, y usa tal cual la «Forma final para el agente de Android» de la parte C del
servidor. **El servidor no cambió en esta fase** y **el esquema de Room tampoco** (`AppDatabase` sigue en v6, sin
tablas nuevas; `catalog.db` igual). Pruebas en copias aisladas (`ISO_TAG=C`): Go temporal en 8231/8491 (modo de
pruebas, base `adel_pos_test_c_*` del clúster 55499, uno nuevo por corrida) y los emuladores `Pixel_8`
(`emulator-5580`, Android 17) y `ADEL_A1_Tablet_API26` (`emulator-5582`, Android 8.0), de solo lectura y uno a la vez.

**Resultado.** En la cuenta de una mesa y en un pedido para llevar (la misma pieza de `core:capture`), al final de la
cuenta, el mesero tiene **«Imprimir precuenta»**, «Imprimir ticket de venta» o «Imprimir cuenta» según el nombre
visible de Administración › Documentos (`establishment.precheck_name`, guardado para verse sin conexión), solo con
`payments.tickets.print` en sus permisos de Go y con la cuenta ya en Go. Sale **solo** en la impresora asignada a ese
dispositivo (A2-D3; la decide Go). Sin conexión el botón no se puede tocar y dice «Para imprimir necesitas conexión con
la computadora principal»; imprimir **nunca** entra en la cola. Las respuestas se dicen en palabras del restaurante
(«Enviada a Caja 2», «La Caja 2 está desconectada», «La impresora no responde», «Este dispositivo no tiene impresora
asignada» con el aviso de pedírsela al administrador…) y el trabajo se sigue unos 15 s sin bloquear la captura;
«Reintentar» sin motivo solo si nunca salió en papel (9F). La aplicación exige además `device-precheck-printer-v1`
(A2-D6). Comprobado de punta a punta contra el Go temporal en los dos emuladores, con los **bytes ESC/POS** que recibe
el agente de impresión simulado de una terminal «Caja 2».

### Qué se hizo

| Commit    | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `01c53aa` | **Dominio y datos.** `core:domain/waiter/Precheck.kt`: `PrecheckName` (la lista cerrada de Go; una clave desconocida es «Precuenta», `Precheck.kt:20`), `PrecheckPrinter` (a dónde va: la terminal de la USB o la impresora), `PrecheckSetup` (lo último que se leyó), `PrintAnswer` (lo que respondió Go), `PrintMessage` (lo que se dice), `PrecheckPrintState`, la interfaz `PrecheckService` (`:179`) y las reglas puras `PrecheckFeedback` (`:213`: estado y problema del trabajo → aviso, `canRetry`, `isFinal`, rechazos, «La Caja 2» / «La terminal Mostrador» como el escritorio). `PrecheckPrinting.kt`: pide, sigue con `getDocumentStatus` cada 2,5 s durante 15 s como máximo (`follow`, `:141`) y reintenta. `WaiterAction.PRINT_PRECHECK` (`payments.tickets.print`). `ServerCompatibility.REQUIRED_CAPABILITIES` con `device-precheck-printer-v1` (`ServerCompatibility.kt:65`); los comentarios que listaban capacidades ahora remiten a esa lista. Datos: `WaiterApi.identity`, `printPrecheck`, `documentStatus`, `retryDocument` y `PrecheckRepository` (`core:data/waiter/PrecheckRepository.kt:108`) con la **sesión** del mesero: sin sesión, sin red o sin respuesta, `NoConnection` sin encolar nada; una respuesta perdida se repite una vez con la misma `operation_id`; los errores de acceso van a `report`. El nombre visible y la impresora se leen de `getIdentity` (al entrar un mesero con conexión, al abrir una cuenta y después de imprimir) y se guardan **antes** de mostrarse en `restaurant_snapshot`, tipo `precheck` (`:296`). Pruebas `PrecheckRulesTest`, `ServerCompatibilityTest` (+1) y `PrecheckRepositoryTest`; el Go simulado (`FakeGoServer`) anuncia la capacidad. |
| `7b4903f` | **Pantalla.** `core:capture/Precheck.kt`: `PrecheckSection` y `PrecheckPrintBar` (`:53`), al final de `OrderPanel` (botón secundario: el principal sigue siendo «Enviar a cocina»); antes de tocar, «Se imprime en Caja 2.» o «Este dispositivo todavía no tiene impresora asignada.»; después, el aviso con su segunda línea y «Reintentar» si corresponde; sin conexión, el botón deshabilitado con «Para imprimir necesitas conexión con la computadora principal.». `CaptureViewModel`: `precheckSetup`, `precheckState`, `canPrintNow` (`:223`: sesión de Go, sin «Vuelve a escribir tu PIN» y red), `onPrintPrecheck`/`onRetryPrecheck`, lectura de `getIdentity` al abrir la cuenta y `reset` al cambiar de cuenta. `CaptureScreen` muestra la sección solo con la cuenta ya en Go y `grants(PRINT_PRECHECK)` (`CaptureScreen.kt:144`). Textos en `core/capture/src/main/res/values/strings.xml`. `CaptureDependencies.precheck`, `WaiterModeContainer.precheck` (con `linkState`) y una línea en `AppContainer`. Prueba `WaiterViewModelsTest.capturaImprimeLaPrecuentaYSigueElTrabajo`; `FakeWaiter` y `TakeawayViewModelTest` con el servicio nuevo. `feature:takeaway` no cambió: usa la captura compartida.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `9b9ca6c` | **Pruebas en el dispositivo.** `PrecheckScreensTest` (Compose con estados fijos, en ambos temas: el botón con cada nombre visible, a dónde va, cada aviso con su texto exacto y su segunda línea, «Reintentar» solo si nunca salió, sin conexión). Contra el Go temporal, `PrecheckOnGoTest` (`p1`–`p7`) con la suite nueva **`precheck`** de `scripts/android-e2e.sh` (`precheck_step`; `p6` con la red cortada por el guion; también en `--suite all`). La prueba hace de computadora principal y del **agente de impresión** de una terminal de prueba «Caja 2» inscrita por la escucha local (`devices/invitations` + `devices/enroll`), que informa su impresora USB (`print-agent/printers`) y reclama los trabajos (`print-agent/jobs/claim`, CP850).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `0cd678a` | **Corrección de la Fase B encontrada aquí** (`TakeawayBoard.entries`, `TakeawayBoard.kt:107`): al «Enviar a cocina» sin conexión un pedido para llevar, la lista podía leer la cola nueva antes de que se fuera la captura (mismo identificador, A1-D19) y Compose cerraba la aplicación por una clave repetida («Key … was already used»). Ahora manda el pedido de la cola. Prueba `TakeawayRulesTest.laCapturaYSuPedidoDeLaColaNuncaSeVenDosVeces`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `60d6228` | **Documentación.** `docs/ANDROID_INSTALAR.md`: sección nueva **7 «Imprimir la precuenta»** (el botón con el nombre visible, «Atendió:», que requiere conexión, la tabla de lo que dice el dispositivo y qué hacer, y «Impresora de precuentas por dispositivo»: Administración › Sistema › Terminales › Editar › «Impresora de precuentas» › «Guardar cambios»); en la sección 4, «Imprimir tickets y precuentas» para el rol Mesero; las secciones siguientes pasan a 8–10 con sus referencias; la 10 ya no dice que la precuenta llega después (solo el aviso). `android/CLAUDE.md`: sección «Precuenta desde el dispositivo (etapa A2, Fase C, para los demás módulos)», versiones con la capacidad nueva, la suite `precheck` en `--suite all` y Room sin cambio.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

### Cómo funciona (para las fases siguientes)

- **Cuándo aparece el botón**: `capture.accountId != null` (la cuenta ya está en Go) y
  `permissions.grants(WaiterAction.PRINT_PRECHECK)` (permiso de Go **conocido**, como «Para llevar»). Sin el permiso
  no aparece; Go responde 403 de todos modos.
- **Se puede tocar** (`canPrintNow`): el mesero tiene sesión de Go, no debe volver a escribir su PIN y el dispositivo
  tiene red (`ConnectionMonitor.linkState`, el del indicador). Si no, el botón está deshabilitado y dice «Para imprimir
  necesitas conexión con la computadora principal.». Nada se guarda para después.
- **Respuesta de `printPrecheck`** (200): `problem == agent_offline` → «La Caja 2 está desconectada.» (sin
  «Reintentar»: sale sola al volver); si no → «Enviada a <terminal o impresora>» («Enviado» con «Ticket de venta»).
  409 `PRINTER_NOT_CONFIGURED` → «Este dispositivo no tiene impresora asignada.» + «Pídele al administrador que la elija
  en Administración › Sistema › Terminales.»; `PRINTER_UNAVAILABLE` → el mensaje de Go + pedir que la revise; 403 →
  «No tienes permiso para imprimir.»; otro → el mensaje de Go.
- **Seguimiento** (`getDocumentStatus`, en el `viewModelScope` de la captura): `submitted` termina («Enviada a…»);
  `unreachable` → «La impresora no responde.» con «Reintentar»; `not_found`/`setup`/`inactive`/`unsupported` → «La
  impresora necesita revisión: avisa al administrador.» (con «Reintentar» si el estado es `error`/`intervention`);
  `uncertain` → «No se sabe si salió en papel. Revisa la impresora antes de imprimir otra vez.» (nunca «Reintentar»);
  `cancelled` → «Se canceló la impresión…». Pendiente o en proceso se sigue consultando hasta 15 s. Una consulta sin
  respuesta deja lo último que se supo.
- **Reintentar** (`retryDocument` con el `ticket_id` y una clave nueva, sin motivo): solo con `error`/`intervention`,
  nunca con `uncertain` ni con la terminal desconectada. `PRINT_NOT_FAILED`, `PRINT_UNCERTAIN` y
  `PRINT_JOB_SUPERSEDED` vuelven a leer el estado y, si sigue en camino, lo siguen. Volver a imprimir algo que ya salió
  es tocar el botón otra vez: **otra precuenta** con otra clave, como en la caja (la regla que dejó la parte C).

### Decisiones propuestas (con su porqué)

Ninguna cambia una regla de negocio ni la especificación; son detalles que §5, A2-D3 y la parte C no fijaban.

1. **El nombre visible y la impresora se guardan en `restaurant_snapshot` (tipo `precheck`)**, no en
   `device_preferences` ni en una tabla nueva: el encargo prohíbe tocar el esquema (la Fase D agrega sus tablas a la v6)
   y ese registro ya lo borra `WaiterDao.eraseAll` (registrado en `deviceDataWipe`), mientras que `device_preferences`
   no se borra al retirar el dispositivo.
2. **Se leen de `getIdentity` al entrar un mesero con conexión, al abrir cada cuenta y después de cada impresión**
   (aparte, sin hacer esperar la respuesta). Go no avisa por el canal de eventos del dispositivo cuando cambian el
   nombre visible o la impresora asignada (son del ámbito de configuración), y una lectura al abrir la cuenta es barata
   y basta para que el botón diga el nombre vigente. La respuesta de Go manda siempre.
3. **Antes de tocar se dice a dónde va** («Se imprime en Caja 2.») o que no hay impresora («Este dispositivo todavía
   no tiene impresora asignada.»), pero el botón **no** se deshabilita por eso: la impresora pudo asignarse después de
   la última lectura y Go decide.
4. **«Sin conexión» es sin sesión de Go o sin red** (el mismo monitor del indicador): con cualquiera de los dos no se
   intenta nada. Con sesión y red pero sin respuesta se dice lo mismo después de tocar.
5. **Sin respuesta, una repetición con la misma `operation_id`** (como «Entregado» de la Fase B): si Go la recibió, no
   imprime dos; si no, la imprime una vez.
6. **El seguimiento vive en la pantalla de la cuenta** (`viewModelScope`): termina al salir de la cuenta (o al
   cambiar de cuenta, al seguir una combinada) y una impresión nueva cancela el seguimiento de la anterior. Un toque
   mientras la petición va en camino se ignora. Así se cumple «mientras el mesero está en la cuenta» sin servicios ni
   trabajos en segundo plano.
7. **«Reintentar» no se ofrece con la terminal desconectada** (`agent_offline`), aunque el estado sea `error`: la
   precuenta sale sola cuando la terminal vuelve (el mismo criterio del escritorio, «no hace falta reintentar»).
8. **Concordancia de las frases**: «Enviado a…» con «Ticket de venta» y «Enviada a…» con «Precuenta» y «Cuenta»; «La
   Caja 2 está desconectada» si el nombre de la terminal empieza con caja, terminal o computadora y, si no, «La
   terminal Mostrador está desconectada» (la regla de `desktop/src/services/printProblems.ts`).
9. **Botón secundario al final de la cuenta**, después de «Enviar a cocina» (regla 12: un solo botón principal por
   panel), sin ícono de impresora (agregarlo tocaba `core:ui`, fuera del alcance de la fase).
10. **«Atendió:»** es quien imprime (sin cambios, como dejó la parte C): en las pruebas sale el mesero del dispositivo.

### Pruebas

Todo en el árbol `$SP/wt/c` con el arnés (`ISO_TAG=C`), JDK de Android Studio. De punta a punta, contra el Go temporal
construido desde este mismo árbol (servidor 0.13.0 con la parte C: anuncia `device-precheck-printer-v1`), uno nuevo
para cada corrida (`server-down` + `server-up`, 8231/8491). Antes de cada corrida se comprobó que nada escuchara en el
puerto 9106 de la Mac (la impresora de red «que no responde»). Un emulador a la vez.

| Qué                                                                                    | Dónde                                                                 | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `:core:domain:test`                                                                    | Mac                                                                   | **128 pruebas, 0 fallas** (20 XML): las 113 de antes, `PrecheckRulesTest` (13: nombre visible, a dónde va, cada estado y problema del trabajo → su aviso, cada rechazo → su aviso, cuándo se ofrece «Reintentar», qué estados ya no se siguen, el permiso, y el seguimiento con tiempo virtual: enviada y aceptada, terminal desconectada que vuelve, 15 s como máximo sin bloquear, no responde → «Reintentar» la misma precuenta, «Reintentar» que ya no hacía falta, sin impresora / sin permiso / sin conexión sin seguir nada, un doble toque no imprime dos y otra cuenta olvida lo anterior), `ServerCompatibilityTest.servidorSinImpresoraDePrecuentasPorDispositivo` y `TakeawayRulesTest.laCapturaYSuPedidoDeLaColaNuncaSeVenDosVeces` |
| `testDebugUnitTest` (JVM + Robolectric SDK 36)                                         | Mac                                                                   | **173 pruebas, 0 fallas**: `core:data` 146 (con `PrecheckRepositoryTest`, 5: nombre visible e impresora leídos con la sesión, guardados y vistos sin conexión y borrados con `eraseAll`; imprimir con la sesión —nunca la credencial de envío— y la misma clave si se pierde la respuesta; `PRINTER_NOT_CONFIGURED`, `PRINTER_UNAVAILABLE`, 403 y la sesión terminada a `report`; sin red, sin sesión o sin servidor no se pide nada y nada queda en la cola; estado del trabajo y «Reintentar» sin motivo con `PRINT_NOT_FAILED`), `core:ui` 9, `feature:access` 7, `feature:takeaway` 5, `feature:waiter` 6 (con `capturaImprimeLaPrecuentaYSigueElTrabajo`). Con el dominio, **301** (antes 280)                                              |
| `ktfmtCheck`, `lintDebug`                                                              | Mac                                                                   | Aprobados (lint sin problemas en ningún módulo)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `assembleDebug`, `assembleRelease` (sin firmar)                                        | Mac                                                                   | Correctos; `app-release-unsigned.apk` 2 487 047 bytes (antes 2 458 743). Sin dependencias nuevas: ningún `gradle.lockfile` ni `verification-metadata.xml` cambió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `connectedDebugAndroidTest` (antes: desinstalar la app, sus `.test` y los de la 0.1.0) | `ADEL_A1_Tablet_API26` (`emulator-5582`, Android 8.0)                 | **118 pruebas, 0 fallas**, 61 omitidas (las de punta a punta sin argumentos, 7 más por `PrecheckOnGoTest`, y `ConnectionOnDeviceTest`/`WorkNetworkOnDeviceTest`, que no corren antes de Android 11): `app` 85 (59), `core:data` 20 (2), `core:ui` 13 — 3 XML contados (12 min). **La migración 5 → 6 de la Fase B corrió en Android 8 y pasó** (`AppDatabaseMigrationTest.de5a6ConservaLasCapturasYLaColaYAdmitePedidosParaLlevar`, sin omitir), como las 1 → 2 … 4 → 5 y `todasLasMigracionesLlevanALaVersionActual`                                                                                                                                                                                                                            |
| Ídem                                                                                   | `Pixel_8` (`emulator-5580`, Android 17)                               | **118 pruebas, 0 fallas**, 59 omitidas: `app` 85 (57), `core:data` 20 (2), `core:ui` 13 — 3 XML (4 min 46 s), con el código final. Una corrida anterior, igual de aprobada (118, 0 fallas), fue antes de los dos últimos cambios pequeños (seguir el trabajo después de un «Reintentar» que ya no hacía falta y la corrección de la lista de Para llevar) y se repitió                                                                                                                                                                                                                                                                                                                                                                           |
| `scripts/android-e2e.sh --suite precheck` (p1–p7)                                      | `Pixel_8`                                                             | **7 de 7 aprobados**, dos veces (la segunda con el código final), cada una con otro Go temporal (≈ 90 s). El registro `AdelOnGo` tiene el texto de cada precuenta que recibió el agente: «PRECUENTA», «TICKET DE VENTA» y «CUENTA» según el nombre visible, «Atendió: Ana A1», «Folio 1 · Mesa P1» con sus partidas, y «Folio 2 · Pedido para llevar»                                                                                                                                                                                                                                                                                                                                                                                            |
| Ídem                                                                                   | `ADEL_A1_Tablet_API26` (red cortada con `adb root` + `svc wifi/data`) | **7 de 7 aprobados** a la primera (≈ 90 s)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `scripts/android-e2e.sh --suite takeaway` (t1–t8), para revisar la captura compartida  | `ADEL_A1_Tablet_API26`                                                | Primera corrida: `t4` **falló** por el cierre de la aplicación de la Fase B (clave repetida en la lista de Para llevar, arriba) y `t5`, `t6` y `t8` fallaron porque dependen de `t4`. Con la corrección (`0cd678a`) y otro Go temporal: **8 de 8 aprobados**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Ídem                                                                                   | `Pixel_8`                                                             | **8 de 8 aprobados** con el código final                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `iso.sh format`, `fmt-check`; `md5` de `docs/etapas/*.md`                              | árbol `c`                                                             | Aprobados; idénticos antes y después                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |

Ninguna prueba usó la red real ni el servicio de la Mac: el ejecutor busca `_adelpostest._tcp`, la aplicación se
empareja por la dirección del Go temporal y ninguna lista mostró un servidor real. Al terminar se apagaron los dos
emuladores y se detuvo el Go temporal (y se borró su base).

### No verificado

- Dispositivo físico (no hay) ni papel real: el agente de impresión es el simulado de la prueba y la impresora de red
  «que no responde» es un puerto de la Mac donde nada escucha.
- `Pixel_Tablet` y `ADEL_A1_Phone_API33`: la matriz de cuatro es de la Fase E.
- La regresión completa (`--suite all` y las suites `access`, `catalog`, `waiter` y `field`) es de la Fase F; la suite
  `takeaway` sí se corrió (arriba) porque la fase toca la captura compartida.
- La corrección de la lista de Para llevar arregla una carrera de tiempos: se comprobó con la prueba de la JVM y con
  la suite `takeaway` en los dos emuladores, pero un caso que depende del tiempo no se demuestra con una corrida.

### Para el coordinador

- **Archivos compartidos con la Fase D** (cambios pequeños): `AppContainer.kt` (una línea en `captureDependencies`),
  `WaiterModeContainer.kt` (`precheck` y `precheck.start()`), `ServerCompatibility.kt` (la capacidad nueva en
  `REQUIRED_CAPABILITIES`; la D agrega `ready-alerts-v1` en la misma lista), `FakeGoServer.kt` y
  `ServerCompatibilityTest.kt` (la lista de capacidades del Go simulado), `scripts/android-e2e.sh` (suite `precheck`,
  también en `--suite all`), `docs/ANDROID_INSTALAR.md` (sección 7 nueva: las siguientes pasan a 8–10; si la D agrega
  su sección de avisos, hay que volver a numerar) y `android/CLAUDE.md`.
- **El texto de «Actualiza el servidor del restaurante»** (`feature/access/.../strings.xml`, `update_server_text`)
  sigue diciendo «le falta la entrada con PIN o Para llevar»: no lo cambié para no chocar con la Fase D (que agrega
  `ready-alerts-v1`); conviene dejarlo general al fusionar («le faltan funciones que usa esta versión…»).
- Los comentarios que listaban las capacidades (`Pairing.kt`, `DeviceAccess.kt`, `PairingRepository.kt`) ahora
  remiten a `ServerCompatibility.REQUIRED_CAPABILITIES`, para que la D no tenga que tocarlos.
- **Room**: sin cambios (v6 tal cual).
- **Corrección fuera de lo pedido** (`0cd678a`, `TakeawayBoard.entries` de la Fase B): la encontró la suite `takeaway`
  en Android 8 (que la Fase B no corrió ahí) y la apliqué porque cerraba la aplicación; son seis líneas del dominio con
  su prueba. Si la Fase D tocara `TakeawayBoard.kt`, revisar la fusión.
- **Servidor**: sin cambios; nada obligó a cambiar una regla de negocio.

### Para CLAUDE.md

- §0/encabezado: Android 0.2.0 con la precuenta desde el dispositivo (Fase C); la aplicación exige el servidor 0.13.0
  con `takeaway-v1` y `device-precheck-printer-v1`.
- Pruebas (§6): `PrecheckRulesTest`, `PrecheckRepositoryTest`, `PrecheckScreensTest` y la suite `precheck` de
  `scripts/android-e2e.sh` (`PrecheckOnGoTest`, `p1`–`p7`; también en `--suite all`).

### Decisiones del coordinador sobre la Fase C

Se aceptan las decisiones propuestas de la fase. La corrección de la Fase B que trae (`0cd678a`: la lista de Para
llevar podía mostrar a la vez una captura y su pedido de la cola con la misma clave y la app se cerraba) se queda en la
fase C; su prueba de la JVM la cubre y la Fase E repite `takeaway` en los cuatro emuladores. Fusión: `etapa-a2-c`
(`60d6228`) en `c2fe821`, sin archivos en común con la parte D del servidor; `android/`, `scripts/android-e2e.sh` y
`docs/ANDROID_INSTALAR.md` quedaron idénticos a los de la rama de la fase. Al fusionar la Fase D se generaliza el texto
de «Actualiza el servidor del restaurante» (`update_server_text`), que todavía nombra solo la entrada con PIN y Para
llevar.

## Fase D · Aviso de platillos listos en el dispositivo

Árbol `$SP/wt/d`, rama `etapa-a2-d` desde `etapa-a2-android` (`0446a81`: fases A y B y partes B, C y D del servidor),
un agente, 18/11/2026, en paralelo con el agente de la Fase C (botón de precuenta en `core:capture`). Aplica §4 de la
especificación con §0-B, A2-D4, A2-D6, A2-D7, A2-D8, A2-D9 y A2-D10 y usa tal cual la «Forma final para el agente de
Android» de la parte D del servidor y, después, el **protocolo ajustado** que pidió el coordinador tras la revisión
independiente del servidor (commit `c4de974`). **El servidor no cambió en esta fase** (no tocó `backend/`, `desktop/`,
`docs/API_V1.md`, `API_CONTRACTS.md` ni `CLAUDE.md` raíz). Pruebas en copias aisladas (`ISO_TAG=D`): Go temporal en
8241/8501 (modo de pruebas, base `adel_pos_test_d_*` del clúster 55499, uno nuevo para cada corrida) y los emuladores
`Pixel_Tablet` (`emulator-5584`, Android 17) y `ADEL_A1_Phone_API33` (`emulator-5586`, Android 13), de solo lectura y
uno a la vez.

**Resultado.** Cuando cocina marca «Listo» en el KDS, el mesero responsable recibe el aviso en su teléfono o tableta
**con la pantalla apagada, en Doze y con la aplicación cerrada** (también después de que Android mate el proceso y lo
reinicie solo), en «¿Quién eres?» y en la pantalla de bloqueo: «Mesa 4 · 2 platillos listos» / «2 × Filete de res»,
una notificación por cuenta que acumula, sin precios. No llega a otro mesero ni a otro dispositivo, transferir la
cuenta cambia el destinatario, y con un corte de red durante el «Listo» llega **una sola vez** al volver. Un servicio
en primer plano `specialUse` con la notificación permanente «AD-EL POS conectado» sostiene el único canal de avisos del
proceso con **solo la credencial del dispositivo**; arranca al entrar un mesero con conexión, al abrir la aplicación y
tras reiniciar el equipo o actualizar la aplicación, y se detiene con `active: false` (también el del cambio de día
comercial si nadie sigue con su sesión), al retirar el dispositivo o con «Emparejar de nuevo». Al tocar un aviso se
abre la cuenta (con el PIN de su mesero si está en «¿Quién eres?»; la vigente si se combinó; «Esta cuenta ya no está
abierta» si se cobró o canceló); con la cuenta a la vista, el aviso es dentro de la aplicación. Sonido y vibración salen de Terminales (canal «Platillos
listos» único, A2-D7). Los permisos se explican antes del diálogo de Android y la ayuda «Avisos de platillos listos»
dice qué falta.

### Qué se hizo

| Commit    | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ceee5b9` | **Dominio, datos y Room v6.** `core:domain/ready` (lógica pura): `ReadyAlert`/`ReadyAlertsBatch`/`ReadyAlertsStatus`, `ReadyAlertsProcessor` (por lote: canal según la configuración → quitar lo ya avisado → mostrar → guardar ids y cursor; sin escribir en Room si nada cambió), `ReadyNotice` (una notificación por cuenta con el acumulado: el mismo platillo se suma, mesa/cliente/mesero del último aviso), `ReadyAlertWords` (texto de A2-D10 con plantillas de `strings.xml`: «Mesa 4 · 2 platillos listos», «2 × Filete · Grande, 1 × Tacos», «Para llevar #12 (Juan) · 1 platillo listo»), `ReadyAlertsLifecycle` (cuándo arrancar, la nueva consulta al pasar el fin del día, qué hacer con cada error, espera creciente 2–30 s, búsqueda de otra dirección) y `ReadyChannels` (A2-D7). `core:data/ready`: `ReadyAlertsClient` (`GET ready-alerts?after=` y el canal `ready-alerts/stream` con `after` y `Last-Event-ID`; en `c4de974`, 45 s sin un byte = conexión muerta y `interrupted` con `{"code"}`), `ReadyAlertsChannel` (el único canal de avisos del proceso), `DeviceReadyAlertsServer` (solo la credencial del dispositivo), `RoomReadyAlertsLedger` y `ReadyAlertsDao`. **`AppDatabase` v6** (la misma de la Fase B): `ready_alerts_state` y `ready_alerts_seen` (`READY_ALERTS_TABLES_V6` al final de la migración 5 → 6, `6.json` regenerado). `ServerCompatibility.check` exige además **`ready-alerts-v1`** (A2-D6) y el texto «Actualiza el servidor» lo nombra. Pruebas `ReadyAlertsRulesTest` (18), `ReadyAlertsChannelTest` (11; 13 desde `c4de974`), `RoomReadyAlertsLedgerTest` (4), el caso de compatibilidad y el 5 → 6 de `AppDatabaseMigrationTest`. |
| `1d705fa` | **Servicio, notificaciones, permisos y ayuda** (`app/ready`). `ReadyAlertsService` (`specialUse` con su subtipo, `startForeground` lo primero de `onStartCommand`, `START_STICKY`), `ReadyAlertsBootReceiver` (`BOOT_COMPLETED`, `MY_PACKAGE_REPLACED` con `goAsync`), `ReadyAlertsController` (`AppContainer.readyAlerts`: arranque y parada, consulta previa, el canal, `deviceDataWipe` «avisos de platillos listos», toque en una notificación, cuenta a la vista, aviso dentro de la aplicación), `ReadyAlertNotifications` (canales, notificación por cuenta con sus avisos en los extras, resumen del grupo, notificación permanente), `ReadyAlertsUi` (explicación de los permisos, ayuda «Avisos de platillos listos», aviso dentro de la aplicación, abrir la cuenta). Manifiesto: `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_SPECIAL_USE`, `POST_NOTIFICATIONS`, `RECEIVE_BOOT_COMPLETED`, `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` (con `tools:ignore="BatteryLife"` y su porqué). Íconos de notificación vectoriales (`ic_stat_connected`, `ic_stat_ready`). `core:ui`: «Avisos de platillos listos» opcional en el menú del mesero. `feature:access`: la entrada con PIN deja elegido al mesero de un aviso tocado (`requestedWaiter`). `AdelTestRunner`: el aviso va apagado salvo `readyAlerts=on` y sus permisos se piden solo con `alertPermissions=ask`. Prueba instrumentada `ReadyAlertsOnDeviceTest` (6).                                                                                                                                                                                                                                                                |
| `ce0f36b` | **Guion**: en `r8b`, si `run-as` no puede matar el proceso (Android 13), con root (imágenes google_apis).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `7f10c5f` | **Punta a punta**: suite **`alerts`** de `scripts/android-e2e.sh` (`AlertsOnGoTest`, `r1`–`r9`, con `r4` y `r8b` hechos por el guion), también en `--suite all`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `c4de974` | **Protocolo ajustado de `ready-alerts`** (aviso del coordinador tras la revisión independiente del servidor): repetidos por la **`key`** de Go (el `id` solo es el cursor; `ready_alerts_seen` con `key` como llave en la misma v6, `6.json` regenerado), canal que solo manda cambios con `: ping` cada 15 s y cierre a los 5 minutos (un cierre normal se reabre enseguida; 45 s sin un byte = muerta), `interrupted` con `{"code"}` (solo `DEVICE_REVOKED` es retirado; otro código, otro `DEVICE_*` o 5xx, espera creciente), sin parar en `day_ends_at` (a esa hora y 30 s se vuelve a preguntar con `GET`; sigue mientras Go diga `active`), y al tocar el aviso de una cuenta que ya no está abierta: la vigente si se combinó o «Esta cuenta ya no está abierta». Pruebas, guía y `android/CLAUDE.md` al día.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `80e8ea9` | **Documentación**: `docs/ANDROID_INSTALAR.md` sección nueva **«Avisos de platillos listos»** (7 en la rama de la fase; **8** después de la fusión con la Fase C), el paso 5 de «Pasar de la 0.1.0 a la 0.2.0», la introducción, «Retirar» y «Lo que todavía no hace»; `android/CLAUDE.md`: regla **22** (un solo canal de eventos y un solo canal de avisos por proceso), sección «Aviso de platillos listos», estructura, Room v6, «Tiempo real» y la suite `alerts`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |

### Cómo funciona (resumen)

- **Arranque** (`ReadyAlertsController.request` → `ReadyAlertsLifecycle.decide` con lo guardado en Room):
  - entrada con conexión → arranca (salvo que se sepa apagado en Terminales: entonces pregunta antes);
  - abrir la aplicación con el dispositivo listo → arranca si Go dijo `active: true` y no terminó el día; si no,
    **pregunta** con `GET ready-alerts` (sin mover el cursor si está activo; moviéndolo si no, como pide la decisión 5
    de la parte D) y arranca solo si `active`;
  - `BOOT_COMPLETED`/`MY_PACKAGE_REPLACED` → solo con la condición guardada (sin red ni pantalla todavía).
    Sin destinatarios en el día no arranca: no hay notificación permanente.
- **Parada**: cuando `ReadyAlertsChannel.run` termina (`INACTIVE`, `DEVICE_NOT_READY`, `REPORTED`, `REJECTED`), cuando
  el estado del dispositivo deja de ser `Ready` y con el borrado del dispositivo. Pasar `day_ends_at` **no** lo detiene:
  a esa hora (y 30 s) vuelve a preguntar con `GET`; lo detiene el `active: false` de Go.
- **Canal** (`ReadyAlertsChannel`): desde el cursor guardado; un cierre normal de Go (a los 5 minutos, aunque solo
  haya habido `: ping`) se reabre enseguida; sin red, 5xx, `interrupted` temporal o conexión muerta (45 s sin un byte),
  espera 2, 4, 8, 16 y 30 s (`reconnectNow` al volver la red o al entrar un mesero); 429 → lee con `GET` mientras
  espera; `DEVICE_REVOKED`, otra huella o 426 → `deviceAccess.report` y se detiene; tres fallas seguidas →
  `deviceAccess.recheck()` (que busca la otra dirección), como mucho una vez por minuto; al abrir después de fallar,
  `orderQueue.retryNow()` (el canal no envía pedidos).
- **Dedupe**: Room (`ready_alerts_seen`, por la `key` de Go) + los extras de la notificación de cada cuenta (las `key`
  que contiene). Mostrar primero y guardar después (una transacción con el cursor).
- **El aviso**: canal «Platillos listos» del id de la configuración vigente; una notificación por cuenta (etiqueta =
  cuenta) que acumula mientras sigue a la vista; resumen con dos o más; pantalla de bloqueo pública (sin precios);
  al tocarla → `MainActivity` (`CLEAR_TOP`/`SINGLE_TOP`) → `pendingOpen` → en «¿Quién eres?» queda elegido su mesero
  → al entrar (él u otro que la vea en sus cuentas abiertas) se abre la captura; con la captura de esa cuenta a la
  vista y la aplicación al frente, aviso arriba de la pantalla 6 s y la notificación de esa cuenta se quita. Si la
  cuenta ya no está abierta, se pregunta a Go: combinada en otra, se abre la vigente; cobrada o cancelada, «Esta cuenta
  ya no está abierta».

### Decisiones propuestas (con su porqué)

Ninguna cambia una regla de negocio ni la especificación; son detalles que §4, A2-D4 y A2-D7 a A2-D10 no fijaban.

1. **Mostrar primero y guardar después** (`ReadyAlertsProcessor.onBatch`, `ReadyAlerts.kt:160-175`). Al revés, un
   proceso que muere entre guardar y mostrar **perdería** el aviso. Así, si muere entre mostrar y guardar, el aviso
   vuelve a llegar desde el cursor guardado y la notificación de su cuenta lo reconoce en sus extras
   (`ReadyAlertNotifications.kt:147`): no vuelve a sonar. Los ids y el cursor se guardan en **una** transacción
   (`ReadyAlertsDao.commit`).
2. **Un aviso es su `key`** (protocolo ajustado: `RoomReadyAlertsLedger.unseen`, `:32-38`). Primero lo resolví con
   número y hora (tras restaurar un respaldo Go vuelve a usar números); el protocolo nuevo da una `key` única que nunca
   se reutiliza y el `id` queda solo como cursor. Si un servidor no mandara `key`, se usa «número@hora».
3. **Los avisados de días anteriores se borran al cambiar `day_ends_at`** (filas con un fin de día anterior al nuevo,
   `ReadyAlertsEntities.kt:102-114`). Go ya no los entrega (filtra por el día comercial) y el cursor se conserva.
4. **Sin escribir en Room con cada mensaje** (Go manda uno cada 750 ms): solo si hay avisos nuevos o cambió el cursor,
   `active`, el fin del día o la configuración; el primer lote de cada arranque del canal se guarda siempre
   (`restart`).
5. **Consulta previa antes de arrancar** al abrir la aplicación si lo guardado no dice `active` (o al entrar con el
   aviso sabido apagado): un `GET ready-alerts` que guarda el estado **sin mover el cursor** si está activo (los avisos
   los entrega el servicio) y moviéndolo si no (decisión 5 de la parte D). Así «sin destinatarios no arranca» sin
   mostrar la notificación permanente ni un instante, y un aviso reactivado en Terminales se descubre al abrir.
6. **Fin del día** (protocolo ajustado): nunca se detiene por la hora: a `day_ends_at` + 30 s cierra el flujo y
   pregunta con `GET` (`ReadyAlertsChannel.kt:103-125`; `ReadyAlertsLifecycle.dayCheckIn` no pregunta en bucle si Go
   sigue dando el mismo fin de día) y sigue mientras Go diga `active: true`. Mi primera versión se detenía si pasaban
   10 minutos del fin del día sin respuesta; el coordinador decidió que sigue mientras `active` sea true. Consecuencia:
   una tableta que se quedó sin la computadora principal de noche sigue reintentando (con la espera de hasta 30 s)
   hasta que vuelva o alguien la abra.
7. **El servicio solo sostiene** el proceso y su notificación; el canal y las decisiones viven en
   `ReadyAlertsController` (uno por proceso), para que el borrado del dispositivo, el estado de la entrada y los
   disparadores puedan detenerlo sin depender de la instancia del servicio. El tipo `specialUse` se pasa a
   `startForeground` desde la API 34; en 29–33 se pasa 0 (ese tipo no existe ahí y el manifiesto no lo exige); antes
   de 29 no hay tipo. Probado en Android 17 y 13.
8. **429 del canal** (con el protocolo nuevo no debería pasar: un canal nuevo reemplaza al anterior): no se abre otro;
   se lee con `GET` mientras tanto y la espera sigue creciendo (2 … 30 s). **Un cierre normal** (la conexión duró 10 s
   o trajo mensajes) se reabre enseguida; uno inmediato sin nada cuenta como falla, para no reconectar sin parar.
9. **Cambio de IP**: tras tres fallas seguidas, `deviceAccess.recheck()` (la búsqueda por `server_id` y huella de
   `PairedConnection`), como mucho una vez por minuto. La búsqueda NSD usa el tipo de la aplicación (en las pruebas,
   `_adelpostest._tcp`).
10. **En las pruebas instrumentadas de la A1 el aviso va apagado** (`AppContainer.Settings.readyAlerts`, que
    `AdelTestRunner` enciende con `readyAlerts=on`): su servicio, su notificación emergente o el diálogo de permisos
    taparían pantallas de esas pruebas. La suite `alerts` lo enciende y prueba la convivencia con la captura y la cola.
11. **Una notificación por cuenta, visible en la pantalla de bloqueo** (`VISIBILITY_PUBLIC`): solo lleva mesa o pedido y
    platillos, que §0-B permite ver en «¿Quién eres?». El resumen no suena (`GROUP_ALERT_CHILDREN`).
12. **Cuenta a la vista** = la captura de esa cuenta abierta con la aplicación al frente (lo dice la navegación de
    `app`, sin tocar `core:capture`). Ahí el aviso va arriba de la pantalla 6 s y la notificación de esa cuenta se
    quita al abrirla.
13. **Al tocar un aviso**: vale 3 minutos (un toque viejo no abre nada cuando entra alguien después); en «¿Quién eres?»
    queda elegido su mesero (`requestedWaiter` de `feature:access`); si entra otro, se abre solo si la cuenta está en
    **sus** cuentas abiertas (las que Go le deja ver); un pedido para llevar sin `takeaway.manage` abre la lista.
14. **Íconos de notificación vectoriales en blanco**: Android solo toma su transparencia. `ColorsOnlyInPaletteTest`
    admite `@android:color/white` solo en `res/drawable/ic_stat_*.xml`, con la explicación en la prueba.
15. **Cambiar sonido o vibración en Terminales quita los avisos a la vista** del canal anterior: es la consecuencia de
    A2-D7 (borrar el canal viejo borra sus notificaciones). Se documenta; los siguientes avisos salen por el nuevo.
16. **El permiso de notificaciones concedido después** no hace aparecer sola la notificación permanente publicada sin
    él (Android 13+): se vuelve a publicar al responder el diálogo y al volver de Ajustes a la ayuda
    (`refreshOngoing`). Lo encontró `r1`.
17. **La ayuda** se abre desde el menú del mesero («Avisos de platillos listos») y al tocar «AD-EL POS conectado».
    «Forzar detención»/«Detener» se detecta solo para la salida anterior del proceso (`REASON_USER_REQUESTED`, API
    30+) y se dice en la ayuda.
18. **Los permisos se piden una sola vez cada uno** (bandera en `device_preferences`), la primera vez que el servicio
    corre con la aplicación al frente.
19. **Cuenta que ya no está abierta al tocar el aviso** (protocolo ajustado, punto 6): si no está entre las cuentas
    abiertas del mesero, se lee la cuenta en Go (`RestaurantSource.bill`, `ReadyAlertsUi.kt:637`): abierta (la lista
    iba atrasada) se abre; combinada (`merged_into_id`) se sigue a la vigente (hasta 3 saltos); cobrada, cancelada o 404,
    «Esta cuenta ya no está abierta»; sin conexión o sin permiso de verla, no se abre nada.
20. **A2-D4 («Cerrar sesión»)**: la aplicación **no tiene** un «Cerrar sesión» explícito (`WaiterWork.kt:33-36`: «si
    alguna pantalla lo ofrece»; el menú del mesero solo tiene «Cambiar de mesero», que por la regla 15 no cierra
    sesiones). La condición de A2-D4 no se da y no se agregó una función que la especificación no pide (ver «Para el
    coordinador»).

### Para el revisor

Rutas relativas a `android/`. `Controller` = `app/src/main/kotlin/com/adelpos/meseros/ready/ReadyAlertsController.kt`,
`Service` = `…/ready/ReadyAlertsService.kt`, `Channel` =
`core/data/src/main/kotlin/com/adelpos/meseros/core/data/ready/ReadyAlertsChannel.kt`, `Processor` =
`core/domain/src/main/kotlin/com/adelpos/meseros/core/domain/ready/ReadyAlerts.kt`.

**Ciclo de vida del servicio.**

- Un solo dueño por proceso: `AppContainer.readyAlerts` (`AppContainer.kt`, `lazy`), arrancado en
  `AdelPosApplication.onCreate` (`Controller.start`, `:173`): vigila la entrada con conexión (`:180-188`), el primer
  plano con el dispositivo listo (`:190-196`), el estado de la entrada (no `Ready` ni `Starting` → `stopService`,
  `:198-204`) y la red (`reconnectNow`, `:206-213`).
- Pedido de arranque: `Controller.requestNow` (`:229`) bajo un `Mutex` → `prepare()` (lee el emparejamiento si nadie lo
  leyó: arranque sin pantallas) → `ReadyAlertsLifecycle.decide` (`ReadyAlertsLifecycle.kt:76`) → `START`, `ASK_FIRST`
  (`probe`, `:265`) o `NONE`. Si ya corre, solo acorta la espera del canal (`:240`); si se está deteniendo, se recuerda
  y se vuelve a pedir en `onServiceDestroyed` (`:324`), para no perderlo.
- `startForegroundService` (`:289-299`): desde primer plano (entrada, aplicación abierta) o con exención
  (`ReadyAlertsBootReceiver`, `goAsync` con 8 s); `IllegalStateException` (incluye
  `ForegroundServiceStartNotAllowedException`) y `SecurityException` se atrapan y se espera al siguiente disparador.
- `Service.onStartCommand` (`Service:56-66`): `startForeground` **antes que nada** (`promote`, con el tipo desde la API
  34), también con `intent == null` (reinicio pegajoso); si Android lo rechaza, `stopSelf` y `START_NOT_STICKY`; si
  no, `Controller.onServiceStarted` (`:302`, un solo canal: no abre otro si ya corre) y `START_STICKY`.
  `onTaskRemoved` no detiene nada; `onDestroy` quita la notificación (`STOP_FOREGROUND_REMOVE`) y avisa al
  controlador, que cancela el canal.
- Parada: el canal termina (`Channel.run`, `:89`) → `onChannelEnded` → `stopService`; el borrado del dispositivo
  (`AppContainer.kt:173` → `Controller.eraseDeviceData`, `:359`: cancela y espera el canal hasta 5 s, detiene el
  servicio, borra Room y quita los avisos). Nunca queda la notificación sin servicio (es la del `startForeground`, la
  quita `onDestroy`) ni el servicio sin notificación (si `startForeground` falla, se detiene).

**Reconexión** (`Channel.run`, `:89-155`, con el protocolo ajustado).

- Cursor inicial = el de Room (`:93`); cada lote lo actualiza en memoria (`:182`) y en Room al procesarlo.
- `Closed` normal (abrió y duró 10 s o trajo mensajes; Go cierra a los 5 minutos y en silencio solo manda `: ping`):
  reabre enseguida (`:128-136`); un cierre inmediato sin nada cuenta como falla.
- `Failed` → `ReadyAlertsLifecycle.onError` (`ReadyAlertsLifecycle.kt:141`): `STOP_AND_REPORT` (`DEVICE_REVOKED`,
  otra huella, 426 → `deviceAccess.report`, `:194`), `STOP` (403/400/404 al abrir), `WAIT_AND_READ` (429: `GET`
  mientras tanto, `:199`), `RETRY` (sin red, 5xx, otro `DEVICE_*`, `interrupted` con otro código, conexión muerta: 45
  s sin un byte, `ReadyAlertsClient.kt:342`; `interrupted` se traduce en `ReadyAlertsClient.kt:265-279`).
- Espera creciente 2, 4, 8, 16, 30 s, acortada por `reconnectNow` (`:152`); tres fallas → `relocate` (`:148`, cambio
  de IP); a `day_ends_at` + 30 s, `GET` y seguir si Go dice `active` (`:103-125`).
- La huella fijada: `AuthorizedServer.client` es el `PinnedHttpClient` del emparejamiento
  (`DeviceReadyAlertsServer.connection`, `ReadyAlertsClient.kt:82`); un certificado distinto da
  `ServerIdentityChanged` → `STOP_AND_REPORT`.
- Al abrir después de fallar se pide un envío de la cola (`onReconnected`, `:169-175`); el canal no envía nada.

**Deduplicación.**

- `Processor.onBatch` (`ReadyAlerts.kt:160-175`): `distinctBy(key)` dentro del lote → `ledger.unseen` (Room, por la
  `key` de Go, `RoomReadyAlertsLedger.kt:32-38`) → `presenter.present` → `ledger.commit` (keys + cursor + estado en
  una transacción, `ReadyAlertsEntities.kt:102-114`, con el `Mutex` del registro).
- La notificación de cada cuenta guarda en sus extras las `key` que contiene (`ReadyAlertNotifications.kt:355-387`);
  `present` no vuelve a publicar una cuenta cuyos avisos ya están todos a la vista (`:147`): cubre el proceso muerto
  entre mostrar y guardar.
- Dos conexiones a la vez no pasan: un solo canal por proceso (`onServiceStarted` revisa `loop?.isActive`) y Go
  rechaza la segunda con 429. Un solo proceso (sin `android:process`).
- Probado: `ReadyAlertsRulesTest` (repetidos, reinicio del proceso, proceso muerto entre mostrar y guardar, cursor tras
  un corte, respaldo restaurado), `ReadyAlertsChannelTest` (`Last-Event-ID`/`after`, repetidos en el canal, 429 con
  `GET`, cierre normal con solo `: ping`, `interrupted` temporal, nueva consulta al pasar el fin del día),
  `RoomReadyAlertsLedgerTest`, `ReadyAlertsOnDeviceTest.unaNotificacionPorCuentaConElAcumuladoYSinRepetir` y,
  de punta a punta, `r4` (corte real con «Listo» durante el corte: una sola notificación con un solo aviso) y `r8b`
  (reinicio pegajoso).

**Lo que conviene mirar con más cuidado.**

- La consulta previa (`probe`) y el canal escriben en el mismo registro: van de uno en uno por el `Mutex` del
  registro, y la consulta solo corre con el servicio detenido; si coincidieran, lo peor es releer avisos que el
  registro descarta.
- `viewingAccount`/`isViewing` supone una sola `MainActivity` (la de la tarea de la aplicación; el toque usa
  `CLEAR_TOP`/`SINGLE_TOP`). Con dos instancias (multiventana) la última que cambie de pantalla manda.
- El reinicio pegajoso tras `kill -9` y el arranque al abrir la aplicación normal se probaron fuera de la
  instrumentación (`r8b`, la instrumentación detiene la aplicación al terminar); `BOOT_COMPLETED` y
  `MY_PACKAGE_REPLACED` no se probaron en esta fase (la Fase 0 probó `BOOT_COMPLETED` con la prueba de humo).

### Pruebas

Todo en el árbol `$SP/wt/d` con el arnés (`ISO_TAG=D`), JDK de Android Studio. Lo de punta a punta contra un Go
temporal **nuevo** para cada corrida, construido desde este mismo árbol (servidor 0.13.0 con la parte D **anterior** al
ajuste del protocolo: el protocolo nuevo todavía no estaba fusionado; el cliente ajustado es compatible con el viejo:
sin `key` usa «número@hora», el cierre a los 30 s con mensajes es un cierre normal y `interrupted` acepta
`{"error"}`), 8241/8501, emparejado por su dirección (sin búsqueda en la red real).

| Qué                                                                                                                    | Dónde                                                                                                                                              | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `:core:domain:test`                                                                                                    | Mac                                                                                                                                                | **132 pruebas, 0 fallas** (113 de antes + `ReadyAlertsRulesTest` 18 + 1 de `ServerCompatibilityTest`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `testDebugUnitTest` (JVM + Robolectric SDK 36)                                                                         | Mac                                                                                                                                                | **184, 0 fallas**: `core:data` 158 (141 de antes + `ReadyAlertsChannelTest` 13 + `RoomReadyAlertsLedgerTest` 4), `core:ui` 9, `feature:access` 7, `feature:takeaway` 5, `feature:waiter` 5. Con el dominio, **316** (antes 280). Antes del ajuste del protocolo: 314 (canal 11), también sin fallas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `ktfmtCheck`, `lintDebug` (todos los módulos), `assembleDebug`, `assembleDebugAndroidTest`                             | Mac                                                                                                                                                | Aprobados; lint sin problemas. La primera corrida de lint encontró `UseKtx` (`Uri.parse` → `toUri`), corregido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `iso.sh format`, `fmt-check` (Prettier, gofmt, cargo fmt, datos del sistema anterior); `md5` de `docs/etapas/*.md`     | árbol `d`                                                                                                                                          | Aprobados; `md5` igual antes y después                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `connectedDebugAndroidTest` (antes: desinstalar la app, sus `.test` y los de la 0.1.0)                                 | `Pixel_Tablet` (`emulator-5584`, Android 17), sobre `80e8ea9`                                                                                      | **123 pruebas, 0 fallas**, 61 omitidas a propósito (las de punta a punta sin argumentos, 9 más por `AlertsOnGoTest`): `app` 90 (59), `core:data` 20 (2; con el caso 5 → 6 ampliado), `core:ui` 13 — 3 XML. La primera corrida tuvo 2 fallas de `ReadyAlertsOnDeviceTest`, las dos reales o de la prueba: el separador «, » de `strings.xml` perdía su espacio (Android quita los espacios de las orillas: ahora entre comillas) y la prueba leía `running` antes de que `onDestroy` lo bajara (ahora espera)                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Ídem                                                                                                                   | `ADEL_A1_Phone_API33` (`emulator-5586`, Android 13), sobre `80e8ea9` y otra vez sobre `c4de974` (protocolo ajustado)                               | **123 pruebas, 0 fallas**, 61 omitidas, 3 XML, las dos veces                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `scripts/android-e2e.sh --suite alerts` (r1–r9 con r8b)                                                                | `Pixel_Tablet`, sobre `80e8ea9`                                                                                                                    | **10 de 10 aprobados** en la corrida final (`tablet-final`). Antes, cuatro corridas encontraron y corrigieron: `r1` la notificación permanente publicada sin el permiso no aparecía al darlo (**corregido en la aplicación**: `refreshOngoing`); `r8b` el servicio reiniciado solo se detenía enseguida porque «el dispositivo listo» venía de un flujo derivado que todavía no se actualizaba tras leer el emparejamiento (**corregido**: se lee en el momento, `DeviceReadyAlertsServer.ready`); `r6` abrir la cuenta se cancelaba a sí mismo al consumir el toque antes de terminar (**corregido**: se consume al final) y, en la prueba, el reloj de los efectos de Compose solo avanza esperando con Compose (`compose.waitUntil`) y ActivityScenario abre su propia tarea (`r6` abre como el lanzador); el guion se salía con `set -e` en `pidof` sin proceso (corregido) y la mesa no cabía en el mapa de Go (corregido) |
| Ídem                                                                                                                   | `ADEL_A1_Phone_API33`, sobre `c4de974` (protocolo ajustado, contra el servidor anterior)                                                           | **9 de 10** en la primera corrida: `r8b` falló porque en Android 13 `run-as … kill -9` no mata el proceso («unknown pid»); el guion ahora usa root de respaldo (`ce0f36b`) y `--steps "r1 r8"` con otro Go temporal: **3 de 3 aprobados** (reinicio pegajoso ≈ 8 s). Los otros nueve pasos, aprobados a la primera                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `MY_PACKAGE_REPLACED` (`$SP/d-work/replaced.sh`: `r1` y después `adb install -r` de la misma aplicación, wifi apagada) | `ADEL_A1_Phone_API33`, sobre `c4de974`                                                                                                             | **Aprobado**: sin el servicio antes de reinstalar; después, `ReadyAlertsBootReceiver` lo arrancó en primer plano («servicio de avisos pedido (BOOT)») sin abrir la aplicación                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Batería (método de §0.5.6), 5 minutos                                                                                  | `Pixel_Tablet`, servicio activo, pantalla apagada, sin cargador, `force-idle` (`IDLE` al empezar y al terminar), wifi apagada (red móvil simulada) | Ver «Batería»                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

**Lo que comprueba cada paso de `alerts`** (`AlertsOnGoTest`; cocina, caja y Terminales por la API de la principal):

- `r1`: con `alertPermissions=ask`, las dos explicaciones (notificaciones en Android 13+ y batería) aparecen antes del
  diálogo de Android y «Ahora no» deja seguir; el servicio arranca al entrar con conexión, en primer plano con «AD-EL POS
  conectado», y Go dice `active`.
- `r2`: Ana captura en la UI (dos «Filete R») y envía a cocina por la cola; la tarea se quita de recientes (el servicio
  sigue: `onTaskRemoved`), pantalla apagada, sin cargador y `force-idle`; cocina marca «Listo»: llega «Mesa R4 · 2
  platillos listos» / «2 × Filete R» en el canal con sonido y vibración, con Doze profundo `IDLE` (registro: el aviso
  llegó ≈ 1 s después de forzar Doze).
- `r3`: un segundo dispositivo `android` emparejado por la API con Beto entrando con su PIN; lo de Beto no llega a este
  y lo de Ana no aparece en `ready-alerts` del otro (sin precios); la cuenta de Ana transferida a Beto antes del «Listo»
  llega a Beto y no a Ana.
- `r4`: el guion corta la red (modo avión), marca «Listo» desde la Mac durante el corte y la devuelve: el canal vio el
  corte (`Waiting`), el aviso llega **una vez** (una notificación con un solo aviso, sin volver a publicarse en 5 s).
- `r5`: dos «Listo» de la misma cuenta = una notificación con el acumulado («Mesa R2 · 2 platillos listos» / «1 ×
  Filete R, 1 × Tacos R»); con otra cuenta, el resumen del grupo.
- `r6`: con la aplicación en «¿Quién eres?» el servicio arranca con lo guardado; llega el aviso; al tocarlo queda
  elegida Ana, se escribe su PIN y se abre la captura de esa cuenta; con la cuenta a la vista, el siguiente «Listo» es el
  aviso dentro de la aplicación y no hay notificación del sistema de esa cuenta.
- `r7`: sonido y vibración apagados en Terminales → un solo canal «Platillos listos» = `listos_silencio`, sin sonido
  ni vibración, y el aviso sale por él; «Avisar platillos listos» apagado → el servicio se detiene sin notificación
  permanente.
- `r8a` + `r8b`: el guion abre la aplicación normal (wifi apagada), el servicio arranca, `kill -9` del proceso,
  **reinicio pegajoso** (nuevo proceso con el servicio en primer plano, ≈ 6 s), «Listo» desde la Mac y el aviso «Mesa
  R10 · 2 platillos listos» en `dumpsys notification --noredact`.
- `r9`: al retirar el dispositivo el servicio se detiene, se va la notificación permanente, se borra el registro del
  aviso y se ve «retirado».

#### Batería (referencia, no medición real)

`dumpsys batterystats com.adelpos.meseros` tras 5 min (300 s) en `Pixel_Tablet` con el servicio en primer plano
(`types=0x40000000`, `specialUse`), pantalla apagada, sin cargador y Doze forzado: red móvil simulada **103,6 KB
recibidos y 25,4 KB enviados** (459/461 paquetes), CPU u = 817 ms y s = 125 ms, **0,00125 mAh** estimados (todo
`fgs`). El perfil de energía del emulador es ficticio. **El tráfico es del protocolo anterior** (Go mandaba un lote
cada 750 ms: ≈ 1,2 MB/h); con el protocolo ajustado (solo cambios y `: ping` cada 15 s) debería bajar mucho; no se
midió (el servidor nuevo no estaba fusionado). La hora completa es de la Fase E.

Ninguna prueba usó la red real ni el servicio de la Mac: el ejecutor busca `_adelpostest._tcp` y el dispositivo se
empareja por la dirección del Go temporal; en `r8b` y la batería (aplicación normal, fuera de la instrumentación) la
wifi del emulador estuvo apagada (Android 11+) y el dispositivo ya estaba emparejado con el Go temporal por su
dirección; ninguna lista mostró un servidor real. Al terminar cada corrida: `force-idle` quitado, `battery reset`,
pantalla encendida, modo avión y wifi como estaban (comprobado con `dumpsys`), aplicación detenida y desinstalada, Go
temporal detenido con su base borrada y emuladores apagados.

### No verificado

- **Contra el servidor con el protocolo ajustado** (`key`, solo cambios con `: ping`, cierre a los 5 minutos,
  `interrupted` con `{"code"}`, sin avisos de antes de entrar): solo con MockWebServer (`ReadyAlertsChannelTest`) y la
  suite `alerts` contra el servidor anterior (compatible). Falta repetir `alerts` cuando se fusione.
- `ADEL_A1_Tablet_API26` (Android 8) y `Pixel_8`: la matriz de cuatro es de la Fase E. En Android 8–10 `r4` corta la
  red con `adb root` antes de la prueba (el guion lo hace; no corrido) y `r8b` no apaga la wifi (no hace falta root
  para lo demás).
- `BOOT_COMPLETED` (reinicio real del emulador) con esta aplicación: el receptor es el mismo que arrancó el servicio
  con `MY_PACKAGE_REPLACED` (probado en Android 13) y su decisión está probada en la JVM; el arranque tras reiniciar lo
  probó la Fase 0 con la prueba de humo.
- El diálogo del sistema de los permisos (se probó la explicación y «Ahora no»; el permiso lo concede la prueba) y la
  pantalla de Ajustes de batería; «Forzar detención»/«Detener» del Administrador de tareas (`REASON_USER_REQUESTED`).
- Dispositivo físico: Doze real, gestores de batería de cada marca, la wifi del restaurante y el sonido real del canal.
- El aviso dentro de la aplicación con dos `MainActivity` a la vez (multiventana): la última que cambia de pantalla
  manda.

### Para el coordinador

- **A2-D4 sin efecto**: la aplicación no tiene «Cerrar sesión» explícito (solo «Cambiar de mesero», que por la regla 15
  no cierra sesiones). Si se quiere que el mesero pueda cerrar su sesión en Go, hay que **agregar** «Cerrar sesión» al
  menú del mesero (con `WaiterWork.blocksLogout` y `POST auth/logout`): no lo hice porque la especificación no lo pide
  y cambia la interfaz. Hoy una sesión vigente dura hasta 12 h y A2-D4 elige la de actividad más reciente.
- **Fusión con la Fase C** (archivos que tocamos los dos o es probable): `ServerCompatibility.REQUIRED_CAPABILITIES` y
  su prueba (cada fase agrega su capacidad: quedan las cuatro), `feature/access/.../strings.xml` (`update_server_text`
  ahora nombra «el aviso de platillos listos»), `scripts/android-e2e.sh` (listas de suites y `--suite all`),
  `AppContainer.kt` (registro de borrado y `readyAlerts`), `AdelPosApp.kt`, `docs/ANDROID_INSTALAR.md` (agregué la
  sección de los avisos; ya resuelto en la fusión `0357705`: 7 precuenta, 8 avisos, 9–11 las demás) y `android/CLAUDE.md` (regla **22**). Room: la v6 de
  la Fase B con mis dos tablas (la Fase C no la cambia).
- **Batería y red con el protocolo anterior**: un lote cada 750 ms eran ≈ 1,2 MB/h por dispositivo en el emulador; el
  protocolo ajustado lo resuelve en el servidor. Conviene medir la hora de la Fase E ya con él.
- Al pasar `day_ends_at` sin respuesta de Go el servicio **sigue** (decisión del coordinador): una tableta que se quedó
  sin la computadora principal de noche sigue reintentando con su notificación permanente hasta que vuelva.
- Cambié dos componentes compartidos, pequeños y opcionales: `ActiveWaiterMenu` (`core:ui`, parámetro
  `onAlertsHelp`) y `AccessContent`/`PinEntryRoute` (`feature:access`, `requestedWaiter`). `ColorsOnlyInPaletteTest`
  admite `@android:color/white` solo en `res/drawable/ic_stat_*.xml`.
- Nada obligó a cambiar una regla de negocio ni el servidor.

### Para CLAUDE.md

- §0/encabezado: Android 0.2.0 con el aviso de platillos listos (Fase D): servicio en primer plano `specialUse`, canal de
  avisos con la credencial del dispositivo, `AppDatabase` v6 con `ready_alerts_state` y `ready_alerts_seen`; la
  aplicación exige `ready-alerts-v1`.
- Pruebas (§6): `ReadyAlertsRulesTest`, `ReadyAlertsChannelTest`, `RoomReadyAlertsLedgerTest`,
  `ReadyAlertsOnDeviceTest` y la suite `alerts` de `scripts/android-e2e.sh` (`AlertsOnGoTest`, `r1`–`r9`, con `r4` y
  `r8b` hechos por el guion; también en `--suite all`). En las instrumentadas el aviso va apagado salvo
  `readyAlerts=on`.
- Regla nueva de Android **22**: un solo canal de eventos y un solo canal de avisos por proceso.

### Fusión con la Fase C (`0357705`)

`etapa-a2-android` (`394cab6`, con la Fase C) fusionada en `etapa-a2-d` como unión de las dos fases:
`ServerCompatibility` exige las cuatro capacidades (`waiter-pin-v1`, `takeaway-v1`, `device-precheck-printer-v1` y
`ready-alerts-v1`) con las dos pruebas (una sin la impresora de precuentas y otra sin el aviso); `FakeGoServer` anuncia
las cuatro; `scripts/android-e2e.sh` con las suites `precheck` y `alerts`, las dos en `--suite all`; `android/CLAUDE.md`
con las secciones de las dos (reglas 1–22, sin repetir); la guía con **7 «Imprimir la precuenta»** y **8 «Avisos de
platillos listos»** (las siguientes pasan a 9–11; referencias a «Retirar» → sección 10 y a los avisos → sección 8); y
«Actualiza el servidor del restaurante» (`update_server_text`) ya no nombra funciones sueltas («le faltan funciones que
la aplicación usa (necesita la versión 0.13.0 o posterior)»). Después de la fusión: `ktfmtCheck`, `lintDebug`,
`assembleDebug`, `assembleDebugAndroidTest`, `bash -n`, `format`/`fmt-check` aprobados; **337 pruebas en la JVM, 0
fallas** (`core:domain` 147, `core:data` 163, `core:ui` 9, `feature:access` 7, `feature:takeaway` 5, `feature:waiter`
6); `md5` de `docs/etapas/*.md` sin cambios. Las suites `alerts` y `precheck` se repiten cuando llegue el servidor con
el protocolo corregido.

### Decisiones del coordinador sobre la Fase D

Se aceptan las decisiones de la fase. **A2-D4, «Cerrar sesión»**: la app no tiene un «Cerrar sesión» explícito
(cambiar de mesero no cierra sesiones, regla 15 de la A1) y la especificación no lo pide, así que no se agrega; el
destinatario lo resuelve el servidor con la sesión de actividad más reciente. La fusión con la Fase C la hizo el agente
de la fase en su rama (`0357705`: unión de las dos en `ServerCompatibility`, el guion, `android/CLAUDE.md` y la guía,
y `update_server_text` general) y se pasó a la etapa sin cambios. Pendiente al fusionar: las suites `alerts` y
`precheck` contra el servidor con las correcciones de la revisión independiente (protocolo acordado: `key`, `: ping`,
reemplazo del canal, `interrupted` con código).

### Verificación del coordinador de las correcciones de la revisión (servidor)

Sobre una copia exacta de `e3a3a88` (`git archive`): `fmt-check`, `sync-check`, `go-unit`, `go-int` de `apicontract`,
`database` y `kitchen` completos, `TestStageA2Ready*` de `operations` (`TestStageA2ReadyAlerts` y
`TestStageA2ReadyReview`, las reproducciones del revisor, que ahora pasan y se quedan como pruebas de regresión) y
`TestStageTwoAcceptance` y `TestStageA1WaiterPIN` de `administration`, todo aprobado. Fusión en `2bf8183`.

# Revisión independiente · Aviso de platillos listos (§4)

Revisor independiente que pide §0 de la especificación («servicio en primer plano, reconexión y entrega sin duplicados
ni pérdidas»). La revisión va en dos partes: la **Parte 1 (servidor)** sobre `etapa-a2-rev` desde `394cab6` (cabeza de
`etapa-a2-android` con la parte D del servidor fusionada) y la **Parte 2 (dispositivo)** cuando la Fase D esté
fusionada. Árbol `$SP/wt/rev`, `ISO_TAG=R` (clúster temporal 55499, bases `adel_pos_test_r_*`, Go temporal 8251/8511 ya
detenido). No se tocó el código del producto ni la instalación real; las reproducciones son una prueba nueva en un
commit de la rama de revisión. Leí el código, la migración, el contrato y las pruebas antes de leer «Servidor · Parte
D» del avance; al final contrasto con ese apartado.

## Parte 1 · Servidor

### Resumen

El diseño es sólido en lo central: el registro `app.ready_notices` se escribe en la misma transacción que el «Listo»,
todos los escritores toman 7820148305 en exclusiva **antes** de insertar (`kitchen/handler.go:72`, `kitchen/worker.go:70`,
`operations/handler.go:54`; `kitchen.Integrate` solo se llama desde `operations`), así que las secuencias se asignan y se
confirman en el mismo orden y ninguna lectura puede ver un hueco detrás del cursor; `UNIQUE (unit_id, version_id)`, el
grupo por operación de cocina y el cursor por grupos completos no repiten nada; la respuesta nunca lleva importes y la
credencial del dispositivo solo abre `GET ready-alerts` y su canal.

Encontré **tres caminos en los que un aviso se pierde en silencio** (los tres confirmados con reproducción), un
problema de **consumo del canal** con la pantalla apagada (confirmado: medición del tráfico) y varios puntos bajos. Ninguno
es crítico ni obliga a cambiar una decisión del propietario; R1–R3 contradicen «ningún aviso se pierde» (§4.1) y R2
además §0-B («en el dispositivo donde tenga sesión abierta»).

Reproducciones: `backend/internal/operations/stagea2_ready_review_test.go`, `TestStageA2ReadyReview`, commit `712e869`
de `etapa-a2-rev`. **Falla a propósito con el código actual** (cada subprueba comprueba lo que pide la especificación y
su mensaje dice lo que pasó): no debe fusionarse en la etapa tal cual; sirve para verificar las correcciones (y quedarse
como prueba de regresión) o se descarta.

### Hallazgos

| #   | Severidad | Estado                                                      | Dónde                                                                                                                       | Escenario de falla                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Corrección sugerida                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --- | --------- | ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1  | Medio     | Confirmado (R1 de la prueba)                                | `kitchen/preparation.go:153-154`; `kitchen/changes.go:165-167` y `184-192`                                                  | Una unidad con tareas en dos estaciones (combo: la parrilla lista, la freidora pendiente). El mesero quita las papas: la tarea de la freidora se **cancela** y la de la parrilla, sin cambio de receta, **sigue lista con su `ready_at` viejo**. La unidad queda «Listo» (`preparation_state: ready`) pero `recordReadyNotices` exige `max(ready_at)=now()`: **0 filas en `ready_notices`, 0 avisos**. El autor de la parte D revisó este camino y concluyó que «las modificaciones no marcan Listo»; esta sí lo hace, por cancelación.                                                                                                                                                                                                                                      | En `recordReadyNotices`, contar también como «pasó a Listo en esta transacción» la unidad activa con todas sus tareas vivas listas y alguna tarea **cancelada en esta transacción que no estaba lista** (`state='cancelled' AND updated_at=now() AND ready_at IS NULL`; la cancelación no borra `ready_at`, así que una tarea que ya estaba lista no vuelve a avisar). Agregar el caso a `TestStageA2ReadyAlerts`.                                                                                |
| R2  | Medio     | Confirmado (R2)                                             | `administration/ready_alerts.go:82-83` frente a `157-158`; `kitchen/preparation.go:143-146`                                 | Beto entró en la tableta 2 antes de la «Hora de inicio del día» y su sesión con PIN sigue vigente (12 h: entró a las 22:00 con la hora de inicio a las 00:00, o a las 18:00 y son las 05:15 con la de 05:00). Al «Listo», el aviso **se fija a la tableta 2** (su sesión vigente más reciente). La tableta 2 responde `active: true` (cuenta la sesión vigente) pero `entered` solo admite sesiones **creadas** desde el inicio del día: no le entrega el aviso y su cursor lo deja atrás; la tableta 1 tampoco (no es para ella). **No llega a nadie**, hasta que Beto vuelva a entrar con PIN (y lo de mientras se pierde si la tableta siguió leyendo, que es lo que pide la «Forma final» para `day_ends_at`, `docs/ETAPA_A2_AVANCE.md:1469`).                           | Que `entered` sea la misma regla que `active`: un mesero cuenta en D si entró con PIN hoy **o** tiene en D una sesión con PIN vigente (`created_at>=$2 OR (revoked_at IS NULL AND expires_at>now())`). Es lo que dice §0-B («en el dispositivo donde tenga sesión abierta») y lo que ya promete `active`. Ajustar la regla 1 de `docs/API_V1.md` («Avisos de platillos listos») y el contrato.                                                                                                    |
| R3  | Medio     | Confirmado (R3, restauración simulada)                      | `migrations/0026_android_a2.sql:57`; `administration/ready_alerts.go:59-60`, `150-154`                                      | Se restaura un respaldo anterior (pg_dump guarda el `setval` de la identidad). El dispositivo vuelve con un cursor mayor que la cabeza: Go relee el día (decisión 6 de la parte D) y el dispositivo descarta los `id` que ya avisó. Pero los avisos **nuevos** reciben otra vez los números que el dispositivo ya vio ese día para **otras cuentas** (en la prueba: la cuenta B llega con `id` 2, el mismo de la cuenta A). Con A2-D9 («descartar los `id` repetidos del día») el dispositivo los descarta: se pierden todos hasta que la secuencia pase el cursor viejo (con un respaldo de la noche anterior, el resto del día).                                                                                                                                           | Dar a cada aviso una clave que no se reutilice tras una restauración y que el dispositivo use para descartar repetidos: por ejemplo `notice_key uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE` en `ready_notices` (la 0026 aún no se entrega) y en `ReadyAlert` la del último aviso del grupo; el número sigue siendo solo el cursor. Alternativa mínima: con `after > head`, responder `reset: true` para que el dispositivo vacíe sus `id` del día (a costa de repetir lo del día restaurado). |
| R4  | Medio     | Confirmado (R4: tráfico medido); consumo real no verificado | `administration/ready_alerts.go:263-285`                                                                                    | El canal manda un mensaje `alerts` completo **cada 750 ms aunque no haya nada** y se cierra a los 30 s. Medido sin ningún «Listo»: **8 mensajes y 1288 bytes en 6 s** → unas 4800 respuestas y ~750 KB por hora y dispositivo, más 120 peticiones nuevas por hora (cada una con su lectura en la base). Para el canal de eventos de la A1 era aceptable (solo en primer plano); aquí corre todo el turno con la pantalla apagada y no deja dormir la wifi. La prueba de humo de la Fase 0 midió la batería con un evento cada 20 s (29 paquetes en 2 min), no con este ritmo. En Go, cada lectura es además una transacción con escritura (`UPDATE devices SET last_seen_at`, `administration/service.go:395`): unas 1,3 confirmaciones con fsync por segundo y dispositivo. | Seguir leyendo cada 750 ms en Go, pero **mandar solo cuando haya avisos o cambien `cursor`, `active`, `settings` o `day_ends_at`**, con un comentario de mantenimiento (`: ping`) cada 15–20 s para que el dispositivo detecte la conexión muerta, y una vida más larga para este canal (unos 5 min). Coordinarlo con el tiempo de «lectura sin datos» del dispositivo (Parte 2). Medir la batería en la Fase E con el ritmo real.                                                                |
| R5  | Bajo      | Confirmado (medición sobre datos sintéticos)                | `administration/ready_alerts.go:82-83` y `157-158`                                                                          | `sessions` nunca se poda y cada entrada con PIN crea una fila (con el bloqueo por inactividad de 2 min, decenas o cientos al día por dispositivo). Las dos lecturas de sesiones del dispositivo se hacen en cada lectura (cada 750 ms) y solo tienen el índice por `device_id`: con 58 000 sesiones por dispositivo (un año a ~160 entradas diarias) y 10 dispositivos, **9,5 ms + 5,9 ms por lectura** (`EXPLAIN ANALYZE`, 58 000 filas descartadas cada vez), con el lock 7820148305 compartido y la fila del dispositivo bloqueada mientras tanto; crece con los años. Con un índice parcial, 0,05 ms y 0,03 ms.                                                                                                                                                          | Índices en la 0026: `sessions (device_id, created_at) WHERE opened_with_pin` y `sessions (device_id) WHERE opened_with_pin AND revoked_at IS NULL`, y `active` como dos `EXISTS` separados (así se midió con los índices; no medí el `OR` dentro de un solo `EXISTS`).                                                                                                                                                                                                                            |
| R6  | Bajo      | Confirmado `id: 0` (R4); el 503 por lectura de código       | `administration/ready_alerts.go:279`; `administration/http.go:141-158`                                                      | «interrupted» lleva `id: 0`: en un cliente SSE estándar el último id pasa a 0 y una reconexión con `Last-Event-ID` relee todo el día. Además no solo llega al revocar: cualquier error transitorio de la lectura (la espera del lock pasa de los 3 s detrás de una escritura larga, la base no responde) llega como `interrupted` con `SERVICE_UNAVAILABLE`/503, y la «Forma final» lo describe como «el dispositivo dejó de valer». Un dispositivo que lo trate como retiro detendría los avisos por un tropiezo de la base.                                                                                                                                                                                                                                                | Mandar «interrupted» sin línea `id:` (o con el cursor vigente) y, en un 5xx, cerrar el canal sin «interrupted» (el dispositivo reconecta como tras un corte). Precisar en `docs/API_V1.md` y en la «Forma final» que solo 401 es definitivo. Verificar en la Parte 2 qué hace la app.                                                                                                                                                                                                             |
| R7  | Bajo      | Plausible (lectura de código)                               | `kitchen/preparation.go:143-146`                                                                                            | El dispositivo se fija al «Listo» sin mirar si puede recibirlo: si la sesión vigente más reciente del mesero está en un dispositivo con «Avisar platillos listos» apagado, en uno que se retira antes de leer o en uno que todavía tiene la 0.1.0 (`local.adel.pos.android`, que no lee avisos), el aviso no llega a ninguno de sus otros dispositivos.                                                                                                                                                                                                                                                                                                                                                                                                                      | Elegir el dispositivo entre los `android` no revocados con `ready_alerts_enabled` (y, si se quiere, con `app_version` ≥ 0.2.0); documentar en la guía que la 0.1.0 debe revocarse antes (ya lo pide §2).                                                                                                                                                                                                                                                                                          |
| R8  | Bajo      | Plausible (lectura de código)                               | `administration/service.go:395`; `administration/ready_alerts.go:137`; `kitchen/stations.go:235`; `maintenance/apply.go:57` | La lectura bloquea la fila del dispositivo (`last_seen_at`) y después pide 7820148305 compartido; quien tiene 7820148305 en exclusiva y luego bloquea la fila de ese android (guardar sus estaciones de cocina con `FOR UPDATE`, o «Quitar datos de prueba» con una impresora de prueba como su impresora de precuentas) forma un interbloqueo. PostgreSQL lo corta en 1 s: falla la lectura (canal interrumpido, R6) o la operación del administrador. No se pierde nada. El mismo patrón ya existe en `restaurant/state`.                                                                                                                                                                                                                                                  | Tomar el lock compartido antes de `authenticateDevice` en esta ruta, o rechazar la configuración de estaciones de cocina en un `android`. Prioridad baja.                                                                                                                                                                                                                                                                                                                                         |
| R9  | Bajo      | Plausible (diseño; la prueba existente lo fija)             | `administration/ready_alerts.go:90`                                                                                         | Un corte de red que cruza la «Hora de inicio del día» pierde los avisos del día anterior que no llegaron (`ready_at>=$2`). Con 05:00 casi nunca pasa; con 00:00 y un servicio hasta la 01:00, un corte de 23:58 a 00:03 pierde lo de 23:58–00:00.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Aceptable como regla del día comercial; si se quiere cerrar, admitir los avisos de las últimas horas del día anterior cuando el cursor del dispositivo es anterior a `dayStart`. Decirlo en la guía.                                                                                                                                                                                                                                                                                              |
| R10 | Bajo      | Plausible (lectura de código)                               | `administration/ready_alerts.go:84-108`                                                                                     | El aviso se entrega tal como se registró aunque, mientras el dispositivo estuvo sin red, la unidad o la cuenta se cancelaran o la cuenta se combinara en otra: «Mesa 4: 1 filete listo» de algo que ya no existe; al tocarlo se abre una cuenta cancelada o combinada.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Filtrar al leer `u.state='active'` y la cuenta abierta (o mandar `account_state`/`merged_into_id` para que la app abra la cuenta destino). Bajo.                                                                                                                                                                                                                                                                                                                                                  |
| R11 | Bajo      | Plausible (depende del dispositivo)                         | `administration/ready_alerts.go:117-123` (`after` vacío = 0)                                                                | La primera lectura sin cursor (instalación nueva, datos borrados) recibe **todo el día** que le corresponde al dispositivo (avisos a «todos» de horas antes): una ráfaga de notificaciones viejas si la app no la descarta.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Que la app tome el cursor de la primera respuesta sin notificar lo anterior (o use `ready_at`). Revisarlo en la Parte 2.                                                                                                                                                                                                                                                                                                                                                                          |
| R12 | Bajo      | Plausible (diseño documentado)                              | `administration/ready_alerts.go:231-237`                                                                                    | Tras un cambio de red, el canal viejo queda medio abierto en Go (escribe en el búfer del socket sin error) hasta su vida de 30 s: la reconexión recibe 429 `READY_ALERTS_CONNECTION_LIMIT` hasta entonces. Sin pérdida (el cursor), pero el aviso puede tardar ~30 s más (más la espera creciente de la app).                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Aceptable; con la vida más larga de R4, cerrar el canal anterior del mismo dispositivo cuando llega uno nuevo (en lugar de 429) evita esperar.                                                                                                                                                                                                                                                                                                                                                    |

### Detalle de las reproducciones (`TestStageA2ReadyReview`)

Una base desde cero (como `TestStageA2ReadyAlerts`), dos estaciones KDS: la categoría va a «Parrilla revisión» y el
producto «Papas producto» a «Freidora revisión» (la ruta por producto gana), Ana en la tableta 1 y Beto en la tableta 2.
Corrida con `ISO_TAG=R ISO_RUN=TestStageA2ReadyReview iso.sh … go-int ./internal/operations` (28 s): **falla en las 4
subpruebas**, con estos mensajes:

- **R1**: «la unidad quedó «Listo» (preparation_state "ready"; parrilla ready, freidora cancelled) al quitarle la
  estación pendiente y no se registró su aviso: 0 filas en ready_notices, 0 avisos en la tableta de Ana». Es la edición
  normal (`edit-preview` + `edit`) del combo con 1 → 0 papas, después de marcar «Listo» solo la parrilla.
- **R2**: «el aviso de Beto quedó fijado a la tableta 2 (1 fila), que responde active true, pero no lo entrega (0
  avisos; cursor 1, cabeza 1: ya lo dejó atrás) y la tableta 1 tampoco (0)». La sesión de Beto se movió a una hora
  antes del inicio del día con `expires_at` en 6 h (lo que deja la entrada de la noche anterior).
- **R3**: «tras restaurar, el aviso nuevo de la cuenta … llega con id 2, el mismo que el dispositivo ya avisó hoy para
  la cuenta …». La restauración se simula en `ready_notices` (`DELETE` desde ese aviso y
  `ALTER … RESTART WITH`, lo que deja un `pg_restore` de un respaldo de justo antes).
- **R4**: «sin novedades el canal mandó 8 mensajes (1288 bytes) en 6.0 s…» y ««interrupted» llega con id: 0…» (al
  retirar el dispositivo con el canal abierto).

R5 se midió aparte, en la base del Go temporal (`iso.sh server-up` con 8251/8511, asistente por HTTP, 580 000 sesiones
sintéticas en 10 dispositivos android con `psql`; `EXPLAIN (ANALYZE, BUFFERS)` de las dos consultas de sesiones antes y
después de crear los dos índices sugeridos; servidor detenido y base borrada con `server-down`). Script:
`$SP/rev-work/psql-r.sh`.

### Revisado sin encontrar defectos

- **Orden del cursor y locks**: todos los caminos que insertan en `ready_notices` pasan por `publishPreparation` dentro
  de `kitchen.Handle` (POST), `Worker.ReleaseDue` o `kitchen.Integrate` (solo desde `operations`), y los tres tienen
  7820148305 en exclusiva desde antes de la primera escritura hasta el commit. La identidad se asigna con el lock, así
  que los números se confirman en orden y cualquier foto de la base ve un prefijo; la lectura toma el lock compartido
  antes de leer la cabeza y los avisos (y la transacción es `READ COMMITTED`). Las secuencias de un mismo grupo son
  contiguas (un solo `INSERT … ORDER BY account_id`), así que el corte de 50 grupos nunca parte un grupo.
- **Caminos de «Listo»**: el único `UPDATE … state=$1` con `ready` es `saveStates` (tareas, pedido de una estación y
  cuenta completa); conserva el `ready_at` de una tarea que ya estaba lista, así que repetir «Listo» o un segundo KDS no
  vuelve a avisar. Una modificación con cambio de receta reinicia las tareas y la versión nueva se avisa una vez; sin
  cambio de receta, una unidad que ya estaba lista no se vuelve a avisar (salvo R1, al revés). Reintentar un cambio
  pendiente usa el mismo código. La liberación automática no marca «Listo».
- **Sin repetición**: `UNIQUE (unit_id, version_id)` con `ON CONFLICT DO NOTHING`, la reproducción idempotente de una
  operación de cocina devuelve la respuesta guardada sin escribir, el `id` del grupo es estable entre lecturas y `after`
  y `Last-Event-ID` dan los mismos `id` (salvo R3).
- **Destinatario**: el responsable al «Listo» (`assigned_user_id`), también en para llevar; transferir cambia lo que no
  estaba listo; dividir/transferir unidades y combinar usan la cuenta actual de la unidad (`notePreparationUnits`).
  Solo sesiones con PIN de `android` no revocados, la de `last_seen_at` más reciente; el canal de avisos no toca
  `sessions.last_seen_at`, así que el servicio en segundo plano no convierte su tableta en «la más reciente».
  `auth/logout` deja de contar. Sin sesión vigente, todos los dispositivos donde entró hoy.
- **Datos y credencial**: el aviso lleva mesa o folio, nombre del cliente, mesero, nombre del producto con su
  presentación y cantidad; ningún importe (la prueba existente busca `price`, `total`, `amount`, `subtotal`,
  `balance`). `ready-alerts` se atiende después de `authenticateDevice` y `matchClientKind` y antes de la sesión, solo
  `GET` y solo la ruta exacta; la credencial de envío se rechaza antes; un no `android` recibe 403; un revocado, 401
  (al abrir) o «interrupted» (en el canal). Un parámetro de más o un cursor inválido, 400.
- **Día comercial y reloj**: `businessday` con la zona del establecimiento y la «Hora de inicio del día»; Go y
  PostgreSQL usan el mismo reloj (misma computadora). `day_ends_at` es el inicio del día siguiente.
- **Poda**: solo lo de más de 2 días y solo al registrar un aviso, así que la cabeza nunca baja por la poda (un cursor
  mayor que la cabeza solo aparece con una base restaurada).
- **Migración 0026 (D)**: tabla nueva vacía, tres columnas con valor por omisión, ninguna fila cambia; las claves
  foráneas apuntan a tablas que Go nunca borra («Quitar datos de prueba» y la importación no tocan cuentas, unidades,
  versiones, usuarios ni dispositivos con avisos).
- **Canal**: un canal por dispositivo (429), revalidación en cada lectura, cierre al apagar el servidor, plazo de
  escritura de 10 s.
- **Prueba existente**: `TestStageA2ReadyAlerts` pasa en este árbol (27 s).

### No revisado o no verificado

- El consumo real de batería con el ritmo de R4 (solo medí el tráfico del lado de Go) y el canal con muchos
  dispositivos a la vez bajo carga real.
- R7, R8, R10, R11 y R12 no tienen reproducción (lectura de código; R8 depende de que la interfaz deje configurar
  estaciones de cocina de un android, que no comprobé).
- Una restauración real con `pg_dump`/`pg_restore` (R3 se simuló en la tabla; el efecto sobre el dispositivo depende de
  la Fase D).
- Escritorio (Terminales › Editar) y la regresión completa del servidor: fuera de esta parte.

### Pruebas ejecutadas

| Qué                                                                                | Dónde                                                   | Resultado                                                                 |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------- |
| `go vet -tags=integration ./internal/operations/` con la prueba nueva              | `$SP/wt/rev/backend`                                    | Sin avisos                                                                |
| `TestStageA2ReadyReview` (R1–R4)                                                   | `iso.sh go-int ./internal/operations`, `ISO_TAG=R`      | **Falla en las 4** (las reproducciones; mensajes arriba)                  |
| `TestStageA2ReadyAlerts` (la de la parte D)                                        | idem                                                    | **ok** (27 s)                                                             |
| R5: `EXPLAIN (ANALYZE, BUFFERS)` de las consultas de sesiones con 580 000 sesiones | Go temporal (`server-up` 8251/8511) y `psql` en su base | 9,5 ms y 5,9 ms por lectura; con los índices sugeridos, 0,05 ms y 0,03 ms |
| `format`, `fmt-check`; md5 de `docs/etapas/*.md` antes y después                   | `$SP/wt/rev`                                            | Correctos; idénticos                                                      |

Commit: `712e869` («etapa A2 revisión §4 (servidor): reproducciones de los hallazgos en TestStageA2ReadyReview…»), solo
`backend/internal/operations/stagea2_ready_review_test.go`. El Go temporal está detenido y su base borrada.

### Contraste con «Servidor · Parte D» del avance (leído después)

- La parte D dice haber revisado los caminos y que «las modificaciones no marcan “Listo”»: R1 es justo una
  modificación que deja la unidad «Listo» sin pasar por `saveStates`.
- La decisión 6 (cursor mayor que la cabeza → releer el día y descartar `id` ya avisados) resuelve la relectura de lo
  restaurado, pero no la reutilización de los números por los avisos nuevos (R3).
- La decisión 4 (`active` con sesión vigente o entrada de hoy) y la «Forma final» para `day_ends_at` («si sigue
  `active`, continuar») hacen que el dispositivo siga leyendo justo en el caso en que `entered` ya no admite al mesero
  (R2): la regla de entrega y la de `active` deben ser la misma.
- «No verificado: el canal con muchos dispositivos… no se midió con carga»: R4 y R5 lo cuantifican en parte.
- La «Forma final» describe «interrupted» como «el dispositivo dejó de valer»; también llega con 503 (R6).

### Para la Parte 2 (dispositivo), lo que conviene contrastar con lo anterior

- Cómo descarta repetidos (¿por `id` numérico? → R3) y cuándo borra los `id` del día; si guarda el cursor después de
  notificar (pérdida si el proceso muere entre notificar y guardar es preferible a repetir, o al revés).
- Qué hace con `interrupted` (¿lo trata como retiro? → R6) y si alguna vez usa el `id: 0` como cursor.
- Qué hace al pasar `day_ends_at` con `active: true` por una sesión vigente (R2) y si hay idas y vueltas del servicio.
- Su tiempo de «lectura sin datos» frente al ritmo del canal (si se aplica R4, debe ser mayor que el `ping`).
- Primera lectura sin cursor (R11), manejo del 429 (R12) y de una cuenta cancelada o combinada al tocar el aviso (R10).

## Parte 2 · Dispositivo

Árbol `$SP/wt/rev` con `etapa-a2-android` fusionada (`afd754e`: fases C y D; fusión `5a0205a`, conserva `712e869`).
Revisé `core:domain/ready` (`ReadyAlerts.kt`, `ReadyAlertsLifecycle.kt`, `ReadyNotice.kt`), `core:data/ready`
(cliente, canal, registro en Room y sus entidades de la v6), `app/ready` (servicio, controlador, notificaciones,
receptor de arranque e interfaz), el manifiesto, `AppContainer.kt`, `MainActivity.kt`, `AdelPosApp.kt`, el arranque de
`AccessRepository` y las pruebas (`ReadyAlertsRulesTest`, `ReadyAlertsChannelTest`, `RoomReadyAlertsLedgerTest`,
`AlertsOnGoTest`). El cliente ya usa el protocolo acordado (`c4de974`); el servidor corregido todavía no existe, así que
todo lo que depende de él se revisó contra el protocolo escrito, no contra Go. Leí el apartado «Fase D» del avance
después de formarme la opinión. Sin emulador: los hallazgos se reproducen en la JVM o se ven en el código, y lo que
solo se ve en un dispositivo físico queda como no verificado.

### Resumen

El ciclo de vida está bien resuelto en lo esencial:

- `startForeground` es lo primero de `onStartCommand`, también tras un reinicio pegajoso.
- Las dos `ForegroundServiceStartNotAllowedException` están atrapadas.
- `specialUse` está declarado con su permiso y su subtipo.
- El canal es uno por proceso.
- El cursor se guarda después de mostrar, y los repetidos se descartan por `key` en Room y en los extras de la
  notificación.
- Solo `DEVICE_REVOKED` detiene; los demás `interrupted` se reintentan y el `id` del canal nunca se usa como cursor.
- La huella fijada vale también en este canal, y el borrado del dispositivo está registrado.

No encontré un camino de pérdida ni de repetición dentro del día.

Lo importante es **cuándo se detiene**. Respondiendo a la pregunta del coordinador:

- **RD1**: una tableta que no alcanza la computadora principal pasado el fin del día sigue reintentando para siempre
  con la notificación permanente.
- **RD2**: aun con la computadora principal encendida, `active` sigue en `true` toda la noche (las sesiones con PIN
  duran 12 h y la app nunca las cierra), así que el servicio no se detiene al cerrar el día en ningún dispositivo.

Las dos contradicen §4.3 («se detiene al cerrar el día comercial… o si nadie ha entrado en él en el día»). Las demás
son bajas.

Reproducción: `android/core/data/src/test/kotlin/com/adelpos/meseros/core/data/ready/ReadyAlertsReviewTest.kt`
(commit `ed34942` de `etapa-a2-rev`). **Falla a propósito** con el código actual (como `TestStageA2ReadyReview`): no
debe fusionarse tal cual en la etapa.

### Hallazgos

| #   | Severidad | Estado                                                                                                                                                                                                                    | Dónde (rutas desde `android/`)                                                                                                                                        | Escenario de falla                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | Corrección sugerida                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| RD1 | Medio     | Confirmado (`ReadyAlertsReviewTest`)                                                                                                                                                                                      | `core/data/…/ready/ReadyAlertsChannel.kt:103-126`, `:207-217`, `:232-241`; `app/…/ready/ReadyAlertsController.kt:302-313`                                             | Lo último que dijo Go fue `active: true`. Pasa `day_ends_at` + 30 s con la Mac apagada (cierre del restaurante), la wifi caída o un teléfono que el mesero se llevó a casa. El `GET` del cambio de día falla (`RETRY`, `null`), el canal sigue con espera creciente y no hay ninguna condición de salida: **21 intentos y una búsqueda de otra dirección en 3 s, sin detenerse**. En la aplicación son un intento cada 30 s y una búsqueda NSD por minuto, toda la noche, con «AD-EL POS conectado · Sin conexión… Reintentando…». El reinicio pegajoso (`onServiceStarted`) vuelve a correr el canal sin mirar si el día terminó. En cambio, tras reiniciar el equipo no arranca (`decide(BOOT)` exige el día en curso): dos respuestas distintas para la misma situación. Es la consecuencia que el agente de la Fase D anotó en su decisión 6. | Pasado `day_ends_at` (y un margen corto, p. ej. 10 min), sin respuesta de Go **y** sin ningún mesero usando el dispositivo (ver RD2), detener el servicio sin mover el cursor. Lo vuelven a arrancar la próxima entrada con PIN con conexión o abrir la aplicación (`ASK_FIRST`), y lo de mientras llega desde el cursor. Aplicar la misma regla al reinicio pegajoso.                                                                                                                                                                                                            |
| RD2 | Medio     | Confirmado por partes (servidor `R2`: «responde active true» con una sesión de antes del inicio del día; `ReadyAlertsChannelTest.pasadoElFinDelDiaVuelveAPreguntarYSigueMientrasGoDigaActivo`); no corrido en un emulador | Servidor `administration/ready_alerts.go:157-158` (y la regla 4 del protocolo acordado); `core/data/…/ReadyAlertsChannel.kt:120-126`; regla 15 de `android/CLAUDE.md` | La app no cierra la sesión de Go («Cambiar de mesero» y el bloqueo por inactividad no lo hacen; A2-D4 quedó sin efecto) y cada sesión con PIN dura 12 h desde la última entrada. Si la última entrada fue a las 22:55, `active` vale `true` hasta las 10:55 del día siguiente. Al pasar `day_ends_at` el dispositivo pregunta, Go dice `active` y el servicio sigue. En un restaurante que abre todos los días **el servicio no se detiene nunca**: todas las tabletas pasan la noche con la notificación permanente y el canal abierto (con el protocolo nuevo, un `: ping` cada 15 s), y los teléfonos que salen del restaurante caen en RD1. La regla 4 acordada («entró» = sesión vigente o creada en el día) lo mantiene.                                                                                                                    | **Decisión del coordinador** (precisa §4.3 «un mesero con sesión en el dispositivo»), dos opciones compatibles entre sí. (a) Servidor: después del inicio del día, una sesión de antes cuenta para `active` y para la entrega solo si se usó hace poco (`last_seen_at` de los últimos 15–30 min: el canal de eventos de la A1 la refresca cada 750 ms mientras un mesero usa la app). (b) Dispositivo: pasado `day_ends_at`, seguir solo si un mesero entró o usó el modo Mesero en este dispositivo después de esa hora; si no, detenerse como en RD1. (b) también resuelve RD1. |
| RD3 | Bajo      | Plausible (lectura de código; `BOOT_COMPLETED` no se probó en la fase)                                                                                                                                                    | `app/…/ready/ReadyAlertsBootReceiver.kt:31-33, 41`; `ReadyAlertsController.kt:244-246`; `core/data/…/access/AccessRepository.kt:103-112, 314-363`                     | Tras reiniciar el equipo, `requestNow(BOOT)` tiene 8 s, pero antes de decidir llama a `prepare()` → `AccessRepository.start()` → `checkLocked()`, que con la wifi conectada **pregunta a Go**. Con la Mac apagada son hasta 5 s de conexión, la búsqueda NSD de otra dirección (unos 4 s) y otro intento: pasa de 8 s, se cancela y el servicio no arranca hasta que alguien abra la app. El comentario dice que «solo lee Room y lo guardado del emparejamiento».                                                                                                                                                                                                                                                                                                                                                                                | En `BOOT`, decidir con lo guardado (Room y el emparejamiento, `loadLocked`, sin `checkLocked`) y arrancar; la comprobación con Go la hace el canal ya en primer plano.                                                                                                                                                                                                                                                                                                                                                                                                            |
| RD4 | Bajo      | Plausible (lectura de código)                                                                                                                                                                                             | `app/…/ready/ReadyAlertsController.kt:104`, `:308-312`                                                                                                                | El canal corre en `CoroutineScope(SupervisorJob() + Dispatchers.Default)` sin `CoroutineExceptionHandler` ni `try`. Una excepción inesperada (Room lleno, `SQLiteFullException`; un `RuntimeException` de `NotificationManager`) **tumba todo el proceso**, también la captura del mesero en primer plano. El reinicio pegajoso vuelve al mismo lote y puede quedar en bucle.                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Atrapar en `onServiceStarted` todo lo que no sea `CancellationException`, registrarlo y tratarlo como `RETRY` (o detener el servicio), sin tumbar la app.                                                                                                                                                                                                                                                                                                                                                                                                                         |
| RD5 | Bajo      | Confirmado (lectura de código)                                                                                                                                                                                            | `app/…/ready/ReadyAlertsService.kt:50`; `ReadyAlertsChannel.kt:100`, `:151`                                                                                           | Sin red, cada intento pasa por `Connecting`, que la notificación permanente cuenta como conectado. Mientras dura cada intento (hasta 5 s) dice «Recibiendo avisos de platillos listos» y vuelve a «Sin conexión…» al fallar: cambia cada ≤30 s y se vuelve a publicar con `startForeground` cada vez.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | Contar como conectado solo `Connected` (o `Connecting` solo en el primer intento después de estar conectado).                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| RD6 | Bajo      | Confirmado (lectura de código)                                                                                                                                                                                            | `app/…/ready/ReadyAlertsUi.kt:175-182`, `:199-205`                                                                                                                    | «Ahora no» en la explicación de un permiso lo marca como pedido para siempre: nunca se vuelve a explicar. En Android 13+ sin el permiso de notificaciones, el servicio corre y los avisos no se ven. Solo lo dice la ayuda, y el mesero no tiene por qué abrirla.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Con «Ahora no», volver a explicarlo en el siguiente arranque (o tras un tiempo). Con las notificaciones sin permiso, un aviso visible en el modo Mesero («Los avisos de platillos listos no se ven: activa las notificaciones»).                                                                                                                                                                                                                                                                                                                                                  |
| RD7 | Bajo      | Plausible (lectura de código)                                                                                                                                                                                             | `core/data/…/ready/ReadyAlertsClient.kt:215`, `:239-246`                                                                                                              | Un mensaje `alerts` que no se puede leer (un campo obligatorio que falta, `day_ends_at` con otro formato) se **salta en silencio**. Con el protocolo nuevo Go ya avanzó su `after` y el siguiente mensaje solo trae lo nuevo, así que sus avisos se pierden.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Tratarlo como falla de la conexión (cerrar y reabrir desde el último cursor guardado), no saltarlo.                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| RD8 | Bajo      | No verificado (solo se ve en un dispositivo físico)                                                                                                                                                                       | `core/data/…/ready/ReadyAlertsClient.kt:342`; `ReadyAlertsChannel.kt:152`                                                                                             | La detección de la conexión muerta (45 s sin un byte) y la espera creciente son temporizadores sin _wake lock_ ni alarma. En un teléfono real con la pantalla apagada la CPU se suspende: un socket muerto o una espera pueden durar hasta que algo despierte el equipo. Mientras llegan `: ping` el equipo despierta solo; tras un corte no. El emulador no suspende la CPU, así que no se ve en las pruebas.                                                                                                                                                                                                                                                                                                                                                                                                                                    | Medirlo en la Fase E o en un equipo físico (corte de wifi con la pantalla apagada y cuánto tarda en volver). Si hace falta, una alarma `setAndAllowWhileIdle` para la reconexión o _keepalive_ TCP.                                                                                                                                                                                                                                                                                                                                                                               |
| RD9 | Bajo      | Plausible (diseño aceptado)                                                                                                                                                                                               | `core/domain/…/ready/ReadyAlerts.kt:160-173`; `app/…/ready/ReadyAlertNotifications.kt:130-153`                                                                        | Mostrar y después guardar es lo correcto (decisión 1 de la fase). El reconocimiento tras morir el proceso usa la notificación a la vista: si el mesero la descartó, o el aviso fue dentro de la aplicación, y el proceso muere antes de guardar, el aviso se repite una vez.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Aceptable; documentarlo junto a la decisión 1.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

### Revisado sin encontrar defectos

- **Arranque y exenciones (API 26 a 37)**:
  - `startForegroundService` desde primer plano (entrada con conexión, aplicación abierta) o con exención
    (`BOOT_COMPLETED`, `MY_PACKAGE_REPLACED`), con `IllegalStateException`/`SecurityException` atrapadas.
  - `startForeground` primero, con el tipo `SPECIAL_USE` desde la API 34 y 0 antes.
  - `START_STICKY` con `intent == null`; `onTaskRemoved` no detiene nada.
  - `onDestroy` quita la notificación (`STOP_FOREGROUND_REMOVE`) y cancela el canal.
  - No vi un camino que deje la notificación sin servicio: `refreshOngoing` exige el canal vivo y corre en el hilo
    principal, como `onDestroy`.
- **Parada**:
  - con `active: false`, con el retiro (`DEVICE_REVOKED` → `report` → `retireLocked` → `deviceDataWipe` →
    `eraseDeviceData`), con «Emparejar de nuevo» o al actualizar (estado no `Ready` → `stopService`);
  - un pedido de arranque mientras se detiene se recuerda y no se pierde.
- **Reconexión**:
  - espera de 2 a 30 s, acortada al volver la red o al entrar un mesero;
  - `after` y `Last-Event-ID` desde el cursor del último lote procesado;
  - 45 s sin un byte frente a `: ping` cada 15 s;
  - el cierre normal a los 5 min se reabre enseguida y uno inmediato cuenta como falla;
  - 429 lee con `GET` mientras tanto;
  - tres fallas piden `recheck` (otra dirección; `DEVICE_REQUIRED` termina en «Emparejar de nuevo» por la lista de
    meseros);
  - 426 y otra huella informan a la entrada y se detienen;
  - huella fijada (`PinnedHttpClient` del emparejamiento).
- **Sin duplicados ni pérdidas dentro del día**:
  - `distinctBy(key)` en el lote, `unseen` por `key` en Room y un `commit` transaccional de las `key`, el cursor y el
    estado;
  - mostrar antes de guardar, con los extras de la notificación para el proceso que muere a la mitad;
  - un solo proceso y un solo canal (`loop?.isActive`);
  - la consulta previa nunca mueve el cursor si está activo;
  - `seenKeys` con `IN` de como mucho 50 grupos (lejos del límite de 999 variables de SQLite en Android 8);
  - los avisados de días anteriores se borran al cambiar `day_ends_at`.
- **Convivencia con la A1**:
  - regla 22: el canal de eventos no cambia y el servicio no lo abre;
  - la cola solo recibe `retryNow` al reconectar (mismo `REPLACE` de la A1; la clave de operación impide duplicados);
  - reglas 15–19 sin cambios; la regla 17 se cumple en los diálogos nuevos (`reportsUserActivity`);
  - sin secretos nuevos (la credencial sale de `AccessRepository`, sellada con el Keystore); Room v6 con sus tablas al
    final de 5 → 6 y su caso de migración.
- **Canales, texto y toque**:
  - A2-D7 con un solo canal visible y el anterior borrado;
  - A2-D10 con singular y plural;
  - una notificación por cuenta con el acumulado y un resumen silencioso;
  - pantalla de bloqueo pública sin precios;
  - el toque abre la cuenta, deja elegido a su mesero en «¿Quién eres?», sigue una cuenta combinada y dice «Esta
    cuenta ya no está abierta»;
  - aviso dentro de la aplicación con la cuenta a la vista.
- **Contraste con la Parte 1**:
  - R3: repetidos por `key`, no por `id`;
  - R6: solo `DEVICE_REVOKED` detiene y el `id: 0` nunca es cursor;
  - R4: 45 s frente a `: ping`;
  - R12: el 429 se lee con `GET`;
  - R10: el toque sigue la combinación;
  - R2 y R11 dependen del servidor corregido.

### No revisado o no verificado

- Contra el **servidor corregido**: no existe todavía; falta la suite `alerts` (y la nueva medición de batería) cuando
  se fusione.
- `BOOT_COMPLETED` real con esta aplicación (RD3), Android 8 y `Pixel_8` en esta fase (la matriz es de la Fase E).
- Doze real, suspensión de la CPU y gestores de batería de cada marca (RD8).
- No abrí ningún emulador: lo que encontré se reproduce en la JVM o se lee en el código.

### Pruebas ejecutadas

| Qué                                                                               | Dónde                                  | Resultado                                                                                                                                                                                            |
| --------------------------------------------------------------------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `:core:data:testDebugUnitTest --tests 'com.adelpos.meseros.core.data.ready.*'`    | `$SP/wt/rev`, `ISO_TAG=R`, `--offline` | `ReadyAlertsChannelTest` 13/13, `RoomReadyAlertsLedgerTest` 4/4; **`ReadyAlertsReviewTest` falla** (RD1: «seguía reintentando a los 3 s: 21 intentos, 1 búsquedas de otra dirección, sin detenerse») |
| `:core:domain:test --tests 'com.adelpos.meseros.core.domain.ready.*'`             | ídem                                   | `ReadyAlertsRulesTest` 18/18                                                                                                                                                                         |
| `ktfmtFormat` (`core:data`), `ktfmtCheck`, `fmt-check`; md5 de `docs/etapas/*.md` | ídem                                   | Correctos; idénticos                                                                                                                                                                                 |

Commit: `ed34942` («etapa A2 revisión §4 (dispositivo): reproducción RD1 en ReadyAlertsReviewTest…»), solo el archivo de
prueba. Sin emuladores ni Go temporal en esta parte; árbol limpio.

### Decisiones del coordinador sobre la revisión independiente

- **Parte 1 (servidor)**: R1 a R12 se corrigieron con el protocolo acordado (`$SP/briefs/correcciones-R-servidor.md`):
  clave `key` por aviso para descartar repetidos (el `id` queda solo como cursor), canal que solo envía con novedades,
  `: ping` cada 15 s y cierre a los 5 minutos, un canal nuevo que reemplaza al anterior (sin 429), `interrupted` con
  código, presencia del mesero y avisos solo posteriores a su entrada en el dispositivo, destinatario fijado solo entre
  dispositivos que pueden recibirlo, índices parciales de `sessions` y la unidad que queda lista porque se canceló la
  parte pendiente. R9 (un corte que cruza la hora de inicio del día) queda como regla del día comercial. Las
  reproducciones del revisor (`TestStageA2ReadyReview`) pasan y se quedan como pruebas de regresión.
- **A2-D12 (RD1 y RD2, precisa §4.3)**: una sesión con PIN creada **antes** del inicio del día comercial en curso solo
  cuenta (para `active`, para «entró en el dispositivo» y para fijar el dispositivo) si sigue vigente y se usó en los
  últimos **30 minutos**; las creadas desde el inicio del día cuentan como siempre. Así se cubre al mesero que trabaja
  justo en el cambio de día sin que las sesiones de 12 h mantengan el servicio vivo toda la noche. En el dispositivo,
  pasado `day_ends_at` + 10 minutos, si no alcanza a Go y ningún mesero lo está usando, el servicio **se detiene sin
  mover el cursor**; lo reabren la siguiente entrada con PIN con conexión o abrir la app, y lo de mientras llega desde
  el cursor. Así se cumple «se detiene al cerrar el día comercial» sin perder avisos.
- **Parte 2 (dispositivo)**: RD1 y RD3 a RD7 se corrigen en la Fase D (sección siguiente); RD8 (temporizadores sin
  _wake lock_ en un equipo real) queda como no verificado sin dispositivo físico y RD9 (una repetición única si el
  proceso muere entre mostrar y guardar un aviso ya descartado) como diseño aceptado.
- Verificación del coordinador de A2-D12 sobre una copia exacta de `b91ae66`: `fmt-check`, `sync-check`, `go-unit`,
  `go-int` de `apicontract`, `database` y `kitchen` completos, `TestStageA2Ready*` (`operations`) y
  `TestStageTwoAcceptance` y `TestStageA1WaiterPIN` (`administration`), todo aprobado.
