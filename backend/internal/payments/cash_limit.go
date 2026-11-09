package payments

import (
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
)

// Tope de caja (decisión del propietario, §0-B; topes_caja del sistema anterior). Es una función
// activable (cash_limit, desactivada por defecto) con un importe máximo de efectivo en pesos y, con
// la función usd_cash activa, en dólares. El tope es global: se compara con el efectivo esperado
// de cada caja por separado.
//
//   - Cuando el efectivo esperado supera el tope, la respuesta incluye un aviso que sugiere un
//     retiro. Solo lo reciben quienes ven el esperado (cash.expected.read) y nunca durante el
//     arqueo de cierre: ningún otro usuario ni endpoint recibe el aviso ni un indicador derivado.
//   - Con la función desactivada no tiene ningún efecto. Nunca bloquea cobros ni operaciones.
//   - Los importes del tope requieren cash.settings.manage para consultarse y cambiarse, y se
//     auditan.

// CashLimitWarning es el aviso de tope superado del turno.
type CashLimitWarning struct {
	Currencies []string `json:"currencies"`
	Message    string   `json:"message"`
}

// CashLimitSettings es la configuración del tope de caja.
type CashLimitSettings struct {
	Enabled      bool    `json:"enabled"`
	USDCash      bool    `json:"usd_cash"`
	CashLimitMXN *string `json:"cash_limit_mxn"`
	CashLimitUSD *string `json:"cash_limit_usd"`
	Revision     int     `json:"revision"`
}

const cashLimitMessage = "El efectivo esperado supera el tope de caja; considera un retiro."

// readCashLimit lee el estado de la función y los importes del tope.
func (handler *service) readCashLimit() (*CashLimitSettings, error) {
	settings := &CashLimitSettings{}
	var err error
	if settings.Enabled, err = features.Enabled(handler.ctx, handler.tx, features.CashLimit); err != nil {
		return nil, err
	}
	if settings.USDCash, err = features.Enabled(handler.ctx, handler.tx, features.USDCash); err != nil {
		return nil, err
	}
	err = handler.tx.QueryRow(handler.ctx, "SELECT cash_limit_mxn::text,cash_limit_usd::text,revision FROM app.payment_settings").Scan(
		&settings.CashLimitMXN, &settings.CashLimitUSD, &settings.Revision)
	return settings, err
}

// cashLimitWarning devuelve el aviso de tope superado del turno, o nil. Antes de cualquier cálculo
// exige que quien consulta pueda ver el esperado del turno (permiso y fuera del arqueo): así ni
// siquiera la presencia o ausencia del aviso revela algo a otros usuarios.
func (handler *service) cashLimitWarning(shift *Shift) (*CashLimitWarning, error) {
	if shift == nil || shift.State != shiftOpen || !handler.expectedVisible(shift) {
		return nil, nil
	}
	limits, err := handler.readCashLimit()
	if err != nil || !limits.Enabled {
		return nil, err
	}
	if limits.CashLimitMXN == nil && (limits.CashLimitUSD == nil || !limits.USDCash) {
		return nil, nil
	}
	expected, err := handler.expectedCash(shift)
	if err != nil {
		return nil, err
	}
	exceeded := []string{}
	if limits.CashLimitMXN != nil && expected.mxn.GreaterThan(decimal.RequireFromString(*limits.CashLimitMXN)) {
		exceeded = append(exceeded, currencyMXN)
	}
	if limits.USDCash && limits.CashLimitUSD != nil && expected.usd.GreaterThan(decimal.RequireFromString(*limits.CashLimitUSD)) {
		exceeded = append(exceeded, currencyUSD)
	}
	if len(exceeded) == 0 {
		return nil, nil
	}
	return &CashLimitWarning{Currencies: exceeded, Message: cashLimitMessage}, nil
}

// readCashLimitSettings atiende GET payments/cash/limit (cash.settings.manage).
func (handler *service) readCashLimitSettings(_ []string) (any, error) {
	return handler.readCashLimit()
}

// optionalLimit valida un importe de tope: vacío o nulo significa sin tope; si no, mayor que cero.
func optionalLimit(value *string, field string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	amount, err := parseAmount(strings.TrimSpace(*value), field)
	if err != nil {
		return nil, err
	}
	if amount.Sign() <= 0 {
		return nil, invalid("El " + field + " debe ser mayor que cero o quedar vacío (sin tope).")
	}
	return optionalMoney(amount), nil
}

// saveCashLimitSettings atiende PUT payments/cash/limit (cash.settings.manage). Exige la función
// activa; el tope en dólares exige además usd_cash. Sin usd_cash, el tope en dólares guardado se
// conserva sin cambios: desactivar una función nunca borra su configuración.
func (handler *service) saveCashLimitSettings(_ []string) (any, error) {
	var input struct {
		OperationID  string  `json:"operation_id"`
		Revision     int     `json:"revision"`
		CashLimitMXN *string `json:"cash_limit_mxn"`
		CashLimitUSD *string `json:"cash_limit_usd"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.readCashLimit()
	if err != nil {
		return nil, err
	}
	if !previous.Enabled {
		return nil, conflict("FEATURE_DISABLED", "El tope de caja está desactivado en las funciones del establecimiento. Actívalo en Configuración → Funciones.")
	}
	limitMXN, err := optionalLimit(input.CashLimitMXN, "tope en pesos")
	if err != nil {
		return nil, err
	}
	limitUSD := previous.CashLimitUSD
	if previous.USDCash {
		if limitUSD, err = optionalLimit(input.CashLimitUSD, "tope en dólares"); err != nil {
			return nil, err
		}
	} else if input.CashLimitUSD != nil && strings.TrimSpace(*input.CashLimitUSD) != "" {
		return nil, featureDisabled()
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La configuración de cobro cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.payment_settings SET cash_limit_mxn=$1::numeric,cash_limit_usd=$2::numeric,
        revision=revision+1,updated_at=now()`, limitMXN, limitUSD); err != nil {
		return nil, err
	}
	saved, err := handler.readCashLimit()
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, nil, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.limit_changed", "payment_settings", nil,
		map[string]any{"cash_limit_mxn": previous.CashLimitMXN, "cash_limit_usd": previous.CashLimitUSD},
		map[string]any{"cash_limit_mxn": saved.CashLimitMXN, "cash_limit_usd": saved.CashLimitUSD})
}
