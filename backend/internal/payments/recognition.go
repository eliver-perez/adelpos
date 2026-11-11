package payments

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Escritura del libro de reconocimiento de ventas (app.sales_recognitions, migración 0017). Se
// ejecuta dentro de closeIfSettled, en la misma transacción (y con el mismo lock 7820148305) en que
// la cuenta queda cerrada: si el cierre se revierte, el reconocimiento también.
//
// Para cada fuente de la cuenta (recognition_targets.go) inserta la diferencia entre su objetivo y
// la suma de lo ya reconocido de esa fuente en cualquier cuenta: el primer cierre reconoce todo; una
// reliquidación después de una reapertura reconoce solo los cambios; si nada cambió no escribe nada.
// Una fuente que ya no forma parte de la cuenta (unidad cancelada, descuento cancelado) se lleva a
// cero si se reconoció antes en esta cuenta o si su unidad pertenece hoy a esta cuenta aunque se
// haya reconocido en otra (unidad transferida después de una reapertura y cancelada aquí), salvo una
// unidad que sigue activa en otra cuenta: esa la reconocerá la cuenta donde está ahora. Como la suma
// es global, cada unidad se cuenta (y se revierte) una sola vez, cierre primero la cuenta que cierre.
//
// Turno y caja: los del pago registrado en la misma operación; sin pago (cortesía o entrega), el
// turno activo de la caja de la terminal que cerró, si existe. Actor y terminal: los de la petición.

// recognitionContext son los datos de la cuenta y de la operación que la cerró.
type recognitionContext struct {
	Folio           int64
	Mode            string
	Origin          string
	TableID         *string
	TableName       *string
	AreaID          *string
	AreaName        *string
	ResponsibleID   string
	PriceListID     *string
	PriceListName   *string
	ShiftID         *string
	RegisterID      *string
	HasRecognitions bool
}

// recognizedSource es lo ya reconocido de una fuente (suma de sus diferencias) con los datos de su
// último renglón y el estado actual de su unidad (cuenta y estado; nil en un ajuste de cuenta).
type recognizedSource struct {
	Line              recognitionLine
	UnitAccountID     *string
	UnitState         *string
	RecognizedInThere bool
}

// recognizeSales registra el reconocimiento de la cuenta que acaba de cerrarse.
func (handler *service) recognizeSales(accountID string) error {
	context, err := handler.readRecognitionContext(accountID)
	if err != nil {
		return err
	}
	units, err := handler.readRecognitionUnits(accountID)
	if err != nil {
		return err
	}
	allocations, err := handler.readRecognitionAllocations(accountID)
	if err != nil {
		return err
	}
	adjustments, err := handler.readRecognitionAdjustments(accountID)
	if err != nil {
		return err
	}
	accountList := recognitionAttributes{PriceListID: context.PriceListID, PriceListName: context.PriceListName}
	targets := buildRecognitionTargets(units, allocations, adjustments, accountList)
	recognized, err := handler.readRecognizedSources(accountID, targets)
	if err != nil {
		return err
	}
	deltas := recognitionDeltas(accountID, targets, recognized)
	if len(deltas) == 0 {
		return nil
	}
	kind := "initial"
	if context.HasRecognitions {
		kind = "resettlement"
	}
	return handler.insertRecognitions(accountID, kind, context, deltas)
}

// recognitionDeltas compara objetivos con lo reconocido y devuelve las diferencias distintas de cero.
func recognitionDeltas(accountID string, targets []recognitionLine, recognized map[recognitionKey]recognizedSource) []recognitionLine {
	deltas := []recognitionLine{}
	seen := map[recognitionKey]bool{}
	for _, target := range targets {
		seen[target.Key] = true
		previous := recognized[target.Key]
		delta := target.Amounts.minus(previous.Line.Amounts)
		if !delta.isZero() {
			deltas = append(deltas, recognitionLine{Key: target.Key, Amounts: delta, Attributes: target.Attributes})
		}
	}
	for key, source := range recognized {
		belongsHere := source.RecognizedInThere || (source.UnitAccountID != nil && *source.UnitAccountID == accountID)
		if seen[key] || !belongsHere {
			continue
		}
		// Una unidad activa en otra cuenta pertenece ahora a esa cuenta: no se resta aquí.
		if key.Kind == sourceUnit && source.UnitState != nil && *source.UnitState == "active" &&
			source.UnitAccountID != nil && *source.UnitAccountID != accountID {
			continue
		}
		delta := recognitionAmounts{}.minus(source.Line.Amounts)
		if !delta.isZero() {
			deltas = append(deltas, recognitionLine{Key: key, Amounts: delta, Attributes: source.Line.Attributes})
		}
	}
	return deltas
}

func (handler *service) readRecognitionContext(accountID string) (*recognitionContext, error) {
	var context recognitionContext
	err := handler.tx.QueryRow(handler.ctx, `SELECT a.folio,a.mode,a.origin,a.table_id,t.name,t.area_id,area.name,a.assigned_user_id,
            a.price_list_id,list.name,EXISTS(SELECT FROM app.sales_recognitions r WHERE r.account_id=a.id)
        FROM app.restaurant_accounts a
        LEFT JOIN app.restaurant_tables t ON t.id=a.table_id
        LEFT JOIN app.restaurant_areas area ON area.id=t.area_id
        LEFT JOIN app.catalog_price_lists list ON list.id=a.price_list_id
        WHERE a.id=$1`, accountID).Scan(&context.Folio, &context.Mode, &context.Origin, &context.TableID, &context.TableName,
		&context.AreaID, &context.AreaName, &context.ResponsibleID, &context.PriceListID, &context.PriceListName,
		&context.HasRecognitions)
	if err != nil {
		return nil, err
	}
	err = handler.tx.QueryRow(handler.ctx, `SELECT shift_id,register_id FROM app.payments WHERE account_id=$1 AND operation_id=$2`,
		accountID, handler.operationID).Scan(&context.ShiftID, &context.RegisterID)
	if isNoRows(err) {
		err = handler.tx.QueryRow(handler.ctx, `SELECT s.id,s.register_id FROM app.devices d
            JOIN app.cash_shifts s ON s.register_id=d.register_id AND s.state IN ('open','closing')
            WHERE d.id=$1`, handler.request.Actor.DeviceID).Scan(&context.ShiftID, &context.RegisterID)
	}
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	return &context, nil
}

// recognitionUnitsQuery lee las unidades activas con sus datos históricos. El snapshot del pedido se
// consulta una vez por pedido y producto (no por unidad), porque contiene el catálogo completo.
const recognitionUnitsQuery = `WITH unit_rows AS (
        SELECT u.id,u.version_id,v.order_id,i.product_id,coalesce(v.quote_line->>'name','') AS product_name,
            nullif(v.quote_line->>'variant_id','') AS variant_id,nullif(v.quote_line->>'category_id','') AS category_id,
            v.tax_rate,u.subtotal,u.discount,u.total,u.created_at,u.item_id,u.ordinal
        FROM app.restaurant_item_units u
        JOIN app.restaurant_item_versions v ON v.id=u.version_id
        JOIN app.restaurant_items i ON i.id=u.item_id
        WHERE u.account_id=$1 AND u.state='active'
    ),
    snapshots AS (
        SELECT p.order_id,p.product_id,p.variant_id,p.category_id,o.price_list_id,o.quote->>'price_list_name' AS price_list_name,
            o.pricing_snapshot->'Products'->(p.product_id::text) AS product,
            o.pricing_snapshot->'Categories'->(p.category_id) AS category
        FROM (SELECT DISTINCT order_id,product_id,variant_id,category_id FROM unit_rows) p
        JOIN app.restaurant_orders o ON o.id=p.order_id
    )
    SELECT r.id,r.version_id,r.order_id,r.product_id,r.product_name,s.product->>'code',s.product->>'kind',r.variant_id,
        (SELECT variant->>'name' FROM jsonb_array_elements(coalesce(s.product->'variants','[]'::jsonb)) variant
            WHERE variant->>'id'=r.variant_id LIMIT 1),
        r.category_id,s.category->>'name',s.price_list_id,s.price_list_name,r.tax_rate,r.subtotal::text,r.discount::text,
        r.total::text
    FROM unit_rows r
    JOIN snapshots s ON s.order_id=r.order_id AND s.product_id=r.product_id
        AND s.variant_id IS NOT DISTINCT FROM r.variant_id AND s.category_id IS NOT DISTINCT FROM r.category_id
    ORDER BY r.created_at,r.item_id,r.ordinal`

func (handler *service) readRecognitionUnits(accountID string) ([]recognitionUnit, error) {
	rows, err := handler.tx.Query(handler.ctx, recognitionUnitsQuery, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []recognitionUnit{}
	for rows.Next() {
		var unit recognitionUnit
		var subtotal, promotion, total string
		attributes := &unit.Attributes
		if err := rows.Scan(&unit.ID, &attributes.VersionID, &attributes.OrderID, &attributes.ProductID, &attributes.ProductName,
			&attributes.ProductCode, &attributes.ProductKind, &attributes.VariantID, &attributes.VariantName, &attributes.CategoryID,
			&attributes.CategoryName, &attributes.PriceListID, &attributes.PriceListName, &unit.TaxRate, &subtotal, &promotion,
			&total); err != nil {
			return nil, err
		}
		unit.Subtotal = decimal.RequireFromString(subtotal)
		unit.Promotion = decimal.RequireFromString(promotion)
		unit.Total = decimal.RequireFromString(total)
		units = append(units, unit)
	}
	return units, rows.Err()
}

// adjustmentColumns son los datos de un ajuste que se copian a sus fuentes.
const adjustmentColumns = `j.id,j.kind,j.scope,j.courtesy,j.reason_id,j.reason_name,j.concept,j.actor_id,j.authorized_by,j.created_at`

func scanAdjustmentAttributes(destination *recognitionAttributes, id, kind *string, scanner func(...any) error, extra ...any) error {
	var scope string
	var created time.Time
	targets := []any{id, kind, &scope, &destination.IsCourtesy, &destination.ReasonID, &destination.ReasonName,
		&destination.Concept, &destination.AdjustmentActorID, &destination.AdjustmentAuthorizedBy, &created}
	if err := scanner(append(targets, extra...)...); err != nil {
		return err
	}
	destination.AdjustmentKind = kind
	destination.AdjustmentScope = &scope
	destination.AdjustmentCreatedAt = &created
	return nil
}

func (handler *service) readRecognitionAllocations(accountID string) ([]recognitionAllocation, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT `+adjustmentColumns+`,a.unit_id,a.amount::text
        FROM app.account_adjustment_allocations a
        JOIN app.account_adjustments j ON j.id=a.adjustment_id
        JOIN app.restaurant_item_units u ON u.id=a.unit_id
        WHERE u.account_id=$1 AND u.state='active' AND j.kind='discount' AND `+activeAdjustment+`
        ORDER BY j.created_at,j.id,a.unit_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []recognitionAllocation{}
	for rows.Next() {
		var allocation recognitionAllocation
		var kind, amount string
		if err := scanAdjustmentAttributes(&allocation.Adjustment, &allocation.AdjustmentID, &kind, rows.Scan,
			&allocation.UnitID, &amount); err != nil {
			return nil, err
		}
		allocation.Amount = decimal.RequireFromString(amount)
		result = append(result, allocation)
	}
	return result, rows.Err()
}

func (handler *service) readRecognitionAdjustments(accountID string) ([]recognitionAdjustment, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT `+adjustmentColumns+`,j.tax_rate,j.amount::text
        FROM app.account_adjustments j
        WHERE j.account_id=$1 AND j.scope='account' AND `+activeAdjustment+`
        ORDER BY j.created_at,j.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []recognitionAdjustment{}
	for rows.Next() {
		var adjustment recognitionAdjustment
		var amount string
		if err := scanAdjustmentAttributes(&adjustment.Attributes, &adjustment.ID, &adjustment.Kind, rows.Scan,
			&adjustment.TaxRate, &amount); err != nil {
			return nil, err
		}
		adjustment.Amount = decimal.RequireFromString(amount)
		result = append(result, adjustment)
	}
	return result, rows.Err()
}

// recognitionColumns son las columnas de datos de un renglón del libro, en el orden de
// recognitionLine (lectura) e insertRecognitions (escritura).
const recognitionColumns = `version_id,order_id,product_id,product_name,product_code,product_kind,variant_id,variant_name,
    category_id,category_name,price_list_id,price_list_name,adjustment_kind,adjustment_scope,is_courtesy,reason_id,reason_name,
    concept,adjustment_actor_id,adjustment_authorized_by,adjustment_created_at`

// qualifiedColumns antepone el alias de la tabla a cada columna de una lista separada por comas.
func qualifiedColumns(alias, columns string) string {
	parts := strings.Split(columns, ",")
	for index, part := range parts {
		parts[index] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ",")
}

func attributeTargets(attributes *recognitionAttributes) []any {
	return []any{&attributes.VersionID, &attributes.OrderID, &attributes.ProductID, &attributes.ProductName, &attributes.ProductCode,
		&attributes.ProductKind, &attributes.VariantID, &attributes.VariantName, &attributes.CategoryID, &attributes.CategoryName,
		&attributes.PriceListID, &attributes.PriceListName, &attributes.AdjustmentKind, &attributes.AdjustmentScope,
		&attributes.IsCourtesy, &attributes.ReasonID, &attributes.ReasonName, &attributes.Concept, &attributes.AdjustmentActorID,
		&attributes.AdjustmentAuthorizedBy, &attributes.AdjustmentCreatedAt}
}

// readRecognizedSources suma lo ya reconocido de las fuentes de la cuenta: las de sus objetivos, las
// que se reconocieron antes en ella y las de sus unidades canceladas (que pudieron reconocerse en la
// cuenta donde estaban antes de transferirse). La suma es global (en cualquier cuenta), porque una
// unidad pudo reconocerse en la cuenta donde estaba antes de transferirse.
func (handler *service) readRecognizedSources(accountID string, targets []recognitionLine) (map[recognitionKey]recognizedSource, error) {
	unitIDs := []string{}
	adjustmentIDs := []string{}
	for _, target := range targets {
		if target.Key.UnitID != "" {
			unitIDs = append(unitIDs, target.Key.UnitID)
		}
		if target.Key.AdjustmentID != "" {
			adjustmentIDs = append(adjustmentIDs, target.Key.AdjustmentID)
		}
	}
	// Cada rama usa su índice (cuenta, unidad, unidades de la cuenta o ajuste): el cierre nunca recorre
	// el libro completo.
	rows, err := handler.tx.Query(handler.ctx, `WITH sources AS (
            SELECT DISTINCT source_kind,unit_id,adjustment_id,tax_rate FROM (
                SELECT source_kind,unit_id,adjustment_id,tax_rate FROM app.sales_recognitions WHERE account_id=$1
                UNION ALL
                SELECT source_kind,unit_id,adjustment_id,tax_rate FROM app.sales_recognitions WHERE unit_id=ANY($2::uuid[])
                UNION ALL
                SELECT r.source_kind,r.unit_id,r.adjustment_id,r.tax_rate FROM app.restaurant_item_units u
                JOIN app.sales_recognitions r ON r.unit_id=u.id WHERE u.account_id=$1 AND u.state='cancelled'
                UNION ALL
                SELECT source_kind,unit_id,adjustment_id,tax_rate FROM app.sales_recognitions WHERE adjustment_id=ANY($3::uuid[])
            ) keys
        ),
        candidates AS (
            SELECT r.* FROM app.sales_recognitions r
            WHERE r.unit_id IN (SELECT unit_id FROM sources WHERE unit_id IS NOT NULL)
            UNION
            SELECT r.* FROM app.sales_recognitions r
            WHERE r.adjustment_id IN (SELECT adjustment_id FROM sources WHERE adjustment_id IS NOT NULL)
        )
        SELECT DISTINCT ON (r.source_kind,r.unit_id,r.adjustment_id,r.tax_rate)
            r.source_kind,coalesce(r.unit_id::text,''),coalesce(r.adjustment_id::text,''),r.tax_rate,
            sum(r.quantity) OVER source,sum(r.gross) OVER source,sum(r.promotion) OVER source,sum(r.discount) OVER source,
            sum(r.courtesy) OVER source,sum(r.charge) OVER source,sum(r.net) OVER source,sum(r.base) OVER source,
            sum(r.tax) OVER source,bool_or(r.account_id=$1) OVER source,u.account_id,u.state,`+qualifiedColumns("r", recognitionColumns)+`
        FROM candidates r
        JOIN sources s ON s.source_kind=r.source_kind AND s.unit_id IS NOT DISTINCT FROM r.unit_id
            AND s.adjustment_id IS NOT DISTINCT FROM r.adjustment_id AND s.tax_rate=r.tax_rate
        LEFT JOIN app.restaurant_item_units u ON u.id=r.unit_id
        WINDOW source AS (PARTITION BY r.source_kind,r.unit_id,r.adjustment_id,r.tax_rate)
        ORDER BY r.source_kind,r.unit_id,r.adjustment_id,r.tax_rate,r.sequence DESC`, accountID, unitIDs, adjustmentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[recognitionKey]recognizedSource{}
	for rows.Next() {
		var source recognizedSource
		var gross, promotion, discount, courtesy, charge, net, base, tax string
		line := &source.Line
		targets := []any{&line.Key.Kind, &line.Key.UnitID, &line.Key.AdjustmentID, &line.Key.TaxRate, &line.Amounts.Quantity,
			&gross, &promotion, &discount, &courtesy, &charge, &net, &base, &tax, &source.RecognizedInThere,
			&source.UnitAccountID, &source.UnitState}
		if err := rows.Scan(append(targets, attributeTargets(&line.Attributes)...)...); err != nil {
			return nil, err
		}
		line.Amounts.Gross = decimal.RequireFromString(gross)
		line.Amounts.Promotion = decimal.RequireFromString(promotion)
		line.Amounts.Discount = decimal.RequireFromString(discount)
		line.Amounts.Courtesy = decimal.RequireFromString(courtesy)
		line.Amounts.Charge = decimal.RequireFromString(charge)
		line.Amounts.Net = decimal.RequireFromString(net)
		line.Amounts.Base = decimal.RequireFromString(base)
		line.Amounts.Tax = decimal.RequireFromString(tax)
		result[line.Key] = source
	}
	return result, rows.Err()
}

func optionalID(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// insertRecognitions escribe las diferencias con la hora del cierre (now(), la misma de closed_at).
func (handler *service) insertRecognitions(accountID, kind string, context *recognitionContext, deltas []recognitionLine) error {
	closingID := newID()
	batch := &pgx.Batch{}
	for _, line := range deltas {
		attributes := line.Attributes
		amounts := line.Amounts
		batch.Queue(`INSERT INTO app.sales_recognitions(closing_id,recognition_kind,recognized_at,account_id,account_folio,mode,origin,
                table_id,table_name,area_id,area_name,responsible_user_id,shift_id,register_id,actor_id,device_id,operation_id,
                source_kind,unit_id,adjustment_id,tax_rate,quantity,gross,promotion,discount,courtesy,charge,net,base,tax,`+recognitionColumns+`)
            VALUES ($1,$2,now(),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22::numeric,$23::numeric,
                $24::numeric,$25::numeric,$26::numeric,$27::numeric,$28::numeric,$29::numeric,
                $30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43,$44,$45,$46,$47,$48,$49,$50)`,
			closingID, kind, accountID, context.Folio, context.Mode, context.Origin, context.TableID, context.TableName,
			context.AreaID, context.AreaName, context.ResponsibleID, context.ShiftID, context.RegisterID,
			optionalID(handler.request.Actor.UserID), optionalID(handler.request.Actor.DeviceID), optionalID(handler.operationID),
			line.Key.Kind, optionalID(line.Key.UnitID), optionalID(line.Key.AdjustmentID), line.Key.TaxRate, amounts.Quantity,
			money(amounts.Gross), money(amounts.Promotion), money(amounts.Discount), money(amounts.Courtesy), money(amounts.Charge),
			money(amounts.Net), money(amounts.Base), money(amounts.Tax),
			attributes.VersionID, attributes.OrderID, attributes.ProductID, attributes.ProductName, attributes.ProductCode,
			attributes.ProductKind, attributes.VariantID, attributes.VariantName, attributes.CategoryID, attributes.CategoryName,
			attributes.PriceListID, attributes.PriceListName, attributes.AdjustmentKind, attributes.AdjustmentScope,
			attributes.IsCourtesy, attributes.ReasonID, attributes.ReasonName, attributes.Concept, attributes.AdjustmentActorID,
			attributes.AdjustmentAuthorizedBy, attributes.AdjustmentCreatedAt)
	}
	results := handler.tx.SendBatch(handler.ctx, batch)
	for range deltas {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	return results.Close()
}
