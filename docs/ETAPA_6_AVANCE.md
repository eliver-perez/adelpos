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

### 6B — Pantalla de cobro

**Implementado**

- Tipos y servicio `desktop/src/services/payments.ts` con los contratos de Go (dinero siempre como cadena; `formatMoney` solo antepone el símbolo). `Account` admite `closed`, `payment_state`/`delivery_state` reales y el resumen `finance`.
- Almacén global de operaciones inciertas (M8): `hooks/pendingOperations.tsx` con `PendingOperationsProvider` en `App`. `useRestaurantMutation(onResolved, key?)` conserva operación, recurso, cuerpo exacto y `operation_id` bajo una clave (`payments:<cuenta>`, `payments:shift`, `payments:configuration`, `kitchen`); sin clave mantiene el comportamiento anterior. La copia guardada descarta `authorization.password`. Mientras exista un pendiente, la cuenta bloquea nuevos cobros y ofrece reintentar con el mismo identificador o consultar el resultado. `KitchenPage` migró al almacén sin otro cambio.
- Teclado numérico fijo `components/NumericKeypad.tsx` (dígitos, `00`, punto, borrar, limpiar) que escribe sobre una cadena con validación estricta (`services/amountInput.ts`: un punto, dos decimales, diez enteros, sin signos). Admite teclado físico sin interceptar otros campos. Importes en texto con `inputMode="decimal"` (M2).
- Exclusión del teclado flotante con `data-virtual-keyboard="off"` en el campo o un contenedor (M11).
- Pantalla de cobro integrada en la operación (sin modal), `pages/restaurant/checkout/`: cuenta y productos con selección por cantidades y totales de Go con IVA; métodos como botones rectangulares con icono y texto (sin casillas) e historial de pagos; captura total/parcial con efectivo recibido, importes rápidos, propina, referencia, cambio destacado, importe aplicado y saldo restante. Toda cifra procede de `payments/accounts/{id}/preview` y el registro envía `expected_balance`. Estados: capturando, validando, registrando, registrado, saldo pendiente, liquidada, error recuperable, conflicto (`BALANCE_CHANGED`) e incierto. Botón «Cobrar $saldo» en la cuenta y consulta de pagos con `payments.read`.
- Apertura de turno en la misma pantalla con fondo explícito (acepta $0) y motivo de Go cuando el turno no es utilizable; nunca se abre automáticamente.
- Pantalla «Cobros» (`pages/payments/`): métodos, IVA con botones 16/8/0, políticas de cobro e impresora de tickets, y movimientos del turno actual con totales por método.
- Tableros y listas con etiquetas de pago (Sin pago / Abono / Pagado) y de entrega independientes de la preparación, y filtro «Cerradas y liquidadas».
- Puente nativo `api.rs`: lista cerrada con las 23 operaciones `payments_*` de la etapa (incluidas las que usan 6C y 6D) y prueba `payment_routes_are_closed_and_keep_write_methods`. Los identificadores anidados (ajuste, ticket) viajan en el cuerpo porque cada ruta Rust admite un solo identificador.

**Pruebas ejecutadas** (copia aislada con la interfaz 6B y el backend del commit 6A; clúster temporal 55499)

- Vitest: 12 archivos, 50 pruebas aprobadas (19 nuevas: teclado S7 con `+`, punto, doble punto y tercer decimal; almacén S6 que conserva identificador y cuerpo tras desmontar y reintenta con el mismo `operation_id` sin contraseña; cobro con métodos sin casillas, cambio desde la previsualización, total de selección, doble envío bloqueado, `BALANCE_CHANGED`, pago incierto tras remontar y apertura de turno con fondo explícito; exclusión del teclado flotante).
- `tsc --noEmit`, `npm run build`, Prettier, gofmt, `cargo fmt --check`, `sync --check`, `go vet` (también con `integration`), `go test -race ./...` y `cargo test --locked --offline` (11 aprobadas, 1 ignorada de hardware): sin errores.
- Playwright Chromium y WebKit con Go temporal en 8183: recorrido operativo existente aprobado en ambos motores (los textos de pago y entrega que verifica siguen vigentes).
- Revisión visual con transporte simulado en Chromium a 1280×720, 1024×768 y 1920×1080, temas claro y oscuro: sin desplazamiento horizontal, objetivos ≥ 44 px, cambio siempre visible. No sustituye WKWebView nativo ni pantalla táctil real.

**Decisiones**

- El pago total envía también `amount` igual al importe calculado por Go, como segunda guarda junto a `expected_balance`.
- El total de una selección se calcula antes de elegir método con un método solo de cálculo (efectivo preferido) que nunca se usa para registrar.
- Los pendientes viven en memoria del proceso (sin `localStorage`); no se descartan manualmente: se resuelven al reintentar o con un rechazo definitivo de Go (4xx).
- «Cobros» no se ofrece como pantalla de inicio: el backend de preferencias solo admite pantallas con un permiso único y esta depende de cualquiera de cuatro.
- Los importes rápidos de efectivo ($100, $200, $500, $1000 y Exacto) son valores fijos; Go decide si alcanzan.
- `print` y `deliver` en el pago, descuentos, cargos, devoluciones y entrega quedan para 6C y 6D.

### 6C — Descuentos, cortesías, cargos, propinas, devoluciones y correcciones

**Implementado**

- Backend (`payments/adjustments.go`, `authorization.go`, `refunds.go`): descuentos por partida o generales, por porcentaje o importe, hasta 100 %; cortesía como descuento total autorizado que liquida sin pago de $0 y conserva productos vendidos; cargos extras generales (concepto, importe, motivo, tasa vigente) distintos de los extras de producto; cancelación compensatoria de ajustes con motivo; devoluciones total, de un pago (parte general) o por productos y cantidades, con método de salida distinto del original y desglose de IVA congelado (`payment_refunds.tax_breakdown`, añadido a la 0006 aún no aplicada); corrección de método con reverso y pago nuevo enlazados. Previsualizaciones de descuento y devolución.
- Autorización puntual de supervisor (`approve`/`recordApproval`): solo cuando falta el permiso propio o se supera el límite; validada en la misma transacción contra permisos efectivos actuales, de un solo uso por `operation_id`, sin autoautorización, con límite de intentos compartido con el inicio de sesión y sin crear sesión. Una autorización incompleta (por ejemplo, el reintento incierto sin contraseña) responde `403 AUTHORIZATION_REQUIRED` como la ausente.
- Límite de descuento por rol (`role_discount_limits`): `discount_limit` en `POST/PATCH /roles`, sujeto a delegación segura (nadie asigna un límite mayor que el suyo); el administrador conserva 100 % en instalaciones nuevas y en la 0006.
- Interfaz: un solo componente «Descuentos y cargos» (`pages/restaurant/adjustments/`) abierto desde las acciones de la cuenta y desde la barra del cobro; vistas de devoluciones y de corrección dentro del cobro (también con la cuenta cerrada o liquidada); formulario en línea de autorización de supervisor (`components/SupervisorAuthorization.tsx`, `hooks/useSupervisorAuthorization.ts`) que borra la contraseña en cada envío; todas las escrituras financieras de una cuenta comparten la clave `payments:<cuenta>` del almacén global (`hooks/useFinancialMutation.ts`); campo «Límite de descuento (%)» en Roles. Propinas siempre separadas de la venta en resumen, historial y devoluciones.

**Pruebas ejecutadas** (copia aislada con la interfaz 6C y el backend 6A + 6C, sin los documentos de 6D; clúster temporal 55499)

- Aceptación PostgreSQL `operations/payments_adjustments_test.go` (encadenada a `TestRestaurantAcceptance`): descuentos por partida y general con base e IVA reducidos (21, 22, A5); cortesía total sin pago ficticio (23, 24); límites por rol, autorización puntual, autoautorización rechazada, autorización incompleta, cortesía sin permiso, supervisor sobre su propio límite, sesión del supervisor intacta, cinco intentos fallidos y auditoría sin contraseñas (25–27, S4, S5); límite sujeto a delegación segura (S3); cargos generales independientes y auditados (28–30); devolución total con otro método y tasa original (34, 37, A6); devolución parcial de un pago, por productos, sin duplicados y modo de pago sin importe rechazado (35, 36, 38, P6); devoluciones con permiso directo, umbral y supervisor (39, 40); corrección de método en turno abierto conservando los movimientos originales (41, 42).
- Integración PostgreSQL de `database`, `administration`, `catalog`, `operations` y `payments`: aprobada (bases `adel_pos_test_*` eliminadas al terminar). Playwright Chromium y WebKit con Go temporal en 8183: recorrido operativo existente aprobado en ambos motores.
- Vitest: 15 archivos, 62 pruebas aprobadas (12 nuevas: autorización requerida que envía `authorization`, contraseña borrada y ausente del almacén, reintento que pide repetir la autorización, `AUTHORIZATION_INVALID` sin cerrar sesión, cortesía al 100 %, cargo con concepto obligatorio, cancelación con motivo, devolución por productos con otro método, `NOTHING_TO_REFUND`, corrección con `SHIFT_CLOSED`, devolución incierta que bloquea un cobro nuevo y reintenta con el mismo `operation_id`, límite de descuento en roles).
- `tsc --noEmit`, `npm run build`, Prettier, gofmt, `cargo fmt --check`, `sync --check`, `go vet` (también con `integration`), `go test -race ./...` y `cargo test --locked --offline` (11 aprobadas, 1 ignorada de hardware): sin errores.

**Decisiones**

- Una corrección escribe el reverso y el pago nuevo en el turno y la caja del pago original (debe seguir abierto), aunque se ejecute desde otra terminal: así el error de captura se corrige dentro del turno en que ocurrió.
- Los ajustes solo se cancelan con la cuenta abierta. Una cuenta cerrada por una cortesía no se reabre cancelando el ajuste; la única reapertura es una devolución que deja saldo.
- Crear un cargo exige `payments.charges.create`; no hay vía de autorización de supervisor para crearlo (sí para cancelarlo).
- Las rutas con identificador anidado (ajuste, pago) lo reciben en el cuerpo; la corrección y la devolución de un pago usan `payment_id` en el cuerpo.
- Los accesos de interfaz aparecen con el permiso propio o con uno cercano desde el que se puede pedir autorización (descuento: aplicar, cortesía o cobrar; devolución: ejecutar o cobrar; corrección: corregir o cobrar). Go sigue siendo la autoridad.
- Commits: el backend de 6D (documentos comerciales) se escribió junto con 6C, pero se separó para que este commit compile y se pruebe sin él.
