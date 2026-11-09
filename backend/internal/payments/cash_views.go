package payments

import (
	"strings"
	"time"
)

// Consultas de solo lectura para la interfaz de caja (etapa 7, fase B). Ninguna escribe: la
// vista previa del conteo valida y totaliza denominaciones con la misma función del arqueo y de
// la apertura, y el resumen en vivo reúne los mismos totales que un corte sin registrarlo. Ambas
// respetan el arqueo ciego y el permiso «Consultar importes esperados y diferencias».

// countPreviewInput es un conteo por denominaciones que se quiere totalizar. shift_id indica el
// turno del arqueo; vacío, el conteo es el desglose del fondo de una apertura.
type countPreviewInput struct {
	ShiftID string `json:"shift_id"`
	CountInput
}

// CountPreview son las líneas y los totales por moneda calculados por Go. Solo contiene lo que
// se está declarando: nunca el efectivo esperado ni un indicador de cercanía.
type CountPreview struct {
	Lines      []CountLineView `json:"lines"`
	ExtraMXN   string          `json:"extra_mxn"`
	ExtraUSD   string          `json:"extra_usd"`
	ExtraNote  string          `json:"extra_note"`
	CountedMXN string          `json:"counted_mxn"`
	CountedUSD string          `json:"counted_usd"`
}

// previewCount valida un conteo sin escribirlo y devuelve sus importes. La apertura exige
// cash.shifts.open; el arqueo de un turno, cash.counts.perform y acceso a ese turno. Usa
// evaluateCount, la misma validación que guarda el borrador del arqueo y el desglose del fondo.
func (handler *service) previewCount(_ []string) (any, error) {
	var input countPreviewInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	shiftID := strings.TrimSpace(input.ShiftID)
	if shiftID == "" {
		if err := handler.request.Actor.Require("cash.shifts.open"); err != nil {
			return nil, err
		}
		tally, err := handler.evaluateCount(input.CountInput, false)
		if err != nil {
			return nil, err
		}
		return countPreview(tally), nil
	}
	if err := handler.request.Actor.Require(permissionCounts); err != nil {
		return nil, err
	}
	if err := validateID(shiftID); err != nil {
		return nil, err
	}
	shift, err := handler.closingShift(shiftID)
	if err != nil {
		return nil, err
	}
	tally, err := handler.evaluateCount(input.CountInput, shiftHandlesDollars(shift))
	if err != nil {
		return nil, err
	}
	return countPreview(tally), nil
}

// countPreview expresa un conteo validado con las líneas en el orden capturado.
func countPreview(tally countTally) CountPreview {
	preview := CountPreview{
		Lines:      []CountLineView{},
		ExtraMXN:   money(tally.extraMXN),
		ExtraUSD:   money(tally.extraUSD),
		ExtraNote:  tally.extraNote,
		CountedMXN: money(tally.mxn),
		CountedUSD: money(tally.usd),
	}
	for _, line := range tally.lines {
		preview.Lines = append(preview.Lines, CountLineView{
			DenominationID: line.denomination.ID,
			Currency:       line.denomination.Currency,
			Kind:           line.denomination.Kind,
			Value:          line.denomination.Value,
			Quantity:       line.quantity,
			Amount:         money(line.amount),
		})
	}
	return preview
}

// PhysicalCash es el efectivo recibido y entregado físicamente en cobros y devoluciones, por
// moneda. Junto con el fondo y las operaciones permite deducir el esperado, por lo que solo se
// entrega con el mismo permiso y nunca durante el arqueo.
type PhysicalCash struct {
	MXNIn  string `json:"mxn_in"`
	MXNOut string `json:"mxn_out"`
	USDIn  string `json:"usd_in"`
	USDOut string `json:"usd_out"`
}

// ShiftSummaryView es el resumen en vivo del turno: los totales de un corte calculados con la
// misma función (readShiftTotals), sin registrar un corte. Durante el arqueo de cierre la captura
// es ciega para todos: el resumen omite totales, esperado y efectivo físico.
type ShiftSummaryView struct {
	Shift           *Shift             `json:"shift"`
	GeneratedAt     time.Time          `json:"generated_at"`
	Blind           bool               `json:"blind"`
	Control         reportControl      `json:"control"`
	PartialCuts     int                `json:"partial_cuts"`
	LastCut         *CashReportSummary `json:"last_cut"`
	Accumulated     *shiftTotals       `json:"accumulated,omitempty"`
	SinceLastCut    *shiftTotals       `json:"since_last_cut,omitempty"`
	ExpectedVisible bool               `json:"expected_visible"`
	Expected        *CashExpectation   `json:"expected,omitempty"`
	Physical        *PhysicalCash      `json:"physical,omitempty"`
	Closure         *ClosureSummary    `json:"closure,omitempty"`
}

// shiftSummary atiende GET payments/shifts/{id}/summary con las reglas de consulta del turno.
func (handler *service) shiftSummary(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	view := &ShiftSummaryView{
		Shift: shift,
		Control: reportControl{
			RegisterName: shift.RegisterName, BusinessDate: shift.BusinessDate, OpenedByName: shift.OpenedByName,
			OpeningFloat: shift.OpeningFloat, FloatUSD: shift.OpeningFloatUSD, USDRate: shift.USDRate,
		},
	}
	if err := handler.tx.QueryRow(handler.ctx, "SELECT clock_timestamp()").Scan(&view.GeneratedAt); err != nil {
		return nil, err
	}
	reports, err := handler.listReports(shift.ID)
	if err != nil {
		return nil, err
	}
	for index := range reports {
		if reports[index].Kind == "partial" {
			view.PartialCuts++
			view.LastCut = &reports[index]
		}
	}
	// Ciego: mientras se cuenta el efectivo no viaja ningún total del que se deduzca el esperado.
	if shift.State == shiftClosing {
		view.Blind = true
		return view, nil
	}
	if err := handler.summaryTotals(shift, view); err != nil {
		return nil, err
	}
	if shift.State == shiftClosed {
		closures, err := handler.closureSummaries([]string{shift.ID})
		if err != nil {
			return nil, err
		}
		view.Closure = closures[shift.ID]
	}
	if !handler.expectedVisible(shift) {
		return view, nil
	}
	expected, err := handler.expectedCash(shift)
	if err != nil {
		return nil, err
	}
	public := expected.public()
	view.ExpectedVisible = true
	view.Expected = &public
	if view.Physical, err = handler.physicalCash(shift.ID); err != nil {
		return nil, err
	}
	return view, nil
}

// summaryTotals calcula los acumulados y lo ocurrido desde el último corte parcial con las mismas
// funciones que un corte, de modo que el resumen coincide con el corte que se registraría.
func (handler *service) summaryTotals(shift *Shift, view *ShiftSummaryView) error {
	totals, err := handler.readShiftTotals(shift)
	if err != nil {
		return err
	}
	since := totals
	_, _, previous, err := handler.previousPartial(shift.ID)
	if err != nil {
		return err
	}
	if previous != nil {
		since = totals.minus(*previous)
	}
	view.Accumulated = &totals
	view.SinceLastCut = &since
	return nil
}

// physicalCash suma lo recibido y entregado en efectivo por moneda con la misma regla que los
// movimientos del turno (currencyTotals.add), incluida la de los movimientos anteriores a la 0007.
func (handler *service) physicalCash(shiftID string) (*PhysicalCash, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT cash_received::text,net_amount::text,mxn_in::text,mxn_out::text,usd_in::text,
        usd_out::text FROM app.cash_movements WHERE shift_id=$1 AND is_cash`, shiftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := &currencyTotals{}
	for rows.Next() {
		var item Movement
		if err := rows.Scan(&item.CashReceived, &item.NetAmount, &item.MXNIn, &item.MXNOut, &item.USDIn, &item.USDOut); err != nil {
			return nil, err
		}
		totals.add(item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &PhysicalCash{
		MXNIn:  money(totals.mxnIn),
		MXNOut: money(totals.mxnOut),
		USDIn:  money(totals.usdIn),
		USDOut: money(totals.usdOut),
	}, nil
}
