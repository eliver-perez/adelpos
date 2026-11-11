# Reportes, indicadores y exportaciones · Etapa 9A

Versión 0.9.0, API v1, esquema 17 (migraciones `0015` a `0017`; el módulo Reportes usa la
`0017_reports.sql`). **Las migraciones 0015–0017 se aplicaron a la instalación real el 10 de noviembre de 2026** con
autorización previa del propietario, tras respaldo y ensayo de restauración (registro en
[ETAPA_9A_AVANCE.md](ETAPA_9A_AVANCE.md), «Instalación real»). Este documento describe solo lo implementado y probado en
la rama `etapa-9a`. Las decisiones del propietario están en [etapas/ETAPA_9A.md](etapas/ETAPA_9A.md) §0-B; las decisiones
menores, las cifras de cada prueba y las mediciones, en [ETAPA_9A_AVANCE.md](ETAPA_9A_AVANCE.md) (fases B y C); los
contratos HTTP completos, en [API_CONTRACTS.md](../API_CONTRACTS.md) (secciones «Etapa 9A» de las partes B1, B2, C1 y
C2); la evidencia de verificación, en [VERIFICATION_STAGE9A.md](VERIFICATION_STAGE9A.md).

**Etapa 9B** (versión 0.9.1, esquema 19; migraciones 0018 y 0019 aplicadas a la instalación real el 11 de noviembre
de 2026 con autorización del propietario): lo único que cambia en este módulo es el **encabezado de las exportaciones**, que lleva
el logotipo del restaurante (o el de AD-EL POS si no hay) con su nombre y sus datos (ver «Exportaciones» y
[DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md)). Contenido, columnas, totales, orden, consultas, permisos, límites y
auditoría siguen igual.

## Contenido

- [Principios](#principios)
- [Arquitectura](#arquitectura)
- [Fechas comerciales y periodos](#fechas-comerciales-y-periodos)
- [Filtros](#filtros)
- [Definiciones de métricas](#definiciones-de-métricas)
- [Ventas](#ventas)
- [Pagos](#pagos)
- [Descuentos y cortesías](#descuentos-y-cortesías)
- [Propinas](#propinas)
- [Caja](#caja)
- [Inventarios](#inventarios)
- [Compras y proveedores](#compras-y-proveedores)
- [Costos y márgenes](#costos-y-márgenes)
- [Empleados](#empleados)
- [Resumen ejecutivo](#resumen-ejecutivo)
- [Interfaz](#interfaz)
- [Permisos](#permisos)
- [Exportaciones](#exportaciones)
- [Seguridad](#seguridad)
- [Pruebas](#pruebas)
- [Mediciones](#mediciones)
- [Limitaciones conocidas](#limitaciones-conocidas)
- [Cómo agregar un reporte](#cómo-agregar-un-reporte)

## Principios

- **Go es la autoridad**: cada cifra, total, porcentaje, periodo y orden se calcula en Go con `shopspring/decimal`
  (dinero, cantidades y porcentajes viajan como cadenas). React muestra los valores tipados con `services/money.ts` y
  nunca suma, resta, redondea ni ordena importes.
- **Registro cerrado**: ningún reporte recibe SQL ni nombres de columnas desde el cliente. Cada reporte se declara en Go
  con un identificador estable (`sales.by_day`) y un parámetro que el reporte no declara se rechaza.
- **Un solo resultado**: la pantalla, el PDF y el Excel salen del mismo `reports.Result`, calculado con la misma consulta,
  los mismos permisos y la misma fotografía de la base.
- **Los permisos de origen siguen mandando**: un permiso `reports.*` nunca sustituye a `cash.expected.read`,
  `inventory.costs.read` ni `inventory.valuation.read`. Lo protegido no se calcula, no se envía, no se totaliza, no se
  ordena y no se exporta para quien no tiene el permiso.
- **Sin efectos sobre la operación**: las consultas corren en una transacción de solo lectura propia, sin locks
  consultivos y con un número limitado de conexiones; un cobro, una captura o una liberación a cocina nunca esperan a un
  reporte.
- **Sin datos inventados**: los reportes leen registros reales (el libro de reconocimiento, cobros, caja, inventario y
  compras). Lo que el sistema no conserva (quién capturó un borrador, una marca de pago mixto) no se reporta o se deriva y
  se rotula como derivado.

## Arquitectura

### Componentes

| Pieza                                        | Responsabilidad                                                                                                                                                                               |
| -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `backend/internal/reports`                   | Registro de reportes (`registry.go`), motor (`engine.go`: orden, página y totales), parámetros (`params.go`), ejecutor (`runner.go`), resumen ejecutivo, opciones de filtros y exportaciones. |
| `backend/internal/reports/operational`       | Paquete solo de pruebas de integración de los reportes de caja, inventarios, costos, compras y empleados (con su propia base temporal).                                                       |
| `backend/internal/businessday`               | Día comercial y periodos: paquete neutral que usan inventario (día operativo) y reportes, para que exista una sola definición del día.                                                        |
| `backend/internal/payments/recognition*.go`  | Reconocimiento de ventas: escribe el libro `app.sales_recognitions` en la misma transacción del cierre de la cuenta.                                                                          |
| `backend/internal/administration/reports.go` | Delegación de `reports/*`: autentica, autoriza y confirma la transacción corta; ejecuta la consulta en el ejecutor de reportes; audita las exportaciones y envía el archivo binario.          |
| `desktop/src/pages/reports/`                 | Pantalla «Reportes» (catálogo, reporte, resumen ejecutivo, gráfico elegido por la forma del resultado).                                                                                       |
| `desktop/src/components/reports/`            | Filtro de periodo, tabla con totales de Go, tarjetas de indicadores, gráficos SVG propios y botones de exportación.                                                                           |
| `desktop/src/services/reports.ts`            | Tipos y llamadas `reports_*`.                                                                                                                                                                 |
| `desktop/src/services/reportExport.ts`       | Llamada a la operación cerrada `export_report`.                                                                                                                                               |
| `desktop/src-tauri/src/export.rs`            | Operación cerrada `export_report`: descarga, validación, diálogo nativo de guardado y escritura atómica.                                                                                      |

`reports` no llama handlers de otros módulos ni toma sus locks: solo usa SQL propio de lectura y funciones puras
exportadas (`payments.AccountStandingSQL`, `payments.ExpectedCash` con `payments.ExpectedPaymentsMXNSQL` /
`ExpectedPaymentsUSDSQL`, `payments.CashOperationTypeLabel`, `inventory.TheoreticalCosts` y los nombres visibles de
inventario, y `businessday`).

### Transacción de solo lectura y semáforo

1. `administration` autentica la petición (terminal, sesión y permisos leídos de PostgreSQL), valida la ruta, los
   permisos del reporte y la forma de los parámetros en su **transacción corta habitual** y la confirma. Una denegación
   se audita como `access.denied`, igual que en el resto de la API. El catálogo y las opciones de filtros (lecturas
   pequeñas sin locks) se responden en esa misma transacción.
2. La consulta del reporte o del resumen corre **después**, en el ejecutor (`reports.Runner`), dentro de otra
   transacción **`REPEATABLE READ READ ONLY`**: una sola fotografía para el periodo, los renglones, los totales, las
   cifras resumidas y la fecha de generación (`now()` de esa transacción). La zona horaria y la «Hora de inicio del día»
   se leen en la misma fotografía.
3. El ejecutor usa un **semáforo propio de 2 conexiones** del pool compartido. Si las dos están ocupadas responde de
   inmediato `503 REPORTS_BUSY` («Hay otros reportes en curso. Espera unos segundos y vuelve a intentarlo.») sin esperar
   ni competir con cobros, captura o cocina.
4. Sin locks consultivos: la aceptación comprueba que un cobro se registra mientras un reporte mantiene abierta su
   transacción (no espera), que el reporte no ve ese cobro y que una escritura dentro de la transacción del reporte falla
   con el código 25006 de PostgreSQL.

### Tiempos

| Límite                                               | Valor                                                                      |
| ---------------------------------------------------- | -------------------------------------------------------------------------- |
| `statement_timeout` de la consulta                   | 30 s (`503 REPORT_TIMEOUT` al excederlo)                                   |
| `lock_timeout`                                       | 2 s (solo toma `AccessShareLock`; protege de una migración o DDL en curso) |
| `idle_in_transaction_session_timeout`                | 45 s                                                                       |
| Respuesta HTTP de `reports/*`                        | 40 s (las demás rutas, 10 s); plazo de escritura ampliado a 45 s           |
| Respuesta HTTP de `reports/export/*`                 | 110 s; plazo de escritura ampliado a 115 s                                 |
| Puente nativo `reports_*` / `export_report`          | 45 s / 120 s                                                               |
| Renglones procesados en memoria (pantalla y archivo) | 100 000 (`422 REPORT_TOO_LARGE`)                                           |
| Página del detalle                                   | 50 por omisión, hasta 200 (la interfaz usa 50)                             |

El servidor HTTP corta toda respuesta a los 15 s (`WriteTimeout` de `main.go`); las rutas de reportes amplían su propio
plazo de escritura con `http.ResponseController`, como ya hacía SSE.

### Registro cerrado

Cada reporte es una `Definition` declarada con `register(...)` en el `init` de su archivo: identificador estable
`grupo.nombre`, grupo de navegación, título, descripción, permisos (todos se exigen, además de `reports.read`), si exige
periodo (`required`) o es una fotografía de la situación actual (`none`), si admite comparación, la fecha que usa
(`DateBasis`), los filtros que admite, las columnas tipadas, el orden por omisión, la consulta y, si hace falta, sus
totales propios. `TestReportRegistryIsClosedAndConsistent` valida el registro completo.

Hay **92 reportes** en 13 grupos, en este orden de navegación:

| Grupo                      | Reportes                                                                                                                                                                                                                                                                                                       |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Resumen ejecutivo          | El resumen (`GET reports/dashboard`), con 9 secciones.                                                                                                                                                                                                                                                         |
| Ventas (15)                | `sales.summary` (por tasa de IVA), `by_day`, `by_week` (ISO), `by_month`, `by_hour`, `by_product`, `by_variant`, `by_mode`, `dine_in` (área y mesa), `takeaway` (origen), `by_price_list`, `by_shift`, `accounts` (cuentas cerradas), `evolution` (con el anterior) e `in_progress` («En curso», sin periodo). |
| Categorías (1)             | `sales.by_category`.                                                                                                                                                                                                                                                                                           |
| Productos (8)              | `products.top`, `bottom` (incluye activos sin ventas), `combos`, `combo_components` (sin importe), `complements`, `cancelled`, `discounted` y `trends`.                                                                                                                                                        |
| Pagos (12)                 | `payments.by_method`, `by_register`, `by_shift`, `by_user`, `by_day`, `list`, `mixed_partial`, `refunds`, `corrections`, `tips`, `currency` (efectivo por moneda) y `pending` (importes pendientes, sin periodo).                                                                                              |
| Descuentos y cortesías (8) | `discounts.summary` (por día con % sobre ventas), `by_reason`, `by_user`, `by_product`, `list`, `general`, `courtesies` y `authorizations`.                                                                                                                                                                    |
| Caja (7)                   | `cash.shifts`, `sales_collections_by_shift`, `by_register`, `by_day`, `operations`, `cuts` y `by_user`.                                                                                                                                                                                                        |
| Cocina y operación (3)     | `kitchen.demand_by_product`, `demand_by_hour` y `stations`.                                                                                                                                                                                                                                                    |
| Inventarios (18)           | `inventory.stock`, `stock_by_warehouse`, `negative`, `below_minimum`, `valuation`, `average_costs`, `kardex`, `entries`, `outflows`, `transfers`, `adjustments`, `waste`, `counts`, `count_differences`, `consumption_by_item`, `consumption_by_day`, `consumption_by_product` y `consumption_by_kitchen`.     |
| Compras (7)                | `purchases.received` (compras recibidas por periodo), `ordered`, `pending`, `by_supplier`, `by_item`, `supplier_returns` y `cash_links`.                                                                                                                                                                       |
| Proveedores (1)            | `suppliers.activity`.                                                                                                                                                                                                                                                                                          |
| Costos y márgenes (9)      | `costs.margin_by_product`, `margin_by_category`, `margin_by_period`, `cost_status`, `theoretical_products`, `theoretical_recipes`, `consumption_valued`, `waste` y `cost_adjustments`.                                                                                                                         |
| Empleados (3)              | `employees.activity`, `responsible_sales` y `confirmations`.                                                                                                                                                                                                                                                   |

Las columnas, los filtros y los permisos de cada reporte están en [API_CONTRACTS.md](../API_CONTRACTS.md) («Reportes
registrados (fase B1)» y «Reportes nuevos (45)» de la parte B2). El catálogo (`GET reports/catalog`) solo incluye los
grupos con reportes que el usuario puede consultar y, en cada reporte, solo las columnas que puede ver.

### Modelo de resultado único

`GET reports/run/{id}` responde un `Result`:

- `report` y `meta`: título, periodo resuelto (etiqueta de Go, primer y último día comercial, instantes
  `[desde, hasta)`, días, año y semana ISO, `in_progress`), comparación, filtros aplicados con su texto legible, fecha de
  generación, zona horaria, hora de inicio del día, fecha usada (`date_basis`), avisos (`warnings`, con código) y notas.
- `columns`: clave, rótulo, tipo (`text`, `code`, `integer`, `quantity`, `money_mxn`, `money_usd`, `percent`, `date`,
  `datetime`), visibilidad (`default` u `optional`), clase de total (`sum`, `custom` o ninguno), si se puede ordenar y
  descripción.
- `rows` (la página), `totals`, `summary` (cifras resumidas), `page` y `sort`.

**Los totales se calculan en Go sobre todo el conjunto filtrado**, no sobre la página: cambiar de página nunca cambia los
totales. El motor ordena, pagina y suma las columnas `sum`; cada reporte completa las `custom` (ticket promedio,
porcentajes). Formato: dinero y porcentajes como cadena con dos decimales, cantidades como cadena decimal exacta, enteros
como número, fechas `AAAA-MM-DD` (día comercial), instantes RFC 3339 en UTC (se muestran en la zona del establecimiento)
y ausente como `null`.

### Errores

| Código                      | Cuándo                                                                                                                                                                                  |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `400 VALIDATION_ERROR`      | Parámetro no declarado o repetido, periodo inválido, orden por una columna no ordenable o protegida, filtro con valor inválido; en la exportación, además, formato, papel o paginación. |
| `403 PERMISSION_DENIED`     | Falta `reports.read`, un permiso del reporte o el de exportación (auditado como `access.denied`).                                                                                       |
| `404 NOT_FOUND`             | Reporte, ruta o fuente de filtros inexistente.                                                                                                                                          |
| `422 REPORT_TOO_LARGE`      | Más de 100 000 renglones en el conjunto filtrado.                                                                                                                                       |
| `422 EXPORT_TOO_LARGE`      | Más de 50 000 renglones al exportar (el mensaje dice cuántos; no se genera nada).                                                                                                       |
| `422 EXPORT_FILE_TOO_LARGE` | Archivo de más de 32 MiB.                                                                                                                                                               |
| `503 REPORTS_BUSY`          | Semáforo de reportes lleno (no espera).                                                                                                                                                 |
| `503 REPORT_TIMEOUT`        | La consulta excedió su `statement_timeout`.                                                                                                                                             |

Los mensajes están en español y nunca contienen importes protegidos.

## Fechas comerciales y periodos

### Hora de inicio del día

El día comercial lo define **una sola hora**, la «Hora de inicio del día» (`inventory_settings.operating_day_cutoff`,
05:00 por omisión), con la zona horaria del establecimiento. Es la misma hora de corte del día operativo del inventario
(etapa 8); no existe otra. Un instante pertenece al día cuya fecha local tiene la hora de inicio en o antes de él, hasta
la hora de inicio del día siguiente:

- **Con 05:00, una venta de la 01:30 del día 11 pertenece al día comercial 10.**
- Con 00:00, el día comercial es la fecha del calendario (la misma venta pertenece al día 11).
- Cambio de horario: si la hora de inicio no existe ese día (adelanto), el día empieza en el primer instante posterior al
  salto; si ocurre dos veces (atraso), empieza en la primera.
- **Cambiarla nunca reescribe registros**: solo cambia cómo se agrupan los instantes guardados.

Se edita **solo** en Configuración general (`PUT settings/business-day`, con `settings.manage` y revisión optimista),
que explica su efecto con el ejemplo de la 01:30; se audita como `settings.business_day_changed` (antes y después) y
publica `inventory.settings.changed`. Inventarios → Parámetros la muestra en solo lectura y `PUT inventory/settings`
rechaza cambiarla («La hora de inicio del día se cambia en Configuración general.»).

Los periodos se filtran por instantes `[desde, hasta)` calculados en Go (`businessday`) con la zona y la hora de inicio,
y las agrupaciones por día en SQL usan `((instante AT TIME ZONE zona) - hora de inicio)::date`, que coincide con Go salvo
en la hora repetida de un atraso de horario junto a la hora de inicio.

### Periodos

| `period`        | Parámetro                   | Periodo                                                     |
| --------------- | --------------------------- | ----------------------------------------------------------- |
| `today`         | —                           | Día comercial en curso (por omisión).                       |
| `yesterday`     | —                           | Día comercial anterior.                                     |
| `current_week`  | —                           | Semana ISO del día en curso, completa (de lunes a domingo). |
| `current_month` | —                           | Mes del día en curso, completo.                             |
| `current_year`  | —                           | Año del día en curso, completo.                             |
| `day`           | `date=AAAA-MM-DD`           | Un día comercial.                                           |
| `iso_week`      | `week=AAAA-Wss`             | Una semana ISO 8601.                                        |
| `month`         | `month=AAAA-MM`             | Un mes.                                                     |
| `year`          | `year=AAAA`                 | Un año.                                                     |
| `range`         | `from`, `until` (incluidos) | Días comerciales consecutivos, hasta 400.                   |

Los periodos «actuales» incluyen los días que faltan y se marcan `in_progress`. Los reportes de situación actual
(`period: none`: «En curso», importes pendientes, existencias, valuación, costos promedio, pendiente de recepción y costos
teóricos) no admiten periodo.

### Semana ISO 8601

La semana empieza en lunes y la semana 1 es la que contiene el primer jueves del año: el 1 de enero puede pertenecer a la
última semana del año anterior y algunos años tienen semana 53. El parámetro es el año ISO con la semana (`2026-W53` va
del 28 de diciembre de 2026 al 3 de enero de 2027) y la etiqueta de Go muestra su rango. El sistema anterior no usaba
semanas ISO (en algunos años se adelantaba una semana y nunca tenía semana 53); gana la decisión del propietario.

### Comparación

`compare=previous` (solo en los reportes con `comparison: true` y en el resumen ejecutivo) agrega el periodo anterior
equivalente: el día, la semana ISO, el mes o el año anteriores; en un rango, los mismos días inmediatamente antes. Un
periodo actual se compara con el anterior completo.

### Fecha que usa cada reporte

Cada reporte declara su fecha (`meta.date_basis`), que la pantalla y los archivos muestran:

| Reportes                                                         | Fecha                                                                |
| ---------------------------------------------------------------- | -------------------------------------------------------------------- |
| Ventas, categorías, productos, descuentos, márgenes, responsable | Cierre de la cuenta (reconocimiento de la venta).                    |
| Productos cancelados                                             | Cancelación de la unidad.                                            |
| Cocina y operación, consumo de inventario                        | Liberación a cocina (`kitchen_unit_releases`).                       |
| Cobros, propinas, devoluciones y correcciones                    | Registro del cobro o de la devolución (`created_at`).                |
| Autorizaciones de supervisor                                     | Registro de la autorización.                                         |
| Caja por turno                                                   | Fecha comercial del turno (fecha de apertura en la zona, como Caja). |
| Caja por día, movimientos de efectivo y cortes                   | Momento de cada registro, con la hora de inicio del día.             |
| Inventarios (movimientos, ajustes, mermas, conteos)              | Fecha del movimiento o del documento (`occurred_at`).                |
| Compras                                                          | Recepción, orden o devolución a proveedor, según el reporte.         |

## Filtros

- Cada reporte declara sus filtros; el catálogo los envía con su rótulo, tipo (`choice`, `id` o texto) y opciones, y Go
  rechaza cualquier otro parámetro. Los filtros de ventas, cobros, descuentos y caja son `mode` (salón o para llevar),
  `area_id`, `table_id`, `category_id`, `product_id`, `price_list_id`, `origin`, `register_id`, `shift_id`,
  `method_id`, `user_id`, `reason_id` (UUID o `none`, «Sin motivo del catálogo»), `kind`, `state`, `group_by` y `scope`;
  los de inventario y compras, `warehouse_id`, `item_id`, `category_id` de insumos, `supplier_id`, `modality` (`order`,
  `direct` o `counter`) y `currency` (`MXN` o `USD`). `state` y `kind` toman valores propios en cada reporte.
- Las opciones salen de `GET reports/filters/{source}` (`registers`, `shifts` —los 200 más recientes—, `users` —sin
  `sa`—, `areas`, `tables`, `products` con búsqueda, `categories`, `payment_methods`, `price_lists`, `discount_reasons`,
  `origins`, `warehouses`, `inventory_items` con búsqueda, `inventory_categories` y `suppliers`): solo nombres y estados,
  nunca importes.
- Con filtro de producto o de categoría, los descuentos generales y los cargos de cuenta no se incluyen (no pertenecen a
  un producto); los reportes por producto los muestran como cifras de conciliación.
- Los reportes de ventas no ofrecen filtro de turno (solo `sales.by_shift` agrupa por turno), para no aislar la venta de
  un turno en arqueo.
- Los filtros aplicados viajan en `meta.filters` con su texto legible; los archivos los repiten en el encabezado (PDF) o
  en la hoja «Filtros» (Excel).

## Definiciones de métricas

Las mismas que documenta `backend/internal/reports/doc.go` (§6 de la especificación), tal como quedaron, más las de caja,
inventario, costos y compras de la fase B2:

- **Venta (reconocimiento)**: la venta cuenta cuando la cuenta se cierra liquidada, en el día del cierre (libro
  `app.sales_recognitions`, escrito por `payments.closeIfSettled` en la misma transacción). Una reliquidación tras una
  reapertura reconoce solo las diferencias en su día; una devolución nunca resta de la venta.
- **Importe bruto**: precio de las unidades antes de promociones (IVA incluido).
- **Promociones**: descuento de las promociones del catálogo (`restaurant_item_units.discount`).
- **Descuentos**: descuentos manuales vigentes, por partida (asignados a unidades) y generales de cuenta, sin cortesías.
  **Cortesías**: importe de las cortesías al 100 %.
- **Cargos**: cargos generales de cuenta (con su tasa de IVA).
- **Venta neta** = bruto − promociones − descuentos − cortesías + cargos (IVA incluido, sin propinas).
- **IVA**: incluido en la venta neta, por cuenta y tasa con la regla de `payments/finance.go` (base = total / (1 + tasa)
  redondeada; IVA = total − base); el IVA del periodo es la suma de los desgloses por cuenta, nunca un recálculo sobre el
  agregado. **Subtotal sin IVA** = neta − IVA.
- **Cuentas cerradas**: cuentas con su primer reconocimiento en el periodo (una reliquidación no vuelve a contarlas).
  **Ticket promedio** = venta neta / cuentas cerradas.
- **Venta antes de descuentos** = bruto − promociones + cargos; **% de descuentos sobre ventas** = (descuentos +
  cortesías) / venta antes de descuentos.
- **En curso**: cuentas abiertas con total, pagado y saldo de `payments.AccountStandingSQL`; no son venta. «Pagada sin
  entregar»: para llevar liquidado sin entrega. «Reabierta»: ya se cerró antes.
- **Cobro**: pago registrado con un método, en su fecha (`payments.created_at`). **Cobrado neto** = pagos − devoluciones
  − reversos de corrección; el pago nuevo de una corrección es un pago. **Propinas netas** = propinas cobradas − propinas
  devueltas (nunca son venta).
- **Devolución**: `payment_refunds` de clase `refund`, en su propia fecha. **Corrección**: clase `correction` más el pago
  que la sustituye.
- **Pago mixto**: cuenta con pagos de más de un método en el periodo; **parcial**: más de un pago del mismo método
  (derivados; no existe una marca).
- **Efectivo por moneda**: pesos y dólares físicos de `cash_movements` (recibido y entregado), con la equivalencia al tipo
  de cambio de cada cobro; nunca se suman pesos y dólares. Exige `cash.expected.read` (paridad con Caja) y excluye los
  turnos en arqueo.
- **Turnos en arqueo** (`closing`): sus cobros se excluyen de todo agregado para todos, con un aviso.
- **Descuento por motivo**: nombre histórico guardado (`reason_name`); sin motivo del catálogo, «Sin motivo del
  catálogo». Los descuentos se fechan con el reconocimiento de su venta.
- **Combos**: el combo es la unidad vendida; sus componentes solo se reportan como cantidades preparadas, sin importe.
- **Demanda y cocina**: fecha de liberación a cocina (`kitchen_unit_releases`), indicada en el reporte.
- **Esperado de caja**: el de un turno abierto es el cálculo único de `payments/cash_expected.go`; el de un turno cerrado,
  el que guardó su cierre. **Diferencia** = contado − esperado, por moneda.
- **Consumo**: el neto de los movimientos `consumption` + `consumption_reversal` de las unidades, fechado con la
  liberación canónica; cada unidad comercial se cuenta una sola vez.
- **Costo de lo vendido**: por cada renglón reconocido, el costo neto actual de su unidad (−Σ del valor de sus consumos y
  reversiones, sin los `cost_adjustment`). **Margen estimado** = venta sin IVA de las unidades con costo − su costo.
- **Compras recibidas**: recepciones del periodo (de una orden, directas o de mostrador); **pagado desde caja** es la
  parte vinculada a gastos de caja, ya incluida en el total (nunca otra compra).

## Ventas

### Reconocimiento inmutable

`restaurant_accounts.closed_at` no sirve para reportar: una devolución reabre la cuenta y lo borra, y al volver a
liquidarse cambia de fecha; una cuenta reabierta cuyas unidades se cancelan queda abierta con saldo 0. Calcular la venta
desde el estado actual movería o borraría la venta del día original. Por eso la 0017 crea el libro
**`app.sales_recognitions`**, de solo inserción (disparadores que rechazan `UPDATE`, `DELETE` y `TRUNCATE`):

- Lo escribe `payments.closeIfSettled` en la **misma transacción** en que la cuenta queda liquidada (por un cobro, un
  ajuste o una entrega).
- Fuentes: `unit` (cada unidad activa: cantidad 1, bruto, promoción y neto), `allocation` (un descuento o cortesía por
  partida asignado a una unidad, que conserva su motivo y su producto) y `adjustment` (un cargo general con su tasa, o un
  descuento o cortesía general repartido entre tasas en proporción con `payments.allocate`; el último registrado recibe
  lo que falta en cada tasa). La clave de una fuente es (clase, unidad, ajuste, tasa).
- **Primer cierre = todo; reliquidación tras una reapertura = solo las diferencias** (una unidad cancelada después de
  reabrir resta en el día del nuevo cierre); nada si nada cambió. La suma es global por fuente: una unidad transferida,
  dividida o combinada se reconoce una sola vez, en la cuenta donde estaba al cerrarse.
- IVA por renglón con la regla de `finance.go`: provisional por renglón y el residuo de centavos de cada cuenta y tasa
  repartido uno a uno entre los renglones de mayor neto, de modo que la suma por cuenta y tasa es exactamente el desglose
  de la cuenta.
- Datos históricos copiados al reconocer: folio, modalidad, origen, mesa y área de la cuenta al cerrar, responsable, turno
  y caja de la operación que cerró (los del pago de esa operación; sin pago, el turno activo de la caja de la terminal),
  actor, terminal, operación, producto (nombre, código y tipo), variante, categoría, lista de precios, motivo histórico,
  concepto, quién aplicó y quién autorizó. Renombrar un producto o un motivo después no cambia los reportes.
- La consulta de diferencias que ejecuta cada cierre usa los índices por cuenta y por unidad (0.14 ms con 200 000
  renglones).

**Venta del periodo = Σ de los renglones con fecha de reconocimiento en el periodo.** Una cortesía al 100 % que cierra la
cuenta reconoce una venta de 0 con su importe en «Cortesías».

### «En curso» y reaperturas

`sales.in_progress` (sin periodo) lista las cuentas abiertas con su total, pagado y saldo (la fórmula única de
`payments.AccountStandingSQL`), que no son venta, y distingue «Pagada sin entregar» (para llevar liquidado sin entrega) y
«Reabierta» (una cuenta que ya se había cerrado). Una devolución se reporta en su propio día (en Pagos) sin alterar la
venta del día original; una cuenta reabierta que nunca vuelve a cerrarse conserva su reconocimiento y aparece en «En
curso» como reabierta.

### Relleno inicial y su limitación

La 0017 rellena el libro para las cuentas **hoy cerradas**: fecha = primer `payments.account_closed` de la auditoría de la
cuenta (o `closed_at` si no existe); turno y caja = los del pago registrado en ese cierre (sin pago, sin turno); actor y
terminal = los de esa auditoría; importes y nombres = los del estado actual. El cálculo es el de Go
(`buildRecognitionTargets`): la prueba de integración ejecuta el bloque de la migración sobre cuentas cerradas por la API
y lo compara con lo que escribió Go (iguales por fuente), y `TestStage9AReportsUpgradeBackfillsClosedAccounts` lo aplica
a una base en esquema 14. **Limitaciones**: el historial de reaperturas anterior a la 9A no se reconstruye (una cuenta
reabierta y vuelta a cerrar antes de la 9A queda con sus importes actuales en la fecha de su primer cierre); las cuentas
reabiertas que hoy siguen abiertas no se rellenan (aparecen en «En curso» como reabiertas y se reconocerán al cerrarse);
la mesa y el área son las actuales de la cuenta.

### Reportes de ventas, productos y cocina

- Ventas por tasa de IVA, día, semana ISO, mes, hora local del cierre, producto, variante, modalidad, salón por área y
  mesa, para llevar por origen (ventanilla es un origen de para llevar), lista de precios, turno, cuentas cerradas y
  evolución con el periodo anterior. `sales.by_day` incluye los días sin ventas.
- **Ventas por turno**: la venta pertenece al turno de la operación que cerró la cuenta (los cobros, al turno de cada
  pago); los turnos en arqueo se excluyen.
- Productos más y menos vendidos («menos vendidos» agrega con cero los productos activos sin ventas del periodo, solo sin
  filtros de modalidad, área, mesa, origen o lista), combos, componentes preparados de combos (cantidades sin importe),
  complementos seleccionados (de la cotización congelada), cancelados con su fecha y motivo, con descuentos y tendencia
  con el anterior.
- Cocina: demanda por producto y por hora y preparación por estación, con la fecha de liberación a cocina.
- **Cuentas divididas y combinadas** no duplican venta: cada unidad se reconoce una vez.

## Pagos

- Cobros por método, caja, turno, usuario y día, y detalle de cobros (`payments.list`), en su fecha de registro.
- Devoluciones (`payments.refunds`) en su propio día y correcciones (`payments.corrections`: el reverso y el pago que lo
  sustituye). Pagos mixtos y parciales (`payments.mixed_partial`), derivados de los pagos registrados.
- Importes pendientes (`payments.pending`, sin periodo): saldos de las cuentas abiertas con la fórmula única.
- **Dólares físicos, cambio y tipo de cambio histórico**: `payments.currency` (efectivo por moneda) da los pesos y dólares
  físicos recibidos y entregados de `cash_movements`, con la equivalencia al tipo de cambio de cada cobro (el fijo de su
  turno); nunca se suman pesos y dólares. Ese reporte y las columnas «Recibido», «Cambio», «Dólares recibidos» y «Tipo de
  cambio» del detalle de cobros exigen además `cash.expected.read`, como en Caja (sumadas por turno permitirían deducir el
  esperado). Las devoluciones son solo en pesos (regla de la etapa 6.1).
- **Turnos en arqueo**: los cobros de un turno en arqueo se excluyen de todo agregado para todos los usuarios, con el
  aviso `SHIFT_CLOSING`, que solo dice cuántos turnos.

## Descuentos y cortesías

- Por día con el % sobre ventas, por motivo, por usuario y por producto; detalle, descuentos generales de cuenta,
  cortesías y autorizaciones de supervisor (con su propia fecha).
- **Motivo histórico**: se agrupa por el nombre que guardó el descuento al registrarse (`reason_name`); renombrar o
  desactivar un motivo después no cambia el reporte. Sin motivo del catálogo (registros anteriores a la etapa 7.1 o sin
  `reason_id`), «Sin motivo del catálogo» (filtro `reason_id=none`).
- **Fecha**: la del reconocimiento de su venta, para que el porcentaje sobre ventas cuadre con las ventas del mismo
  periodo. Los descuentos anulados se excluyen.
- Los descuentos no se atribuyen a turnos ni cajas en ningún reporte de caja: el resumen del turno de Caja tiene un
  defecto conocido (`readAdjustmentTotals`, en `payments/cash_reports.go`, atribuye descuentos por la caja actual y sin
  tope de fecha) que **no se replicó**.

## Propinas

- `payments.tips` y la cifra del resumen salen de los cobros, en su fecha; **propinas netas** = cobradas − devueltas.
- Las propinas **nunca son venta** ni llevan IVA: no entran en el reconocimiento ni en la venta neta, y en caja se
  muestran separadas de las ventas cobradas, como en el corte.
- No hay distribución de propinas (excluida por §34 de la especificación).

## Caja

Con `reports.cash.read`: turnos (`cash.shifts`), ventas y cobros por turno, historial por caja, caja por día,
movimientos de efectivo, cortes parciales y cierres, y operaciones de caja por responsable; en el resumen, las secciones
«Caja» y «Diferencias de caja».

- **Vista por turno**: los turnos cuya fecha comercial (fecha de apertura en la zona, como el historial de Caja) cae en el
  periodo, con todos sus importes: fondos, cobros (ventas cobradas, propinas, devoluciones, neto y neto en efectivo, como
  el corte), ingresos, retiros, gastos y correcciones por moneda sin regularizaciones, cortes parciales y lo contado al
  cerrar.
- **Vista por día** (`cash.by_day`): cada registro en el día comercial en que ocurrió (con la hora de inicio del día):
  aperturas y fondos, cobros, operaciones de efectivo, cortes, cierres y lo contado. Un turno de varios días reparte sus
  movimientos entre los días; en la vista por turno aparece en la fecha de su apertura con todo. Los dos calendarios son
  distintos y se rotulan.
- **Ventas y cobros por turno**: venta = reconocimientos cuyo cierre ocurrió en el turno (columnas con
  `reports.sales.read`); cobro = pagos del turno. Nunca se suman entre sí.
- **Paridad exacta con Caja**: sin `cash.expected.read` no se calculan ni se envían el esperado, el efectivo físico por
  moneda, las diferencias, sobrantes, faltantes, su equivalencia ni el resultado (el SQL ni siquiera los selecciona); sí
  lo que Caja muestra (fondo, cobros por método, operaciones por moneda y lo contado). Con el permiso, el esperado de un
  turno abierto es `payments.ExpectedCash` sobre componentes sumados con las mismas expresiones que Caja, y el de un turno
  cerrado el que guardó su cierre; la prueba lo compara con `GET payments/shifts/{id}/summary` en pesos y dólares.
- **Turnos en arqueo**: excluidos de todo agregado de caja, cobros y empleados para todos (también en la vista por día y
  en el resumen), con el aviso `SHIFT_CLOSING`; en la lista de turnos aparecen sin ningún importe.
- **Alcance de Caja**: solo se incluyen los turnos que el usuario puede consultar en Caja (`cash.shifts.read`; otra caja
  con `cash.shifts.read_all`; los cerrados con `cash.history.read` o si él los cerró), con el aviso `CASH_SCOPE` cuando el
  alcance está limitado.
- **Diferencias por moneda** con el tipo de cambio histórico del turno (por ejemplo, −2.50 MXN y +1 USD, equivalentes a
  15.50 con un tipo de cambio de 18.00), solo con `cash.expected.read`.

## Inventarios

Con `reports.inventory.read` y los permisos de la etapa 8:

| Reportes                                                                                                        | Permiso adicional                                                                    |
| --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Existencias actuales y por almacén, negativas, bajo mínimo, conteos y diferencias de conteo                     | `inventory.stock.read` (el teórico de un conteo revela existencias)                  |
| Kardex, entradas, salidas, transferencias, ajustes, mermas y consumos (por ingrediente, día, producto y cocina) | `inventory.movements.read`                                                           |
| Valuación                                                                                                       | `inventory.valuation.read` (el valor por insumo exige además `inventory.costs.read`) |
| Costos promedio                                                                                                 | `inventory.costs.read`                                                               |

- Sin `inventory.costs.read` no viajan costos unitarios, valores, bases de costo, «costo pendiente» ni los ajustes de
  costo del kardex; la valuación después de cada movimiento exige costos y valuación. El valor total de la valuación solo
  exige `inventory.valuation.read`, como en la etapa 8.
- **Consumo**: el neto de `consumption` + `consumption_reversal`, con la fecha de la liberación canónica; una unidad
  enviada a varias estaciones consume y cuenta una sola vez (el resumen da las unidades distintas aunque aparezcan en dos
  almacenes). Con solicitudes de consumo pendientes o con error en el periodo, el aviso `CONSUMPTION_PENDING` advierte
  que los datos pueden estar incompletos.
- **Mermas**: la merma operativa con su motivo y las cancelaciones «Preparado / merma» con el consumo conservado de la
  unidad, más las cuentas de cancelaciones «No preparado» y por clasificar. Una devolución de dinero nunca es merma.
- **Kardex**: existencia después de cada movimiento y, al filtrar un insumo, cifras de existencia inicial, entradas,
  salidas y final (del almacén, si se filtra).
- No hay valuación por almacén (un costo por insumo, como en la etapa 8): las existencias por almacén solo dan
  cantidades.

## Compras y proveedores

Con `reports.purchases.read`:

- **Compras recibidas por periodo** (`purchases.received`): fecha, folio de la recepción y de la orden, modalidad,
  proveedor, comercio, almacén, insumo, cantidad y unidad capturada y base, precio unitario sin IVA, descuento, importe sin
  IVA, tasa e IVA, importe y quién recibió; filtros de proveedor, insumo, categoría de insumo, almacén, modalidad y estado
  de la orden; cifras de recepciones, total, **pagado desde caja** (ya incluido en el total, nunca otra compra) y
  devoluciones del periodo (aparte). El valor de inventario y el costo por unidad base exigen `inventory.costs.read`.
- **Modalidad**: `order` (recepción de una orden de compra), `direct` (recepción directa de un proveedor formal) y
  `counter` («Compras de mostrador»: el proveedor especial `inventory_suppliers.kind = 'counter'` de la 0016, con el
  comercio concreto de la recepción).
- Compras ordenadas (con lo recibido y lo pendiente por renglón, sin borradores; las canceladas solo con su filtro),
  pendiente de recepción (situación actual, con un importe proporcional de referencia), compras por proveedor (recibido,
  ordenado, devoluciones y pagado desde caja, sin sumarlos entre sí) y por insumo, devoluciones a proveedor a su valor
  asociado, compras pagadas desde caja (los datos del gasto exigen además `cash.movements.read`, `cash.shifts.read_all` y
  `cash.history.read`) y proveedores (contacto, compras del periodo, última recepción y órdenes abiertas).
- Un vínculo con gastos de caja nunca suma como compra, ni el gasto como otra compra.

## Costos y márgenes

Con `reports.sales.read` **y** `inventory.costs.read`:

- **Margen estimado** por producto, categoría y día con el costo real de los consumos de las unidades reconocidas en el
  periodo (renglón × costo neto actual de su unidad; una unidad retirada en una reliquidación resta su costo). Cada unidad
  queda en uno de tres estados:

  | Estado    | Cuándo                                                                                                                            |
  | --------- | --------------------------------------------------------------------------------------------------------------------------------- |
  | Sin costo | Sin consumo registrado (vendida antes de activar el inventario o sin receta) o con la solicitud de consumo pendiente o con error. |
  | Estimado  | Algún consumo con costo pendiente o valuado con el último costo válido o el de referencia, o con configuración incompleta.        |
  | Firme     | En cualquier otro caso.                                                                                                           |

- Margen = venta sin IVA de las unidades con costo − su costo; % del margen sobre esa venta; % de la venta (sin IVA) con
  costo firme. Las cifras del periodo agregan los ajustes de costo (concepto aparte) y los descuentos y cargos generales
  de cuenta (que no pertenecen a un producto). Nota en cada reporte: **no es utilidad contable**.
- Una unidad cancelada después de venderse y clasificada «No preparado» queda en costo cero; «Preparado / merma» conserva
  su costo, que pasa a mermas cuando la reliquidación la retira.
- `costs.cost_status`: unidades sin costo o estimadas por producto, con **qué dato falta**.
- Costo teórico por producto (contra el precio vigente sin IVA) y por receta con el motor de la etapa 8
  (`inventory.TheoreticalCosts`, solo lectura); consumo valuado por categoría de insumo; costo de mermas; ajustes de costo
  del periodo. Los costos unitarios por unidad base son cantidades con 4 decimales.
- En el resumen, el costo de lo vendido, el margen, su porcentaje y la parte firme son `null` sin datos suficientes
  (ninguna unidad vendida con costo en el periodo).

## Empleados

Con `reports.employees.read` (actividad por empleado, ventas por responsable y confirmaciones), **solo relaciones
registradas**:

- **Se reporta**: quién confirmó (las unidades creadas en la transacción de su confirmación; las agregadas después por una
  modificación no cuentan), quién canceló, el responsable de la cuenta al cerrar (la venta se atribuye solo a él: nunca
  dos veces), quién cobró (sin turnos en arqueo), quién aplicó descuentos y cortesías, quién autorizó como supervisor,
  quién devolvió y quién registró movimientos de caja.
- **No se reporta**: quién capturó (el borrador se borra al confirmar y no se conserva), estadísticas de productividad,
  costos ni el esperado de caja.

## Resumen ejecutivo

`GET reports/dashboard` (`reports.read`) con el periodo y, por omisión, la comparación con el anterior equivalente. Cada
sección solo se calcula y se envía con sus permisos; un indicador lleva `report_id` solo si el usuario puede abrir ese
reporte.

| Sección              | Permisos                                                                       | Indicadores                                                                                                                       |
| -------------------- | ------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| Ventas               | `reports.sales.read`                                                           | Ventas, venta neta, cuentas, ticket promedio, unidades, cuentas abiertas y saldo en curso (actuales); serie diaria de venta neta. |
| Cobros               | `reports.payments.read`                                                        | Cobrado neto, cobros, devoluciones y propinas.                                                                                    |
| Descuentos           | `reports.discounts.read`                                                       | Descuentos, cortesías y % sobre ventas.                                                                                           |
| Caja                 | `reports.cash.read`                                                            | Turnos abiertos y en arqueo (actuales), aperturas, efectivo neto, ingresos, retiros y gastos.                                     |
| Diferencias de caja  | `reports.cash.read` + `cash.expected.read`                                     | Turnos cerrados, sobrantes, faltantes y diferencia en pesos.                                                                      |
| Compras              | `reports.purchases.read`                                                       | Compras recibidas, recepciones, ordenado, órdenes pendientes (actual) y devoluciones.                                             |
| Inventario           | `reports.inventory.read` + `inventory.movements.read`                          | Unidades con consumo, mermas y solicitudes pendientes (actual).                                                                   |
| Costos de inventario | `reports.inventory.read` + `inventory.movements.read` + `inventory.costs.read` | Consumo valuado, costo de mermas y ajustes de costo.                                                                              |
| Margen estimado      | `reports.sales.read` + `inventory.costs.read`                                  | Costo de lo vendido, margen, % de margen, parte firme y unidades sin costo.                                                       |

La variación porcentual es `null` si el anterior es cero (la pantalla muestra entonces la variación absoluta). El resumen
**no se recalcula con cada evento SSE**: la pantalla lo pide al abrir, con «Actualizar» y con un temporizador de 5 minutos
que solo consulta con la ventana visible.

## Interfaz

La sección **Reportes** (`reports.read`; también pantalla de inicio) es genérica sobre el catálogo de Go: los grupos, los
reportes, los filtros, las columnas, el periodo y la comparación salen de la declaración de cada reporte, de modo que un
reporte nuevo aparece sin escribir una pantalla.

- **Navegación**: un grupo por pestaña («Resumen ejecutivo» primero), en columnas iguales de una o dos filas completas;
  dentro del grupo, la lista de reportes a la izquierda desde 1440 px de ancho o un selector «Reporte» por debajo. Cada
  grupo recuerda su último reporte. Sin reportes visibles se explica el permiso que falta.
- **Filtro de periodo** (`components/reports/PeriodFilter.tsx`): rápidos (Hoy, Ayer, Semana actual, Mes actual, Año
  actual) y con dato (Día, Semana del año, Mes, Año, Personalizado), solo los que Go lista. Los campos se completan con el
  último periodo que resolvió Go (no con la fecha de la terminal) y se aplican con «Aplicar periodo» o Intro; las flechas
  de semana cruzan el cambio de año ISO. La semana se describe con su rango antes de consultar, pero vale el periodo que
  devuelve Go («Periodo aplicado», «Comparado con», «Inicio del día · zona»).
- **Filtros**: un solo panel con el periodo y los filtros del reporte; periodo y filtros son de la pantalla y se conservan
  al cambiar de reporte, guardados por parámetro y fuente (una categoría del catálogo nunca llega a un reporte de
  insumos). Productos e insumos se buscan en Go solo al pulsar «Buscar».
- **Tabla** (`components/reports/ReportTable.tsx`): las columnas tal como llegan (las protegidas no llegan y no se
  dibujan; las opcionales, con «Mostrar columnas adicionales»), cifras a la derecha con `tabular-nums`, totales de Go en
  `<tfoot>`, orden en Go con `aria-sort`, paginación de Go de 50 renglones y estados de carga, vacío y error con
  «Reintentar».
- **Gráficos SVG propios** (sin dependencias): tendencia, barras comparativas, barra segmentada y minigráfico, cada uno
  con «Ver como tabla». Se eligen por la forma del resultado (secuencia → tendencia, con la serie anterior en gris; hasta 5
  categorías → barra segmentada; más → barras de los primeros 10; detalle → sin gráfico). Paleta validada en claro y
  oscuro como tokens `--chart-*`; los rótulos son los textos de Go.
- Los reportes y el catálogo **no escuchan eventos SSE**: se consultan al abrir, al cambiar periodo, filtros, orden o
  página, y con «Actualizar».

## Permisos

Permisos nuevos de la 0017, registrados también en Go (`reports.Permissions`) y concedidos **solo al rol administrador**
(para que siga contando como administrador completo); ningún otro rol los recibe por inferencia:

| Permiso                  | Permite                                                                 |
| ------------------------ | ----------------------------------------------------------------------- |
| `reports.read`           | Consultar el módulo Reportes y el resumen ejecutivo (se exige siempre). |
| `reports.sales.read`     | Reportes de ventas, productos, categorías, cocina y operación.          |
| `reports.payments.read`  | Reportes de cobros, devoluciones y propinas.                            |
| `reports.discounts.read` | Reportes de descuentos, cortesías y autorizaciones.                     |
| `reports.cash.read`      | Reportes de caja.                                                       |
| `reports.inventory.read` | Reportes de inventarios.                                                |
| `reports.purchases.read` | Reportes de compras y proveedores.                                      |
| `reports.employees.read` | Reportes por empleado.                                                  |
| `reports.export.pdf`     | Exportar reportes a PDF.                                                |
| `reports.export.excel`   | Exportar reportes a Excel.                                              |

Permisos de origen que se reutilizan (la fase B2 no agregó permisos):

| Permiso                                                               | Efecto en los reportes                                                                                                                                                           |
| --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cash.expected.read`                                                  | Esperado, efectivo físico por moneda, diferencias, sobrantes, faltantes y resultado; efectivo por moneda; columnas físicas del detalle de cobros; sección «Diferencias de caja». |
| `cash.shifts.read`, `cash.shifts.read_all`, `cash.history.read`       | Alcance de los turnos en los reportes de caja (como en Caja).                                                                                                                    |
| `cash.movements.read` (+ `cash.shifts.read_all`, `cash.history.read`) | Datos del gasto en «Compras pagadas desde caja».                                                                                                                                 |
| `inventory.stock.read`                                                | Existencias, negativas, bajo mínimo, conteos y diferencias; cantidades de la valuación y de los costos promedio.                                                                 |
| `inventory.movements.read`                                            | Kardex, entradas, salidas, transferencias, ajustes, mermas y consumos; secciones de inventario del resumen.                                                                      |
| `inventory.costs.read`                                                | Costos, valores y costo pendiente; costos y márgenes (con `reports.sales.read`); costo y valor de las compras.                                                                   |
| `inventory.valuation.read`                                            | Valuación (el valor por insumo exige además `inventory.costs.read`).                                                                                                             |
| `settings.read` / `settings.manage`                                   | Consultar / cambiar la «Hora de inicio del día».                                                                                                                                 |

Todo se comprueba en Go en cada petición, también al llamar a la API directamente (probado reporte por reporte). Sin el
permiso, una columna protegida, sus valores, sus totales, el orden por ella y su cifra resumida no existen en la respuesta
ni en el archivo, y un aviso nunca lleva importes protegidos.

## Exportaciones

### Ruta y límites

`GET reports/export/{id}?format=pdf|xlsx&…` con los mismos parámetros que `reports/run/{id}` salvo la paginación (`page`
y `page_size` se rechazan: el archivo lleva todos los renglones del conjunto filtrado), más `paper=letter|a4` en el PDF.
Exige los permisos del reporte **y** `reports.export.pdf` o `reports.export.excel`, comprobados en la transacción de
autenticación (403 auditado como `access.denied`). El resumen ejecutivo no se exporta.

- **Misma consulta y mismo resultado que la pantalla**: el motor ejecuta el reporte en la transacción de solo lectura (el
  nombre del establecimiento se lee en la misma fotografía) y el archivo usa `Result.Columns`, `Result.AllRows`,
  `Result.Totals`, `Result.Summary` y `Result.Meta`. Nada se recalcula; las columnas y cifras protegidas no existen en el
  `Result` de quien no tiene su permiso ni, por tanto, en el archivo.
- La exportación conserva su lugar del semáforo de reportes hasta terminar el archivo (nunca hay más de dos consultas o
  archivos a la vez). Límites: **50 000 renglones** (`422 EXPORT_TOO_LARGE` con el número de renglones, sin generar),
  **32 MiB** (`422 EXPORT_FILE_TOO_LARGE`) y 110 s; la generación se interrumpe si la petición se cancela.
- Respuesta binaria con `Content-Type` exacto, `Content-Disposition: attachment` con un nombre ASCII de reporte y periodo
  (`ventas-por-dia_2026-11-01_2026-11-30.pdf`) y `Content-Length`. **El archivo no se guarda en el servidor** y no hay
  identificador de descarga.
- El archivo incluye todas las columnas que el usuario puede ver, también las opcionales que la pantalla oculta de
  inicio, en el orden del registro: el PDF y el Excel tienen las mismas columnas.

### PDF

`codeberg.org/go-pdf/fpdf` con las fuentes Go Regular y Bold (UTF-8 incrustadas; el texto se puede copiar y buscar):

- Desde la etapa 9B, la primera página empieza con el **encabezado de identidad**: el logotipo del restaurante (a lo más
  50 × 14 mm, con su proporción) y, a su derecha, el nombre (negrita 11 pt), la dirección y «Tel. … · correo» (8 pt) del
  establecimiento, solo los datos que existan en Configuración general, y una línea divisoria. Sin logotipo del
  restaurante (o con uno guardado que ya no pasa la validación) se usa el de AD-EL POS con el nombre del establecimiento.
  La imagen se incrusta una sola vez por documento (PNG RGB o JPEG de a lo más 600 × 168 px, codificado otra vez sin
  metadatos). La primera página crece ≈ 14.5 mm, así que un reporte que llenaba sus páginas puede ocupar una más.
- Primera página con establecimiento, reporte, descripción, periodo, comparación, filtros, fecha usada, hora de inicio
  del día y zona, orden, renglones y fecha de generación; cifras resumidas y avisos.
- Tabla con encabezados repetidos en cada página, renglones alternados, totales de Go y notas; encabezado breve en las
  páginas siguientes y pie «Página n de m».
- Carta (o A4) vertical si la tabla cabe con 8 pt; si no, horizontal con la letra necesaria (de 8 a 5.5 pt). Importes,
  cantidades, fechas y códigos nunca se parten; el texto se ajusta en varias líneas (un texto que excede una página entera
  se recorta con «…»; el Excel lo conserva completo).
- Importes con `moneytext` (`$1,234.56`, `US$20.05`), cada moneda en su columna.

### Excel

`github.com/xuri/excelize/v2`, escrito por flujo:

- Hoja «Datos» con encabezado distinguible y congelado, anchos según el contenido, números exactos con formato por
  columna (pesos `"$"#,##0.00` y dólares `"US$"#,##0.00` en columnas separadas), porcentajes como fracción (12.50 → 0.125
  con formato `0.00%`), fechas comerciales y horas del restaurante como fechas de Excel, códigos y textos con formato «@»
  y un renglón «Total» con los totales de Go como números.
- Hoja «Filtros» con los metadatos (establecimiento, reporte, periodo, comparación, fecha usada, hora de inicio, zona,
  orden, renglones, generación, filtros, resumen, avisos y notas). Desde la etapa 9B: renglón 1 con el logotipo en A1
  (anclado con `oneCellAnchor`, a lo más 200 × 56 px, sin hipervínculo, con texto alternativo «Logotipo del restaurante» o
  «Logotipo de AD-EL POS») y el nombre del establecimiento en B1; renglón 2 vacío; la tabla «Concepto | Valor» empieza en
  el renglón 3 con «Establecimiento» y, solo si existen, «Dirección», «Teléfono» y «Correo electrónico» antes de
  «Reporte». La hoja «Datos» es idéntica byte a byte a la de la 9A.
- Un importe con más de 15 dígitos significativos se escribe como texto exacto (Excel lo truncaría) y la hoja «Filtros»
  lo indica.
- **Escape de fórmulas**: ningún valor es fórmula; un texto que empieza por `=`, `+`, `-`, `@`, tabulador, retorno o
  salto de línea (o sus formas de ancho completo `＝ ＋ － ＠`) lleva un apóstrofo inicial visible. Sin hipervínculos,
  nombres definidos, vínculos externos ni macros; la única imagen es el logotipo del encabezado de la hoja «Filtros»
  (etapa 9B).

### Auditoría

`report.exported` en una transacción corta aparte, **después de generar el archivo y antes de enviarlo** (si no se puede
auditar, no se entrega): reporte, formato, papel, orientación, periodo, comparación, filtros con sus valores tal como se
enviaron (identificadores o claves), orden, renglones y bytes. Sin nombres de personas, importes ni contenido. Un 400 o
un 422 no se audita como exportación.

### Puente y diálogo de guardado

Operación cerrada `export_report(report, format, query)` en `desktop/src-tauri/src/export.rs`:

- Lista cerrada de los 92 reportes (`EXPORTABLE_REPORTS`, igual al registro de Go: `TestExportBridgeListsEveryReport` lee
  `export.rs` y exige que coincidan) y parámetros de la lista blanca `reports_*` de `api.rs` sin paginación ni búsqueda
  (más `paper` en PDF); valores de hasta 120 caracteres sin caracteres de control.
- Credenciales e invalidación como `api_request` (los tokens nunca llegan al WebView), 120 s, tope de 32 MiB y una
  exportación a la vez («Ya hay una exportación en curso.»).
- Antes de abrir el diálogo valida el `Content-Type` exacto y la firma del archivo (`%PDF-` o `PK\x03\x04`); el nombre
  sugerido se sanea (`[A-Za-z0-9._ -]`, sin carpetas ni punto inicial, extensión forzada, hasta 100 caracteres).
- Diálogo nativo «Guardar como» de **`tauri-plugin-dialog` 2.8.3, usado solo desde Rust**: la ventana no tiene permisos
  `dialog:*` ni `fs:*` (`capabilities/main.json` sin cambios, comprobado por una prueba Rust). Escritura atómica: temporal
  oculto en la misma carpeta, `sync_all` y renombrado; el temporal se borra si algo falla. Si Rust tiene que agregar la
  extensión al nombre elegido y ese archivo ya existe, no lo sustituye (el panel no lo confirmó) y pide volver a exportar.
- React recibe solo `{result: "saved" | "cancelled", file_name, size}`: nunca bytes, ruta completa ni tokens. Los botones
  «Exportar PDF» y «Exportar Excel» (`components/reports/ExportButtons.tsx`) solo aparecen con el permiso y con la
  exportación disponible en el catálogo, envían la misma consulta de la pantalla y comunican el resultado con
  `notify.ts` («Cobros por método: PDF guardado.», «Exportación a PDF cancelada: no se guardó ningún archivo.» o el error
  de Go o del puente).
- Playwright simula la operación en `desktop/e2e/native-driver.ts` (misma lista cerrada y misma lista blanca leídas de los
  archivos Rust, descarga desde Node y cola `nextSaveDialog('save' | 'cancel')`); los bytes quedan en Node.

## Seguridad

- **Ninguna ruta acepta SQL**: los reportes son un registro cerrado en Go, y el puente valida en Rust la clave del reporte
  (minúsculas, `.` y `_`, hasta 64 caracteres, sin `..` ni barras) y una lista blanca de parámetros.
- **Solo lectura**: transacción `READ ONLY` (una escritura falla con 25006), sin locks consultivos ni llamadas a handlers
  de otros módulos, con semáforo y límites de tiempo; un reporte no puede bloquear cobros, captura ni cocina.
- **Permisos de origen**: el esperado y el efectivo físico exigen `cash.expected.read` y quedan ocultos para todos durante
  el arqueo (cierre ciego); los costos exigen `inventory.costs.read`, la valuación `inventory.valuation.read` y las
  existencias `inventory.stock.read`. Lo que permitiría deducir un dato protegido no se calcula para quien no lo puede
  ver: la prueba revisa el JSON completo de los reportes de caja, del catálogo y del resumen sin esos valores para un
  usuario sin `cash.expected.read`.
- **Exportaciones**: mismos permisos que la pantalla más el de exportar; los 25 reportes con columnas protegidas se
  revisaron en PDF y en Excel sin ellas; escape de fórmulas y ningún contenido activo en Excel; auditoría antes de
  entregar; el archivo no se guarda en el servidor; validación de tipo y firma en Rust; diálogo y escritura solo desde
  Rust.
- **Datos personales**: la fuente `users` de los filtros excluye `sa`; la auditoría de exportación no guarda nombres ni
  importes.
- Los reportes no publican eventos SSE; el cambio de la hora de inicio del día publica `inventory.settings.changed` sin
  contenido.

## Pruebas

Detalle de lo ejecutado en [VERIFICATION_STAGE9A.md](VERIFICATION_STAGE9A.md).

- **Go unitarias**: `businessday/day_test.go` (venta de la 01:30 con 00:00 y con 05:00, día con hora de inicio, cambios de
  horario) y `period_test.go` (semanas ISO con cambio de año y semana 53, periodos relativos y comparaciones, periodos
  inválidos con mensaje en español); `reports/reports_test.go` (registro cerrado y consistente, permisos de Go iguales a
  la migración y solo del administrador, parámetros, totales, orden, página y formato); `reports/export_test.go`
  (contenido del PDF y del Excel, columnas protegidas ausentes, orientación y papel, archivo grande, textos y números de
  Excel con el escape, nombres de archivo, permiso de formato y lista del puente igual al registro);
  `payments/recognition_targets_test.go` (reconocimiento igual al desglose de la cuenta, cortesía con venta 0, residuo de
  IVA) y `payments/cash_expected_export_test.go` (esperado exportado igual al del turno).
- **Integración PostgreSQL** (`//go:build integration`): `reports/reports_integration_test.go` (`TestReportsAcceptance`,
  15 subpruebas: reportes sin turnos; venta al cerrar y «En curso»; pagos parciales, mixtos, propinas y correcciones;
  devolución en su día y cuenta reabierta con saldo 0; reliquidación con solo diferencias; motivo histórico y «Sin motivo
  del catálogo»; combos, divisiones y combinaciones sin doble conteo; dólares físicos con el tipo de cambio histórico;
  para llevar pagado sin entregar; totales del detalle iguales a los resumidos y paginación; permisos de cada reporte por
  API directa; resumen con comparación y filtros; `REPORTS_BUSY` y cobro sin esperar a un reporte; relleno de la 0017
  igual a Go; hora de inicio 00:00 y 05:00 con una venta de la 01:30 sin reescribir registros);
  `runner_integration_test.go` (transacción de solo lectura, sin locks consultivos, `REPORTS_BUSY` y `REPORT_TIMEOUT`);
  `export_integration_test.go` (`TestReportExports`: exportaciones por la API con sus permisos, auditoría, límites y los
  25 reportes con columnas protegidas); `reports/operational/operational_test.go` (`TestOperationalReports`, 14
  subpruebas: caja por turno con el esperado del cálculo único, caja por día con un turno de varios días, turno en arqueo
  excluido para todos, cierre con sobrante y cortes, esperado y diferencias ausentes de todo el JSON, alcance de Caja,
  márgenes «Sin costo», «Estimado» y firme, costo teórico igual al de recetas, existencias, kardex, mermas, conteos y
  consumos con sus permisos, compras sin duplicar el gasto vinculado, empleados sin doble conteo, diferencias por moneda,
  resumen bajo sus permisos y, en todos los reportes registrados, la suma del detalle igual al total);
  `database/stage9a_reports_upgrade_test.go` (la 0017 sobre una base en esquema 14 con cuentas cerradas).
  `./internal/reports` e `./internal/reports/operational` están en la tupla de `scripts/database.py`.
- **Vitest**: `pages/reports/reports.test.tsx` (27 pruebas: navegación por catálogo y permisos, cada periodo, semana ISO
  53 y cambio de año, rango personalizado, totales de Go en el pie, paginación sin cambiar totales, columna que Go no
  entrega, orden con `aria-sort`, filtros por parámetro y fuente, estados, resumen sin recarga por SSE, exportación por
  permiso con guardado, cancelación y error, elección de gráficos y ningún `type="number"`),
  `services/reportExport.test.ts` y `pages/businessDay.test.tsx` (la hora de inicio en Configuración general y las
  operaciones del puente).
- **Etapa 9B (encabezado)**: unitarias de `reports/export_identity_test.go` (`TestPrepareExportLogo`,
  `TestHeaderLogoChoice`, `TestContactLine`, `TestPDFHeaderWithRestaurantLogo`, `TestPDFHeaderFallsBackToBrandLogo`,
  `TestXLSXHeaderWithRestaurantLogo`, `TestXLSXHeaderFallsBackToBrandLogo` y `TestXLSXHeaderEscapesFormulaText`; un
  `tEXt` del original desaparece) y `brand` (marca embebida igual al original); integración: subtest «encabezado con el
  logotipo del restaurante o el de AD-EL POS» de `TestReportExports` (fuente del logotipo en la transacción de solo
  lectura, sin locks consultivos) y `identity` (PDF y Excel con el logotipo del restaurante y con el respaldo). Comparado
  con el código de la 9A: las cadenas del PDF y la hoja «Datos» del Excel son idénticas. `assertSafeWorkbook` admite
  exactamente el dibujo y la imagen del encabezado.
- **Rust**: `report_routes_are_closed_read_only_and_keyed` (`api.rs`) y las pruebas de `export.rs` (lista cerrada,
  parámetros en lista blanca, tipo y firma antes del diálogo, nombres saneados, escritura atómica, cancelación sin
  escribir, una exportación a la vez y ventana sin permisos de diálogo ni de archivos).
- **Playwright** (Chromium y WebKit, con Go y PostgreSQL temporales): `e2e/workflow-reports-9a.spec.ts` (resumen con la
  cifra de Go, tarjeta que abre «Ventas por día» con tendencia y tabla, ventas por producto por semana actual y semana ISO
  con el rango de Go, cobros por método con barra segmentada, exportaciones guardadas y una cancelada, sin desbordamiento
  a 1280 × 720 y 1920 × 1080 en ambos temas) y `e2e/workflow-reports-export-9a.spec.ts` (PDF guardado, Excel cancelado y
  guardado desde los botones, y rechazos de la operación cerrada).

## Mediciones

Todas en un solo equipo (Apple M1 Max), con un clúster PostgreSQL temporal y una base aislada, sin varias terminales.

**Ventas, cobros y descuentos** (`TestReportsVolumeMeasurement`, opcional con `ADEL_REPORTS_MEASURE=1`): 200 000
renglones de reconocimiento repartidos en un año y 100 000 cobros. `reports.Run` completo para el año actual:

| Reporte                                 | Renglones | Tiempo |
| --------------------------------------- | --------- | ------ |
| `sales.by_day`                          | 365       | 245 ms |
| `sales.by_product`                      | 5         | 323 ms |
| `sales.by_month`                        | 11        | 281 ms |
| `payments.by_method`                    | 2         | 107 ms |
| `payments.by_day`                       | 315       | 27 ms  |
| `discounts.summary`                     | 365       | 127 ms |
| `sales.evolution` (con el año anterior) | 365       | 116 ms |
| Resumen (año actual contra el anterior) | —         | 236 ms |

**Caja, inventarios, costos, compras y empleados** (`TestOperationalVolume`, opcional con `ADEL_REPORTS_MEASURE=1`, con
`-race`): 120 000 unidades vendidas y reconocidas en un año (100 000 con consumo procesado y 20 000 «Sin costo»), 200 000
movimientos de consumo y 60 000 cobros; 12 ejecuciones de cada reporte (p95 = la más lenta). Los más lentos para 365
días: `costs.margin_by_category` 838 ms de media (911 ms p95), `costs.margin_by_product` 815 ms (1 002 ms),
`costs.margin_by_period` 812 ms (916 ms), `inventory.consumption_by_product` 778 ms (1 065 ms) y el resumen con todas sus
secciones, 1 049 ms (1 173 ms); los de caja, entre 77 y 92 ms; los de compras, menos de 20 ms; el kardex de un mes de un
insumo (3 020 renglones), 119 ms; los de un día, entre 2 y 13 ms. Tabla completa en el avance.

**Cobro concurrente**: con el reporte de margen de un año en curso (933 ms) y su transacción de solo lectura abierta, la
consulta del saldo y el pago por la API tardaron 95 ms (54 ms sin reporte); la prueba exige que el cobro termine con la
transacción del reporte abierta y en menos de 3 s.

**Planes de consulta** (`EXPLAIN (ANALYZE, BUFFERS)`): las consultas de un día usan `sales_recognitions_recognized_idx`,
el índice de pagos por fecha, `inventory_movements_occurred_idx`, `inventory_movements_unit_idx` e
`inventory_consumption_requests_unit_idx`; las de un año recorren las tablas completas porque el año es todo el volumen.
Ningún reporte se acercó al límite de 30 s, por lo que no se agregó otra migración de índices (no existe la 0018).

**Exportaciones** (`TestReportExports`, con `-race`): `payments.list` con 3 004 renglones, PDF de 95 páginas (307 912
bytes) en 884 ms y Excel de 177 789 bytes en 2.9 s; con 50 000 renglones, PDF de 4.7 MB en 11.1 s y Excel de 2.7 MB en
7.7 s.

## Limitaciones conocidas

- **Instalación real**: las migraciones 0015–0017 se aplicaron a `adel_pos` el 10 de noviembre de 2026; el relleno
  inicial reconoció las 2 cuentas cerradas (9 renglones, venta neta reconocida $695.00) y los 10 permisos `reports.*`
  quedaron solo para el administrador. El recorrido de Reportes en la ventana nativa de la instalación real no se
  verificó.
- **Sin verificar en hardware**: la ventana nativa (WKWebView), el diálogo nativo real de guardado y la escritura en disco
  desde la ventana Tauri, y la pantalla táctil física. La escritura atómica se probó como función pura con un destino
  simulado y el flujo completo con la simulación de Playwright.
- **Relleno inicial**: el historial de reaperturas anterior a la 9A no se reconstruye; las cuentas reabiertas que hoy
  siguen abiertas no se rellenan; la mesa y el área del relleno son las actuales.
- **Días y calendarios**: la agrupación por día en SQL difiere de Go solo en la hora repetida de un atraso de horario
  junto a la hora de inicio; la vista de caja por día (hora de inicio) y por turno (fecha de apertura) usan calendarios
  distintos (se rotulan).
- **Volumen**: un conjunto filtrado de más de 100 000 renglones responde `REPORT_TOO_LARGE` (el kardex de un año con el
  volumen medido lo excede: se consulta por mes o con filtros); una exportación admite hasta 50 000 renglones y 32 MiB.
  Mediciones de un solo equipo, sin varias terminales reales.
- **Costos**: no hay valuación por almacén; el costo teórico es el de hoy (sin historial) y no incluye complementos
  elegidos ni componentes de combo (limitación de la etapa 8); el esperado de un turno cerrado antes de la etapa 7 (sin
  cierre registrado) no existe.
- **PDF**: las fuentes Go no tienen ideogramas CJK ni emojis (se ven como un recuadro; el Excel los conserva).
- **Encabezado (etapa 9B)**: un logotipo del restaurante más pequeño que el recuadro se amplía al mostrarse y puede verse
  borroso; un JPEG CMYK usa la conversión simple de Go; cada archivo pesa ≈ 28–29 KB más con la marca de AD-EL POS (fijo
  por archivo). Los PDF con el encabezado no se imprimieron.
- **Interfaz**: un periodo que no cabe en una página (un año por día, 365 renglones) no dibuja tendencia (se sugiere el
  reporte por semana o por mes); las barras de categorías muestran los primeros 10 renglones de la página; las opciones de
  filtros se consultan una vez por fuente mientras la pantalla está abierta; la interfaz no ofrece elegir papel A4 (Go lo
  admite con `paper`); a 1280 × 720 los reportes con muchas columnas se desplazan a lo ancho dentro de su tabla.
- **Extensión**: un reporte nuevo debe agregarse a `EXPORTABLE_REPORTS` de `export.rs` (la prueba Go lo exige) y un
  filtro nuevo a la lista blanca `reports_*` de `api.rs`.
- **Funciones del sistema anterior no incluidas** (reportadas sin implementar; requieren confirmación del propietario):
  reporte de modificaciones de cantidades autorizadas, lista de cuentas anuladas con quién autorizó, columna «Recibió» en
  compras (el reporte muestra quién registró la recepción), folio del comprobante de tarjeta y conteo de canceladas en el
  resumen.

## Cómo agregar un reporte

La guía completa está en el comentario del paquete (`backend/internal/reports/doc.go`). En resumen:

1. Declarar la `Definition` con `register(...)` en el `init` de un archivo del paquete: identificador `grupo.nombre`,
   grupo de `Groups`, título, descripción, permisos, fecha usada, filtros, columnas tipadas y orden por omisión.
2. Marcar las columnas protegidas con `requiring(columna, permiso)` y las cifras con `Figure.Permissions`; no calcular lo
   que permita deducir un dato protegido.
3. Escribir la consulta con `query.Args()`, `inPeriod`, `businessDate` y `localHour` (los mismos límites que Go) y
   devolver todos los renglones del conjunto filtrado: el motor ordena, pagina y suma en Go.
4. Avisos y notas en `Dataset.Warnings` / `Notes`, nunca con importes protegidos; indicadores del resumen con
   `registerDashboard(...)`.
5. Agregar el identificador a `EXPORTABLE_REPORTS` (`desktop/src-tauri/src/export.rs`) y cualquier filtro nuevo a
   `allowed_query_keys` de `api.rs`; la interfaz lo muestra sin pantalla nueva.
6. Pruebas: el registro se valida solo (`reports_test.go`); agregar integración con totales contra los registros,
   permisos por API directa y paginación.
