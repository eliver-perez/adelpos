# Administración de caja, arqueos y cortes · Etapa 7

Estado: **fase A** (migración 0008, modelo y backend con pruebas de integración). Las pantallas (fase B), la impresión de comprobantes y cortes, el cajón de dinero, el tope de caja (fase C) y la entrega final (fase D) completarán este documento. Solo se describe lo implementado y probado; las decisiones del propietario están en [etapas/ETAPA_7.md](etapas/ETAPA_7.md) §0-B y el avance en [ETAPA_7_AVANCE.md](ETAPA_7_AVANCE.md). Contratos HTTP: [API_CONTRACTS.md](../API_CONTRACTS.md) (sección «Caja, arqueos y cortes»).

## Autoridad y estructura

- Todo vive en `backend/internal/payments` (autoridad financiera), en archivos `cash_*.go`: modelo y permisos (`cash_model.go`), acceso y estados (`cash_access.go`), efectivo esperado y diferencias (`cash_expected.go`), conteos por denominaciones (`cash_counts.go`), ingresos, retiros y gastos (`cash_operations.go`), correcciones y regularizaciones (`cash_corrections.go`), cortes (`cash_reports.go`), arqueo y cierre (`cash_closing.go`), consultas e historial (`cash_history.go`), catálogo (`cash_settings.go`) y rutas (`cash_routes.go`). La apertura ampliada sigue en `shifts.go`.
- Rutas bajo `/api/v1/payments/…`, delegadas desde `administration/routes.go` con la misma transacción, `Actor`, autorizador de supervisor y auditoría. Toda escritura toma el lock 7820148305 exclusivo (el mismo de cobros), guarda su resultado idempotente en `restaurant_operations`, audita y publica su evento en la misma transacción. Un cobro y el cierre de su turno nunca se intercalan.
- El puente nativo (`desktop/src-tauri/src/api.rs`) agrega las rutas a su lista cerrada, con un solo identificador por ruta; los tipos y consultas para la interfaz están en `desktop/src/services/cash.ts`.

## Modelo (migración 0008)

| Tabla / cambio                                 | Contenido                                                                                                                                                                                                                                                           |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cash_shifts`                                  | Estado `open`, `closing` («Cierre en curso») o `closed`; fondo en dólares opcional, observaciones, fecha comercial, quién/cuándo/dónde inició el arqueo y cuántos registros tenía, terminal del cierre. Índice único de turno activo (`open` o `closing`) por caja. |
| `cash_denominations`                           | Catálogo por moneda: MXN billetes 1000–20 y monedas 20–0.50; USD billetes 100–1 (monedas de dólar no sembradas, se agregan con permiso). Valor, moneda y tipo inmutables; solo se activan o desactivan.                                                             |
| `cash_counts`, `cash_count_lines`              | Conteos por denominaciones: el desglose de apertura (final) y el arqueo de cierre (borrador en el servidor hasta confirmar). Importe adicional identificado, nunca negativo.                                                                                        |
| `cash_expense_categories`                      | Insumos, Servicios, Mantenimiento y Otros; editables con orden por posiciones.                                                                                                                                                                                      |
| `cash_operations`                              | Ingresos, retiros, gastos, correcciones y regularizaciones inmutables, por moneda, con dirección física, autorización (`self`/`supervisor`), operación original enlazada y tipo de cambio del turno para las de dólares.                                            |
| `cash_shift_reports`                           | Cortes parciales (varios por turno) y el corte definitivo (uno), con snapshot de totales.                                                                                                                                                                           |
| `cash_shift_closures`, `cash_closure_comments` | Resultado histórico del cierre por moneda (esperado, contado, diferencia, resultado, equivalencia) y comentarios posteriores del supervisor.                                                                                                                        |
| `payment_settings.cash_limit_*`, `cash_limit`  | Tope de caja (fase C): columnas en NULL y función activable sembrada desactivada (aún no registrada en `internal/features`).                                                                                                                                        |
| `cash_drawer_settings`, `cash_drawer_events`   | Cajón de dinero (fase C): solo esquema.                                                                                                                                                                                                                             |
| `cash_documents`, `kitchen_tickets.kind`       | Comprobantes y cortes impresos (fase C): tabla y tipos `cash_report`/`cash_voucher` en la cola durable.                                                                                                                                                             |
| `realtime_events.scope`                        | Ámbito `cash`.                                                                                                                                                                                                                                                      |

Restricciones con nombre explícito e `IS NOT NULL` en las ramas con columnas opcionales. `cash_shifts_closed_fields_check` (responsable y terminal del cierre) se creó `NOT VALID` para no fallar sobre datos históricos inesperados: rige para toda fila nueva o modificada y Go también la cumple. Los nombres heredados (`cash_shifts_state_check`, `cash_shifts_one_open_idx`, `kitchen_tickets_kind_check`, `realtime_events_scope_check`) se verificaron en una base temporal migrada. La 0008 es repetible y se probó sobre una base en esquema 7 con un turno abierto, que conserva sus datos.

## Apertura y fondos iniciales

- `POST payments/shifts` conserva el contrato anterior y agrega `opening_float_usd`, `opening_notes` y `count` (desglose opcional). El fondo en pesos es obligatorio y puede ser $0; con $0 no se pide desglose. Si viene el desglose, Go calcula el total por moneda y exige que coincida con los fondos declarados (`COUNT_TOTAL_MISMATCH`). El fondo y el desglose en dólares exigen la función `usd_cash`.
- La fecha comercial es la fecha de apertura en la zona horaria del establecimiento, calculada en la misma sentencia que fija `opened_at`. Los turnos anteriores (sin fecha guardada) la calculan con `opened_at` en la zona vigente. Los movimientos conservan su fecha y hora reales.
- Un segundo turno activo en la misma caja se rechaza (`SHIFT_ALREADY_OPEN`), también en una carrera entre terminales: el lock serializa y el índice único de turno activo lo garantiza; una violación de ese índice se traduce al mismo código.
- Un turno en arqueo sigue activo: no se abre otro, la caja no se desactiva y sus terminales no cambian de caja (`administration/registers.go` y `devices.go`).

## Turnos compartidos y responsables

Se respeta `payment_settings.shared_shifts`: sin turno compartido, solo quien abrió registra cobros y operaciones de efectivo (`SHIFT_NOT_SHARED`). Cada operación conserva su usuario, terminal y caja. Las operaciones se registran solo desde una terminal asociada a la caja del turno (`SHIFT_NOT_IN_REGISTER`); el identificador del turno viaja en la ruta, de modo que el reintento de una operación incierta nunca cae en otro turno.

## Ingresos, retiros y gastos

- **Ingreso** (`cash.income.record`): sin segunda contraseña; tipos cerrados (aportación al fondo, cambio del encargado, reintegro, otro). No es una venta.
- **Retiro** (`cash.withdrawals.record`) y **gasto** (`cash.expenses.record`, con categoría activa y referencia opcional): siempre autorizados. Si quien registra tiene `cash.outflows.authorize`, su propio permiso basta (`authorization_mode: "self"`); si no, la autorización puntual de **otra** persona con ese permiso (`"supervisor"`), con el autorizador de la etapa 6 (límite de intentos, sin sesión, sin autoautorización). Un gasto no crea compras ni inventario.
- Importes positivos con dos decimales. Una salida que excede el efectivo esperado de su moneda se rechaza con `CASH_INSUFFICIENT` y un mensaje sin cifras («El importe no puede autorizarse con el efectivo de la caja; pide a un supervisor que lo revise»). La autorización se valida antes de comparar con el disponible. Una salida **excepcional** (`exceptional: true`, motivo de 5–500) exige que quien autoriza tenga además `cash.expected.read`; si no excede, se registra como ordinaria.
- Dólares: con `usd_cash` activa y el tipo de cambio del turno; cada operación guarda ese tipo de cambio para su equivalencia en pesos.

## Correcciones y regularizaciones

- `cash.corrections.record` + autorización con la misma regla (`cash.corrections.authorize` propio o de otra persona). Registrar ingresos no implica poder corregirlos sin autorización.
- La corrección es una operación nueva que invierte exactamente la original (misma moneda e importe, dirección opuesta); la original no se modifica y solo se corrige una vez. Una corrección no se corrige. Para cambiar un importe se corrige la original y se registra una nueva.
- Original en el mismo turno abierto: actualiza el esperado del turno. Original en un turno **cerrado** de la misma caja: es una **regularización** en el turno vigente, enlazada y con motivo; no cambia la diferencia registrada del cierre ni el efectivo esperado del turno vigente (no movió dinero en él), por lo que no sirve para ocultar diferencias. La consulta del cierre lista sus regularizaciones aparte. No existe «ajuste de diferencia».

## Cálculo del efectivo esperado

Una sola función (`cash_expected.go`), usada por movimientos, cortes, cierre y validación de salidas, a partir de registros reales y por moneda, sin convertir:

- Pesos = fondo en pesos + Σ(pesos que entran − pesos que salen) de los cobros, devoluciones y reversos en efectivo (un movimiento anterior a la 0007 cuenta su neto) + ingresos y correcciones que entran − retiros, gastos y correcciones que salen en pesos.
- Dólares = fondo en dólares + Σ(dólares que entran − salen) de los cobros + operaciones en dólares.
- Las regularizaciones no cuentan. Tarjeta, transferencia y plataformas no mueven efectivo. El cambio nunca suma dos veces (se cuentan lo recibido y lo entregado físicamente).
- Equivalencia: dólares × tipo de cambio fijo del turno, redondeado a centavos mitad alejándose de cero; el total equivalente nunca sustituye las cantidades físicas.

`GET payments/shifts/{id}/movements` conserva su contrato, pero `expected_cash` (ahora: pesos físicos esperados, incluidas las operaciones de efectivo) y `by_currency` solo se incluyen con `cash.expected.read`.

## Cortes parciales

- `cash.partial_cuts.create`. No cierran el turno, no reinician acumulados, no mueven dinero, no pausan la caja ni generan movimientos. Varios por turno, idempotentes.
- El snapshot guarda datos de control, totales **acumulados** (cobros, devoluciones, correcciones de pago, operaciones de efectivo, ventas, propinas y devoluciones por método, descuentos y cortesías vigentes, ingresos/retiros/gastos/correcciones por moneda, regularizaciones) y **desde el corte anterior** (acumulado actual menos el anterior: ningún movimiento cuenta dos veces ni se pierde aunque su transacción empezara antes del corte), y siempre el efectivo esperado en el bloque `sensitive`.
- Descuentos y cortesías pertenecen a la cuenta, no al turno: el corte atribuye los vigentes registrados desde terminales asociadas a la caja desde la apertura del turno.
- La consulta (`GET payments/cash-reports/{id}`) omite `sensitive` sin `cash.expected.read` y a todos durante el arqueo; la respuesta de creación nunca lo incluye.

## Arqueo y cierre ciego

1. `POST shifts/{id}/closing` (`cash.counts.perform`): el turno pasa a `closing` con quién, cuándo (`clock_timestamp()` tras el lock) y desde qué terminal, guarda cuántos movimientos y operaciones tenía y crea el borrador del conteo. La caja queda pausada: cobros, devoluciones, correcciones de pago, ingresos, retiros, gastos, correcciones, cortes parciales y tipo de cambio responden 409 `SHIFT_CLOSING` («La caja X está en arqueo de cierre…»). No se mantienen bloqueos de base de datos durante el conteo.
2. `PUT shifts/{id}/closing/count`: borrador en el servidor con revisión (`COUNT_CHANGED`), por denominaciones más importe adicional identificado y observaciones. Solo devuelve lo declarado.
3. `POST shifts/{id}/closing/cancel`: vuelve a `open`; cancelar un arqueo ajeno exige `cash.closing.cancel_any`. Un arqueo nuevo vacía el borrador anterior.
4. `POST shifts/{id}/closing/confirm` (`cash.shifts.close`, con `count_revision`): bajo el lock exclusivo verifica que el turno siga en arqueo, que el conteo sea el revisado y que no haya registros posteriores al inicio (por cantidad y por fecha; `CLOSING_MOVEMENTS_CHANGED`); calcula esperado y diferencias por moneda, guarda el corte definitivo y el cierre, fija el conteo y cierra el turno (`closed_at`, `closed_by`, `closed_device_id`). Dos confirmaciones simultáneas: una cierra y la otra recibe `SHIFT_CLOSED`.

Durante `closing`, ninguna respuesta (turno, movimientos, operaciones, cortes, arqueo, historial, cajas) contiene el efectivo esperado ni diferencias, ni siquiera para el administrador. Las respuestas de escritura nunca los incluyen y la auditoría tampoco los registra. Las cuentas abiertas no impiden cerrar.

## Diferencias y resultado del cierre

- Diferencia = contado − esperado por moneda; `over` (sobrante), `short` (faltante) o `even`. Una diferencia en dólares permanece en dólares; su equivalente en pesos (diferencia en pesos + diferencia en dólares × tipo de cambio del turno) solo informa. Las diferencias no bloquean el cierre ni se convierten en ajustes; se conservan en `cash_shift_closures` con CHECK que verifican la aritmética.
- `GET shifts/{id}/closure`: quien cerró ve lo que declaró, sus observaciones y que el turno quedó cerrado; esperado, diferencias y comentarios del supervisor solo con `cash.expected.read` (los comentarios también con `cash.closures.review`). Otros usuarios necesitan `cash.history.read`.
- `POST shifts/{id}/closure/comments` (`cash.closures.review`): comentarios posteriores auditados (sin copiar el texto en la auditoría) que no cambian la diferencia.

## Operaciones inciertas

El servidor es la fuente de verdad: una operación pertenece al turno si su transacción se confirmó. Un reintento con el mismo `operation_id` de una operación confirmada devuelve su resultado sin duplicarla; uno que no se confirmó recibe `SHIFT_CLOSING` o `SHIFT_CLOSED` y debe registrarse como operación nueva en un turno abierto. Las operaciones de efectivo llevan el turno en la ruta; el cobro y la devolución aceptan `shift_id` opcional en el cuerpo (parte de la huella) con el mismo efecto (`SHIFT_CHANGED` si la caja ya tiene otro turno). La interfaz de la fase B lo enviará y explicará el rechazo con el almacén global de pendientes.

## Permisos

Nuevos (registro de Go y 0008, concedidos solo al administrador): `cash.shifts.read_all`, `cash.income.record`, `cash.withdrawals.record`, `cash.expenses.record`, `cash.outflows.authorize`, `cash.corrections.record`, `cash.corrections.authorize`, `cash.movements.read`, `cash.partial_cuts.create`, `cash.counts.perform`, `cash.shifts.close`, `cash.closing.cancel_any`, `cash.history.read`, `cash.expected.read`, `cash.closures.review`, `cash.reports.print`, `cash.reports.reprint`, `cash.drawer.open`, `cash.settings.manage`. Se conservan `cash.shifts.open` y `cash.shifts.read`. Todo permiso se valida en Go antes de la primera escritura.

## Auditoría y eventos

Se auditan apertura (`cash.shift_opened`), operaciones (`cash.operation_recorded`), correcciones (`cash.operation_corrected`, `cash.operation_regularized`), autorizaciones fallidas (las del autorizador), cortes (`cash.partial_cut_created`), arqueo (`cash.closing_started`, `cash.closing_count_saved`, `cash.closing_cancelled`), cierre (`cash.shift_closed`), comentarios y catálogo. Sin contraseñas, hashes ni importes esperados. Eventos SSE en el ámbito `cash` (solo invalidaciones, sin importes) para quien tiene `cash.shifts.read` o `payments.read`.

## Pruebas ejecutadas (fase A)

- Integración PostgreSQL en `backend/internal/operations/cash_*_test.go`, encadenadas a `TestRestaurantAcceptance` (C1–C25): apertura con fondo $0, positivo y por denominaciones (total que no coincide), dos aperturas simultáneas, reinicio, turno compartido y responsables, ingresos, retiros y gastos con todas las variantes de autorización, exceso sin cifras y excepcional, correcciones, cortes parciales con y sin permiso, arqueo ciego, rechazos `SHIFT_CLOSING`/`SHIFT_CLOSED`, reintentos inciertos, borrador y cancelación, movimiento posterior detectado, cierre con faltante, sin diferencia y con sobrante/faltante por moneda, confirmación única, regularización, MXN/USD, historial, fecha comercial que cruza medianoche y catálogo.
- `backend/internal/database/existing_install_test.go`: actualización de una base en esquema 7 y CHECK de operaciones y cierres.
- Unitarias en `backend/internal/payments/cash_expected_test.go` y `cash_counts_test.go` (esperado por moneda, equivalencias, diferencias, totales desde el corte anterior, denominaciones y permisos de la 0008). Prueba Rust de las rutas del puente y Vitest de `ShiftOverview`.

## Limitaciones reales (fase A)

- Sin pantallas de caja (fase B): la interfaz existente solo se ajustó para no mostrar el esperado cuando Go lo omite y para explicar la pausa del arqueo. El cobro aún no envía `shift_id`.
- Sin impresión de comprobantes ni cortes, cajón de dinero ni tope de caja (fase C; solo esquema). `cash_limit` no está registrada en `internal/features`, por lo que no aparece en Configuración.
- Un usuario con permiso de autorizar salidas, pero sin `cash.expected.read`, podría inferir por tanteo si un importe excede el disponible (cada rechazo no revela cifras ni queda auditado).
- Las filas individuales de movimientos y operaciones se muestran a quien tiene su permiso de consulta aunque no tenga `cash.expected.read`; los totales que equivalen al esperado se omiten.
- `config/product.json` conserva `schema_version: 7` hasta el versionado de la fase D (0.7.0); la 0008 no se ha aplicado a la instalación real.
