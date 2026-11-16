package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Historial importado del sistema anterior en los reportes (etapa 9H, fase C parte 2; §4.2 de la especificación).
//
// El historial vive en tablas propias app.legacy_* (0024, sección C), de solo lectura, con un solo lote vigente y solo
// las filas de ese lote (al reemplazarlo se borran las del anterior en la misma transacción). Los reportes de ventas,
// productos, cobros, por día y por turno lo leen junto con lo de AD-EL POS:
//
//   - Filtro «Datos» (data_source): «AD-EL POS y sistema anterior» (all, por omisión, para comparar contra años
//     anteriores), «Solo AD-EL POS» (native) y «Solo sistema anterior» (legacy). Solo existe con un lote vigente: sin
//     lote el catálogo no lo ofrece, los metadatos no lo describen, si llega se ignora y cada consulta es la misma SQL
//     de antes (los reportes dan exactamente lo mismo). Igual las columnas «Datos» y «Diferencia del sistema
//     anterior» (Column.Legacy).
//   - Ventas y productos: el libro legacy_sales_recognitions se une al de AD-EL POS con las mismas columnas
//     (ledgerFrom). Los renglones de una misma dimensión (día, semana, mes, hora, tasa, modalidad, área y mesa,
//     categoría y producto; el historial copia los identificadores del catálogo, las mesas y los usuarios importados)
//     se suman en uno solo marcado «AD-EL POS y sistema anterior». Las ventas, los turnos (cortes), las cajas, las
//     formas de pago, las cancelaciones y los cortes del anterior son renglones propios marcados «Sistema anterior».
//   - Importes: numeric(19,4) tal como los guardó el anterior; se suman exactos y se redondean solo al mostrar.
//   - Venta neta del anterior = lo que se debía de cada venta pagada no cancelada (total + cargo − descuentos; Σ net
//     por venta = legacy_sales.amount_due). Lo que no cuadra con sus renglones es la «Diferencia del sistema
//     anterior» (decisión 2 del propietario): una medida propia (legacy_difference), incluida en la venta neta, fuera
//     del importe bruto, y nunca repartida entre productos (no tiene producto). Venta neta = bruto − promociones −
//     descuentos − cortesías + cargos + diferencia del sistema anterior.
//   - Cuentas: cada venta del anterior cuenta una vez (un identificador sintético por venta, legacyAccountSQL, que no
//     choca con los UUID de AD-EL POS). En latest() lo de AD-EL POS siempre es más reciente que el historial.
//   - Día comercial, periodos y semana ISO: los mismos inPeriod/businessDate sobre el instante guardado.
//   - Filtros de algo que solo existe en AD-EL POS (caja, turno, método de pago, lista de precios, origen distinto de
//     «Sistema anterior», motivo del catálogo, quién canceló o modificó) excluyen el historial; los de lo importado
//     (producto, categoría, área, mesa, usuario) lo filtran por el identificador nuevo que copió la importación.
//   - Cobros del anterior: tal como se guardaron (también en cero o negativos y los de ventas canceladas o abiertas,
//     como los cuenta la validación §4.4), sin devoluciones, correcciones ni propinas (no las registraba) y fuera del
//     alcance de Caja y de los turnos en arqueo (paymentScope), que son de los turnos de AD-EL POS.
//   - Cortes del anterior en «Turnos de caja»: solo para quien consulta en Caja todas las cajas y el historial.
//
// Todo corre en la misma transacción de solo lectura del Runner (sin locks consultivos), con su semáforo y su
// statement_timeout. El resumen ejecutivo no incluye el historial (es la operación de AD-EL POS y no tiene filtros).

const (
	dataSourceKey = "data_source"

	// Valores del filtro «Datos».
	DataSourceAll    = "all"
	DataSourceNative = "native"
	DataSourceLegacy = "legacy"

	// Marcas de la columna «Datos».
	NativeSourceLabel = "AD-EL POS"
	LegacySourceLabel = "Sistema anterior"
	BothSourcesLabel  = "AD-EL POS y sistema anterior"

	// legacyFolioPrefix distingue los folios del anterior (como los códigos SA-<id> de sus productos).
	legacyFolioPrefix = "SA-"

	// legacyNoRateLabel es la tasa de los descuentos, cargos y diferencias del anterior en «Ventas del periodo».
	legacyNoRateLabel = "Sin tasa (sistema anterior)"
)

func filterDataSource() Filter {
	filter := filterChoice(dataSourceKey, "Datos", DataSourceAll,
		Option{Value: DataSourceAll, Label: BothSourcesLabel},
		Option{Value: DataSourceNative, Label: "Solo AD-EL POS"},
		Option{Value: DataSourceLegacy, Label: "Solo sistema anterior"})
	filter.Legacy = true
	return filter
}

// withDataSource antepone el filtro «Datos» a los filtros de un reporte que incluye el historial.
func withDataSource(filters []Filter) []Filter {
	return append([]Filter{filterDataSource()}, filters...)
}

// legacyScope es lo que un reporte incluye: Available si hay un lote importado vigente; Native y History, lo de
// AD-EL POS y lo del historial según el filtro «Datos».
type legacyScope struct {
	Available bool
	Native    bool
	History   bool
}

// legacyAvailable indica si hay un lote importado vigente (índice único parcial de la 0024).
func legacyAvailable(ctx context.Context, tx pgx.Tx) (bool, error) {
	var available bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.legacy_import_batches WHERE status='completed')`).Scan(&available)
	return available, err
}

func declaresDataSource(definition *Definition) bool {
	for _, filter := range definition.Filters {
		if filter.Key == dataSourceKey {
			return true
		}
	}
	return false
}

// resolveLegacy decide qué datos incluye el reporte. Sin lote vigente, o si el reporte no declara el filtro, solo
// AD-EL POS (el valor que llegue se ignora).
func resolveLegacy(definition *Definition, params *Params, available bool) legacyScope {
	scope := legacyScope{Available: available, Native: true}
	if !available || definition == nil || !declaresDataSource(definition) {
		return scope
	}
	switch params.Value(dataSourceKey) {
	case DataSourceNative:
	case DataSourceLegacy:
		scope.Native, scope.History = false, true
	default:
		scope.History = true
	}
	return scope
}

// Columnas del historial (solo con un lote vigente).
var (
	columnDataSource = legacyColumn(described(textColumn("data_source", "Datos"),
		"De dónde viene el renglón: AD-EL POS, el sistema anterior (historial importado) o los dos."))
	columnLegacyDifference = legacyColumn(described(moneyColumn("legacy_difference", "Diferencia del sistema anterior"),
		"Ventas del sistema anterior cuyo total guardado no cuadra con sus renglones: la diferencia entre los dos. Ya está "+
			"incluida en la venta neta (se usa el total guardado) y nunca se reparte entre productos."))
)

func legacyColumn(column Column) Column {
	column.Legacy = true
	return column
}

// withLegacyColumns agrega al final la «Diferencia del sistema anterior» (si difference) y «Datos».
func withLegacyColumns(columns []Column, difference bool) []Column {
	result := append([]Column{}, columns...)
	if difference {
		result = append(result, columnLegacyDifference)
	}
	return append(result, columnDataSource)
}

// legacyAccountSQL es el identificador de cuenta de una venta del anterior en la unión: un UUID sintético de la versión
// 0 (los de AD-EL POS son de la versión 4), único por venta del lote vigente.
const legacyAccountSQL = `('00000000-0000-0000-0000-'||lpad(to_hex(l.legacy_sale_id),12,'0'))::uuid`

// legacyProductSQL es el producto de un renglón del anterior: el del catálogo importado o, si su producto no existía en
// el anterior (no ocurre en el respaldo real), uno sintético para que sus ventas sigan en los reportes por producto.
const legacyProductSQL = `CASE WHEN l.entry_kind='line' THEN coalesce(l.product_id,
        ('00000000-0000-0000-0001-'||lpad(to_hex(l.legacy_product_id),12,'0'))::uuid) END`

// nativeLedgerBranch y legacyLedgerBranch son las dos ramas de la unión del libro, con las columnas que leen los
// reportes de ventas y productos (los tipos de ambas coinciden para que PostgreSQL empuje las condiciones del periodo
// a cada rama y use sus índices de recognized_at). El historial no tiene promociones ni cortesías; su secuencia queda
// por debajo de la de AD-EL POS (latest() prefiere los nombres de AD-EL POS); sus descuentos, cargos y diferencias son
// «adjustment» y sus renglones «unit».
const (
	nativeLedgerBranch = `SELECT r.sequence,r.recognition_kind,r.recognized_at,r.account_id,r.mode,r.origin,r.table_id,r.table_name,
            r.area_id,r.area_name,r.shift_id,r.register_id,r.price_list_id,r.source_kind,r.product_id,r.product_name,
            r.product_code,r.category_id,r.category_name,r.tax_rate,r.quantity::numeric AS quantity,r.gross::numeric AS gross,
            r.promotion::numeric AS promotion,r.discount::numeric AS discount,r.courtesy::numeric AS courtesy,
            r.charge::numeric AS charge,r.net::numeric AS net,r.base::numeric AS base,r.tax::numeric AS tax,
            0::numeric AS legacy_difference,false AS legacy
        FROM app.sales_recognitions r`
	legacyLedgerBranch = `SELECT l.sequence-4611686018427387904 AS sequence,'legacy'::text AS recognition_kind,l.recognized_at,
            ` + legacyAccountSQL + ` AS account_id,l.mode,'` + LegacySourceLabel + `'::text AS origin,l.table_id,l.table_name,
            l.area_id,l.area_name,NULL::uuid AS shift_id,NULL::uuid AS register_id,NULL::uuid AS price_list_id,
            CASE WHEN l.entry_kind='line' THEN 'unit' ELSE 'adjustment' END AS source_kind,` + legacyProductSQL + ` AS product_id,
            l.product_name,l.product_code,l.category_id,l.category_name,l.tax_rate,l.quantity::numeric AS quantity,
            (CASE WHEN l.entry_kind='difference' THEN 0 ELSE l.gross END)::numeric AS gross,0::numeric AS promotion,
            l.discount::numeric AS discount,0::numeric AS courtesy,l.charge::numeric AS charge,l.net::numeric AS net,
            l.base::numeric AS base,l.tax::numeric AS tax,
            (CASE WHEN l.entry_kind='difference' THEN l.net ELSE 0 END)::numeric AS legacy_difference,true AS legacy
        FROM app.legacy_sales_recognitions l`
)

// ledgerFrom es la fuente (alias r) de los reportes de ventas y productos: el libro de AD-EL POS tal como antes, el
// historial o la unión de los dos.
func (query *Query) ledgerFrom() string {
	switch {
	case !query.legacy.History:
		return "app.sales_recognitions r"
	case !query.legacy.Native:
		return "(" + legacyLedgerBranch + ") r"
	}
	return "(" + nativeLedgerBranch + "\n        UNION ALL\n        " + legacyLedgerBranch + ") r"
}

// dataSourceMarkSQL marca un grupo de la unión: solo del historial, solo de AD-EL POS o de los dos.
const dataSourceMarkSQL = `CASE WHEN bool_and(r.legacy) THEN '` + LegacySourceLabel + `' WHEN bool_or(r.legacy) THEN '` +
	BothSourcesLabel + `' ELSE '` + NativeSourceLabel + `' END`

// ledgerExtras son las medidas del historial de un grupo del libro: nada sin lote vigente (la consulta de antes),
// constantes si el reporte solo incluye AD-EL POS y, con el historial, la diferencia y la marca «Datos».
func (query *Query) ledgerExtras() string {
	switch {
	case !query.legacy.Available:
		return ""
	case !query.legacy.History:
		return `,0::numeric AS legacy_difference,'` + NativeSourceLabel + `'::text AS data_source`
	}
	return `,coalesce(sum(r.legacy_difference),0) AS legacy_difference,` + dataSourceMarkSQL + ` AS data_source`
}

// legacyLedgerMeasures son las medidas de venta de un grupo de renglones del historial l (las mismas de
// ledgerMeasures; el bruto sin la diferencia, que es su propia medida).
const legacyLedgerMeasures = `count(DISTINCT l.legacy_sale_id)::bigint AS accounts,coalesce(sum(l.quantity),0)::bigint AS quantity,
    coalesce(sum(l.gross) FILTER (WHERE l.entry_kind <> 'difference'),0) AS gross,0::numeric AS promotion,
    coalesce(sum(l.discount),0) AS discount,0::numeric AS courtesy,coalesce(sum(l.charge),0) AS charge,
    coalesce(sum(l.net),0) AS net,coalesce(sum(l.base),0) AS base,coalesce(sum(l.tax),0) AS tax,
    coalesce(sum(l.net) FILTER (WHERE l.entry_kind = 'difference'),0) AS legacy_difference,
    '` + LegacySourceLabel + `'::text AS data_source`

// excludeLegacy agrega «false» si se indicó un filtro de algo que solo existe en AD-EL POS.
func (list *conditions) excludeLegacy(query *Query, keys ...string) {
	for _, key := range keys {
		if query.Has(key) {
			list.add("false")
			return
		}
	}
}

// legacyLedgerConditions son las condiciones del periodo y de los filtros sobre el historial l (las mismas de
// ledgerConditions).
func legacyLedgerConditions(query *Query) conditions {
	list := conditions{inPeriod("l.recognized_at")}
	list.when(query, "mode", "l.mode = @mode")
	list.when(query, "area_id", "l.area_id = @area_id::uuid")
	list.when(query, "table_id", "l.table_id = @table_id::uuid")
	list.when(query, "category_id", "l.category_id = @category_id::uuid")
	list.when(query, "product_id", "l.product_id = @product_id::uuid")
	list.when(query, "origin", "@origin = '"+LegacySourceLabel+"'")
	list.excludeLegacy(query, "price_list_id", "register_id", "shift_id", "method_id")
	return list
}

// legacySalesNote explica los importes del historial en los reportes de ventas y productos.
const legacySalesNote = "Sistema anterior: importes tal como los guardó (cuatro decimales; se redondean solo al mostrar). Su venta " +
	"neta es lo que se debía de cada venta pagada (total + cargo − descuentos) y la parte que no cuadra con sus renglones es la " +
	"«Diferencia del sistema anterior», que no pertenece a ningún producto ni se reparte entre ellos. Las ventas canceladas no " +
	"son venta."

// legacyNotes son las notas del historial de un reporte de ventas o productos (solo si lo incluye).
func legacyNotes(query *Query) []string {
	if !query.legacy.History {
		return nil
	}
	return []string{legacySalesNote}
}

// markNative marca los renglones de AD-EL POS de un reporte que agrega los del historial como renglones propios (solo
// con un lote vigente; sin lote los renglones quedan como antes).
func (query *Query) markNative(rows []Row) {
	if !query.legacy.Available {
		return
	}
	for _, row := range rows {
		row["data_source"] = NativeSourceLabel
		if _, found := row["legacy_difference"]; !found {
			row["legacy_difference"] = decimal.Zero
		}
	}
}

// mergeKey es la clave de unión de un valor (fecha comercial, identificador o texto).
func mergeKey(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		return typed.Format("2006-01-02")
	}
	return fmt.Sprint(value)
}

// mergeRows junta los renglones con la misma clave (los de AD-EL POS y los del historial del mismo día o del mismo
// usuario importado): suma las columnas con total «sum», conserva los textos del primero (AD-EL POS va antes) y marca
// el renglón con los dos orígenes.
func mergeRows(rows []Row, key string, columns []Column) []Row {
	order := []string{}
	merged := map[string]Row{}
	for _, row := range rows {
		identity := mergeKey(row[key])
		existing, found := merged[identity]
		if !found {
			merged[identity] = row
			order = append(order, identity)
			continue
		}
		for _, column := range columns {
			if column.Total != TotalSum {
				continue
			}
			if column.Type == TypeInteger {
				left, _ := existing[column.Key].(int64)
				right, _ := row[column.Key].(int64)
				existing[column.Key] = left + right
				continue
			}
			existing[column.Key] = decimalOf(existing, column.Key).Add(decimalOf(row, column.Key))
		}
		if existing["data_source"] != row["data_source"] {
			existing["data_source"] = BothSourcesLabel
		}
	}
	result := make([]Row, 0, len(order))
	for _, identity := range order {
		result = append(result, merged[identity])
	}
	return result
}
