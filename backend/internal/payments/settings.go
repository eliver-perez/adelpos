package payments

import (
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/events"
)

type Settings struct {
	TaxRate                      string    `json:"tax_rate"`
	TaxRates                     []string  `json:"tax_rates"`
	SharedShifts                 bool      `json:"shared_shifts"`
	PrintReceiptDefault          bool      `json:"print_receipt_default"`
	ReceiptPrinterID             *string   `json:"receipt_printer_id"`
	ReceiptPrinterName           *string   `json:"receipt_printer_name"`
	RefundAuthorizationThreshold *string   `json:"refund_authorization_threshold"`
	Revision                     int       `json:"revision"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

func (handler *service) settings() (*Settings, error) {
	var settings Settings
	var rate int
	err := handler.tx.QueryRow(handler.ctx, `SELECT s.tax_rate,s.shared_shifts,s.print_receipt_default,s.receipt_printer_id,p.name,
        s.refund_authorization_threshold::text,s.revision,s.updated_at
        FROM app.payment_settings s LEFT JOIN app.kitchen_printers p ON p.id=s.receipt_printer_id`).Scan(
		&rate, &settings.SharedShifts, &settings.PrintReceiptDefault, &settings.ReceiptPrinterID, &settings.ReceiptPrinterName,
		&settings.RefundAuthorizationThreshold, &settings.Revision, &settings.UpdatedAt)
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
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
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
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.payment_settings SET shared_shifts=$1,print_receipt_default=$2,
        receipt_printer_id=$3,refund_authorization_threshold=$4::numeric,revision=revision+1,updated_at=now()`,
		input.SharedShifts, input.PrintReceiptDefault, input.ReceiptPrinterID, threshold)
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
