package legacyimport

import (
	"strconv"
	"time"

	"local/ad-el-pos/server/internal/plural"
)

// Resumen de un paquete para el asistente (Fase D): lo mismo en el resumen previo del análisis y en el informe final
// de la importación. Solo conteos, fechas, totales agregados y los avisos de la correspondencia del catálogo; la lista
// detallada de datos incoherentes (folios, fechas e importes) solo está en la base y en el PDF.

// PackageView son los datos del paquete.
type PackageView struct {
	FormatVersion  int        `json:"format_version"`
	CreatedAt      *time.Time `json:"created_at"`
	Tool           string     `json:"tool"`
	ProductVersion string     `json:"product_version"`
	Fingerprint    string     `json:"fingerprint"`
	BackupTakenAt  *time.Time `json:"backup_taken_at"`
	SizeBytes      int64      `json:"size_bytes"`
	SHA256         string     `json:"sha256"`
}

// Period es el rango de fechas de las ventas.
type Period struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Label string `json:"label"`
}

// FlagCount es cuántas filas tienen una marca.
type FlagCount struct {
	Entity string `json:"entity"`
	Code   string `json:"code"`
	Label  string `json:"label"`
	Count  int64  `json:"count"`
	Listed bool   `json:"listed"`
}

// YearAmount es un total de un año.
type YearAmount struct {
	Year   int    `json:"year"`
	Amount string `json:"amount"`
}

// MethodYearAmount es un total de una forma de pago en un año.
type MethodYearAmount struct {
	Method string `json:"method"`
	Label  string `json:"label"`
	Year   int    `json:"year"`
	Amount string `json:"amount"`
}

// Totals son los totales del paquete (los esperados de la validación).
type Totals struct {
	SalesByYear          []YearAmount       `json:"sales_by_year"`
	PaymentsByMethodYear []MethodYearAmount `json:"payments_by_method_year"`
	CancelledLegacyStyle string             `json:"cancelled_legacy_style"`
	Withdrawals          string             `json:"withdrawals"`
	Expenses             string             `json:"expenses"`
}

// Summary es el resumen de un paquete.
type Summary struct {
	Package     PackageView      `json:"package"`
	Counts      map[string]int64 `json:"counts"`
	SalesPeriod Period           `json:"sales_period"`
	TaxRates    map[string]int   `json:"tax_rates"`
	Totals      Totals           `json:"totals"`
	Flags       []FlagCount      `json:"flags"`
	Notes       []PlanNote       `json:"notes"`
	Warnings    []string         `json:"warnings"`
}

// BatchView es un lote importado.
type BatchView struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	ImportedAt    time.Time  `json:"imported_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	ImportedBy    string     `json:"imported_by"`
	BackupTakenAt *time.Time `json:"backup_taken_at"`
	Fingerprint   string     `json:"fingerprint"`
	Sales         int64      `json:"sales"`
	SalesFirstAt  *time.Time `json:"sales_first_at"`
	SalesLastAt   *time.Time `json:"sales_last_at"`
	Differences   bool       `json:"has_differences"`
}

// Preview es el resumen previo del análisis.
type Preview struct {
	Summary
	CurrentBatch *BatchView `json:"current_batch"`
	WillReplace  bool       `json:"will_replace"`
	Clean        bool       `json:"clean"`
	Blockers     []Blocker  `json:"blockers"`
}

// Report es el informe final de una importación.
type Report struct {
	Summary
	BatchID         string           `json:"batch_id"`
	ReplacedBatchID string           `json:"replaced_batch_id,omitempty"`
	Validation      Validation       `json:"validation"`
	Written         map[string]int64 `json:"written"`
	Retired         int              `json:"retired"`
	TimingsMS       map[string]int64 `json:"timings_ms"`
	BackupFile      string           `json:"backup_file"`
}

// parseBackupTime interpreta la fecha del respaldo de origen en la zona del establecimiento.
func parseBackupTime(text string, location *time.Location) *time.Time {
	if text == "" {
		return nil
	}
	parsed, err := ParseLocalTime(text, location)
	if err != nil || parsed.IsZero() {
		return nil
	}
	return &parsed
}

// buildSummary arma el resumen de un paquete con su correspondencia y sus totales esperados.
func buildSummary(plan *Plan, expected Expected, sizeBytes int64, sha256 string) Summary {
	model := plan.Model
	info := model.Info
	view := PackageView{
		FormatVersion:  info.FormatVersion,
		Tool:           info.Tool,
		ProductVersion: info.ProductVersion,
		Fingerprint:    info.Fingerprint,
		BackupTakenAt:  parseBackupTime(info.BackupTakenAt, model.Location),
		SizeBytes:      sizeBytes,
		SHA256:         sha256,
	}
	if !info.CreatedAt.IsZero() {
		created := info.CreatedAt
		view.CreatedAt = &created
	}
	first, last, label := plan.SalesPeriod()
	summary := Summary{
		Package:     view,
		Counts:      map[string]int64{},
		SalesPeriod: Period{First: first, Last: last, Label: label},
		TaxRates:    map[string]int{},
		Notes:       plan.Catalog.Notes,
		Flags:       countFlags(plan),
		Warnings:    []string{},
	}
	for key, value := range expected.Counts {
		summary.Counts[key] = value
	}
	summary.Counts["option_groups"] = int64(len(plan.Catalog.Groups))
	summary.Counts["areas"] = int64(len(plan.Catalog.Areas))
	inactive := int64(0)
	for _, product := range plan.Catalog.Products {
		summary.TaxRates[strconv.Itoa(product.TaxRate)]++
		if !product.Active {
			inactive++
		}
	}
	summary.Counts["products_inactive"] = inactive
	if summary.Notes == nil {
		summary.Notes = []PlanNote{}
	}
	for _, year := range sortedYears(yearsOf(expected.SalesByYear)) {
		summary.Totals.SalesByYear = append(summary.Totals.SalesByYear, YearAmount{Year: year, Amount: expected.SalesByYear[year].String()})
	}
	for _, method := range []string{MethodCash, MethodCard, MethodOther} {
		for _, year := range sortedYears(yearsOf(expected.PaymentsByMethodYear[method])) {
			summary.Totals.PaymentsByMethodYear = append(summary.Totals.PaymentsByMethodYear, MethodYearAmount{
				Method: method, Label: methodLabels[method], Year: year,
				Amount: expected.PaymentsByMethodYear[method][year].String(),
			})
		}
	}
	summary.Totals.CancelledLegacyStyle = expected.CancelledLegacyStyle.String()
	summary.Totals.Withdrawals = expected.WithdrawalsTotal.String()
	summary.Totals.Expenses = expected.ExpensesTotal.String()
	summary.Warnings = append(summary.Warnings,
		"El sistema anterior no guardaba dólares ni tipo de cambio: los pagos en dólares quedaron como efectivo en pesos.",
		"Las ventas canceladas se importan como canceladas y no cuentan como venta (el sistema anterior las contaba).",
		"El inventario, las impresoras, las cajas como equipos y la configuración del equipo no se importan.",
		"Los usuarios se importan sin contraseña y sin acceso; asígnales una en Usuarios para que puedan entrar.")
	if expected.Purchases > 0 {
		summary.Warnings = append(summary.Warnings, "El paquete trae compras recibidas del sistema anterior; no se importan.")
	}
	summary.Warnings = append(summary.Warnings, shiftDateWarnings(plan.History)...)
	return summary
}

// shiftDateWarnings explica, con su conteo, los cortes del anterior sin fecha de inicio o de fin (revisión de la Fase C,
// M-8). Sin ninguno, no dice nada.
func shiftDateWarnings(history *HistoryPlan) []string {
	var started, ended, undated int
	for _, state := range history.Shifts {
		if state.Flags.has(FlagStartEstimated) {
			started++
		}
		if state.Flags.has(FlagEndEstimated) {
			ended++
		}
		if state.Flags.has(FlagNoStart) {
			undated++
		}
	}
	warnings := []string{}
	if started > 0 {
		warnings = append(warnings, plural.Count(started, "corte", "cortes")+" del sistema anterior "+
			plural.Word(started, "no tenía", "no tenían")+" fecha de inicio: se usa la de su primera venta o pago para que "+
			plural.Word(started, "aparezca", "aparezcan")+" en «Turnos de caja».")
	}
	if ended > 0 {
		warnings = append(warnings, plural.Count(ended, "corte cerrado", "cortes cerrados")+" del sistema anterior "+
			plural.Word(ended, "no tenía", "no tenían")+" fecha de fin: se usa la de su última venta o pago.")
	}
	if undated > 0 {
		warnings = append(warnings, plural.Count(undated, "corte", "cortes")+" del sistema anterior sin fecha de inicio ni "+
			"ventas o pagos "+plural.Word(undated, "queda", "quedan")+" sin fecha y no "+
			plural.Word(undated, "aparece", "aparecen")+" en «Turnos de caja».")
	}
	return warnings
}

// replaceWarnings es el aviso de repetir la migración sobre un lote vigente, en el resumen previo y en el informe
// final: lo importado vuelve a quedar como en el paquete y lo que ya no viene se borra o se retira.
func replaceWarnings() []string {
	return []string{
		"Esta migración reemplaza la que ya se hizo. Los usuarios importados vuelven a tener el nombre de usuario, el " +
			"nombre visible, el rol y el estado (activo o inactivo) que traen en el paquete: lo que hayas cambiado en ellos " +
			"se pierde (las contraseñas que les asignaste se conservan). Lo mismo pasa con los productos, las categorías, " +
			"los complementos y las mesas importados.",
		"Lo que se importó la vez anterior y ya no viene en este paquete se borra; si ya se usó en AD-EL POS, se " +
			"desactiva y queda como retirado.",
	}
}

func yearsOf(values map[int]Money) map[int]bool {
	years := map[int]bool{}
	for year := range values {
		years[year] = true
	}
	return years
}

// countFlags cuenta las marcas de cada entidad en el orden de la lista cerrada (solo las que aparecen).
func countFlags(plan *Plan) []FlagCount {
	counts := map[[2]string]int64{}
	add := func(entity string, flags flagSet) {
		for _, flag := range flags {
			counts[[2]string{entity, flag}]++
		}
	}
	history := plan.History
	for _, state := range history.Sales {
		add(EntitySale, state.Flags)
	}
	for _, state := range history.Lines {
		add(EntityLine, state.Flags)
	}
	for _, state := range history.Complements {
		add(EntityComplement, state.Flags)
	}
	for _, state := range history.Payments {
		add(EntityPayment, state.Flags)
	}
	for _, state := range history.Discounts {
		add(EntityDiscount, state.Flags)
	}
	for _, state := range history.Shifts {
		add(EntityShift, state.Flags)
	}
	for _, state := range history.Withdrawals {
		add(EntityWithdrawal, state.Flags)
	}
	for _, state := range history.Prices {
		add(EntityPrice, state.Flags)
	}
	result := []FlagCount{}
	for _, info := range Flags {
		count := counts[[2]string{info.Entity, info.Code}]
		if count == 0 {
			continue
		}
		result = append(result, FlagCount{Entity: info.Entity, Code: info.Code, Label: info.Label, Count: count, Listed: info.Listed})
	}
	return result
}
