package payments

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Consultas de caja: cajas con su turno activo, detalle de un turno e historial filtrado por caja
// y fecha comercial. Toda cifra sensible respeta cash.expected.read y el arqueo ciego.

// ShiftDetail es el detalle de un turno: datos de apertura, conteo inicial, cortes y, con
// permiso y fuera del arqueo, el efectivo esperado vigente.
type ShiftDetail struct {
	Shift           *Shift              `json:"shift"`
	OpeningCount    *CountView          `json:"opening_count"`
	Reports         []CashReportSummary `json:"reports"`
	Summary         ShiftSummary        `json:"summary"`
	ExpectedVisible bool                `json:"expected_visible"`
	Expected        *CashExpectation    `json:"expected,omitempty"`
	// CashLimitWarning es el aviso del tope de caja; solo con el esperado visible (fase C).
	CashLimitWarning *CashLimitWarning `json:"cash_limit_warning,omitempty"`
}

// ShiftSummary cuenta los registros del turno sin importes.
type ShiftSummary struct {
	Payments       int `json:"payments"`
	Refunds        int `json:"refunds"`
	CashOperations int `json:"cash_operations"`
	PartialCuts    int `json:"partial_cuts"`
}

func (handler *service) readShiftDetail(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	detail := &ShiftDetail{Shift: shift}
	if detail.OpeningCount, err = handler.readCount(shift.ID, "opening"); err != nil {
		return nil, err
	}
	if detail.Reports, err = handler.listReports(shift.ID); err != nil {
		return nil, err
	}
	err = handler.tx.QueryRow(handler.ctx, `SELECT
        (SELECT count(*) FROM app.cash_movements WHERE shift_id=$1 AND kind='payment'),
        (SELECT count(*) FROM app.cash_movements WHERE shift_id=$1 AND kind='refund'),
        (SELECT count(*) FROM app.cash_operations WHERE shift_id=$1),
        (SELECT count(*) FROM app.cash_shift_reports WHERE shift_id=$1 AND kind='partial')`, shift.ID).Scan(
		&detail.Summary.Payments, &detail.Summary.Refunds, &detail.Summary.CashOperations, &detail.Summary.PartialCuts)
	if err != nil {
		return nil, err
	}
	if handler.expectedVisible(shift) {
		expected, err := handler.expectedCash(shift)
		if err != nil {
			return nil, err
		}
		public := expected.public()
		detail.ExpectedVisible = true
		detail.Expected = &public
	}
	if detail.CashLimitWarning, err = handler.cashLimitWarning(shift); err != nil {
		return nil, err
	}
	return detail, nil
}

// RegisterShift es una caja física con su turno activo (abierto o en arqueo), si lo tiene.
type RegisterShift struct {
	Register Register `json:"register"`
	Shift    *Shift   `json:"shift"`
	Own      bool     `json:"own"`
	// DrawerEnabled indica si la caja tiene un cajón de dinero activado (fase C).
	DrawerEnabled bool `json:"drawer_enabled"`
}

// listCashRegisters devuelve la caja de la terminal y, con cash.shifts.read_all, las demás. Una
// caja inactiva solo aparece si conserva un turno activo.
func (handler *service) listCashRegisters(_ []string) (any, error) {
	own, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	all := handler.request.Actor.Can(permissionShiftsReadAll)
	rows, err := handler.tx.Query(handler.ctx, `SELECT r.id,r.name,r.active FROM app.registers r
        WHERE r.active OR EXISTS(SELECT FROM app.cash_shifts s WHERE s.register_id=r.id AND s.state IN ('open','closing'))
        ORDER BY r.name,r.id`)
	if err != nil {
		return nil, err
	}
	registers := []Register{}
	for rows.Next() {
		var register Register
		if err := rows.Scan(&register.ID, &register.Name, &register.Active); err != nil {
			rows.Close()
			return nil, err
		}
		isOwn := own != nil && own.ID == register.ID
		if all || isOwn {
			registers = append(registers, register)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := []RegisterShift{}
	for _, register := range registers {
		shift, err := handler.activeShift(register.ID)
		if err != nil {
			return nil, err
		}
		drawer, err := handler.drawerEnabled(register.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, RegisterShift{
			Register: register, Shift: shift, Own: own != nil && own.ID == register.ID, DrawerEnabled: drawer,
		})
	}
	return result, nil
}

// ClosureSummary resume el cierre de un turno en el historial: lo declarado siempre; esperado y
// diferencias solo con cash.expected.read.
type ClosureSummary struct {
	CountedMXN    string  `json:"counted_mxn"`
	CountedUSD    string  `json:"counted_usd"`
	ExpectedMXN   *string `json:"expected_mxn,omitempty"`
	ExpectedUSD   *string `json:"expected_usd,omitempty"`
	DifferenceMXN *string `json:"difference_mxn,omitempty"`
	DifferenceUSD *string `json:"difference_usd,omitempty"`
	ResultMXN     *string `json:"result_mxn,omitempty"`
	ResultUSD     *string `json:"result_usd,omitempty"`
	EquivalentMXN *string `json:"difference_equivalent_mxn,omitempty"`
}

// ShiftHistoryItem es un turno del historial con el resumen de su cierre.
type ShiftHistoryItem struct {
	Shift   *Shift          `json:"shift"`
	Closure *ClosureSummary `json:"closure"`
}

var datePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// historyPageSize es el tamaño de página del historial.
const historyPageSize = 50

// shiftFilter acumula condiciones parametrizadas del historial; los valores nunca se concatenan.
type shiftFilter struct {
	conditions []string
	arguments  []any
}

func (filter *shiftFilter) add(condition string, value any) {
	filter.arguments = append(filter.arguments, value)
	filter.conditions = append(filter.conditions, strings.ReplaceAll(condition, "?", "$"+strconv.Itoa(len(filter.arguments))))
}

func (filter *shiftFilter) where() string {
	if len(filter.conditions) == 0 {
		return "true"
	}
	return strings.Join(filter.conditions, " AND ")
}

// parseBusinessDate valida una fecha comercial AAAA-MM-DD.
func parseBusinessDate(value, field string) (string, error) {
	if !datePattern.MatchString(value) {
		return "", invalid("La fecha " + field + " debe tener el formato AAAA-MM-DD.")
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", invalid("La fecha " + field + " no es válida.")
	}
	return value, nil
}

// historyFilter traduce los parámetros de consulta. Sin cash.shifts.read_all solo se consulta la
// caja de la terminal.
func (handler *service) historyFilter() (*shiftFilter, bool, error) {
	query := handler.request.Query
	filter := &shiftFilter{}
	registerID := strings.TrimSpace(query.Get("register_id"))
	if !handler.request.Actor.Can(permissionShiftsReadAll) {
		own, err := handler.deviceRegister()
		if err != nil {
			return nil, false, err
		}
		if own == nil {
			return nil, false, nil
		}
		if registerID != "" && registerID != own.ID {
			return nil, false, denied()
		}
		registerID = own.ID
	}
	if registerID != "" {
		if err := validateID(registerID); err != nil {
			return nil, false, err
		}
		filter.add("s.register_id=?", registerID)
	}
	if value := strings.TrimSpace(query.Get("from")); value != "" {
		from, err := parseBusinessDate(value, "inicial")
		if err != nil {
			return nil, false, err
		}
		filter.add(businessDateExpression+">=?::date", from)
	}
	if value := strings.TrimSpace(query.Get("until")); value != "" {
		until, err := parseBusinessDate(value, "final")
		if err != nil {
			return nil, false, err
		}
		filter.add(businessDateExpression+"<=?::date", until)
	}
	if state := strings.TrimSpace(query.Get("state")); state != "" {
		if state != shiftOpen && state != shiftClosing && state != shiftClosed {
			return nil, false, invalid("El estado del turno no es válido.")
		}
		filter.add("s.state=?", state)
	}
	return filter, true, nil
}

// listShifts atiende el historial de turnos (cash.history.read), paginado y ordenado por fecha
// comercial descendente.
func (handler *service) listShifts(_ []string) (any, error) {
	page := 1
	if value := strings.TrimSpace(handler.request.Query.Get("page")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100000 {
			return nil, invalid("La página no es válida.")
		}
		page = parsed
	}
	result := map[string]any{"items": []ShiftHistoryItem{}, "page": page, "page_size": historyPageSize, "total": 0}
	filter, allowed, err := handler.historyFilter()
	if err != nil || !allowed {
		return result, err
	}
	var total int
	if err := handler.tx.QueryRow(handler.ctx, `SELECT count(*) FROM app.cash_shifts s LEFT JOIN app.establishment e ON true WHERE `+
		filter.where(), filter.arguments...).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, shiftQuery+" WHERE "+filter.where()+" ORDER BY "+businessDateExpression+
		" DESC,s.opened_at DESC,s.id LIMIT "+strconv.Itoa(historyPageSize)+" OFFSET "+strconv.Itoa((page-1)*historyPageSize),
		filter.arguments...)
	if err != nil {
		return nil, err
	}
	items := []ShiftHistoryItem{}
	identifiers := []string{}
	for rows.Next() {
		shift, err := scanShift(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, ShiftHistoryItem{Shift: shift})
		identifiers = append(identifiers, shift.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	closures, err := handler.closureSummaries(identifiers)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index].Closure = closures[items[index].Shift.ID]
	}
	result["items"] = items
	result["total"] = total
	return result, nil
}

// closureSummaries lee los cierres de los turnos indicados y omite lo sensible sin permiso.
func (handler *service) closureSummaries(identifiers []string) (map[string]*ClosureSummary, error) {
	result := map[string]*ClosureSummary{}
	if len(identifiers) == 0 {
		return result, nil
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT shift_id,counted_mxn::text,counted_usd::text,expected_mxn::text,expected_usd::text,
        difference_mxn::text,difference_usd::text,result_mxn,result_usd,difference_equivalent_mxn::text
        FROM app.cash_shift_closures WHERE shift_id=ANY($1::uuid[])`, identifiers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	visible := handler.request.Actor.Can(permissionExpectedRead)
	for rows.Next() {
		var shiftID string
		var summary ClosureSummary
		var expectedMXN, expectedUSD, differenceMXN, differenceUSD, resultMXN, resultUSD string
		var equivalent *string
		if err := rows.Scan(&shiftID, &summary.CountedMXN, &summary.CountedUSD, &expectedMXN, &expectedUSD, &differenceMXN,
			&differenceUSD, &resultMXN, &resultUSD, &equivalent); err != nil {
			return nil, err
		}
		if visible {
			summary.ExpectedMXN, summary.ExpectedUSD = &expectedMXN, &expectedUSD
			summary.DifferenceMXN, summary.DifferenceUSD = &differenceMXN, &differenceUSD
			summary.ResultMXN, summary.ResultUSD = &resultMXN, &resultUSD
			summary.EquivalentMXN = equivalent
		}
		result[shiftID] = &summary
	}
	return result, rows.Err()
}
