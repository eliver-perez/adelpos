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
