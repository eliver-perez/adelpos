# Cobros, pagos, descuentos y devoluciones · Etapa 6

Versión 0.6.0, esquema 6. Este documento describe únicamente lo implementado y probado. La administración completa de caja y cortes pertenece a la etapa 7.

## Autoridad y estructura

- `backend/internal/payments` es la autoridad de saldos, pagos, asignaciones a unidades, cambio, descuentos manuales, cortesías, cargos, propinas, IVA, devoluciones, correcciones, turno mínimo, movimientos, documentos comerciales y entregas. `catalog` sigue siendo la única autoridad de precios: los pagos reutilizan los importes históricos de `restaurant_item_units` y nunca los recalculan.
- Todas las rutas viven bajo `/api/v1/payments/` y se delegan desde `administration/routes.go` con la misma transacción, sesión, terminal, permisos efectivos y auditoría que el resto del sistema.
- Toda escritura toma primero el lock operativo `7820148305` (el mismo de transferencias y combinaciones) y registra su resultado idempotente en `restaurant_operations` en la misma transacción. No se agregaron locks nuevos.
- Los registros financieros son inmutables: pagos, asignaciones, devoluciones, movimientos y ajustes nunca se editan ni se borran; las correcciones son registros compensatorios enlazados.

## Modelo de datos (migración 0006)

| Tabla                                                  | Contenido y restricciones principales                                                                                                                                          |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `payment_settings`                                     | Tasa de IVA (`CHECK tax_rate IN (0,8,16)`, 16 inicial), turno compartido, impresión de ticket predeterminada, impresora de tickets, umbral de autorización de devoluciones.    |
| `payment_methods`                                      | Métodos configurables; cinco iniciales con `code` estable (`cash`, `debit`, `credit`, `transfer`, `platforms`). Un método de efectivo no admite referencia.                    |
| `cash_shifts`                                          | Turno mínimo con fondo inicial ≥ 0; índice único parcial: un turno abierto por caja.                                                                                           |
| `payments`                                             | Venta aplicada (`amount > 0`), propina, efectivo recibido, cambio (`cambio = recibido − importe − propina`), referencia, caja, turno, usuario, terminal, `operation_id` único. |
| `payment_allocations`                                  | Importe asignado a cada unidad en un pago por selección.                                                                                                                       |
| `payment_refunds`, `payment_refund_allocations`        | Devoluciones y reversos de corrección por pago original, con desglose de IVA congelado y reverso de asignaciones.                                                              |
| `account_adjustments` (+ asignaciones y cancelaciones) | Descuentos manuales por partida o generales, cortesías y cargos generales; la cancelación es un registro aparte.                                                               |
| `payment_authorizations`                               | Autorización puntual de supervisor ligada a una operación (`operation_id` único, `requested_by <> authorized_by`).                                                             |
| `role_discount_limits`                                 | Límite máximo de descuento por rol (administrador 100 %).                                                                                                                      |
| `cash_movements`                                       | Movimientos de turno firmados para la etapa 7: venta, propina, neto, efectivo recibido y cambio.                                                                               |
| `takeaway_deliveries`                                  | Una entrega por cuenta para llevar (clave primaria por cuenta).                                                                                                                |
| `payment_documents`                                    | Precuentas y tickets ligados a `kitchen_tickets` y su trabajo en la cola de impresión.                                                                                         |

Cambios a tablas existentes, sin modificar 0001–0005: `restaurant_accounts.state` admite `closed` (+ `closed_at`); `restaurant_item_versions.tax_rate` (16 para versiones existentes; obligatoria en nuevas); `kitchen_tickets.kind` admite `receipt` y `precheck`. 21 permisos nuevos concedidos solo al administrador.

## Saldo y estados

- **Total** = unidades activas (con promociones históricas) − descuentos manuales vigentes + cargos vigentes.
- **Pagado** = pagos − devoluciones (incluidos reversos de corrección). **Propinas** se acumulan aparte y no forman parte del total ni de la base gravable.
- **Saldo** = total − pagado. Ninguna operación deja un saldo negativo.
- `payment_state`: `not_recorded`, `partial` o `paid`. Una cuenta con productos y total cero por cortesía queda `paid` sin pago ficticio. `delivery_state`: `not_delivered` o `delivered`. La preparación sigue siendo independiente.
- Una cuenta de salón liquidada y sin borrador se cierra (`closed`) y libera la mesa. Una cuenta para llevar liquidada permanece abierta en el tablero hasta entregarse; al entregarse se cierra. Una devolución que deja saldo reabre la cuenta.

## Turno mínimo

- Cada cobro y devolución exige que la terminal tenga caja física asignada y un turno abierto en esa caja. Nunca se abre un turno automáticamente.
- `POST payments/shifts` abre el turno de la caja de la terminal con fondo inicial obligatorio (se permite `0`). Un segundo turno abierto en la misma caja se rechaza (servicio e índice único).
- `shared_shifts = false` (inicial): solo quien abrió el turno cobra en él. Con `true`, varios usuarios cobran en el mismo turno; cada movimiento conserva su usuario.
- Una caja con turno abierto no se desactiva y su terminal no cambia de caja hasta el cierre del turno, que pertenece a la etapa 7.

## Cobro

- **Total**: se propone el saldo (o el total de la selección). **Parcial**: el cajero captura el abono; nunca puede exceder lo pendiente.
- **Mixto**: pagos sucesivos con distintos métodos; cada uno es un registro con método, usuario, terminal, caja, turno y referencia.
- **Por selección**: `selections: [{version_id, quantity}]`. Go elige unidades con importe pendiente (primero las de pago parcial, luego por ordinal) y reparte el importe en proporción a lo pendiente de cada una con un repartidor exacto en centavos. Una selección no puede superar el saldo de la cuenta (por ejemplo, si existen abonos generales).
- **Efectivo y cambio**: el importe aplicado y el efectivo recibido son distintos. Ejemplo: abono de $100 con $200 recibidos → aplicado $100, cambio $100, saldo restante. El movimiento de caja guarda recibido y cambio, pero el ingreso neto es solo venta + propina.
- **Métodos electrónicos**: sin cambio ni sobrepago; el pago es una operación registrada por el cajero a partir de una confirmación externa, no un cobro bancario automático. La referencia puede ser no admitida, opcional u obligatoria según el método.
- **Propina**: opcional, explícita, solo si el método la permite y el usuario tiene `payments.tips.record`. El excedente nunca se convierte en propina por sí solo.
- **Concurrencia**: el registro exige `expected_balance` igual al saldo vigente; si otra terminal cobró antes, responde `409 BALANCE_CHANGED`. El lock exclusivo serializa los cobros sobre la misma cuenta.
- **Idempotencia**: repetir el mismo `operation_id` con el mismo cuerpo devuelve el pago ya registrado; con otro cuerpo, `409 IDEMPOTENCY_CONFLICT`.
- `POST payments/accounts/{id}/preview` devuelve el plan calculado por Go (aplicado, cambio, saldo después, total de selección y asignaciones) sin escribir.

## Reglas sobre partidas con pago

- Una unidad con pago neto asignado (liquidada o parcial) o con descuento manual vigente no se transfiere, divide, combina, cancela, edita ni anula. Para cambiarla primero se revierte su pago con una devolución (o se cancela el descuento); después sigue las reglas normales.
- Las unidades sin pago siguen las reglas de las etapas 4 y 5, con una condición: la cuenta nunca puede quedar con total menor que lo pagado (`409 ACCOUNT_BELOW_PAYMENTS`).
- Transferir o cancelar una cantidad toma primero unidades libres.
- La cuenta de origen de una combinación no puede tener pagos registrados ni ajustes vigentes; la de destino sí. Cada pago permanece en la cuenta donde se registró.
- Agregar productos después de un pago está permitido: aumenta el saldo sin tocar pagos anteriores.
- Anular una cuenta exige que todo lo cobrado ya se haya devuelto y no tenga ajustes vigentes.

## Descuentos, cortesías y cargos

- Descuento por partida (unidades sin pago ni descuento previo) o general (sobre el total vigente), por porcentaje o importe fijo, hasta 100 %. El importe se calcula en Go, se reparte entre las unidades elegidas y nunca supera el saldo pendiente.
- **Cortesía**: descuento del 100 % marcado como cortesía. Liquida la cuenta sin registrar un pago de $0 y conserva productos, preparación e importe descontado.
- **Límites por rol**: cada rol puede tener un límite máximo (`role_discount_limits`); sin límite, 0 %. El límite se administra con el campo opcional `discount_limit` de los roles y sigue la delegación segura: nadie asigna, ni a sí mismo ni a otros, un límite mayor que el suyo.
- Si el descuento supera el límite del usuario, Go responde `403 AUTHORIZATION_REQUIRED`; el cliente reenvía la operación con `authorization: {username, password}` de un supervisor con `payments.discounts.authorize` (o `payments.courtesy` para cortesías) y límite suficiente. Si el usuario ya tiene permiso y límite, no se pide contraseña.
- **Cargos generales**: concepto, importe positivo, motivo y responsable; forman parte de la venta, llevan la tasa de IVA vigente y son independientes de los extras de productos. Crearlos exige `payments.charges.create` (no hay vía de autorización de supervisor para crearlos). Su cancelación requiere `payments.charges.manage` (o autorización) y queda auditada; cancelar un descuento requiere `payments.discounts.apply` (o autorización).
- Los ajustes solo se registran o cancelan con la cuenta abierta. Una cuenta cerrada por una cortesía o un descuento total no se reabre cancelando el ajuste; la única reapertura es una devolución que deja saldo.

## Autorización puntual de supervisor

- Se valida en Go, en la misma transacción de la operación, contra los permisos efectivos actuales del supervisor (rol + excepciones) y su límite de descuento.
- No crea ni reemplaza sesiones: el supervisor conserva su sesión en su terminal.
- Comparte el límite de intentos del inicio de sesión: cinco fallos en quince minutos por supervisor y terminal bloquean nuevas autorizaciones (`429 AUTHORIZATION_THROTTLED`). Los fallos se auditan sin contraseña.
- Un usuario no puede autorizarse a sí mismo (`403 SELF_AUTHORIZATION`); un supervisor sin permiso o límite suficiente recibe `403 AUTHORIZATION_INSUFFICIENT`.
- Es de un solo uso: queda en `payment_authorizations` ligada a `operation_id`, cuenta e importe. La contraseña no forma parte de la huella idempotente, por lo que un reintento puede enviarse sin ella y recupera el resultado ya confirmado. Si la operación original no llegó a confirmarse, el reintento sin contraseña recibe `403 AUTHORIZATION_REQUIRED` (igual que una autorización ausente o incompleta) y el supervisor debe autorizar de nuevo.

## IVA

- Los precios ya incluyen IVA; el impuesto se desglosa, nunca se suma. Tasas permitidas: **16 %**, **8 %** y **0 %**, elegidas desde una lista cerrada (`PUT payments/tax` con `"16"`, `"8"` o `"0"`; cualquier otro valor se rechaza en Go y en PostgreSQL). Cambiarla requiere `payments.tax.manage` y se audita con valor anterior y nuevo.
- Cada versión confirmada copia la tasa vigente; una edición que conserva importes conserva su tasa histórica. Cambiar la tasa solo afecta lo que se confirme después.
- **Nivel de cálculo**: por cuenta, agrupando por tasa. Para cada tasa, base = total ÷ (1 + tasa) redondeada a centavos e IVA = total − base, de modo que base + IVA es exactamente el total. Calcularlo sobre la suma por tasa evita acumular redondeos unidad por unidad.
- Los descuentos manuales reducen el bruto de su tasa; un descuento general se reparte entre tasas en proporción. Los cargos llevan la tasa vigente al registrarse. Las propinas no llevan IVA.
- Las devoluciones guardan su desglose (`tax_breakdown`) con las tasas de la venta original al momento de devolver.

## Devoluciones

- Modalidades: **total** (todo lo neto de cada pago, incluidas propinas y asignaciones), **de un pago** (importe y propina de la parte general de un pago concreto) y **por productos** (unidades pagadas seleccionadas por cantidad; se revierte exactamente su asignación).
- Se impide devolver dos veces lo mismo: cada pago conserva cuánto queda devolvible y cada unidad, su asignación neta.
- El método de salida puede ser distinto del original y debe admitir devoluciones; se registra el impacto real en el turno abierto de la terminal (movimiento negativo).
- Autorización configurable: con `payments.refunds.execute` el cajero devuelve sin contraseña; sin ese permiso, o si el importe supera el umbral configurado, se requiere un supervisor con `payments.refunds.authorize`.
- Una devolución financiera no reintegra inventario ni cancela la preparación. Tras devolver unidades, estas vuelven a poder cancelarse, editarse o transferirse con las reglas normales.

## Correcciones de pago

- `POST payments/accounts/{id}/corrections` corrige el método de un pago capturado por error: registra un reverso compensatorio del pago original (movimiento `correction_reversal`) y un pago nuevo con el método correcto que enlaza al original y conserva sus asignaciones. El ingreso neto no se duplica.
- Solo procede mientras el **turno original permanece abierto** (`409 SHIFT_CLOSED` en otro caso) y si el pago no tiene devoluciones ni una corrección previa (`409 PAYMENT_NOT_CORRECTABLE`). Requiere `payments.corrections` o autorización de supervisor.
- El reverso y el pago nuevo se registran en el turno y la caja del pago original, aunque la corrección se ejecute desde otra terminal: el error de captura se corrige dentro del turno en que ocurrió.

## Precuentas y tickets

- `GET payments/accounts/{id}/precheck` devuelve la precuenta (productos, notas, descuentos, cargos, subtotal sin IVA, IVA con su tasa, total, pagos registrados y saldo) sin registrar venta ni documento.
- `POST …/precheck/print` y el campo `print` del pago encolan el documento en la impresora de tickets configurada, usando la cola durable de la etapa 5. Sin `print`, se aplica la preferencia del establecimiento si el usuario tiene `payments.tickets.print`.
- El ticket de pago muestra además el pago concreto: método, referencia, efectivo recibido, cambio y propina separada, más pagos registrados y saldo. Ambos documentos indican que **no son un CFDI**.
- Un fallo de impresión nunca revierte el pago: el trabajo queda con su estado en la cola (error, incierto o intervención) y la cuenta lo muestra. Sin impresora configurada, el pago se registra y la respuesta lo indica.
- `POST …/documents/reprint` (`payments.tickets.reprint`) genera otra copia idéntica sin duplicar cobros. Exige que el trabajo original esté enviado o cancelado (con error o incierto se resuelve antes en la cola de impresión, `400`) y se rechaza mientras exista otra copia pendiente o incierta (`409 PRINT_COPY_PENDING`). Los documentos comerciales no aparecen entre las comandas de cocina.
- El formato base separa datos (`CommercialDocument`), plantilla de texto y transporte; el editor visual llegará en la etapa 9.

## Entrega de pedidos para llevar

- `POST payments/accounts/{id}/deliver` (`takeaway.deliver`) registra la entrega solo si el pedido está liquidado; con saldo pendiente responde `409 BALANCE_PENDING` y la interfaz abre el cobro. Una cuenta solo se entrega una vez (`409 ALREADY_DELIVERED`); un reintento con el mismo `operation_id` devuelve el mismo resultado.
- En el cobro, `deliver: true` registra la entrega únicamente si ese pago liquida el pedido; un pago parcial no lo entrega. Pagado no implica entregado.

## Interfaz

- **Cobro**: desde la cuenta, «Cobrar $saldo» abre la pantalla de cobro dentro de la operación (sin modal): cuenta y productos con su estado de pago y selección por cantidades; métodos como botones rectangulares con icono y texto (sin casillas) e historial; captura con teclado numérico fijo, importes rápidos, efectivo recibido, propina, referencia y el cambio destacado junto al botón de registro. Toda cifra procede de la previsualización de Go. Con `payments.read` sin permiso de cobro se ofrece «Consultar pagos y saldo».
- **Turno**: si la caja de la terminal no tiene turno utilizable, la misma pantalla muestra el motivo de Go y, con `cash.shifts.open`, la apertura con fondo explícito.
- **Descuentos y cargos**: un único componente, accesible desde las acciones de la cuenta y desde la barra del cobro, con descuento, cortesía y cargo, previsualización y lista de ajustes con cancelación.
- **Devoluciones y correcciones**: dentro del cobro (también con la cuenta cerrada o liquidada). La devolución elige modo, pago de origen y método de salida; la corrección se inicia con «Corregir método» en el historial. Para métodos electrónicos, la interfaz aclara que AD-EL POS no procesa reembolsos bancarios.
- **Autorización**: formulario en línea con usuario y contraseña del supervisor, mostrado cuando la previsualización la exige o Go responde `AUTHORIZATION_REQUIRED`. La contraseña se borra en cada envío y nunca entra al almacén de pendientes.
- **Impresión y entrega**: el botón «Imprimir ticket» funciona como interruptor resaltado (sin casilla) con el valor predeterminado del establecimiento; en pedidos para llevar, «Marcar como entregado» (inicialmente desactivado) registra la entrega si el pago liquida. Desde la cuenta para llevar, «Marcar como entregado» entrega directamente si está liquidada o abre el cobro con la entrega activada si queda saldo.
- **Precuenta y documentos**: «Precuenta» muestra la vista previa del documento y permite imprimirla; «Documentos de la cuenta» lista tickets y precuentas con el estado de su último trabajo y la reimpresión.
- **Operaciones inciertas**: todas las escrituras financieras de una cuenta comparten la clave `payments:<cuenta>` del almacén global de pendientes (la impresión de precuentas y reimpresiones usa `documents:<cuenta>`). Mientras exista una operación incierta, la cuenta bloquea nuevas escrituras financieras y ofrece reintentar con el mismo `operation_id` o consultar el resultado.
- **Cobros (administración)**: métodos de pago, IVA con botones 16/8/0, políticas (turno compartido, impresión predeterminada, impresora de tickets, umbral de devoluciones) y movimientos del turno actual.

## Contratos para la etapa 7

- `cash_shifts` ya distingue `open`/`closed` y conserva fondo inicial, quién abrió, terminal y hora; el cierre, cortes, arqueos y conteos se agregarán sin modificar pagos.
- `cash_movements` contiene cada efecto en caja (cobro, devolución, reverso de corrección) firmado, por método, con efectivo recibido y cambio. `GET payments/shifts/{id}/movements` ya calcula totales por método y efectivo esperado = fondo inicial + efectivo neto.
- Cerrar un turno no exigirá liquidar cuentas pendientes; los pagos quedan siempre ligados a su turno y caja.

## Pruebas

Aceptación PostgreSQL en `backend/internal/operations/payments_acceptance_test.go`, `payments_adjustments_test.go` y `payments_documents_test.go`, encadenadas a `TestRestaurantAcceptance`; unitarias en `backend/internal/payments/money_test.go`. La evidencia ejecutada está en [VERIFICATION.md](VERIFICATION.md).
