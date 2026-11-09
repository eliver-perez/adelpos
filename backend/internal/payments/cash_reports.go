package payments

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

// Cortes del turno. Un corte parcial es un informe: no cierra el turno, no reinicia acumulados,
// no mueve dinero, no pausa la caja y no genera movimientos. Guarda un snapshot con los totales
// acumulados del turno y los ocurridos desde el corte anterior; estos se obtienen restando el
// acumulado anterior, de modo que ningún movimiento cuenta dos veces ni se pierde aunque su
// transacción comenzara antes del corte. El corte definitivo se registra al confirmar el cierre.
//
// El snapshot conserva siempre el efectivo esperado (y en el definitivo, las diferencias) en el
// bloque «sensitive»; la consulta lo omite a quien no tenga cash.expected.read y a todos mientras
// el turno está en arqueo de cierre. Las respuestas de creación nunca lo incluyen: se guardan para
// reintentos idempotentes y el permiso se evalúa al consultar.

// methodTotals son las ventas por método de pago del turno.
type methodTotals struct {
	MethodName string `json:"method_name"`
	IsCash     bool   `json:"is_cash"`
	Sales      string `json:"sales"`
	Tips       string `json:"tips"`
	Refunds    string `json:"refunds"`
	Net        string `json:"net"`
}

// currencyOperations son las operaciones de efectivo de una moneda (sin regularizaciones).
type currencyOperations struct {
	Currency       string `json:"currency"`
	Income         string `json:"income"`
	Withdrawals    string `json:"withdrawals"`
	Expenses       string `json:"expenses"`
	CorrectionsIn  string `json:"corrections_in"`
	CorrectionsOut string `json:"corrections_out"`
}

// shiftTotals son los totales informativos de un turno. No incluyen el efectivo esperado.
type shiftTotals struct {
	Payments           int                  `json:"payments"`
	Refunds            int                  `json:"refunds"`
	PaymentCorrections int                  `json:"payment_corrections"`
	CashOperations     int                  `json:"cash_operations"`
	Regularizations    int                  `json:"regularizations"`
	Sales              string               `json:"sales"`
	Tips               string               `json:"tips"`
	Refunded           string               `json:"refunded"`
	Discounts          string               `json:"discounts"`
	DiscountCount      int                  `json:"discount_count"`
	Courtesies         string               `json:"courtesies"`
	CourtesyCount      int                  `json:"courtesy_count"`
	Methods            []methodTotals       `json:"methods"`
	Cash               []currencyOperations `json:"cash"`
}

// declaredCash es lo que el cajero declaró en el arqueo de cierre.
type declaredCash struct {
	CountedMXN   string `json:"counted_mxn"`
	CountedUSD   string `json:"counted_usd"`
	ExtraMXN     string `json:"extra_mxn"`
	ExtraUSD     string `json:"extra_usd"`
	CashierNotes string `json:"cashier_notes"`
}

// sensitiveCash es el bloque que solo ve quien tiene cash.expected.read.
type sensitiveCash struct {
	Expected    CashExpectation  `json:"expected"`
	Differences *CashDifferences `json:"differences,omitempty"`
}

// reportControl son los datos generales de control del corte.
type reportControl struct {
	RegisterName string  `json:"register_name"`
	BusinessDate string  `json:"business_date"`
	OpenedByName string  `json:"opened_by_name"`
	OpeningFloat string  `json:"opening_float"`
	FloatUSD     *string `json:"opening_float_usd"`
	USDRate      *string `json:"usd_rate"`
}

// reportSnapshot es el contenido guardado de un corte.
type reportSnapshot struct {
	Control       reportControl  `json:"control"`
	Accumulated   shiftTotals    `json:"accumulated"`
	SincePrevious shiftTotals    `json:"since_previous"`
	Declared      *declaredCash  `json:"declared,omitempty"`
	Sensitive     *sensitiveCash `json:"sensitive,omitempty"`
}

// CashReport es un corte consultado. ExpectedVisible indica si el bloque sensible se incluyó.
type CashReport struct {
	ID               string         `json:"id"`
	ShiftID          string         `json:"shift_id"`
	Kind             string         `json:"kind"`
	Sequence         int            `json:"sequence"`
	PreviousReportID *string        `json:"previous_report_id"`
	CoveredFrom      time.Time      `json:"covered_from"`
	CoveredTo        time.Time      `json:"covered_to"`
	CreatedByName    string         `json:"created_by_name"`
	DeviceName       string         `json:"device_name"`
	CreatedAt        time.Time      `json:"created_at"`
	ExpectedVisible  bool           `json:"expected_visible"`
	Snapshot         reportSnapshot `json:"snapshot"`
}

// CashReportSummary identifica un corte en las listas del turno.
type CashReportSummary struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Sequence      int       `json:"sequence"`
	CoveredFrom   time.Time `json:"covered_from"`
	CoveredTo     time.Time `json:"covered_to"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// moneyMinus resta dos importes guardados por el propio sistema con dos decimales.
func moneyMinus(current, previous string) string {
	return money(decimal.RequireFromString(current).Sub(decimal.RequireFromString(previous)))
}

// minus calcula lo ocurrido desde el corte anterior: el acumulado actual menos el anterior.
func (totals shiftTotals) minus(previous shiftTotals) shiftTotals {
	result := shiftTotals{
		Payments:           totals.Payments - previous.Payments,
		Refunds:            totals.Refunds - previous.Refunds,
		PaymentCorrections: totals.PaymentCorrections - previous.PaymentCorrections,
		CashOperations:     totals.CashOperations - previous.CashOperations,
		Regularizations:    totals.Regularizations - previous.Regularizations,
		Sales:              moneyMinus(totals.Sales, previous.Sales),
		Tips:               moneyMinus(totals.Tips, previous.Tips),
		Refunded:           moneyMinus(totals.Refunded, previous.Refunded),
		Discounts:          moneyMinus(totals.Discounts, previous.Discounts),
		DiscountCount:      totals.DiscountCount - previous.DiscountCount,
		Courtesies:         moneyMinus(totals.Courtesies, previous.Courtesies),
		CourtesyCount:      totals.CourtesyCount - previous.CourtesyCount,
		Methods:            []methodTotals{},
		Cash:               []currencyOperations{},
	}
	for _, method := range totals.Methods {
		before := methodTotals{Sales: "0.00", Tips: "0.00", Refunds: "0.00", Net: "0.00"}
		for _, candidate := range previous.Methods {
			if candidate.MethodName == method.MethodName && candidate.IsCash == method.IsCash {
				before = candidate
			}
		}
		result.Methods = append(result.Methods, methodTotals{
			MethodName: method.MethodName,
			IsCash:     method.IsCash,
			Sales:      moneyMinus(method.Sales, before.Sales),
			Tips:       moneyMinus(method.Tips, before.Tips),
			Refunds:    moneyMinus(method.Refunds, before.Refunds),
			Net:        moneyMinus(method.Net, before.Net),
		})
	}
	for _, currency := range totals.Cash {
		before := emptyCurrencyOperations(currency.Currency)
		for _, candidate := range previous.Cash {
			if candidate.Currency == currency.Currency {
				before = candidate
			}
		}
		result.Cash = append(result.Cash, currencyOperations{
			Currency:       currency.Currency,
			Income:         moneyMinus(currency.Income, before.Income),
			Withdrawals:    moneyMinus(currency.Withdrawals, before.Withdrawals),
			Expenses:       moneyMinus(currency.Expenses, before.Expenses),
			CorrectionsIn:  moneyMinus(currency.CorrectionsIn, before.CorrectionsIn),
			CorrectionsOut: moneyMinus(currency.CorrectionsOut, before.CorrectionsOut),
		})
	}
	return result
}

func emptyCurrencyOperations(currency string) currencyOperations {
	return currencyOperations{
		Currency: currency, Income: "0.00", Withdrawals: "0.00", Expenses: "0.00", CorrectionsIn: "0.00", CorrectionsOut: "0.00",
	}
}

// readShiftTotals obtiene los totales informativos del turno desde los registros reales.
func (handler *service) readShiftTotals(shift *Shift) (shiftTotals, error) {
	totals := shiftTotals{Methods: []methodTotals{}, Cash: []currencyOperations{}}
	if err := handler.readMethodTotals(shift, &totals); err != nil {
		return totals, err
	}
	if err := handler.readAdjustmentTotals(shift, &totals); err != nil {
		return totals, err
	}
	if err := handler.readOperationTotals(shift, &totals); err != nil {
		return totals, err
	}
	return totals, nil
}

// readMethodTotals suma por método los cobros, propinas, devoluciones y reversos de corrección.
// Las ventas incluyen pagos y reversos (que restan); las devoluciones se informan aparte. Las
// sumas vacías se expresan con dos decimales («0.00»), como todo importe del contrato.
func (handler *service) readMethodTotals(shift *Shift, totals *shiftTotals) error {
	rows, err := handler.tx.Query(handler.ctx, `SELECT method_name,is_cash,
        count(*) FILTER (WHERE kind='payment'),count(*) FILTER (WHERE kind='refund'),count(*) FILTER (WHERE kind='correction_reversal'),
        coalesce(sum(sale_amount) FILTER (WHERE kind<>'refund'),0)::numeric(18,2)::text,
        coalesce(sum(tip_amount) FILTER (WHERE kind<>'refund'),0)::numeric(18,2)::text,
        coalesce(-sum(net_amount) FILTER (WHERE kind='refund'),0)::numeric(18,2)::text,
        coalesce(sum(net_amount),0)::numeric(18,2)::text
        FROM app.cash_movements WHERE shift_id=$1 GROUP BY method_name,is_cash ORDER BY min(created_at),method_name`, shift.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	sales, tips, refunded := zero, zero, zero
	for rows.Next() {
		var method methodTotals
		var payments, refunds, corrections int
		if err := rows.Scan(&method.MethodName, &method.IsCash, &payments, &refunds, &corrections, &method.Sales, &method.Tips,
			&method.Refunds, &method.Net); err != nil {
			return err
		}
		totals.Payments += payments
		totals.Refunds += refunds
		totals.PaymentCorrections += corrections
		sales = sales.Add(decimal.RequireFromString(method.Sales))
		tips = tips.Add(decimal.RequireFromString(method.Tips))
		refunded = refunded.Add(decimal.RequireFromString(method.Refunds))
		totals.Methods = append(totals.Methods, method)
	}
	totals.Sales, totals.Tips, totals.Refunded = money(sales), money(tips), money(refunded)
	return rows.Err()
}

// readAdjustmentTotals suma los descuentos manuales y cortesías vigentes registrados desde
// terminales asociadas a la caja desde la apertura del turno. Los descuentos pertenecen a la
// cuenta, no al turno; esta regla los atribuye a la caja donde se aplicaron.
func (handler *service) readAdjustmentTotals(shift *Shift, totals *shiftTotals) error {
	var discounts, courtesies string
	err := handler.tx.QueryRow(handler.ctx, `SELECT
        coalesce(sum(a.amount) FILTER (WHERE NOT a.courtesy),0)::numeric(18,2)::text,count(*) FILTER (WHERE NOT a.courtesy),
        coalesce(sum(a.amount) FILTER (WHERE a.courtesy),0)::numeric(18,2)::text,count(*) FILTER (WHERE a.courtesy)
        FROM app.account_adjustments a JOIN app.devices d ON d.id=a.device_id
        WHERE a.kind='discount' AND d.register_id=$1 AND a.created_at>=$2
        AND NOT EXISTS(SELECT FROM app.account_adjustment_voids v WHERE v.adjustment_id=a.id)`,
		shift.RegisterID, shift.OpenedAt).Scan(&discounts, &totals.DiscountCount, &courtesies, &totals.CourtesyCount)
	if err != nil {
		return err
	}
	totals.Discounts, totals.Courtesies = discounts, courtesies
	return nil
}

// readOperationTotals suma ingresos, retiros, gastos y correcciones por moneda. Los pesos siempre
// aparecen; los dólares, solo si el turno los manejó.
func (handler *service) readOperationTotals(shift *Shift, totals *shiftTotals) error {
	rows, err := handler.tx.Query(handler.ctx, `SELECT currency,
        coalesce(sum(amount) FILTER (WHERE kind='income'),0)::numeric(18,2)::text,
        coalesce(sum(amount) FILTER (WHERE kind='withdrawal'),0)::numeric(18,2)::text,
        coalesce(sum(amount) FILTER (WHERE kind='expense'),0)::numeric(18,2)::text,
        coalesce(sum(amount) FILTER (WHERE kind='correction' AND direction='in'),0)::numeric(18,2)::text,
        coalesce(sum(amount) FILTER (WHERE kind='correction' AND direction='out'),0)::numeric(18,2)::text,
        count(*)
        FROM app.cash_operations WHERE shift_id=$1 AND NOT regularization GROUP BY currency`, shift.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	byCurrency := map[string]currencyOperations{}
	for rows.Next() {
		var item currencyOperations
		var count int
		if err := rows.Scan(&item.Currency, &item.Income, &item.Withdrawals, &item.Expenses, &item.CorrectionsIn,
			&item.CorrectionsOut, &count); err != nil {
			return err
		}
		totals.CashOperations += count
		byCurrency[item.Currency] = item
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, currency := range []string{currencyMXN, currencyUSD} {
		item, found := byCurrency[currency]
		if !found && currency == currencyUSD && !shiftHandlesDollars(shift) {
			continue
		}
		if !found {
			item = emptyCurrencyOperations(currency)
		}
		totals.Cash = append(totals.Cash, item)
	}
	return handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.cash_operations WHERE shift_id=$1 AND regularization",
		shift.ID).Scan(&totals.Regularizations)
}

// previousPartial devuelve el último corte parcial del turno, si existe.
func (handler *service) previousPartial(shiftID string) (*string, *time.Time, *shiftTotals, error) {
	var id string
	var coveredTo time.Time
	var snapshot []byte
	err := handler.tx.QueryRow(handler.ctx, `SELECT id,covered_to,snapshot FROM app.cash_shift_reports
        WHERE shift_id=$1 AND kind='partial' ORDER BY sequence DESC LIMIT 1`, shiftID).Scan(&id, &coveredTo, &snapshot)
	if isNoRows(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var stored reportSnapshot
	if err := json.Unmarshal(snapshot, &stored); err != nil {
		return nil, nil, nil, err
	}
	return &id, &coveredTo, &stored.Accumulated, nil
}

// reportExtras son los datos propios del corte definitivo.
type reportExtras struct {
	declared    *declaredCash
	differences *cashDifferences
	expected    cashExpectation
}

// recordReport calcula y guarda un corte del turno con el instante consistente de la transacción.
func (handler *service) recordReport(shift *Shift, kind string, extras *reportExtras) (string, error) {
	totals, err := handler.readShiftTotals(shift)
	if err != nil {
		return "", err
	}
	previousID, previousTo, previousTotals, err := handler.previousPartial(shift.ID)
	if err != nil {
		return "", err
	}
	snapshot := reportSnapshot{
		Control: reportControl{
			RegisterName: shift.RegisterName, BusinessDate: shift.BusinessDate, OpenedByName: shift.OpenedByName,
			OpeningFloat: shift.OpeningFloat, FloatUSD: shift.OpeningFloatUSD, USDRate: shift.USDRate,
		},
		Accumulated:   totals,
		SincePrevious: totals,
	}
	coveredFrom := shift.OpenedAt
	if previousTotals != nil {
		snapshot.SincePrevious = totals.minus(*previousTotals)
		coveredFrom = *previousTo
	}
	if extras == nil {
		expected, err := handler.expectedCash(shift)
		if err != nil {
			return "", err
		}
		snapshot.Sensitive = &sensitiveCash{Expected: expected.public()}
	} else {
		snapshot.Declared = extras.declared
		snapshot.Sensitive = &sensitiveCash{Expected: extras.expected.public()}
		if extras.differences != nil {
			differences := extras.differences.public()
			snapshot.Sensitive.Differences = &differences
		}
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_shift_reports(id,shift_id,kind,sequence,previous_report_id,covered_from,
        covered_to,snapshot,created_by,device_id,operation_id)
        VALUES ($1,$2,$3,CASE WHEN $3='final' THEN 1 ELSE
            (SELECT coalesce(max(sequence),0)+1 FROM app.cash_shift_reports WHERE shift_id=$2 AND kind='partial') END,
        $4,$5,greatest(clock_timestamp(),$5::timestamptz),$6,$7,$8,$9)`,
		id, shift.ID, kind, previousID, coveredFrom, jsonValue(snapshot), handler.request.Actor.UserID, handler.request.Actor.DeviceID,
		handler.operationID)
	return id, err
}

// CreatedReport es la respuesta de un corte parcial: el corte sin bloque sensible y, si se pidió,
// el estado de su impresión (que nunca revierte el corte).
type CreatedReport struct {
	*CashReport
	Printing *PrintOutcome `json:"printing,omitempty"`
}

// createPartialCut registra un corte parcial informativo del turno activo y, si se pide, lo imprime.
func (handler *service) createPartialCut(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Print       *bool  `json:"print"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	printDocument, err := handler.printRequested(input.Print)
	if err != nil {
		return nil, err
	}
	shift, err := handler.readShift("s.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
	}
	// Durante el arqueo de cierre no se generan cortes: la captura es ciega.
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	if err := handler.requireShiftAccess(shift); err != nil {
		return nil, err
	}
	id, err := handler.recordReport(shift, "partial", nil)
	if err != nil {
		return nil, err
	}
	report, err := handler.readReportRecord(id)
	if err != nil {
		return nil, err
	}
	result := &CreatedReport{CashReport: report}
	if printDocument {
		// El documento se renderiza con el snapshot completo y los permisos de quien lo genera; la
		// respuesta, guardada para reintentos, nunca incluye el bloque sensible.
		printing := handler.printAfterRecording(cashDocumentSource{kind: documentPartialCut, shift: shift, report: report}, "el corte")
		result.Printing = &printing
	}
	report.hideSensitive()
	if err := events.Append(handler.ctx, handler.tx, "cash.report.created", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return result, handler.audit(handler.ctx, handler.tx, "cash.partial_cut_created", "cash_shift", &shift.ID, nil, map[string]any{
		"report_id": id, "sequence": report.Sequence, "covered_from": report.CoveredFrom, "covered_to": report.CoveredTo,
	})
}

// hideSensitive omite el efectivo esperado y las diferencias del corte.
func (report *CashReport) hideSensitive() {
	report.Snapshot.Sensitive = nil
	report.ExpectedVisible = false
}

const reportQuery = `SELECT r.id,r.shift_id,r.kind,r.sequence,r.previous_report_id,r.covered_from,r.covered_to,u.display_name,d.name,
    r.created_at,r.snapshot
    FROM app.cash_shift_reports r JOIN app.users u ON u.id=r.created_by JOIN app.devices d ON d.id=r.device_id`

// readReportRecord lee un corte completo, con el bloque sensible incluido.
func (handler *service) readReportRecord(id string) (*CashReport, error) {
	var report CashReport
	var snapshot []byte
	err := handler.tx.QueryRow(handler.ctx, reportQuery+" WHERE r.id=$1", id).Scan(&report.ID, &report.ShiftID, &report.Kind,
		&report.Sequence, &report.PreviousReportID, &report.CoveredFrom, &report.CoveredTo, &report.CreatedByName, &report.DeviceName,
		&report.CreatedAt, &snapshot)
	if isNoRows(err) {
		return nil, notFound("El corte no existe.")
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(snapshot, &report.Snapshot); err != nil {
		return nil, err
	}
	report.ExpectedVisible = report.Snapshot.Sensitive != nil
	return &report, nil
}

// readReport consulta un corte con las reglas de acceso del turno y la visibilidad del esperado.
func (handler *service) readReport(parts []string) (any, error) {
	report, err := handler.readReportRecord(parts[1])
	if err != nil {
		return nil, err
	}
	shift, err := handler.readableShift(report.ShiftID)
	if err != nil {
		return nil, err
	}
	if !handler.expectedVisible(shift) {
		report.hideSensitive()
	}
	return report, nil
}

// listReports devuelve los cortes del turno sin su contenido.
func (handler *service) listReports(shiftID string) ([]CashReportSummary, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT r.id,r.kind,r.sequence,r.covered_from,r.covered_to,u.display_name,r.created_at
        FROM app.cash_shift_reports r JOIN app.users u ON u.id=r.created_by
        WHERE r.shift_id=$1 ORDER BY r.kind DESC,r.sequence`, shiftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CashReportSummary{}
	for rows.Next() {
		var item CashReportSummary
		if err := rows.Scan(&item.ID, &item.Kind, &item.Sequence, &item.CoveredFrom, &item.CoveredTo, &item.CreatedByName,
			&item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
