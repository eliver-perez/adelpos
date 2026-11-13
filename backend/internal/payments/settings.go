package payments

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
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
	// («Precuenta»), sales_ticket («Ticket de venta») o bill («Cuenta»).
	PrecheckName string `json:"precheck_name"`
	// Rango aceptable del tipo de cambio (MXN por USD) y diferencia contra el turno anterior que
	// exige confirmación. Solo se aplican con la función usd_cash activa.
	USDRateMin          string    `json:"usd_rate_min"`
	USDRateMax          string    `json:"usd_rate_max"`
	USDRateAlertPercent string    `json:"usd_rate_alert_percent"`
	Revision            int       `json:"revision"`
	UpdatedAt           time.Time `json:"updated_at"`
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
	return handler.settings()
}

func (handler *service) saveSettings(_ []string) (any, error) {
	var input struct {
		OperationID                  string  `json:"operation_id"`
		Revision                     int     `json:"revision"`
		SharedShifts                 bool    `json:"shared_shifts"`
		PrintReceiptDefault          bool    `json:"print_receipt_default"`
		ReceiptPrinterID             *string `json:"receipt_printer_id"`
		RefundAuthorizationThreshold *string `json:"refund_authorization_threshold"`
		// Opcional (etapa 9F): un cliente anterior que no lo envía conserva el nombre guardado.
		PrecheckName *string `json:"precheck_name"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.PrecheckName != nil && !ValidPrecheckName(*input.PrecheckName) {
		return nil, invalid("Elige el nombre de la precuenta: Precuenta, Ticket de venta o Cuenta.")
	}
	previous, err := handler.settings()
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
	if input.ReceiptPrinterID != nil {
		if err := validateID(*input.ReceiptPrinterID); err != nil {
			return nil, err
		}
		var exists bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.kitchen_printers WHERE id=$1)", *input.ReceiptPrinterID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, invalid("La impresora de tickets no existe.")
		}
	}
	precheckName := previous.PrecheckName
	if input.PrecheckName != nil {
		precheckName = *input.PrecheckName
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.payment_settings SET shared_shifts=$1,print_receipt_default=$2,
        receipt_printer_id=$3,refund_authorization_threshold=$4::numeric,precheck_name=$5,revision=revision+1,updated_at=now()`,
		input.SharedShifts, input.PrintReceiptDefault, input.ReceiptPrinterID, threshold, precheckName)
	if err != nil {
		return nil, err
	}
	saved, err := handler.settings()
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
