# Etapa 7 · Avance por fases

Especificación: [etapas/ETAPA_7.md](etapas/ETAPA_7.md). Rama local `etapa-7`, creada desde `main`. La especificación indica que `main` contiene hasta la 6.1; en realidad también contiene la 6.2 (0.6.2, esquema 7, sin migraciones propias), por lo que la etapa parte de 0.6.2. Commits locales por fase, sin push ni merge. Las pruebas se ejecutan en una copia aislada con un clúster PostgreSQL temporal (55499) y Go temporal (8183); la instalación real no se toca.

## Fase 0 · Verificación contra el código

### Lo que ya existe y se reutiliza

| Concepto                 | Implementación real                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cajas físicas            | `app.registers` (0002): nombre único, descripción, activa. `devices.register_id` asocia terminales. `administration/registers.go` impide desactivar una caja con terminales o turno abierto; `devices.go` impide cambiar la caja de una terminal con turno abierto.                                                                                                                                                                                                         |
| Turno mínimo             | `app.cash_shifts` (0006): `state` `open`/`closed` (CHECK sin nombre → `cash_shifts_state_check`), fondo `opening_float` en pesos, `opened_by`, `opened_device_id`, `opened_at`, `operation_id`, `closed_at`, `closed_by` (sin uso), `revision`; índice único parcial `cash_shifts_one_open_idx` (un turno `open` por caja). `payments/shifts.go`: `openShift`, `currentShift`, `usableShift` (`REGISTER_REQUIRED`, `SHIFT_REQUIRED`, `SHIFT_NOT_SHARED`), `shiftMovements`. |
| Turno compartido         | `payment_settings.shared_shifts` (global, falso por omisión): sin él solo cobra quien abrió el turno. Cada movimiento conserva su usuario y terminal.                                                                                                                                                                                                                                                                                                                       |
| Tipo de cambio por turno | `cash_shifts.usd_rate` + autor y hora (0007), rango y alerta en `payment_settings`, `POST payments/shifts/{id}/usd-rate` una sola vez. Función `usd_cash` en `internal/features`.                                                                                                                                                                                                                                                                                           |
| Movimientos por moneda   | `app.cash_movements` (0006 + 0007): `kind` `payment`/`refund`/`correction_reversal`, ligado obligatoriamente a cuenta, método y pago o devolución; `mxn_in/mxn_out/usd_in/usd_out/usd_rate` por moneda (NULL en filas anteriores = solo pesos). `expected_cash` = fondo + neto en efectivo; `by_currency` con `expected_mxn` y `expected_usd` (sin fondo en dólares).                                                                                                       |
| Autorización puntual     | Autorizador `administration/supervisor.go` (contraseña del supervisor, límite de intentos de `login_attempts`, permisos efectivos actuales, sin sesión, rechazo de autoautorización) usado por `payments/authorization.go` (`approve`: si el propio permiso basta no se pide contraseña; `recordApproval` guarda `payment_authorizations`, ligada a cuenta y `operation_id`). La huella idempotente excluye la contraseña.                                                  |
| Idempotencia y locks     | `restaurant_operations` + lock 7820148305 (compartido en lecturas, exclusivo en escrituras) en `payments.Handle`; resultado, auditoría y `realtime_events` en la misma transacción.                                                                                                                                                                                                                                                                                         |
| Funciones activables     | `establishment_features`, `internal/features` (`Known`: solo `usd_cash`).                                                                                                                                                                                                                                                                                                                                                                                                   |
| Impresión persistente    | Cola `kitchen_print_jobs`/`kitchen_print_attempts` con estados `pending`…`uncertain`/`intervention`; documentos comerciales `receipt`/`precheck` (`payment_documents`, ligados a cuenta) encolados con `kitchen.EnqueueCommercial` y reimpresión `ReprintCommercial`.                                                                                                                                                                                                       |
| SSE                      | `realtime_events` con ámbitos `restaurant`, `takeaway`, `kitchen`, `printing`, `configuration` (filtrado por permisos en `events/store.go`); los eventos de turno usan `configuration`, que exige `kitchen.stations.read`.                                                                                                                                                                                                                                                  |
| Interfaz                 | Apertura de turno y tipo de cambio dentro del cobro (`checkout/ShiftOpening.tsx`), turno actual y movimientos en Cobros (`payments/ShiftOverview.tsx`), cajas en `RegistersPage.tsx`; almacén global de operaciones inciertas `hooks/pendingOperations.tsx`.                                                                                                                                                                                                                |
| Zona horaria             | `app.establishment.timezone`; la sesión de base de datos trabaja en UTC.                                                                                                                                                                                                                                                                                                                                                                                                    |

Permisos existentes de caja: `cash.shifts.open` y `cash.shifts.read`.

### Lo que no existe

Denominaciones, ingresos, retiros, gastos y sus categorías, correcciones y regularizaciones de efectivo, cortes parciales, arqueos, estado «Cierre en curso», cierre de turno, diferencias, fecha comercial, comprobantes y cortes impresos, **cajón de dinero** (ningún transporte envía el pulso ESC/POS ni existe configuración), tope de caja y un ámbito de eventos propio de caja.

### Hallazgos que condicionan el diseño (sin cambiar decisiones del propietario)

1. `cash_movements` exige cuenta, método y pago o devolución: no admite ingresos, retiros ni gastos. Se agrega una tabla de **operaciones de efectivo** del turno y el efectivo esperado suma ambas fuentes en una sola función de Go. No se alteran restricciones ni filas de `cash_movements`.
2. `payment_authorizations` exige cuenta y `requested_by <> authorized_by`, por lo que no puede registrar la «autoautorización» que §0-B pide para retiros y gastos. La autorización de las operaciones de efectivo se registra en la propia operación (quién autorizó y si fue por permiso propio o por supervisor), reutilizando el mismo autorizador y su límite de intentos.
3. **El efectivo esperado hoy se filtra**: `GET payments/shifts/{id}/movements` devuelve `expected_cash` y `by_currency.expected_*` a cualquiera con `cash.shifts.read`, y Cobros los muestra. Por §0-B se restringen al permiso nuevo «Consultar importes esperados y diferencias» (concedido al administrador).
4. «Cierre en curso» requiere ampliar `cash_shifts_state_check` y sustituir el índice de turno abierto por uno que cubra `open` y `closing`. Además, todos los puntos que hoy comprueban `state='open'` (cobro, corrección, tipo de cambio, apertura, desactivar caja, cambiar la caja de una terminal) deben tratar `closing` como turno activo pausado.
5. Las correcciones de pago de la etapa 6 exigen el turno original abierto; con la caja en arqueo quedan rechazadas, como pide §0-B.
6. La apertura concurrente que choca con el índice único devuelve hoy el error genérico `ALREADY_EXISTS`; se traducirá a `SHIFT_ALREADY_OPEN`.
7. Los documentos comerciales están ligados a una cuenta y la cola solo acepta `receipt`/`precheck`: los cortes y comprobantes de caja necesitan su propio registro y nuevos tipos de documento.

No se encontraron contradicciones de fondo con las decisiones del propietario ni con `CLAUDE.md`; la etapa continúa sin esperar.

### Sistema anterior (CLAUDE.md §21)

Revisión de solo lectura de `IniciarCorte.cs`, `CorteParcial.cs`, `CerrarCorte.cs`, `Cortes.cs`, `Egresos.cs`, `RegistroEgresos.cs` y del esquema de referencia (no se abrió `App.config`). `Egresos.cs`/`RegistroEgresos.cs` son en realidad un reporte de compras a proveedores (tabla `entradas`); `salidas` y `topes_caja` existen en el esquema pero ningún archivo las usa; el esquema de referencia está desactualizado respecto al código.

- **Apertura**: solo un monto inicial total (sin moneda ni denominaciones), con el cajero autenticado; acepta 0. El corte era por **cajero + terminal**, no por caja física: podía haber dos cortes abiertos en la misma terminal. Sin corte abierto se bloqueaban captura, cobro, cancelación y división.
- **«Corte parcial»** era un **retiro**: monto y razón libre, validado contra el disponible (inicial + ventas − retiros), sin clave de supervisor (se registraba al mismo cajero), sin comprobante ni cajón.
- **Cierre**: un único «monto en caja», **no ciego** (mostraba esperado y diferencia en vivo); el esperado sumaba todas las ventas del corte, **incluidas las de tarjeta**, cargos extra y restaba descuentos y retiros; no separaba monedas; no bloqueaba por diferencias ni cuentas abiertas; no imprimía.
- **Cajón**: se abría tras **cualquier** cobro, incluso con tarjeta (ESC/POS por el puerto 9100), y había un botón manual sin autorización ni registro.

Funciones o reglas del sistema anterior que no están en este plan (no se implementan sin confirmación): filtrar reportes de ventas por corte y por empleado; acreditar la venta al corte de quien la cobra (aquí el pago pertenece al turno de la caja donde se registra, que es equivalente cuando cada cajero usa su caja); bloquear la captura de pedidos sin turno abierto (aquí solo se exige turno para cobrar); el reporte «Egresos» de compras a proveedores (etapa 8). Diferencias deliberadas del plan: cierre ciego, autorización de retiros, cajón solo con efectivo, esperado sin tarjetas y separado por moneda.

## Fase A · Migración, modelo y backend de caja

Implementada sobre el diseño aprobado. Detalle funcional en [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md) y contratos en [API_CONTRACTS.md](../API_CONTRACTS.md) («Caja, arqueos y cortes · esquema 8»). Sin pantallas nuevas (fase B), sin impresión, cajón ni tope (fase C, solo esquema). La 0008 no se aplicó a la instalación real.

### Esquema (0008_cash_shifts_and_reconciliation.sql)

- `cash_shifts`: `cash_shifts_state_check` sustituido por `open`/`closing`/`closed`; columnas `opening_float_usd`, `opening_notes`, `business_date`, `closing_started_by/at`, `closing_device_id`, `closing_movement_count`, `closing_operation_count`, `closed_device_id`; CHECK de campos del arqueo y de turno abierto; `cash_shifts_closed_fields_check` `NOT VALID`; `cash_shifts_one_open_idx` sustituido por `cash_shifts_one_active_idx` (`open` y `closing`) e índice de historial.
- Tablas nuevas: `cash_denominations` (18 sembradas), `cash_counts` + `cash_count_lines`, `cash_expense_categories` (4 sembradas), `cash_operations`, `cash_shift_reports`, `cash_shift_closures`, `cash_closure_comments`, `cash_drawer_settings`, `cash_drawer_events`, `cash_documents`.
- `payment_settings.cash_limit_mxn/usd`, función `cash_limit` sembrada desactivada, `kitchen_tickets_kind_check` con `cash_report`/`cash_voucher`, `realtime_events_scope_check` con `cash`.
- 19 permisos nuevos solo para el administrador (Go y migración; una prueba unitaria compara ambos).
- Nombres heredados verificados en una base temporal migrada antes de escribir los `DROP`. Repetible y probada sobre una base en esquema 7 con un turno abierto.

### Backend

- `backend/internal/payments/cash_*.go` (modelo, acceso, esperado, conteos, operaciones, correcciones, cortes, arqueo y cierre, historial, catálogo y rutas) y ajustes en `shifts.go` (apertura ampliada, turno activo, `usableShiftFor`), `collect.go`/`refunds.go` (`shift_id` opcional, `SHIFT_CLOSING`), `handler.go` (rutas y validación de identificadores) y `context.go` (permisos).
- `administration/registers.go` y `devices.go` tratan `closing` como turno activo; `events/store.go` filtra el ámbito `cash` por `cash.shifts.read` o `payments.read`; `ordering` agrega el alcance `CashExpenseCategories`.
- Puente nativo: 22 operaciones nuevas en la lista cerrada de `api.rs` (formato que lee `native-driver.ts`, verificado), parámetros de consulta permitidos solo para el historial, prueba Rust `cash_routes_are_closed_and_keep_write_methods`. Tipos y consultas en `desktop/src/services/cash.ts`; `services/api.ts` y `services/payments.ts` actualizados.
- Interfaz existente: `ShiftOverview.tsx` muestra el esperado solo si Go lo entrega y la pausa del arqueo (quién y cuándo); el cobro trata `SHIFT_CLOSING` como código de turno (vuelve a consultar el turno) y `ShiftOpening` muestra el motivo de Go sin ofrecer otra apertura.

### Decisiones menores y desviaciones del diseño

1. **Regularizaciones fuera del esperado del turno vigente.** El diseño sumaba todas las operaciones al esperado; una regularización corrige el registro de un turno cerrado sin mover dinero en el vigente, y contarla permitiría ocultar una diferencia del turno vigente (lo prohíbe §16). Se excluye del esperado y de los totales del corte (se informa su cantidad) y la consulta del cierre original la lista enlazada.
2. **Detección de registros posteriores al arqueo por cantidad y por fecha.** Se agregaron `closing_movement_count` y `closing_operation_count` a `cash_shifts` (no estaban en el diseño): la confirmación compara los conteos guardados al iniciar y, además, busca registros con fecha posterior. `closing_started_at` usa `clock_timestamp()` tomado tras el lock exclusivo para evitar falsos positivos.
3. **Respuestas de escritura sin esperado.** Crear un corte o confirmar el cierre nunca devuelve esperado ni diferencias (la respuesta se guarda para reintentos); se consultan después con el permiso vigente. La auditoría tampoco los registra (el comentario del supervisor se audita sin copiar su texto).
4. **Visibilidad por permiso y estado.** Esperado, `by_currency` y el bloque `sensitive` solo con `cash.expected.read` y nunca durante `closing`. Turno de otra caja: `cash.shifts.read_all`; turno cerrado: `cash.history.read` salvo quien lo cerró. Las filas individuales de movimientos y operaciones siguen visibles con su permiso de consulta.
5. **`expected_cash` de `GET shifts/{id}/movements`** ahora expresa los pesos físicos esperados (incluye operaciones de efectivo); `by_currency.expected_*` usa la misma función. Con solo cobros en pesos el valor es el mismo que antes.
6. **Rutas** bajo `/payments/shifts/{id}/…`, `/payments/cash/…` y `/payments/cash-reports/{id}` con un solo identificador. Ingresos, retiros y gastos tienen ruta propia para comprobar su permiso antes de leer el cuerpo.
7. **Eventos de turno** (`cash.shift.opened`, `cash.shift.usd_rate_set`) pasan del ámbito `configuration` al nuevo `cash`; lo reciben también los usuarios con `payments.read` para que una terminal de cobro sepa pronto que su caja entró en arqueo.
8. **Monedas de dólar no sembradas**; se agregan con `cash.settings.manage`. Las denominaciones no tienen orden manual (se ordenan por moneda, tipo y valor); las categorías usan el orden por posiciones.
9. **Arqueo desde otra terminal**: iniciar, contar, cancelar y confirmar se permiten desde la caja del turno o, con `cash.shifts.read_all`, desde otra terminal; la política de turno compartido solo aplica a cobros y operaciones de efectivo.
10. **Corrección de un ingreso** no se valida contra el disponible (corrige un registro, no es una salida física nueva).
11. **Prueba existente adaptada**: `payments_adjustments_test.go` cerraba un turno con SQL directo; ahora también fija `closed_by` y `closed_device_id`, que la 0008 exige. La devolución verifica el turno antes que la autorización del supervisor, para no consumir un intento con la caja pausada.

### Limitaciones de la fase A

- Sin pantallas de caja ni envío de `shift_id` desde el cobro (fase B). Sin impresión, cajón ni tope (fase C); `cash_limit` no está registrada en `internal/features`.
- Un usuario con permiso de autorizar salidas sin `cash.expected.read` puede inferir por tanteo si un importe excede el disponible; los rechazos no revelan cifras ni se auditan.
- `config/product.json` sigue en `schema_version: 7` y 0.6.2 hasta el versionado de la fase D.

### Verificación (entorno aislado: copia del repositorio, clúster temporal 55499, Go temporal 8183)

| Suite                                                         | Resultado                                                                                                                                                                                    |
| ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Prettier, gofmt, cargo fmt, `sync-metadata --check`, `go vet` | Aprobados (`go vet` también con `-tags=integration`).                                                                                                                                        |
| `go test -race ./...` (unitarias)                             | `ok` en todos los paquetes con pruebas, incluidos `payments` (esperado, diferencias, conteos, permisos de la 0008) y `ordering`.                                                             |
| Integración PostgreSQL (`-race -tags=integration`)            | `ok` en `database` (incluye `TestStage7UpgradeOfExistingInstallation`), `administration`, `catalog`, `operations` (`TestRestaurantAcceptance` con los subtests de caja C1–C25) y `payments`. |
| Vitest                                                        | 25 archivos, 144 pruebas aprobadas (incluye `pages/payments/shiftOverview.test.tsx`).                                                                                                        |
| `tsc --noEmit`, `npm run build`                               | Aprobados.                                                                                                                                                                                   |
| `cargo test --locked --offline`                               | 13 aprobadas, 1 ignorada (incluye `cash_routes_are_closed_and_keep_write_methods`).                                                                                                          |
| Playwright                                                    | Recorrido existente aprobado en Chromium y WebKit.                                                                                                                                           |

Las suites se repitieron de forma independiente sobre la copia sincronizada con el mismo resultado (sin fallos; Playwright `1 passed` en Chromium y WebKit). Commit local en `etapa-7`.

## Fase B · Interfaz de administración de caja

Implementada sobre el backend de la fase A. Detalle funcional en [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md) («Interfaz (fase B)») y contratos de las rutas nuevas en [API_CONTRACTS.md](../API_CONTRACTS.md). Sin impresión, cajón, tope ni cableado SSE del ámbito `cash` (fase C). La 0008 no se modificó ni se aplicó a la instalación real.

### Pantallas

- **Caja** (`desktop/src/pages/cash/`, id `cash`, permiso `cash.shifts.read`) en la navegación y como pantalla de inicio (Go la acepta en `administration/preferences.go`). Secciones según permisos: Operación, Movimientos, Historial, Reportes y Configuración.
- **Operación**: caja seleccionada (la de la terminal; otras con `cash.shifts.read_all`), estado, responsable, apertura y fecha comercial del turno, aviso «Cierre en curso» (quién, cuándo y desde qué terminal), apertura ampliada, ingreso, retiro, gasto, corte parcial, arqueo y cierre; las correcciones y regularizaciones se abren desde el detalle del movimiento.
- **Movimientos**: cobros, devoluciones y reversos junto con las operaciones de efectivo, filtros por turno, tipo y usuario, detalle sin identificadores internos.
- **Historial**: filtros por caja, fechas comerciales, estado y página; detalle con apertura, cierre, responsables, totales, cortes parciales y definitivo, movimientos, regularizaciones enlazadas y comentarios del supervisor; diferencias solo si Go las entrega.
- **Reportes**: resumen en vivo del turno actual y cierres recientes.
- **Configuración**: denominaciones (activar, desactivar, agregar) y categorías de gasto (crear, renombrar, activar, ordenar por posiciones).
- **Apertura ampliada** compartida por el cobro y Caja (`checkout/ShiftOpening.tsx` + `useShiftOpening.ts`): por total o por denominaciones, fondo en dólares y tipo de cambio solo con `usd_cash`, observaciones.
- **Cobro y devolución**: envían `shift_id`; los reintentos rechazados con `SHIFT_CLOSING`, `SHIFT_CLOSED` o `SHIFT_CHANGED` se explican y el pendiente se descarta.

### Rutas nuevas (solo lectura)

- `POST payments/cash/count-preview`: vista previa del conteo con `evaluateCount` (la misma del arqueo y de la apertura), sin escribir; `payments.read` más `cash.shifts.open` (apertura) o `cash.counts.perform` y acceso al turno (arqueo).
- `GET payments/shifts/{id}/summary`: resumen en vivo con `readShiftTotals` y lo ocurrido desde el último corte; esperado y efectivo físico por moneda solo si `expectedVisible`; ciego durante el arqueo.
- Ambas en la lista cerrada de `desktop/src-tauri/src/api.rs` (formato que lee `native-driver.ts`) y en la prueba Rust `cash_routes_are_closed_and_keep_write_methods`.

### Decisiones menores y desviaciones

1. **Resumen durante el arqueo**: además del esperado, omite los totales (`blind: true`). La regla pedía solo el esperado; se eligió lo más conservador porque el neto en efectivo del turno, con el fondo y las operaciones, permite deducirlo.
2. **Efectivo recibido y entregado por moneda** (`physical`) solo con `cash.expected.read` y fuera del arqueo, con el mismo criterio que `by_currency` de los movimientos (fase A).
3. **Vista previa del conteo** con permiso base `payments.read` (el del catálogo) y el permiso propio dentro de la acción; se admite durante el arqueo porque solo devuelve lo declarado.
4. **Defecto de la fase A corregido**: el catálogo de denominaciones se ordenaba como texto (500, 50, 200, 20, 1000…) porque `ORDER BY value` tomaba la columna de salida `value::text`. Se calificó con la tabla en `payments/cash_settings.go` y se agregó la prueba C26. No cambia esquema ni datos.
5. **Apertura por denominaciones**: envía los totales que calculó Go en la vista previa como fondo y el desglose como `count`; Go vuelve a validar la coincidencia. No ofrece importe adicional en la apertura (sí en el arqueo).
6. **Borrador del arqueo**: guardado automático 0.9 s después del último cambio, con una sola petición en vuelo (almacén de pendientes). Un sondeo solo adopta revisiones mayores y sin cambios locales; tras `COUNT_CHANGED` el guardado automático se detiene hasta que el cajero pulsa «Guardar mi conteo» (su captura sustituye la de la otra terminal de forma explícita).
7. **Correcciones** solo desde el detalle de una operación de la caja de la terminal, en su turno abierto (corrección) o como regularización si el turno original está cerrado. Los cobros se siguen corrigiendo desde el cobro de la cuenta.
8. **Cobro**: `SHIFT_CLOSED` se muestra en el aviso general de cobros y devoluciones (antes solo en correcciones) y `SHIFT_CLOSED`/`SHIFT_CHANGED` vuelven a consultar el turno.
9. **Componentes compartidos**: `NumericKeypad` admite `integer` (desactiva el punto para cantidades); `ShiftRateCapture` pasó a su propio archivo; las fechas comerciales se muestran como dd/mm/aaaa.
10. **Playwright**: el recorrido de caja se ejecuta antes del cobro de la mesa y cierra su turno; el recorrido de cobro existente abre después un turno nuevo en la misma caja. El recorrido de caja no captura tipo de cambio para no provocar la confirmación por diferencia del siguiente turno.

### Limitaciones de la fase B

- Sin cableado SSE del ámbito `cash`, impresión de comprobantes y cortes, cajón ni tope (fase C); la interfaz se actualiza por sondeo y por la invalidación general existente.
- Un pendiente incierto de un turno que luego se cierra queda guardado con la clave de ese turno y no se muestra en Caja (su reintento sería rechazado sin duplicar).
- Lo tecleado en el arqueo durante los 0.9 s previos al guardado automático se pierde si se cierra el proceso.
- Verificado en Chromium y WebKit con el puente de pruebas; ventana nativa, pantalla táctil física y Keychain no verificados.
- `config/product.json` sigue en 0.6.2 y esquema 7 hasta la fase D.

### Verificación (entorno aislado: copia del repositorio, clúster temporal 55499, Go temporal 8183)

| Suite                                                         | Resultado                                                                                                                                                                                                                                      |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Prettier, gofmt, cargo fmt, `sync-metadata --check`, `go vet` | Aprobados (`go vet` también con `-tags=integration`).                                                                                                                                                                                          |
| `go test -race ./...` (unitarias)                             | `ok` en todos los paquetes con pruebas (incluye la pantalla `cash` como inicio en `administration`).                                                                                                                                           |
| Integración PostgreSQL (`-race -tags=integration`)            | `ok` en `database`, `administration`, `catalog`, `operations` y `payments`; en `operations`, `TestRestaurantAcceptance` incluye C26–C30 (orden del catálogo, vista previa sin escritura, resumen con y sin permiso, ciego en arqueo, cerrado). |
| Vitest                                                        | 29 archivos, 161 pruebas aprobadas (nuevas: `pages/cash/opening`, `operations`, `closing`, `history` y dos casos en `checkout/checkout.test.tsx`).                                                                                             |
| `tsc --noEmit`, `npm run build`                               | Aprobados.                                                                                                                                                                                                                                     |
| `cargo test --locked --offline`                               | 13 aprobadas, 1 ignorada (la prueba de rutas de caja incluye las dos operaciones nuevas).                                                                                                                                                      |
| Playwright                                                    | `1 passed` en Chromium y en WebKit: recorrido existente más `e2e/cash-flow.ts` (apertura por denominaciones, ingreso y retiro autoautorizado, corte parcial, arqueo que bloquea el cobro, conteo, cierre con faltante e historial).            |

Las capturas de Playwright (claro y oscuro, 1280 × 720 y 1920 × 1080) se revisaron; mostraron el orden de texto de las denominaciones (decisión 4) y una puntuación doble tras las horas, ambos corregidos antes de la verificación final. Las suites se repitieron de forma independiente sobre la copia sincronizada con el mismo resultado (Vitest 161 aprobadas, integración sin fallos, Playwright `1 passed` en Chromium y WebKit). Commit local en `etapa-7`.

## Fase C · Impresión de documentos de caja, cajón de dinero, tope de caja y eventos SSE

Implementada sobre las fases A y B. Detalle funcional en [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md) («Impresión de documentos de caja», «Cajón de dinero», «Tope de caja» y «Tiempo real»), contratos en [API_CONTRACTS.md](../API_CONTRACTS.md) y transporte en [KITCHEN_AND_PRINTING.md](KITCHEN_AND_PRINTING.md). **Sin hardware**: la impresión y el pulso del cajón se verificaron con el adaptador controlado de la cola; ninguna prueba envió nada a CUPS, a una impresora ni a una conexión TCP. La 0008 no se aplicó a la instalación real.

### Cambios a la 0008 (no aplicada en la instalación real)

- `kitchen_tickets_kind_check` admite además `cash_drawer` (el comando de apertura del cajón viaja por la cola durable, pero no es un documento).
- `cash_documents`: `includes_expected` (si el contenido impreso incluye el esperado o las diferencias), `operation_id` (único, diferible, hacia `restaurant_operations`), CHECK `cash_documents_expected_check` (un comprobante nunca incluye el esperado) e índices únicos parciales `cash_documents_one_per_report_idx` y `cash_documents_one_per_operation_idx` (un documento por corte o por operación; las copias son reimpresiones del mismo ticket).
- `cash_drawer_events`: motivo `refund` además de `payment`, `operation` y `manual`; `notes` (≤ 500, `cash_drawer_events_notes_length_check`) con `cash_drawer_events_manual_check` (la apertura manual exige motivo de 5 o más caracteres); `job_id` obligatorio y únicos `operation_id` y `job_id` (una operación emite como máximo un comando y cada comando tiene su trabajo).
- Solo comentarios en el tope de caja y en `cash_drawer_settings` (sin cambios de columnas). Sigue transaccional, con nombres explícitos e `IS NOT NULL` en las ramas opcionales; `TestStage7UpgradeOfExistingInstallation` (actualización desde esquema 7) agrega `checkPrintingConstraints` y el tipo `cash_drawer`.

### Backend

- `payments/cash_documents.go`: plantilla de texto de comprobantes (ingreso, retiro, gasto) y cortes (parcial y definitivo) con el contenido de §20; vista previa de solo lectura; impresión al crear (bandera `print`, parte de la huella) o después; reimpresión; lista de documentos del turno. Impresora: la de tickets de Cobros (mismo criterio que los documentos comerciales de la etapa 6).
- `payments/cash_drawer.go`: configuración por caja, apertura automática (cobros y devoluciones en efectivo; ingresos, retiros y gastos) y manual (`cash.drawer.open`, motivo, idempotente y auditada).
- `payments/cash_limit.go` y `features.CashLimit`: tope global por caja (`payment_settings.cash_limit_mxn/usd`), aviso solo con el esperado visible.
- `kitchen`: tipos `cash_report`, `cash_voucher` y `cash_drawer`; `EnqueueDrawerPulse`, `CompatibleDrawerTransport` y `DrawerPulseBytes` (`ESC p m 25 250`, sin `ESC @`); el trabajador entrega el pulso solo por ESC/POS TCP, nunca reintenta un comando del cajón ni lo pasa a una impresora de respaldo y avisa `cash.print.updated` en el ámbito de caja; la cola no reimprime ni reenvía documentos o comandos de caja (solo cancelar o reconocer) y muestra un corte con esperado solo con `cash.expected.read` y fuera del arqueo; el listado de comandas excluye los tipos de caja.
- Respuestas ampliadas (compatibles): ingreso, retiro y gasto agregan `printing` y `drawer`; el corte parcial y el cierre, `printing`; el cobro y la devolución, `drawer`. Un fallo al encolar documento o comando se revierte a un punto de guardado y la operación o el cierre se confirman con `status: "failed"`.
- Eventos `cash` nuevos (sin importes): `cash.movement.recorded` (cobros, devoluciones y correcciones de pago), `cash.document.printed`, `cash.document.reprinted`, `cash.drawer.command_queued`, `cash.print.updated`; `cash.settings.changed` también al configurar cajón y tope y al activar o desactivar `cash_limit` o `usd_cash`.

### Rutas nuevas (lista cerrada de `api.rs`, prueba Rust `cash_printing_drawer_and_limit_routes_are_closed`)

| Operación del puente              | Ruta                                        | Permiso                                       |
| --------------------------------- | ------------------------------------------- | --------------------------------------------- |
| `payments_cash_report_preview`    | `GET payments/cash-reports/{id}/preview`    | `cash.shifts.read` y acceso al turno          |
| `payments_print_cash_report`      | `POST payments/cash-reports/{id}/print`     | `cash.reports.print`                          |
| `payments_cash_operation_preview` | `GET payments/cash-operations/{id}/preview` | `cash.movements.read` y acceso al turno       |
| `payments_print_cash_operation`   | `POST payments/cash-operations/{id}/print`  | `cash.reports.print`                          |
| `payments_reprint_cash_document`  | `POST payments/cash-documents/{id}/reprint` | `cash.reports.reprint` (+ esperado si aplica) |
| `payments_shift_documents`        | `GET payments/shifts/{id}/documents`        | `cash.shifts.read` y acceso al turno          |
| `payments_cash_drawers`           | `GET payments/cash/drawers`                 | `cash.settings.manage`                        |
| `payments_save_cash_drawer`       | `PUT payments/cash/drawers/{caja}`          | `cash.settings.manage`                        |
| `payments_open_drawer`            | `POST payments/cash/open-drawer`            | `cash.drawer.open`                            |
| `payments_cash_limit`             | `GET payments/cash/limit`                   | `cash.settings.manage`                        |
| `payments_save_cash_limit`        | `PUT payments/cash/limit`                   | `cash.settings.manage`                        |

### Interfaz

- **Imprimir** (`PrintToggle`, solo con `cash.reports.print`, apagado por omisión) en ingreso, retiro, gasto, corte parcial y confirmación del cierre. **Vista previa, Imprimir y Reimprimir** (`CashDocumentActions`) en cada corte (Operación, Historial y resultado del cierre) y en el detalle de ingresos, retiros y gastos; «Reimprimir» solo con `cash.reports.reprint` y, si el documento incluye el esperado, con `cash.expected.read`. Los mensajes dicen «Documento enviado a la cola de impresión» o «enviado a la impresora», nunca que el papel salió.
- **Cajón**: «Abrir cajón» con motivo en la caja de la terminal (solo con el cajón activado y `cash.drawer.open`); avisos «Comando de apertura enviado» tras cobros, devoluciones y operaciones, nunca «cajón abierto».
- **Tope**: aviso «Tope de caja superado» en la vista operativa, el resumen de Reportes y el cobro, solo si Go lo entrega y nunca durante el arqueo; configuración en Caja → Configuración junto al cajón (`CashHardwareSettings`). La función aparece en Configuración → Funciones.
- **Tiempo real**: `useRealtime` publica además `adel-cash-change` con los tipos y turnos de los eventos de caja; `useCashEvents` vuelve a consultar de inmediato Caja (`useCashWorkspace`) y el turno del cobro (`CheckoutPanel`). El sondeo sigue como recuperación.

### Decisiones menores y desviaciones

1. **Impresora**: la de tickets de Cobros para todos los documentos de caja; no se agregó impresora por caja porque no hizo falta (el cajón sí tiene la suya).
2. **Impresión opcional**: sin bandera `print` no se imprime; no hay preferencia del establecimiento para documentos de caja (más conservador que el ticket de pago).
3. **Un documento por corte u operación**: la segunda impresión responde 409 `CASH_DOCUMENT_EXISTS` y se usa «Reimprimir», que copia el contenido exacto con las reglas de la etapa 6 (original enviado o cancelado; `PRINT_COPY_PENDING`). Si el primer documento se generó sin el esperado, la copia tampoco lo trae; el esperado se consulta en pantalla.
4. **Esperado en documentos**: se decide al generar el documento (permiso de quien lo genera y turno fuera del arqueo) y queda en `includes_expected`; reimprimirlo exige `cash.expected.read` y que el turno no esté en arqueo (409 `SHIFT_CLOSING`).
5. **Correcciones y regularizaciones** no tienen comprobante propio (409 `CASH_DOCUMENT_UNSUPPORTED`); §20 solo pide ingreso, retiro y gasto.
6. **Cajón**: un trabajo por comando con copias forzadas a 1; un error confirmado tampoco se reintenta solo (un cajón que se abre más tarde sin nadie presente es un riesgo físico) y nunca pasa a la impresora de respaldo; desde la cola solo se cancela o se reconoce. La apertura manual solo desde una terminal de la caja y se admite sin turno activo (por ejemplo, para preparar el fondo). Una corrección del método de pago no abre el cajón; una devolución lo abre una vez aunque abarque varios pagos. Con la impresora del cajón desactivada o incompatible no se encola nada (`status: "unavailable"`) y la operación sigue.
7. **Tope**: «supera» es estrictamente mayor; la configuración comparte la revisión de `payment_settings` (un cambio simultáneo en Cobros responde `REVISION_CONFLICT`); guardar exige la función activa (409 `FEATURE_DISABLED`) y el tope en dólares exige `usd_cash` (sin ella, el tope en dólares guardado se conserva).
8. **Defecto de las fases A y B corregido**: las sumas vacías de los totales del corte y del resumen se devolvían como `"0"`; ahora siempre con dos decimales (`numeric(18,2)`), y la plantilla normaliza cualquier importe.
9. **Pendientes separados**: la impresión usa la clave `cash-documents:<turno>` y el cajón `cash-drawer:<caja>`, para que una impresión incierta no bloquee las operaciones de efectivo del turno.
10. **Revisión de la coordinación**: la consulta de la impresora de tickets y la de la configuración del cajón se movieron dentro del punto de guardado de `printAfterRecording` y `openDrawerAfter`. Fuera de él, un error de PostgreSQL en esa lectura habría abortado la transacción completa y, con ella, la operación o el cierre que la solicitó.

### Limitaciones de la fase C

- Sin hardware: pulso del cajón (pin, tiempos 50/500 ms), impresoras ESC/POS con cajón, papel y corte no verificados físicamente; solo adaptador controlado y prueba unitaria de los bytes. Una impresora por CUPS no puede abrir el cajón (no entrega los bytes sin alterar) hasta que exista el agente o una cola sin filtrado.
- La vista previa y la impresión usan texto plano de 42 o 30 columnas; el diseñador de documentos sigue reservado a la etapa 9.
- Los usuarios sin `events.read` se actualizan por sondeo; `cash.print.updated` identifica el trabajo, no el turno.
- Tiempo real verificado con el API de eventos (integración), Vitest y el puente de pruebas; no con dos terminales nativas simultáneas.
- `config/product.json` sigue en 0.6.2 y esquema 7 hasta la fase D.

### Verificación (entorno aislado: copia del repositorio, clúster temporal 55499, Go temporal 8183)

| Suite                                                         | Resultado                                                                                                                                                                                                                                |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Prettier, gofmt, cargo fmt, `sync-metadata --check`, `go vet` | Aprobados (`go vet` también con `-tags=integration`).                                                                                                                                                                                    |
| `go test -race ./...` (unitarias)                             | `ok` en todos los paquetes con pruebas, incluidos `payments` (contenido de cortes y comprobantes), `kitchen` (bytes del pulso y transporte incompatible) y `features` (`cash_limit` registrada).                                         |
| Integración PostgreSQL (`-race -tags=integration`)            | `ok` en `database` (actualización desde esquema 7 con los CHECK de la fase C), `administration`, `catalog`, `operations` (`TestRestaurantAcceptance` con C31–C47 de impresión, cajón, tope y eventos) y `payments`.                      |
| Vitest                                                        | 30 archivos, 175 pruebas aprobadas (nuevas: `pages/cash/printing.test.tsx`, tres casos en `checkout/checkout.test.tsx` y uno en `hooks/useRealtime.test.tsx`).                                                                           |
| `tsc --noEmit`, `npm run build`                               | Aprobados.                                                                                                                                                                                                                               |
| `cargo test --locked --offline`                               | 14 aprobadas, 1 ignorada (nueva: `cash_printing_drawer_and_limit_routes_are_closed`).                                                                                                                                                    |
| Playwright                                                    | `1 passed` en Chromium y en WebKit: recorrido existente más vista previa del corte parcial y del corte definitivo y tope activado (aviso visible) y desactivado (sin aviso) en `e2e/cash-flow.ts`; capturas en claro y oscuro revisadas. |

La prueba física opt-in (`TestControlledCUPSPrinter`) se omitió como siempre: ninguna suite envió trabajos a CUPS, a una impresora ni a una conexión TCP. Las capturas mostraron importes `$0` en sumas vacías de los totales (defecto de las fases A y B, decisión 8), corregido antes de la verificación final. Verificación independiente de la coordinación de la etapa sobre la copia sincronizada con el ajuste de la decisión 10: estáticas aprobadas; `go test -race` `ok` en los doce paquetes con pruebas; integración `ok` en `database`, `administration`, `catalog`, `operations` (106 s), `payments` y `kitchen`; Vitest 175/175 en 30 archivos; `tsc` y `build` correctos; Rust 14 aprobadas y 1 ignorada; Playwright `1 passed` en Chromium y en WebKit.

## Fase D · Regresiones, documentación y entrega

### Versionado y documentación

- Versión **0.7.0** y `schema_version: 8` en `config/product.json`, con `sync-metadata` (metadatos de Go y del escritorio, `tauri.conf.json`, `package.json`, `Cargo.toml`) y la entrada raíz de `Cargo.lock` actualizada a mano.
- Actualizados `CLAUDE.md` (estado de la etapa 7, decisiones vigentes, estructura, riesgos y reglas del esquema), `AGENTS.md` (reglas permanentes desde la etapa 7), `README.md`, `ARCHITECTURE.md`, `API_CONTRACTS.md`, `NEXT_STEPS_FOR_SOL.md`, `DEVELOPMENT_MACOS.md`, `docs/PAYMENTS_AND_REFUNDS.md` (integración con la caja), `docs/KITCHEN_AND_PRINTING.md` y `docs/CASH_SHIFTS_AND_RECONCILIATION.md` (movimientos por moneda, reglas de concurrencia, pruebas y limitaciones). La verificación de la etapa 6.2 quedó archivada en `docs/VERIFICATION_STAGE6_2.md` y la nueva [VERIFICATION.md](VERIFICATION.md) registra solo lo ejecutado.

### Corrección de la fase D

Guardar el tope de caja con la función `cash_limit` desactivada respondía `FEATURE_DISABLED` con el mensaje del pago en dólares. Ahora explica que el tope está desactivado y dónde activarlo; el código de error no cambia.

### Regresión final

Sobre el árbol 0.7.0 en el entorno aislado (copia del repositorio, clúster temporal 55499, Go temporal 8183), todo aprobado:

- estáticas y metadatos;
- `go vet`, con y sin `integration`;
- `go test -race`: `ok` en los doce paquetes con pruebas;
- integración PostgreSQL: `ok` en `database`, `administration`, `catalog`, `operations` (106 s), `payments` y `kitchen`;
- Vitest: 175 pruebas en 30 archivos;
- `tsc` y `build`;
- Rust: 14 aprobadas y 1 ignorada (Keychain real, opt-in);
- Playwright: `1 passed` en Chromium y en WebKit;
- `.app` 0.7.0 arm64 de 9.48 MiB, compilado en un destino temporal.

Detalle en [VERIFICATION.md](VERIFICATION.md).

### Procedimiento para aplicar la 0008 a la instalación real

Propuesto al cierre de la fase D, con la instalación real en 0.6.2 y esquema 7 sin modificar. El propietario lo autorizó y se ejecutó el 9 de noviembre de 2026 (ver «Aplicación en la instalación real»). Con autorización expresa del propietario, desde la rama `etapa-7`:

1. **Preparar.** Cerrar AD-EL POS en todas las terminales, sin cobros ni arqueos en curso. `./scripts/dev.sh status` confirma el servidor 0.6.2 en 8181.
2. **Respaldar.** `./scripts/dev.sh backup` genera `backups/adel_pos-<fecha>.dump` y lo comprueba con `pg_restore --list`.
3. **Ensayar la restauración.** `python3 scripts/restore-check.py backups/NOMBRE.dump` restaura en una base temporal, compara los conteos de todas las tablas (79 en esquema 7) y escribe `.local/stage8-restore-verification.json`. Si no coinciden, se detiene el procedimiento.
4. **Detener.** `./scripts/dev.sh server-stop`, de forma ordenada.
5. **Migrar.** `./scripts/dev.sh migrate` compila el servidor 0.7.0 y aplica la 0008 en una sola transacción. Resultado esperado `applied:1`; una segunda ejecución debe dar `applied:0`. Si falla, la transacción no deja cambios.
6. **Arrancar.** `./scripts/dev.sh server-start`.
   - `GET http://127.0.0.1:8181/api/v1/health/ready` debe responder 200, con PostgreSQL disponible y esquema compatible.
   - `GET /api/v1/version` debe indicar 0.7.0 y esquema 8.
7. **Comprobar.** Comprobaciones de solo lectura, desde la aplicación:
   - Los turnos existentes conservan su estado (un turno abierto sigue abierto y se cierra con el arqueo nuevo).
   - La función «Tope de efectivo en caja» está desactivada.
   - El rol administrador tiene los diecinueve permisos `cash.*` nuevos.
   - Ningún otro rol cambió.
8. **Instalar la aplicación.** `./scripts/dev.sh build-macos` y `./scripts/dev.sh open-app`.

Efectos de la 0008 sobre los datos existentes:

- **No toca registros históricos.** No modifica pagos, devoluciones ni movimientos.
- **Turnos anteriores.** La fecha comercial se deriva al consultarlos.
- **Semillas.** Siembra las denominaciones habituales, cuatro categorías de gasto y `cash_limit` desactivada.
- **Sin datos ficticios.** No crea turnos ni operaciones.

Vuelta atrás:

- **Antes del paso 5:** basta `server-start` desde `main` (0.6.2).
- **Después de aplicarla:** solo con indicación del propietario, restaurar el respaldo del paso 2 con la aplicación cerrada y volver a la 0.6.2.

Mientras no se autorice, **no** ejecutar `migrate`, `server-start` ni `build-macos` sobre la instalación real desde `etapa-7`:

- `migrate` aplicaría la 0008 sin el respaldo previo.
- `server-start` arrancaría un servidor 0.7.0 que vería el esquema 7 como incompatible (readiness 503).

### Aplicación en la instalación real

9 de noviembre de 2026, con autorización del propietario («Autorizo aplicar la 0008 con el procedimiento del punto 18»), desde `etapa-7` y con la aplicación cerrada:

- **Preparación.** La comprobación de solo lectura previa encontró: migraciones 1–7, sin turnos, 1 caja, 0 pagos, 5 cuentas y 94 permisos.
- **Respaldo y ensayo.**
  - Respaldo `backups/adel_pos-20261109T184542Z-fb3ebf.dump`.
  - `restore-check.py`: 79 tablas con conteos idénticos.
  - El procedimiento decía 78, el número del esquema 6 antes de la 0007; el criterio de conteos idénticos se cumplió.
- **Migración.** `server-stop` ordenado; `migrate` → `applied:1`; la repetición → `applied:0`.
- **Arranque.** `server-start` inició el 0.7.0: readiness 200, esquema compatible y versión 0.7.0 con esquema 8.
- **Comprobación de solo lectura posterior.**
  - Mismos conteos de datos.
  - 113 permisos: los 19 nuevos solo en el administrador; los demás roles sin cambios.
  - `cash_limit` y `usd_cash` desactivadas.
  - Denominaciones (12 MXN, 6 USD) y 4 categorías de gasto.
  - Ningún dato de caja ficticio.
- **Aplicación.** `build-macos` (`.app` 0.7.0) y `open-app`: la aplicación se inició y el servidor respondió 200.
- **Inmutabilidad.** La 0008 es ahora inmutable; la siguiente ampliación requiere 0009.

### Limitaciones finales

Las de [CASH_SHIFTS_AND_RECONCILIATION.md](CASH_SHIFTS_AND_RECONCILIATION.md) («Limitaciones reales»). Además:

- No se verificó en la ventana nativa, con pantalla táctil física, con dos terminales nativas ni con impresoras o cajón reales.
