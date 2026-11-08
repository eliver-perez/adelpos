# Etapa 6 · Avance por fases

Especificación: [docs/etapas/ETAPA_6.md](etapas/ETAPA_6.md). Rama local `etapa-6`, sin push. Cada fase termina con pruebas, `format-check`, este resumen y un commit local.

## Punto de partida

- Commit `f9de1e0`: correcciones previas H1, M1, H2, M3, M4 y B1 con pruebas negativas (prerrequisitos de seguridad de §0 ya resueltos; no se repiten en 6A).
- Versión 0.5.0, esquema 5. La instalación real **no** se migra sin autorización del propietario (§23).

## Diseño acordado (aplica a 6A–6D)

### Módulo y autoridad

- `backend/internal/payments`: autoridad de saldos, pagos, asignaciones, cambio, descuentos manuales, cortesías, cargos, propinas, IVA, devoluciones, correcciones, turno mínimo, movimientos, documentos comerciales y entregas. `catalog` sigue siendo la única autoridad de precios.
- Rutas bajo el prefijo `payments/`, delegadas desde `administration/routes.go` con la misma transacción, `Actor`, callback de auditoría y un callback de autorización de supervisor.
- Dependencias: `operations → payments` (guardas sobre unidades con pago y estado financiero de la cuenta); `payments → kitchen` (cola de impresión) y `events`. `payments` no importa `operations` ni `administration`.
- Lock: toda escritura toma primero 7820148305 exclusivo; las lecturas, compartido. No se crean locks nuevos.
- Idempotencia: la misma tabla `restaurant_operations` (actor + terminal + ruta + huella). La huella excluye `authorization.password`; un reintento puede enviarse sin contraseña y recupera el resultado guardado.

### Esquema 6 (`0006_payments_and_refunds.sql`)

Una sola migración con todo el modelo de la etapa. Registros financieros inmutables: las correcciones son registros compensatorios enlazados.

| Tabla                                             | Propósito                                                                                                                                                 |
| ------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `payment_settings`                                | IVA (`CHECK tax_rate IN (0,8,16)`, 16 inicial), turno compartido, impresión predeterminada, impresora de tickets, umbral de autorización de devoluciones. |
| `payment_methods`                                 | Catálogo configurable; cinco métodos iniciales con `code` estable.                                                                                        |
| `cash_shifts`                                     | Turno mínimo; índice único parcial: un turno abierto por caja.                                                                                            |
| `payments`, `payment_allocations`                 | Pagos (venta, propina, efectivo recibido, cambio, referencia) y asignaciones a unidades.                                                                  |
| `payment_refunds`, `payment_refund_allocations`   | Devoluciones y reversos de corrección por pago original, con reverso de asignaciones.                                                                     |
| `account_adjustments`, `…_allocations`, `…_voids` | Descuentos manuales, cortesías y cargos generales; cancelación por registro compensatorio.                                                                |
| `payment_authorizations`                          | Autorización puntual de supervisor ligada a una operación (`operation_id` único).                                                                         |
| `role_discount_limits`                            | Límite máximo de descuento por rol (administrador 100 %).                                                                                                 |
| `cash_movements`                                  | Movimientos de turno para la etapa 7 (cobros, devoluciones, reversos), con efectivo recibido y cambio.                                                    |
| `takeaway_deliveries`                             | Una entrega por cuenta para llevar.                                                                                                                       |
| `payment_documents`                               | Precuentas y tickets ligados a `kitchen_tickets`/`kitchen_print_jobs`.                                                                                    |

Cambios a tablas existentes: `restaurant_accounts.state` admite `closed` (+ `closed_at`); `restaurant_item_versions.tax_rate` (16 para filas existentes, sin valor predeterminado para nuevas); `kitchen_tickets.kind` admite `receipt` y `precheck`.

### Reglas financieras

- Total de la cuenta = unidades activas (con promociones históricas) − descuentos manuales vigentes + cargos vigentes. Pagado = pagos − devoluciones. Saldo = total − pagado; nunca negativo.
- Pago general: reduce saldo sin asignarse a unidades. Pago por selección: se asigna a las unidades elegidas en proporción a lo pendiente de cada una (repartidor determinista de centavos); nunca supera el saldo de la cuenta.
- Unidad con cualquier asignación neta (liquidada o parcial) o con descuento manual vigente: no se transfiere, divide, combina, cancela, edita ni anula. Unidades sin pago siguen las reglas de etapas 4–5, siempre que la cuenta no quede con total menor que lo pagado. Origen de combinación sin pagos ni ajustes.
- Efectivo: `recibido ≥ importe + propina`; `cambio = recibido − importe − propina`. Métodos electrónicos sin cambio ni sobrepago. El excedente nunca se convierte en propina sin captura explícita.
- Cierre: una cuenta de salón liquidada (saldo 0, sin borrador) se cierra y libera la mesa. Para llevar se cierra al entregarse liquidada. Una devolución que deja saldo reabre la cuenta.
- IVA incluido en precios: tasa del establecimiento (16/8/0) copiada a cada versión confirmada; desglose por cuenta agrupando por tasa (base = total/(1+tasa) redondeada; IVA = total − base, por lo que base + IVA = total exacto). Descuentos de cuenta y devoluciones generales se reparten entre tasas en proporción; cargos llevan la tasa vigente al registrarse; propinas sin IVA.

## Fases

### 6A — Modelo financiero y pagos en backend

**Implementado**

- Migración `0006_payments_and_refunds.sql` con el modelo completo de la etapa (tabla del diseño). Versión 0.6.0, esquema 6 en `config/product.json` (sincronizado; `Cargo.lock` actualizado solo para el paquete raíz).
- Paquete `backend/internal/payments`: `money.go` (lectura estricta de importes, repartidor proporcional exacto en centavos con `math/big`, desglose de IVA), `finance.go` (estado financiero completo y resumen `Brief`), `guards.go` (reglas sobre partidas con pago), `handler.go` (rutas, permisos, lock 7820148305, idempotencia), `settings.go` (políticas y tasa de IVA), `methods.go`, `shifts.go` (turno mínimo y movimientos), `collect.go` (previsualización y registro de pagos totales, parciales, mixtos y por selección; efectivo, cambio, propina, referencia; cierre de cuenta), `delivery.go` (entrega de para llevar).
- Rutas: `GET/PUT payments/settings`, `PUT payments/tax`, `GET/POST payments/methods`, `PATCH payments/methods/{id}`, `GET payments/shifts/current`, `POST payments/shifts`, `GET payments/shifts/{id}/movements`, `GET payments/accounts/{id}`, `POST payments/accounts/{id}/preview`, `POST payments/accounts/{id}/payments`, `POST payments/accounts/{id}/deliver`.
- Administración: delegación de `payments/` con el mismo `Actor`, auditoría y un autorizador de supervisor (`administration/supervisor.go`) que reutiliza el límite de intentos del inicio de sesión (`recordFailedAttempt`), rechaza la autoautorización y no crea sesiones. `routes.go` traduce también `payments.Error` y `kitchen.Error` dentro de operaciones comerciales (B9). Una caja con turno abierto no se desactiva y una terminal no cambia de caja mientras la suya tenga turno abierto.
- Operaciones: la tasa vigente se copia a cada versión confirmada (las ediciones que conservan importes conservan la tasa histórica); `payment_state`, `delivery_state` y `finance` provienen de registros reales; guardas H3 en transferir, dividir, combinar, cancelar, editar y anular; transferir o cancelar una cantidad toma primero unidades libres; listado admite `state=closed`.
- `scripts/database.py` incluye `./internal/payments`.

**Pruebas ejecutadas**

- Unitarias `payments`: importes estrictos, repartidor sin pérdida de centavos, desglose exacto de IVA en las tres tasas, lista cerrada de tasas, porcentaje de descuento.
- Aceptación PostgreSQL (`operations/payments_acceptance_test.go`, encadenada a `TestRestaurantAcceptance`), 14 recorridos: turno requerido/único/fondo $0 (43–46), total en efectivo con cambio y cierre (1, 5, 47), tarjeta y referencias opcional/obligatoria (2, 7, 8), parcial y mixto sucesivo con cambio del abono (3, 4, 6), importes inválidos y sobrepagos (9, 10), propinas separadas (31–33), selección, combos y bloqueo de unidades pagadas (11–13, 18, P1–P3), saldos, divisiones, combinaciones y productos nuevos (14–16, 20, P4, P5), idempotencia y saldo obsoleto (61–63), concurrencia, 403 sin escrituras, sesión revocada (48, 64–66, S2), IVA de lista cerrada en Go y PostgreSQL con conservación histórica (A1–A4), entrega y persistencia tras reiniciar el servicio (50, 56–59), cajas y terminales con turno abierto.
- Suites anteriores (database, administration, catalog, operations) sin cambios de resultado.

**Decisiones**

- Una caja con turno abierto no puede desactivarse ni cambiar de caja su terminal hasta el cierre de turno, que pertenece a la etapa 7. Mientras tanto el turno permanece abierto.
- `shared_shifts` es `false` inicialmente: solo quien abrió el turno cobra en él, salvo que el establecimiento permita compartirlo.
- La entrega solicitada en un pago que no liquida la cuenta no se registra (el pago sí); la respuesta lo indica con `delivered: false`.
- La anulación de una cuenta exige que todo lo cobrado ya se haya devuelto (neto cero) y sin ajustes vigentes; combinar exige que el origen no tenga ningún pago registrado.
- `package-lock.json` conserva la versión 0.4.0 que traía de etapas anteriores (no afecta `npm ci`).
