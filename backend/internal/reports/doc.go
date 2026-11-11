// Package reports es el módulo Reportes de la etapa 9A: un registro cerrado de reportes (ninguna
// ruta acepta SQL), un modelo de resultado único para la pantalla y las exportaciones, el resumen
// ejecutivo y las opciones de filtros.
//
// # Ejecución
//
// administration autentica y autoriza la petición en su transacción corta habitual y la confirma
// (Handle valida ahí la ruta, los permisos y la forma de los parámetros; una denegación se audita
// como en el resto de la API). La consulta corre después en el Runner: transacción REPEATABLE READ
// READ ONLY con statement_timeout de 30 s, lock_timeout de 2 s e idle_in_transaction_session_timeout
// de 45 s, sobre un semáforo propio de Slots conexiones (ocupado: 503 REPORTS_BUSY sin esperar).
// Nunca toma locks consultivos ni llama handlers de otros módulos: solo SQL propio y funciones puras
// exportadas (payments.AccountStandingSQL, businessday). La hora de la fotografía (now() de la
// transacción) es Meta.GeneratedAt y el periodo se resuelve con la zona horaria y la «Hora de inicio
// del día» leídas en la misma fotografía.
//
// # Definiciones de métricas (§6)
//
//   - Venta (reconocimiento): la venta cuenta cuando la cuenta se cierra liquidada, en el día del
//     cierre (libro app.sales_recognitions, escrito por payments.closeIfSettled en la misma
//     transacción). Una reliquidación tras una reapertura reconoce solo las diferencias en su día;
//     una devolución nunca resta de la venta.
//   - Importe bruto: precio de las unidades antes de promociones (IVA incluido).
//   - Promociones: descuento de las promociones del catálogo (restaurant_item_units.discount).
//   - Descuentos: descuentos manuales vigentes, por partida (asignados a unidades) y generales de
//     cuenta, sin cortesías. Cortesías: importe de las cortesías al 100 %.
//   - Cargos: cargos generales de cuenta (con su tasa de IVA).
//   - Venta neta = bruto − promociones − descuentos − cortesías + cargos (IVA incluido, sin propinas).
//   - IVA: incluido en la venta neta, por cuenta y tasa con la regla de payments/finance.go (base =
//     total / (1 + tasa) redondeada; IVA = total − base); la suma del periodo es la suma de los
//     desgloses por cuenta, nunca un recálculo sobre el agregado. Subtotal sin IVA = neta − IVA.
//   - Cuentas cerradas: cuentas con su primer reconocimiento en el periodo (una reliquidación no
//     vuelve a contarlas). Ticket promedio = venta neta / cuentas cerradas.
//   - Venta antes de descuentos = bruto − promociones + cargos; % de descuentos sobre ventas =
//     (descuentos + cortesías) / venta antes de descuentos.
//   - En curso: cuentas abiertas con total, pagado y saldo de payments.AccountStandingSQL; no son
//     venta. «Pagada sin entregar»: para llevar liquidado sin entrega. «Reabierta»: ya se cerró antes.
//   - Cobro: pago registrado con un método, en su fecha (payments.created_at). Cobrado neto = pagos −
//     devoluciones − reversos de corrección; el pago nuevo de una corrección es un pago. Propinas
//     netas = propinas cobradas − propinas devueltas (nunca son venta).
//   - Devolución: payment_refunds kind=refund en su propia fecha; Corrección: kind=correction más el
//     pago que la sustituye.
//   - Pago mixto: cuenta con pagos de más de un método en el periodo; parcial: más de un pago del
//     mismo método (derivados; no existe marca).
//   - Efectivo por moneda: pesos y dólares físicos de cash_movements (recibido y entregado), con la
//     equivalencia al tipo de cambio de cada cobro; nunca se suman pesos y dólares. Exige
//     cash.expected.read (paridad con Caja) y excluye los turnos en arqueo.
//   - Turnos en arqueo (closing): sus cobros se excluyen de todo agregado para todos, con un aviso.
//   - Descuento por motivo: nombre histórico guardado (reason_name); sin motivo del catálogo,
//     «Sin motivo del catálogo». Los descuentos se fechan con el reconocimiento de su venta.
//   - Combos: el combo es la unidad vendida; sus componentes solo se reportan como cantidades
//     preparadas, sin importe.
//   - Demanda y cocina: fecha de liberación a cocina (kitchen_unit_releases), indicada en el reporte.
//
// # Cómo agregar un reporte (fases B2 y C1)
//
//  1. Crear (o ampliar) un archivo del paquete con un init() que llame a register(Definition{...}):
//     ID estable «grupo.nombre» (minúsculas, puntos y guiones bajos), Group de la lista Groups,
//     Title, Description, Permissions (todas se exigen, además de reports.read: p. ej.
//     reports.cash.read), DateBasis (qué fecha usa y con qué hora de inicio), Filters (solo los que
//     aplican: filterRegister(), filterShift(), filterChoice(...)…), Columns tipadas (textColumn,
//     moneyColumn, countColumn, dollarColumn, percentColumn, dateColumn…) y DefaultSort.
//  2. Las columnas con datos protegidos llevan requiring(columna, permiso): sin el permiso, la
//     columna, sus valores, sus totales y el orden por ella no existen en la respuesta ni en las
//     exportaciones. Una cifra resumida protegida lleva Figure.Permissions. Lo que permita deducir
//     un dato protegido (p. ej. el esperado de caja) no debe calcularse para quien no lo puede ver.
//  3. Query recibe *Query: usar query.Args() (argumentos con nombre @from, @until, @tz, @cutoff y
//     uno por filtro), inPeriod(columna), businessDate(columna) y localHour(columna) para que los
//     límites coincidan con los de Go, conditions.when(...) para los filtros y query.collect(...)
//     con alias SQL iguales a las claves de las columnas (columnas auxiliares no declaradas se
//     descartan antes de responder). Devolver todos los renglones del conjunto filtrado: el motor
//     ordena, pagina y suma en Go las columnas con Total «sum»; Totals completa las «custom».
//  4. Avisos y notas en Dataset.Warnings/Notes; nunca importes protegidos en ellos.
//  5. Indicadores del resumen ejecutivo: registerDashboard(DashboardSection{...}) con su permiso y
//     Compute(periodo); cada indicador apunta a su reporte detallado (ReportID).
//  6. Exportaciones (C1): llamar a Run(ctx, runner, actor, id, parámetros) con el mismo actor y los
//     mismos parámetros que la pantalla y usar Result.AllRows, Result.Columns, Result.Totals y
//     Result.Meta (valores tipados: decimal.Decimal, int64, time.Time, string); tomar un lugar del
//     semáforo con Runner.Acquire si la generación del archivo es larga.
//  7. Pruebas: reports_test.go valida el registro; las de integración (reports_integration_test.go)
//     comprueban totales contra los registros, permisos por API directa y paginación. El puente
//     nativo ya admite cualquier id del registro (reports_run) y los parámetros de su lista blanca:
//     un filtro nuevo debe agregarse a allowed_query_keys en desktop/src-tauri/src/api.rs.
package reports
