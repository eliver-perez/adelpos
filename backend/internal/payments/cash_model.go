package payments

import (
	"time"
)

// Administración de caja (etapa 7): turnos con arqueo de cierre, operaciones de efectivo
// (ingresos, retiros, gastos, correcciones y regularizaciones), cortes parciales, cierre ciego y
// diferencias. Este archivo reúne las constantes y los contratos públicos del módulo de caja; la
// lógica vive en los demás archivos cash_*.go.

// Permisos de caja. Las claves se registran en Permissions y en la migración 0008, concedidas
// solo al administrador.
const (
	permissionShiftsRead           = "cash.shifts.read"
	permissionShiftsReadAll        = "cash.shifts.read_all"
	permissionIncome               = "cash.income.record"
	permissionWithdrawals          = "cash.withdrawals.record"
	permissionExpenses             = "cash.expenses.record"
	permissionOutflowsAuthorize    = "cash.outflows.authorize"
	permissionCorrections          = "cash.corrections.record"
	permissionCorrectionsAuthorize = "cash.corrections.authorize"
	permissionMovementsRead        = "cash.movements.read"
	permissionPartialCuts          = "cash.partial_cuts.create"
	permissionCounts               = "cash.counts.perform"
	permissionClose                = "cash.shifts.close"
	permissionCancelAnyClosing     = "cash.closing.cancel_any"
	permissionHistoryRead          = "cash.history.read"
	permissionExpectedRead         = "cash.expected.read"
	permissionClosuresReview       = "cash.closures.review"
	permissionReportsPrint         = "cash.reports.print"
	permissionReportsReprint       = "cash.reports.reprint"
	permissionDrawerOpen           = "cash.drawer.open"
	permissionCashSettings         = "cash.settings.manage"
)

// Estados del turno. «closing» es el estado persistente «Cierre en curso»: la caja queda pausada
// mientras se cuenta el efectivo, sin mantener bloqueos de base de datos.
const (
	shiftOpen    = "open"
	shiftClosing = "closing"
	shiftClosed  = "closed"
)

// Tipos de operación de efectivo y dirección de su efecto físico en el cajón.
const (
	operationIncome     = "income"
	operationWithdrawal = "withdrawal"
	operationExpense    = "expense"
	operationCorrection = "correction"
	directionIn         = "in"
	directionOut        = "out"
)

// Ámbito de los eventos en tiempo real de caja (filtrado en events/store.go).
const cashScope = "cash"

// Resultados de una diferencia de arqueo por moneda.
const (
	resultEven  = "even"
	resultOver  = "over"
	resultShort = "short"
)

// subtypeOption es un tipo de ingreso o de retiro de la lista cerrada que valida Go.
type subtypeOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// Tipos de ingreso de efectivo distintos de una venta.
var incomeTypes = []subtypeOption{
	{Code: "float_contribution", Label: "Aportación adicional al fondo"},
	{Code: "change_supply", Label: "Cambio proporcionado por el encargado"},
	{Code: "cash_return", Label: "Reintegro de efectivo"},
	{Code: "other", Label: "Otro ingreso identificado"},
}

// Motivos de retiro de efectivo. Un retiro es un movimiento físico; no es un corte parcial.
var withdrawalTypes = []subtypeOption{
	{Code: "safekeeping", Label: "Resguardo"},
	{Code: "partial_withdrawal", Label: "Retiro parcial de efectivo acumulado"},
	{Code: "admin_delivery", Label: "Entrega a administración"},
	{Code: "transfer", Label: "Traslado a otro depósito de efectivo"},
	{Code: "other", Label: "Otro concepto identificado"},
}

// subtypeLabel devuelve el nombre visible de un tipo de ingreso o retiro, o "" si no existe.
func subtypeLabel(kind, code string) string {
	options := incomeTypes
	if kind == operationWithdrawal {
		options = withdrawalTypes
	}
	for _, option := range options {
		if option.Code == code {
			return option.Label
		}
	}
	return ""
}

// CashOperationTypeLabel expone subtypeLabel a los reportes de caja (etapa 9A), para que nombren el
// tipo de ingreso o de retiro con la misma lista cerrada que Caja.
func CashOperationTypeLabel(kind, code string) string {
	return subtypeLabel(kind, code)
}

// CashOperation es una operación de efectivo del turno tal como la consulta la interfaz. Los
// importes son físicos en su moneda; EquivalentMXN solo informa la equivalencia en pesos con el
// tipo de cambio guardado en la operación.
type CashOperation struct {
	ID                  string    `json:"id"`
	ShiftID             string    `json:"shift_id"`
	RegisterID          string    `json:"register_id"`
	RegisterName        string    `json:"register_name"`
	Kind                string    `json:"kind"`
	Currency            string    `json:"currency"`
	Amount              string    `json:"amount"`
	Direction           string    `json:"direction"`
	Concept             string    `json:"concept"`
	Type                *string   `json:"type"`
	TypeLabel           *string   `json:"type_label"`
	CategoryID          *string   `json:"category_id"`
	CategoryName        *string   `json:"category_name"`
	Reference           string    `json:"reference"`
	Notes               string    `json:"notes"`
	Reason              string    `json:"reason"`
	CorrectsOperationID *string   `json:"corrects_operation_id"`
	CorrectedByID       *string   `json:"corrected_by_operation_id"`
	Regularization      bool      `json:"regularization"`
	Exceptional         bool      `json:"exceptional"`
	USDRate             *string   `json:"usd_rate"`
	EquivalentMXN       string    `json:"equivalent_mxn"`
	ActorName           string    `json:"actor_name"`
	DeviceName          string    `json:"device_name"`
	AuthorizedByName    *string   `json:"authorized_by_name"`
	AuthorizationMode   *string   `json:"authorization_mode"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
}

// Denomination es un billete o moneda del catálogo configurable.
type Denomination struct {
	ID        string    `json:"id"`
	Currency  string    `json:"currency"`
	Value     string    `json:"value"`
	Kind      string    `json:"kind"`
	Active    bool      `json:"active"`
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExpenseCategory es una categoría de gasto pagado desde caja.
type ExpenseCategory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sort_order"`
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CountInput es un conteo físico por denominaciones: el usuario captura cantidades y Go valida y
// calcula los importes. El importe adicional identificado cubre efectivo sin desglosar.
type CountInput struct {
	Lines     []CountLineInput `json:"lines"`
	ExtraMXN  string           `json:"extra_mxn"`
	ExtraUSD  string           `json:"extra_usd"`
	ExtraNote string           `json:"extra_note"`
}

// CountLineInput es la cantidad contada de una denominación.
type CountLineInput struct {
	DenominationID string `json:"denomination_id"`
	Quantity       int    `json:"quantity"`
}

// CountView es un conteo guardado. Solo contiene lo declarado: nunca el efectivo esperado.
type CountView struct {
	ID            string          `json:"id"`
	ShiftID       string          `json:"shift_id"`
	Purpose       string          `json:"purpose"`
	State         string          `json:"state"`
	Lines         []CountLineView `json:"lines"`
	ExtraMXN      string          `json:"extra_mxn"`
	ExtraUSD      string          `json:"extra_usd"`
	ExtraNote     string          `json:"extra_note"`
	Notes         string          `json:"notes"`
	CountedMXN    string          `json:"counted_mxn"`
	CountedUSD    string          `json:"counted_usd"`
	Revision      int             `json:"revision"`
	UpdatedByName string          `json:"updated_by_name"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CountLineView es una denominación contada con el importe calculado por Go.
type CountLineView struct {
	DenominationID string `json:"denomination_id"`
	Currency       string `json:"currency"`
	Kind           string `json:"kind"`
	Value          string `json:"value"`
	Quantity       int    `json:"quantity"`
	Amount         string `json:"amount"`
}

// CashExpectation es el efectivo físico esperado por moneda. Solo se entrega a quien tiene
// cash.expected.read y nunca mientras el turno está en arqueo de cierre.
type CashExpectation struct {
	MXN                string  `json:"mxn"`
	USD                string  `json:"usd"`
	USDRate            *string `json:"usd_rate"`
	USDEquivalentMXN   *string `json:"usd_equivalent_mxn"`
	TotalEquivalentMXN *string `json:"total_equivalent_mxn"`
}

// CashDifferences son las diferencias de arqueo por moneda (contado − esperado). Una diferencia
// en dólares permanece en dólares; su equivalente en pesos solo informa.
type CashDifferences struct {
	MXN           string  `json:"mxn"`
	USD           string  `json:"usd"`
	ResultMXN     string  `json:"result_mxn"`
	ResultUSD     string  `json:"result_usd"`
	EquivalentMXN *string `json:"equivalent_mxn"`
}
