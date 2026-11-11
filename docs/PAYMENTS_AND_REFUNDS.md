# Cobros, pagos, descuentos y devoluciones · Etapa 6

Versión 0.9.1, esquema 19 (la etapa 9B cambia solo la presentación de precuentas y tickets: plantillas, identidad e impresión con la plantilla vigente, con las migraciones 0018 y 0019, **no aplicadas** a la instalación real hasta la autorización del propietario; ver «Plantillas e impresión con la plantilla vigente · etapa 9B». La etapa 8 no modifica cobros; sus migraciones 0010–0014 se aplicaron a la instalación real el 10 de noviembre de 2026; la etapa 9A agrega la vigencia de la precuenta por revisión y el reconocimiento de ventas al cierre, con las migraciones 0015 y 0017, aplicadas a la instalación real el 10 de noviembre de 2026; ver «Reportes · etapa 9A»): etapa 6 más las ampliaciones de la etapa 6.1 (fase C, migración 0007): IVA por producto y pago en dólares en efectivo como función activable; y de la etapa 7.1 (fase D, migración 0009, aplicada a la instalación real el 9 de noviembre de 2026): motivos de descuento y total en dólares en precuentas y tickets. Este documento describe únicamente lo implementado y probado. La administración de caja de la etapa 7 (arqueo, cierre ciego, cortes, operaciones de efectivo, documentos de caja, cajón y tope; migración 0008) está en [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md); aquí solo se resume su efecto sobre cobros y devoluciones.

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

Migración 0007 (etapa 6.1), sin modificar 0001–0006: `establishment_features` (funciones activables; `usd_cash` sembrada desactivada), `catalog_products.tax_rate` (NULL hereda la tasa general), `payment_settings.usd_rate_min/usd_rate_max/usd_rate_alert_percent` (10, 40 y 10 iniciales), `cash_shifts.usd_rate/usd_rate_set_by/usd_rate_set_at` (los tres juntos), `payments.received_mxn/received_usd/usd_rate/change_currency/change_usd` (con la igualdad `received = received_mxn + round(received_usd × usd_rate, 2)` y `change_usd = round(change_given / usd_rate, 2)` cuando el cambio sale en dólares, comprobadas por CHECK) y `cash_movements.mxn_in/mxn_out/usd_in/usd_out/usd_rate`. Todas las restricciones nuevas tienen nombre explícito. Permisos nuevos solo para el administrador: `settings.features.manage` y `payments.usd_rate.manage`.

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
- Con el pago en dólares activo, la apertura acepta además el tipo de cambio del turno (`usd_rate`, permiso `payments.usd_rate.manage`). Ver «Pago en dólares».
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

### Motivos de descuento · etapa 7.1

- Catálogo configurable `app.discount_reasons` (migración 0009), sembrado con **Atención, Promoción, Demora, Cortesía de la casa y Otro**; solo «Otro» exige descripción (`requires_description`). Se administra en Cobros → «Motivos de descuento» con `payments.settings.manage` (permiso existente): alta, nombre, si exige descripción, activación y orden visual por posiciones (`internal/ordering`), con revisión optimista, idempotencia, auditoría (`payments.discount_reason_created`, `payments.discount_reason_updated`, `payments.discount_reasons_reordered`) y evento `payments.discount_reasons.changed` en la misma transacción. No se borran: se desactivan, y debe quedar al menos uno activo (`LAST_ACTIVE_REASON`).
- Al aplicar un descuento o una cortesía, la interfaz muestra los motivos activos como botones; al elegir uno aparece su descripción, obligatoria de 5 a 500 caracteres si el motivo la exige y opcional (hasta 500) en los demás.
- Go valida antes de escribir: con `reason_id` el motivo debe existir y estar activo (`DISCOUNT_REASON_INACTIVE`). Sin `reason_id` (compatibilidad con la etapa 6) el motivo escrito sigue siendo obligatorio de 5 a 500 y el descuento se guarda sin motivo del catálogo, como los anteriores.
- `account_adjustments.reason_id` y `reason_name` (NULL en filas históricas y en cargos; CHECK con nombre impiden un motivo en un cargo y exigen que el nombre acompañe siempre al motivo) guardan el motivo y su nombre al registrar, para los reportes (desde la etapa 9A, los reportes de descuentos agrupan por ese nombre histórico). Desactivar o renombrar un motivo no modifica los descuentos registrados: conservan su `reason_id`, el nombre que tenía el motivo y su descripción. El historial de ajustes de la cuenta muestra «Motivo: Atención · descripción», la auditoría del descuento incluye `reason_id` y `reason_name`, y la precuenta y el ticket imprimen «Concepto · Motivo» (por ejemplo «Descuento manual · Atención»).

## Autorización puntual de supervisor

- Se valida en Go, en la misma transacción de la operación, contra los permisos efectivos actuales del supervisor (rol + excepciones) y su límite de descuento.
- No crea ni reemplaza sesiones: el supervisor conserva su sesión en su terminal.
- Comparte el límite de intentos del inicio de sesión: cinco fallos en quince minutos por supervisor y terminal bloquean nuevas autorizaciones (`429 AUTHORIZATION_THROTTLED`). Los fallos se auditan sin contraseña.
- Un usuario no puede autorizarse a sí mismo (`403 SELF_AUTHORIZATION`); un supervisor sin permiso o límite suficiente recibe `403 AUTHORIZATION_INSUFFICIENT`.
- Es de un solo uso: queda en `payment_authorizations` ligada a `operation_id`, cuenta e importe. La contraseña no forma parte de la huella idempotente, por lo que un reintento puede enviarse sin ella y recupera el resultado ya confirmado. Si la operación original no llegó a confirmarse, el reintento sin contraseña recibe `403 AUTHORIZATION_REQUIRED` (igual que una autorización ausente o incompleta) y el supervisor debe autorizar de nuevo.

## IVA

- Los precios ya incluyen IVA; el impuesto se desglosa, nunca se suma. Tasas permitidas: **16 %**, **8 %** y **0 %**, elegidas desde una lista cerrada (`PUT payments/tax` con `"16"`, `"8"` o `"0"`; cualquier otro valor se rechaza en Go y en PostgreSQL). Cambiarla requiere `payments.tax.manage` y se audita con valor anterior y nuevo.
- **Tasa por producto (etapa 6.1)**: cada producto o combo puede tener tasa propia (16, 8 o 0) o heredar la general. Al confirmar, cada versión copia su **tasa efectiva**: la del producto raíz de la línea (en un combo, la del combo para todo su importe, aunque sus componentes tengan otra) o, si hereda, la general vigente. Una edición con nueva cotización (más cantidad o cambio de receta) toma la tasa efectiva vigente; una edición que conserva importes (observaciones o reducción) conserva su tasa histórica. Cambiar la tasa del producto o la general solo afecta lo que se confirme después.
- Una cuenta con tasas mezcladas se desglosa con una línea de base e IVA por cada tasa presente, en la precuenta, el ticket y las devoluciones; en cada grupo base + IVA = total exacto. Probado con una cuenta 16/8/0 que incluye un combo al 8 % con un componente exento.
- **Nivel de cálculo**: por cuenta, agrupando por tasa. Para cada tasa, base = total ÷ (1 + tasa) redondeada a centavos e IVA = total − base, de modo que base + IVA es exactamente el total. Calcularlo sobre la suma por tasa evita acumular redondeos unidad por unidad.
- Los descuentos manuales reducen el bruto de su tasa; un descuento general se reparte entre tasas en proporción. Los cargos llevan la tasa vigente al registrarse. Las propinas no llevan IVA.
- Las devoluciones guardan su desglose (`tax_breakdown`) con las tasas de la venta original al momento de devolver.

## Pago en dólares (función `usd_cash`, etapa 6.1)

Función activable del establecimiento, **desactivada por defecto** (Configuración → Funciones del establecimiento, permiso `settings.features.manage`). Con ella desactivada no aparece nada en dólares y Go rechaza con `409 FEATURE_DISABLED` cualquier dato en dólares (`received_usd`, `change_currency: "USD"`, tipo de cambio en la apertura, captura posterior del tipo de cambio o configuración del rango), aunque se llame a la API directamente. Desactivarla no borra ni altera los pagos registrados en dólares.

**Tipo de cambio por turno.** Se captura al abrir el turno (`usd_rate` en `POST payments/shifts`) o, si el turno no lo tiene (por ejemplo, la función se activó con el turno abierto), una sola vez con `POST payments/shifts/{id}/usd-rate`; queda fijo hasta el cierre. Requiere `payments.usd_rate.manage`. Go valida formato (positivo, hasta dos decimales), el rango configurable (`usd_rate_min`–`usd_rate_max`, 10 a 40 inicial; fuera de él `400 USD_RATE_OUT_OF_RANGE`) y la diferencia contra el último tipo capturado en otro turno de cualquier caja: si supera `usd_rate_alert_percent` (10 % inicial) responde `409 USD_RATE_CONFIRMATION_REQUIRED` hasta recibir `confirm_rate_difference: true`. Así se evita, por ejemplo, 1.95 en lugar de 19.50. Un turno sin tipo de cambio no acepta dólares (`409 USD_RATE_REQUIRED`) pero sigue cobrando en pesos. La captura queda auditada (`cash.shift_usd_rate_set`, o en la apertura del turno). El rango y el porcentaje se ajustan en Cobros → Pago en dólares (`PUT payments/settings/usd`, permiso `payments.settings.manage`).

**Cobro.** Solo en efectivo (otro método con dólares responde 400). En una misma operación se capturan pesos recibidos y dólares recibidos. Go calcula:

- Dólares recibidos en pesos = dólares × tipo de cambio, redondeado a centavos.
- Total recibido en pesos (`received`) = pesos recibidos + dólares convertidos; debe cubrir importe + propina.
- Cambio en pesos (`change_given`) = total recibido − importe − propina, y su equivalente en dólares = cambio ÷ tipo de cambio redondeado a centavos.
- «Cambio entregado en: Pesos / Dólares» (pesos por omisión) registra la moneda real del cambio; un cambio de cero se registra en pesos.
- La vista previa devuelve el saldo y el importe de este pago en dólares, lo recibido y el cambio en ambas monedas; si el efectivo no alcanza, informa el faltante en pesos y la menor cantidad de dólares que lo cubre (`covered:false`), y el registro se rechaza.

**Redondeo.** Comercial a centavos, mitad alejándose de cero, en cada conversión (`Decimal.Round(2)` y `Decimal.DivRound(…, 2)` en Go; `round(numeric, 2)` en los CHECK de PostgreSQL). Solo el faltante en dólares se redondea hacia arriba al centavo, porque es el mínimo que cubre lo que falta y no se registra. Ejemplo: tipo $18.50, saldo $290.00 (equivale a US$15.68); $100 + US$20 ($370.00) = $470.00 recibidos; cambio $180.00 o US$9.73.

**Registro.** Cada pago guarda pesos recibidos, dólares recibidos, tipo de cambio aplicado, importe aplicado en pesos, cambio en pesos, moneda del cambio y, si salió en dólares, el cambio en dólares. Un efectivo sin dólares se registra como en la etapa 6 (columnas por moneda en NULL). Cada movimiento de efectivo nuevo guarda pesos y dólares físicos que entraron y salieron (`mxn_in`, `mxn_out`, `usd_in`, `usd_out`, `usd_rate`); los movimientos anteriores a la 0007 conservan NULL y su efecto físico es su neto en pesos. `GET payments/shifts/{id}/movements` agrega `by_currency` con entradas, salidas, pesos físicos esperados (fondo inicial incluido) y dólares físicos esperados, para que el corte de la etapa 7 cuente cada moneda por separado. No es un arqueo.

**Devoluciones solo en pesos.** Una devolución, aunque revierta un pago recibido en dólares, sale en pesos (movimiento `mxn_out`). Registrar devoluciones en dólares físicos complicaría el modelo sin un caso aprobado; queda documentado como límite.

**Correcciones.** El reverso de corrección invierte lo que el pago original movió por moneda (los dólares recibidos salen y el cambio entregado entra), de modo que el cajón queda como antes del pago erróneo. Si el método correcto también es efectivo y el original tenía dólares, el pago nuevo conserva lo recibido y entregado en cada moneda.

**Ticket.** El ticket del pago muestra pesos recibidos, dólares recibidos, tipo de cambio («$18.50/US$»), total recibido y el cambio con su moneda («Cambio en dólares US$1.22 · Equivale a $22.50»).

## Devoluciones

- Modalidades: **total** (todo lo neto de cada pago, incluidas propinas y asignaciones), **de un pago** (importe y propina de la parte general de un pago concreto) y **por productos** (unidades pagadas seleccionadas por cantidad; se revierte exactamente su asignación).
- Se impide devolver dos veces lo mismo: cada pago conserva cuánto queda devolvible y cada unidad, su asignación neta.
- El método de salida puede ser distinto del original y debe admitir devoluciones; se registra el impacto real en el turno abierto de la terminal (movimiento negativo).
- Autorización configurable: con `payments.refunds.execute` el cajero devuelve sin contraseña; sin ese permiso, o si el importe supera el umbral configurado, se requiere un supervisor con `payments.refunds.authorize`.
- Una devolución financiera no reintegra inventario ni cancela la preparación. Tras devolver unidades, estas vuelven a poder cancelarse, editarse o transferirse con las reglas normales.
- **Inventario (etapa 8)**: `payments` no toca solicitudes de consumo ni movimientos de inventario. Una devolución (total, de un pago o por productos) **no reintegra ingredientes**: el consumo de una unidad liberada a cocina solo cambia con una modificación de la partida o si la unidad se cancela y se clasifica «No preparado» en Inventarios (o lo hace el reconciliador al corte del día operativo). Cobrar tampoco consume: el consumo nace de la liberación a cocina. Probado en `backend/internal/operations/inventory_consumption_test.go` (devolución financiera sin reintegración). Ver [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

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
- El formato base separa datos (`CommercialDocument`), plantilla de texto y transporte; el editor visual corresponde a la etapa 9B (desde la 9B la plantilla de texto es el renderizador único de `internal/documents`; ver la sección de la etapa 9B).
- **Vigencia de la precuenta (etapa 9A, §33; migración 0015)**: al imprimir una precuenta, Go guarda en `payment_documents` la revisión de la cuenta (`account_revision`) y el número de ajustes (`adjustment_count`: descuentos y cargos registrados más sus cancelaciones), leídos bajo el lock exclusivo de cobros. La precuenta es **vigente** mientras la cuenta conserve esa revisión y ese número de ajustes; el predicado único es `precheck_current` de `AccountStandingSQL` (`payments/standing.go`), que usa el estado «Esperando pago» del mapa de mesas. Cualquier cambio de contenido (borrador, confirmación, modificación, cancelación, datos, mesa, combinación, transferencia, cierre o reapertura) incrementa la revisión y la invalida; un ajuste o su cancelación la invalida por el número de ajustes, sin cambiar la revisión de la cuenta (así no provoca conflictos de revisión en las capturas abiertas); un pago no la invalida. Ya no se comparan instantes de inicio de transacción: una modificación que esperaba el lock mientras se imprimía la precuenta se confirma después con la revisión siguiente y la deja obsoleta (prueba concurrente que controla el lock en `operations/stage9a_test.go`). Las precuentas anteriores a la 0015 (columnas en NULL) conservan la regla por marcas de tiempo (impresa después del último cambio de la cuenta y del último ajuste). Los tickets de pago no guardan estos datos (restricción `payment_documents_precheck_version_check`). No es una segunda fuente de verdad: ambos valores se derivan de la cuenta y de sus ajustes de solo inserción.
- **Total en dólares (etapa 7.1)**: con la función `usd_cash` activa y tipo de cambio disponible, la precuenta y el ticket agregan después del TOTAL la línea informativa «Total en dólares (T.C. $Y)» con `US$X`, calculada en Go con `mxnToUSD` (total en pesos entre el tipo de cambio, redondeado a centavos) y el formato único de `internal/moneytext`. La precuenta usa el tipo de cambio del turno activo (abierto o en arqueo) de la caja de la terminal que la consulta o imprime; el ticket, el del turno en que se registró su pago. Sin la función, sin caja, sin turno o sin tipo de cambio la línea no aparece. No cambia ningún importe del documento ni del cobro, y la reimpresión conserva el contenido original.

## Entrega de pedidos para llevar

- `POST payments/accounts/{id}/deliver` (`takeaway.deliver`) registra la entrega solo si el pedido está liquidado; con saldo pendiente responde `409 BALANCE_PENDING` y la interfaz abre el cobro. Una cuenta solo se entrega una vez (`409 ALREADY_DELIVERED`); un reintento con el mismo `operation_id` devuelve el mismo resultado.
- En el cobro, `deliver: true` registra la entrega únicamente si ese pago liquida el pedido; un pago parcial no lo entrega. Pagado no implica entregado.

## Interfaz

- **Cobro**: desde la cuenta, «Cobrar $saldo» abre la pantalla de cobro dentro de la operación (sin modal): cuenta y productos con su estado de pago y selección por cantidades; métodos como botones rectangulares con icono y texto (sin casillas) e historial; captura con teclado numérico fijo, importes rápidos, efectivo recibido, propina, referencia y el cambio destacado junto al botón de registro. Toda cifra procede de la previsualización de Go. Con `payments.read` sin permiso de cobro se ofrece «Consultar pagos y saldo».
- **Turno**: si la caja de la terminal no tiene turno utilizable, la misma pantalla muestra el motivo de Go y, con `cash.shifts.open`, la apertura con fondo explícito. Con el pago en dólares activo y `payments.usd_rate.manage`, la apertura agrega «Tipo de cambio del dólar»; el teclado fijo edita el campo activo y una diferencia grande se confirma en un diálogo propio (sin `window.confirm`). Si el turno abierto no tiene tipo de cambio, el cobro lo indica y, con el permiso, ofrece «Capturar tipo de cambio».
- **Dólares** (solo con la función activa): el cobro muestra el tipo de cambio del turno y el saldo con su equivalente en dólares; en efectivo agrega «Pesos recibidos», «Dólares recibidos» (el teclado fijo escribe en el campo activo; billetes US$5–US$100) y «Cambio entregado en: Pesos / Dólares». El cambio se destaca en la moneda en que se entrega y muestra su equivalente en la otra. Todas las cifras proceden de la vista previa de Go.
- **Descuentos y cargos**: un único componente, accesible desde las acciones de la cuenta y desde la barra del cobro, con descuento, cortesía y cargo, previsualización y lista de ajustes con cancelación.
- **Devoluciones y correcciones**: dentro del cobro (también con la cuenta cerrada o liquidada). La devolución elige modo, pago de origen y método de salida; la corrección se inicia con «Corregir método» en el historial. Para métodos electrónicos, la interfaz aclara que AD-EL POS no procesa reembolsos bancarios.
- **Autorización**: formulario en línea con usuario y contraseña del supervisor, mostrado cuando la previsualización la exige o Go responde `AUTHORIZATION_REQUIRED`. La contraseña se borra en cada envío y nunca entra al almacén de pendientes.
- **Impresión y entrega**: el botón «Imprimir ticket» funciona como interruptor resaltado (sin casilla) con el valor predeterminado del establecimiento; en pedidos para llevar, «Marcar como entregado» (inicialmente desactivado) registra la entrega si el pago liquida. Desde la cuenta para llevar, «Marcar como entregado» entrega directamente si está liquidada o abre el cobro con la entrega activada si queda saldo.
- **Precuenta y documentos**: «Precuenta» muestra la vista previa del documento y permite imprimirla; «Documentos de la cuenta» lista tickets y precuentas con el estado de su último trabajo y la reimpresión.
- **Operaciones inciertas**: todas las escrituras financieras de una cuenta comparten la clave `payments:<cuenta>` del almacén global de pendientes (la impresión de precuentas y reimpresiones usa `documents:<cuenta>`). Mientras exista una operación incierta, la cuenta bloquea nuevas escrituras financieras y ofrece reintentar con el mismo `operation_id` o consultar el resultado.
- **Cobros (administración)**: métodos de pago, motivos de descuento (etapa 7.1), IVA con botones 16/8/0, políticas (turno compartido, impresión predeterminada, impresora de tickets, umbral de devoluciones), «Pago en dólares» (rango y porcentaje de alerta, solo con la función activa) y movimientos del turno actual con su tipo de cambio y, si movió dólares, el efectivo físico por moneda.

## Integración con la etapa 7

- `cash_shifts` distingue `open`, `closing` (arqueo de cierre en curso) y `closed`, con un solo turno activo por caja; los pagos, devoluciones y reversos quedan siempre ligados a su turno y caja, y cerrar un turno no exige liquidar cuentas pendientes.
- Durante el arqueo, cobros, devoluciones y correcciones de pago responden 409 `SHIFT_CLOSING`; tras el cierre, `SHIFT_CLOSED`. El cobro y la devolución aceptan `shift_id` opcional (parte de la huella): la interfaz envía el turno que muestra, de modo que un reintento incierto sobre una caja pausada o cerrada se rechaza en lugar de registrarse en otro turno (`SHIFT_CHANGED` si la caja ya tiene otro).
- `cash_movements` contiene cada efecto en caja (cobro, devolución, reverso de corrección) firmado, por método, con efectivo recibido y cambio y, desde la 0007, entradas y salidas físicas por moneda. `GET payments/shifts/{id}/movements` conserva su contrato, pero `expected_cash` y `by_currency` solo se entregan con `cash.expected.read` y nunca durante el arqueo; el esperado se calcula en `cash_expected.go` e incluye las operaciones de efectivo de la caja.
- El cobro y la devolución con efectivo físico abren el cajón cuando la caja lo tiene activado (respuesta `drawer`), nunca con pagos exclusivamente electrónicos; una corrección del método no lo abre. El turno actual del cobro incluye `cash_limit_warning` solo para quien ve el esperado. Ninguno de los dos impide cobrar.
- Ingresos, retiros, gastos, cortes, arqueo, cierre, documentos de caja, cajón y tope: [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md).

## Reportes · etapa 9A

`payments` no calcula reportes, pero escribe el dato del que salen las ventas y expone funciones puras que los reportes reutilizan. Detalle en [REPORTS.md](REPORTS.md).

- **Reconocimiento de ventas** (`recognition.go`, migración 0017): `closeIfSettled`, en la misma transacción en que la cuenta queda liquidada (por un cobro, un ajuste o una entrega), inserta en el libro inmutable `app.sales_recognitions` la diferencia de cada fuente (unidad, descuento o cortesía por partida asignado a la unidad, y ajuste general por tasa) respecto de lo ya reconocido, con el IVA de `finance.go` repartido de modo que la suma por cuenta y tasa es el desglose de la cuenta. El primer cierre reconoce todo; una reliquidación tras una reapertura, solo las diferencias. La venta del periodo es la suma de ese libro: una devolución que reabre la cuenta y borra `closed_at` no mueve ni borra la venta del día original.
- **Devoluciones y correcciones** se reportan en su propia fecha (`payment_refunds.created_at`), en el grupo Pagos; nunca restan de la venta. El cobrado neto es pagos − devoluciones − reversos de corrección.
- **Propinas**: nunca son venta ni llevan IVA; los reportes las toman de los cobros, en su fecha (propinas netas = cobradas − devueltas).
- **Dólares**: el efectivo por moneda y las columnas físicas del detalle de cobros (recibido, cambio, dólares recibidos y tipo de cambio) exigen `cash.expected.read`, como en Caja; nunca se suman pesos y dólares.
- **Turnos en arqueo**: sus cobros se excluyen de todo agregado de los reportes para todos, con un aviso.
- **Motivos de descuento**: los reportes agrupan por el nombre histórico guardado (`reason_name`); sin motivo del catálogo, «Sin motivo del catálogo».
- **Funciones reutilizadas sin locks**: `AccountStandingSQL` (saldo y «En curso»), `ExpectedCash` con `ExpectedPaymentsMXNSQL` / `ExpectedPaymentsUSDSQL` (las mismas expresiones que usa `readCashFlows`) y `CashOperationTypeLabel`.

## Plantillas e impresión con la plantilla vigente · etapa 9B

La 9B no cambia ningún importe, cobro ni regla de `payments`: la precuenta y el ticket se arman con el mismo
`CommercialDocument` y solo cambia cómo se convierten en líneas. Guía en [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md).

- **Renderizado**: `renderDocument` dejó de ser una plantilla en código; `payments/document_data.go` convierte el
  documento a los datos comerciales de `internal/documents` y lo renderiza con la versión activa del tipo (`receipt` o
  `precheck`). La versión 1 del sistema produce exactamente el mismo texto que antes (168 precuentas y tickets comparados
  byte a byte con una copia literal de la plantilla anterior, con `usd_cash`, motivos y ambos anchos).
- **Datos guardados**: el documento guardado en `kitchen_tickets.document` agrega `template_version_id` y una copia de la
  identidad (`identity`); también la vista previa `GET payments/accounts/{id}/precheck`. Los importes y el `content` no
  cambian con la plantilla predeterminada.
- **Reimpresión exacta** (`POST …/documents/reprint`, `payments.tickets.reprint`): como antes, copia el contenido (y, si
  lo hay, el layout) del trabajo original, aunque la plantilla o la identidad hayan cambiado; sin leyenda de reimpresión.
- **Imprimir con la plantilla vigente** (`POST payments/accounts/{id}/documents/rerender`, `payments.tickets.reprint` +
  `documents.history.render_current`, `{operation_id, ticket_id}`): vuelve a renderizar el `CommercialDocument` guardado
  —los mismos productos, importes y pagos— con la plantilla activa hoy y la identidad vigente, en un ticket
  `document_rerender` con su trabajo en la impresora del original. **Nunca** crea un `payment_documents`: no cambia
  «Esperando pago», la vigencia de la precuenta ni ningún cobro. Auditoría `payments.document_rerendered`. Se rechaza
  mientras el original esté pendiente o incierto y mientras haya una copia sin resolver de la reimpresión o de otra
  impresión con la plantilla vigente del mismo documento (`409 PRINT_COPY_PENDING`; bloqueo mutuo desde las correcciones
  de la revisión independiente).
- **Aviso fiscal**: «Documento comercial. No es un comprobante fiscal (CFDI).» es un bloque obligatorio de la precuenta y
  del ticket: se puede mover o cambiar su formato, nunca quitar ni ocultar.
- **Pruebas**: equivalencia en `payments/legacy_documents_test.go` y `documents_equivalence_test.go`; integración en
  `operations/documents_stage9b_test.go` (versión activa guardada, reimpresión idéntica tras editar la plantilla,
  impresión con la plantilla vigente con permiso y auditoría, sin documentos de cobro nuevos). Sin papel.

## Pruebas

Aceptación PostgreSQL en `backend/internal/operations/payments_acceptance_test.go`, `payments_adjustments_test.go`, `payments_documents_test.go`, `payments_tax_test.go` (IVA por producto, etapa 6.1) y `payments_usd_test.go` (funciones activables y dólares, etapa 6.1), encadenadas a `TestRestaurantAcceptance`; actualización de una instalación en esquema 6 en `backend/internal/database/existing_install_test.go`; unitarias en `backend/internal/payments/money_test.go` y `currency_test.go`. La evidencia ejecutada está en [VERIFICATION.md](VERIFICATION.md).

Etapa 7.1 (fase D): `operations/stage71_phase_d_test.go`, encadenada al final de `TestRestaurantAcceptance` con caja, cajero y turno propios: siembra y orden de los motivos; descuento y cortesía con motivo (`reason_id` guardado y auditado); «Otro» sin descripción, motivo inexistente o inválido rechazados sin registros; compatibilidad sin `reason_id`; motivo inactivo rechazado y desactivación sin alterar lo registrado; administración con y sin permiso, idempotente, auditada, con orden y al menos un motivo activo; motivo en la precuenta, el ticket impreso y el historial; total en dólares en la precuenta y el ticket con la función activa y tipo de cambio, y ausente sin la función, sin tipo de cambio o sin turno en la caja de la terminal. Unitarias de la plantilla en `payments/documents_test.go`. Actualización desde el esquema 8 en `database/existing_install_test.go`.

Etapa 9A (parte A1): `operations/stage9a_test.go`, encadenada al final de `TestRestaurantAcceptance`: la precuenta guarda la revisión y el número de ajustes; un cargo y su cancelación la invalidan sin cambiar la revisión de la cuenta; dos carreras controladas (un borrador y un cargo que inician su transacción antes que la impresión y esperan el lock 7820148305 detrás de ella) dejan la precuenta obsoleta aunque la regla anterior por marcas de tiempo la habría mostrado vigente; las precuentas históricas (sin revisión) siguen la regla anterior; los tickets no guardan revisión. Además, el precio reducido por complementos con IVA propio del 8 %, la promoción y el descuento manual posterior sin doble descuento. Actualización desde el esquema 14 y restricción de la 0015 en `database/stage9a_catalog_precheck_upgrade_test.go`.
