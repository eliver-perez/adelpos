package payments

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/kitchen"
)

type Settings struct {
	TaxRate                      string   `json:"tax_rate"`
	TaxRates                     []string `json:"tax_rates"`
	SharedShifts                 bool     `json:"shared_shifts"`
	PrintReceiptDefault          bool     `json:"print_receipt_default"`
	ReceiptPrinterID             *string  `json:"receipt_printer_id"`
	ReceiptPrinterName           *string  `json:"receipt_printer_name"`
	RefundAuthorizationThreshold *string  `json:"refund_authorization_threshold"`
	// PrecheckName es el nombre visible del documento previo al cobro (etapa 9F, §4.5): precheck
	// («Precuenta»), sales_ticket («Ticket de venta») o bill («Cuenta»). Solo se informa: desde la revisión
	// 9F (M3) se elige en Administración › Documentos (PUT documents/identity, documents.identity.manage).
	PrecheckName string `json:"precheck_name"`
	// Rango aceptable del tipo de cambio (MXN por USD) y diferencia contra el turno anterior que
	// exige confirmación. Solo se aplican con la función usd_cash activa.
	USDRateMin          string    `json:"usd_rate_min"`
	USDRateMax          string    `json:"usd_rate_max"`
	USDRateAlertPercent string    `json:"usd_rate_alert_percent"`
	Revision            int       `json:"revision"`
	UpdatedAt           time.Time `json:"updated_at"`
	// RegisterPrinters es la impresora de tickets de cada caja (etapa 9G, fase E, decisión D10): las cajas activas y
	// las que tienen una propia; sin impresora propia, la caja usa la general. Solo en la consulta y al guardar.
	RegisterPrinters []RegisterPrinter `json:"register_printers,omitempty"`
}

// RegisterPrinter es la impresora de tickets propia de una caja, con la terminal que la tiene conectada si es la
// impresora USB de una terminal.
type RegisterPrinter struct {
	RegisterID     string  `json:"register_id"`
	RegisterName   string  `json:"register_name"`
	RegisterActive bool    `json:"register_active"`
	PrinterID      *string `json:"printer_id"`
	PrinterName    *string `json:"printer_name"`
	TerminalName   *string `json:"terminal_name"`
}

// registerPrinters lee la impresora de tickets de cada caja activa o con impresora propia.
func (handler *service) registerPrinters() ([]RegisterPrinter, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT r.id::text,r.name,r.active,rp.printer_id::text,p.name,d.name
        FROM app.registers r LEFT JOIN app.register_receipt_printers rp ON rp.register_id=r.id
        LEFT JOIN app.kitchen_printers p ON p.id=rp.printer_id LEFT JOIN app.devices d ON d.id=p.agent_device_id
        WHERE r.active OR rp.register_id IS NOT NULL ORDER BY r.name,r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RegisterPrinter{}
	for rows.Next() {
		var item RegisterPrinter
		if err := rows.Scan(&item.RegisterID, &item.RegisterName, &item.RegisterActive, &item.PrinterID, &item.PrinterName,
			&item.TerminalName); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// settingsWithRegisters es la configuración de cobro con la impresora de tickets de cada caja.
func (handler *service) settingsWithRegisters() (*Settings, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	settings.RegisterPrinters, err = handler.registerPrinters()
	return settings, err
}

func (handler *service) settings() (*Settings, error) {
	var settings Settings
	var rate int
	err := handler.tx.QueryRow(handler.ctx, `SELECT s.tax_rate,s.shared_shifts,s.print_receipt_default,s.receipt_printer_id,p.name,
        s.refund_authorization_threshold::text,s.usd_rate_min::text,s.usd_rate_max::text,s.usd_rate_alert_percent::text,
        s.precheck_name,s.revision,s.updated_at
        FROM app.payment_settings s LEFT JOIN app.kitchen_printers p ON p.id=s.receipt_printer_id`).Scan(
		&rate, &settings.SharedShifts, &settings.PrintReceiptDefault, &settings.ReceiptPrinterID, &settings.ReceiptPrinterName,
		&settings.RefundAuthorizationThreshold, &settings.USDRateMin, &settings.USDRateMax, &settings.USDRateAlertPercent,
		&settings.PrecheckName, &settings.Revision, &settings.UpdatedAt)
	if err != nil {
		return nil, err
	}
	settings.TaxRate = strconv.Itoa(rate)
	settings.TaxRates = []string{"16", "8", "0"}
	return &settings, nil
}

func (handler *service) readSettings(_ []string) (any, error) {
	return handler.settingsWithRegisters()
}

// registerPrinterChoice es la impresora de tickets elegida para una caja (nil: usar la general).
type registerPrinterChoice struct {
	RegisterID string  `json:"register_id"`
	PrinterID  *string `json:"printer_id"`
}

// usablePrinter valida una impresora elegida para tickets (general o de una caja): existe y, si cambió, puede
// imprimir precuentas, tickets y documentos de caja (revisión 9F, B3; desde la 9G la impresora USB de una
// terminal es válida).
func (handler *service) usablePrinter(printerID string, previous *string) error {
	if err := validateID(printerID); err != nil {
		return err
	}
	var exists bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.kitchen_printers WHERE id=$1)", printerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return invalid("La impresora de tickets no existe.")
	}
	if previous != nil && *previous == printerID {
		return nil
	}
	printer, err := kitchen.ReadPrinter(handler.ctx, handler.tx, printerID)
	if err != nil {
		return err
	}
	if problem := kitchen.PrinterProblem(printer); problem != "" {
		return &Error{Status: 422, Code: "RECEIPT_PRINTER_UNAVAILABLE",
			Message: kitchen.UnavailablePrinterMessage(problem, printer.Name) + " Elige otra impresora de tickets.",
			Details: map[string]string{"problem": problem, "printer_name": printer.Name}}
	}
	return nil
}

// saveRegisterPrinters guarda la impresora de tickets de cada caja indicada (D10): una impresora la asigna y nil
// vuelve a la general. Las cajas que no se envían no cambian.
func (handler *service) saveRegisterPrinters(choices []registerPrinterChoice, previous []RegisterPrinter) error {
	before := map[string]*string{}
	for _, item := range previous {
		before[item.RegisterID] = item.PrinterID
	}
	seen := map[string]bool{}
	for _, choice := range choices {
		if err := validateID(choice.RegisterID); err != nil {
			return err
		}
		if seen[choice.RegisterID] {
			return invalid("Cada caja aparece una sola vez en las impresoras de tickets por caja.")
		}
		seen[choice.RegisterID] = true
		var exists bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.registers WHERE id=$1)", choice.RegisterID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return invalid("La caja no existe.")
		}
		if choice.PrinterID != nil && strings.TrimSpace(*choice.PrinterID) == "" {
			choice.PrinterID = nil
		}
		if choice.PrinterID == nil {
			if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.register_receipt_printers WHERE register_id=$1", choice.RegisterID); err != nil {
				return err
			}
			continue
		}
		if err := handler.usablePrinter(*choice.PrinterID, before[choice.RegisterID]); err != nil {
			return err
		}
		if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.register_receipt_printers(register_id,printer_id) VALUES ($1,$2)
            ON CONFLICT (register_id) DO UPDATE SET printer_id=EXCLUDED.printer_id,updated_at=now()`,
			choice.RegisterID, *choice.PrinterID); err != nil {
			return err
		}
	}
	return nil
}

func (handler *service) saveSettings(_ []string) (any, error) {
	var input struct {
		OperationID                  string  `json:"operation_id"`
		Revision                     int     `json:"revision"`
		SharedShifts                 bool    `json:"shared_shifts"`
		PrintReceiptDefault          bool    `json:"print_receipt_default"`
		ReceiptPrinterID             *string `json:"receipt_printer_id"`
		RefundAuthorizationThreshold *string `json:"refund_authorization_threshold"`
		// Etapa 9G (fase E, D10): impresora de tickets por caja; sin el campo, no cambia.
		RegisterPrinters *[]registerPrinterChoice `json:"register_printers"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.settingsWithRegisters()
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La configuración de cobro cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	var threshold *string
	if input.RefundAuthorizationThreshold != nil && strings.TrimSpace(*input.RefundAuthorizationThreshold) != "" {
		value, err := parseAmount(strings.TrimSpace(*input.RefundAuthorizationThreshold), "umbral de devoluciones")
		if err != nil {
			return nil, err
		}
		formatted := money(value)
		threshold = &formatted
	}
	// Revisión 9F (B3): una impresora que no puede imprimir precuentas, tickets ni documentos de caja (decisión D4:
	// desactivada o de otra terminal sin terminal elegida) no se elige como impresora de tickets. La que ya estaba
	// elegida y dejó de servir no impide guardar lo demás: sus documentos avisan con «Configurar impresora».
	if input.ReceiptPrinterID != nil {
		if err := handler.usablePrinter(*input.ReceiptPrinterID, previous.ReceiptPrinterID); err != nil {
			return nil, err
		}
	}
	if input.RegisterPrinters != nil {
		if err := handler.saveRegisterPrinters(*input.RegisterPrinters, previous.RegisterPrinters); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.payment_settings SET shared_shifts=$1,print_receipt_default=$2,
        receipt_printer_id=$3,refund_authorization_threshold=$4::numeric,revision=revision+1,updated_at=now()`,
		input.SharedShifts, input.PrintReceiptDefault, input.ReceiptPrinterID, threshold)
	if err != nil {
		return nil, err
	}
	saved, err := handler.settingsWithRegisters()
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.settings.changed", "configuration", nil, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.settings_changed", "payment_settings", nil, previous, saved)
}

// saveTaxRate cambia la tasa de IVA a una de las tres opciones permitidas. Solo afecta a lo que
// se confirme después: cada versión confirmada conserva su propia tasa.
func (handler *service) saveTaxRate(_ []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Revision    int    `json:"revision"`
		TaxRate     string `json:"tax_rate"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	// Solo se aceptan las cadenas exactas "16", "8" o "0"; nunca fracciones, signos ni otros valores.
	rate, err := strconv.Atoi(input.TaxRate)
	if err != nil || strconv.Itoa(rate) != input.TaxRate || !validTaxRate(rate) {
		return nil, invalid("La tasa de IVA debe ser 16 %, 8 % o 0 %.")
	}
	previous, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La configuración de cobro cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.payment_settings SET tax_rate=$1,revision=revision+1,updated_at=now()", rate); err != nil {
		return nil, err
	}
	saved, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.tax.changed", "configuration", nil, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.tax_rate_changed", "payment_settings", nil,
		map[string]string{"tax_rate": previous.TaxRate}, map[string]string{"tax_rate": saved.TaxRate})
}

// saveUSDSettings ajusta el rango del tipo de cambio y el porcentaje de diferencia que exige
// confirmación. Es un dato en dólares: con la función usd_cash desactivada se rechaza.
func (handler *service) saveUSDSettings(_ []string) (any, error) {
	var input struct {
		OperationID         string `json:"operation_id"`
		Revision            int    `json:"revision"`
		USDRateMin          string `json:"usd_rate_min"`
		USDRateMax          string `json:"usd_rate_max"`
		USDRateAlertPercent string `json:"usd_rate_alert_percent"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.requireUSDCash(); err != nil {
		return nil, err
	}
	minimum, err := parseRate(input.USDRateMin)
	if err != nil {
		return nil, err
	}
	maximum, err := parseRate(input.USDRateMax)
	if err != nil {
		return nil, err
	}
	if !minimum.LessThan(maximum) {
		return nil, invalid("El tipo de cambio mínimo debe ser menor que el máximo.")
	}
	if !percentRegex.MatchString(input.USDRateAlertPercent) {
		return nil, invalid("El porcentaje de alerta debe estar entre 0 y 100, con hasta dos decimales.")
	}
	alert := decimal.RequireFromString(input.USDRateAlertPercent)
	if alert.GreaterThan(hundred) {
		return nil, invalid("El porcentaje de alerta debe estar entre 0 y 100, con hasta dos decimales.")
	}
	previous, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La configuración de cobro cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.payment_settings SET usd_rate_min=$1,usd_rate_max=$2,
        usd_rate_alert_percent=$3,revision=revision+1,updated_at=now()`, money(minimum), money(maximum), money(alert)); err != nil {
		return nil, err
	}
	saved, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.settings.changed", "configuration", nil, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.usd_settings_changed", "payment_settings", nil,
		map[string]string{"usd_rate_min": previous.USDRateMin, "usd_rate_max": previous.USDRateMax, "usd_rate_alert_percent": previous.USDRateAlertPercent},
		map[string]string{"usd_rate_min": saved.USDRateMin, "usd_rate_max": saved.USDRateMax, "usd_rate_alert_percent": saved.USDRateAlertPercent})
}

// requireUSDCash hace cumplir el estado de la función usd_cash aunque se llame a la API directamente.
func (handler *service) requireUSDCash() error {
	enabled, err := features.Enabled(handler.ctx, handler.tx, features.USDCash)
	if err != nil {
		return err
	}
	if !enabled {
		return featureDisabled()
	}
	return nil
}
