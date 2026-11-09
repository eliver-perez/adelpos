package payments

import (
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/features"
)

// Conteos físicos por denominaciones (fondo inicial y arqueo de cierre). El usuario captura
// cantidades; Go valida cada denominación y calcula importes y totales por moneda. Ningún
// componente de la interfaz suma billetes.

// Límites de captura: cantidades enteras por denominación y un conteo acotado.
const (
	maxCountQuantity = 100000
	maxCountLines    = 100
)

// countedLine es una denominación contada con su importe calculado.
type countedLine struct {
	denomination Denomination
	quantity     int
	amount       decimal.Decimal
}

// countTally es un conteo validado: líneas, importe adicional identificado y totales por moneda.
type countTally struct {
	lines     []countedLine
	extraMXN  decimal.Decimal
	extraUSD  decimal.Decimal
	extraNote string
	mxn       decimal.Decimal
	usd       decimal.Decimal
}

// tallyCount valida un conteo contra el catálogo de denominaciones y calcula sus importes. Es una
// función pura: el catálogo llega cargado de PostgreSQL. allowUSD indica si el turno admite
// dólares; sin él, cualquier cantidad en dólares se rechaza con FEATURE_DISABLED. El importe
// adicional nunca es negativo y debe identificarse, de modo que no sirve para cuadrar.
func tallyCount(input CountInput, catalog map[string]Denomination, allowUSD bool) (countTally, error) {
	tally := countTally{}
	if len(input.Lines) > maxCountLines {
		return tally, invalid("El conteo admite hasta 100 denominaciones.")
	}
	seen := map[string]bool{}
	for _, line := range input.Lines {
		if err := validateID(line.DenominationID); err != nil {
			return tally, invalid("Una denominación del conteo no es válida.")
		}
		if seen[line.DenominationID] {
			return tally, invalid("Una denominación está repetida en el conteo.")
		}
		seen[line.DenominationID] = true
		denomination, found := catalog[line.DenominationID]
		if !found || !denomination.Active {
			return tally, invalid("Una denominación del conteo no existe o está desactivada.")
		}
		if line.Quantity < 0 || line.Quantity > maxCountQuantity {
			return tally, invalid("Cada cantidad del conteo debe ser un entero entre 0 y 100000.")
		}
		if denomination.Currency == currencyUSD && line.Quantity > 0 && !allowUSD {
			return tally, featureDisabled()
		}
		amount := decimal.RequireFromString(denomination.Value).Mul(decimal.NewFromInt(int64(line.Quantity)))
		tally.lines = append(tally.lines, countedLine{denomination: denomination, quantity: line.Quantity, amount: amount})
		if denomination.Currency == currencyUSD {
			tally.usd = tally.usd.Add(amount)
		} else {
			tally.mxn = tally.mxn.Add(amount)
		}
	}
	extraMXN, err := parseOptionalAmount(strings.TrimSpace(input.ExtraMXN), "importe adicional en pesos")
	if err != nil {
		return tally, err
	}
	extraUSD, err := parseOptionalAmount(strings.TrimSpace(input.ExtraUSD), "importe adicional en dólares")
	if err != nil {
		return tally, err
	}
	if extraUSD.Sign() > 0 && !allowUSD {
		return tally, featureDisabled()
	}
	tally.extraNote = strings.TrimSpace(input.ExtraNote)
	if textLength(tally.extraNote) > 200 {
		return tally, invalid("La identificación del importe adicional admite hasta 200 caracteres.")
	}
	if (extraMXN.Sign() > 0 || extraUSD.Sign() > 0) && textLength(tally.extraNote) < 3 {
		return tally, invalid("Identifica el importe adicional, por ejemplo «monedas sueltas sin desglosar».")
	}
	tally.extraMXN = extraMXN
	tally.extraUSD = extraUSD
	tally.mxn = tally.mxn.Add(extraMXN)
	tally.usd = tally.usd.Add(extraUSD)
	return tally, nil
}

// denominationCatalog carga todas las denominaciones (activas e inactivas) por identificador.
func (handler *service) denominationCatalog() (map[string]Denomination, error) {
	list, err := handler.listDenominations()
	if err != nil {
		return nil, err
	}
	catalog := map[string]Denomination{}
	for _, denomination := range list {
		catalog[denomination.ID] = denomination
	}
	return catalog, nil
}

// evaluateCount valida un conteo dentro de la transacción. withUSD indica que el turno maneja
// dólares aunque la función se desactivara después (por ejemplo, tiene fondo en dólares).
func (handler *service) evaluateCount(input CountInput, withUSD bool) (countTally, error) {
	catalog, err := handler.denominationCatalog()
	if err != nil {
		return countTally{}, err
	}
	allowUSD := withUSD
	if !allowUSD {
		allowUSD, err = features.Enabled(handler.ctx, handler.tx, features.USDCash)
		if err != nil {
			return countTally{}, err
		}
	}
	return tallyCount(input, catalog, allowUSD)
}

// shiftHandlesDollars indica si un turno admite dólares en su arqueo aunque la función se haya
// desactivado después: tiene fondo en dólares o tipo de cambio capturado.
func shiftHandlesDollars(shift *Shift) bool {
	return shift.OpeningFloatUSD != nil || shift.USDRate != nil
}

// insertCount registra un conteo con sus líneas y devuelve su identificador.
func (handler *service) insertCount(shiftID, purpose, state string, tally countTally, notes string) (string, error) {
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_counts(id,shift_id,purpose,state,counted_mxn,counted_usd,
        extra_mxn,extra_usd,extra_note,notes,updated_by,updated_device_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		id, shiftID, purpose, state, money(tally.mxn), money(tally.usd), money(tally.extraMXN), money(tally.extraUSD),
		tally.extraNote, notes, handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return "", err
	}
	return id, handler.insertCountLines(id, tally)
}

// insertCountLines guarda las líneas de un conteo con el importe calculado por Go.
func (handler *service) insertCountLines(countID string, tally countTally) error {
	for _, line := range tally.lines {
		if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_count_lines(count_id,denomination_id,quantity,amount)
            VALUES ($1,$2,$3,$4)`, countID, line.denomination.ID, line.quantity, money(line.amount)); err != nil {
			return err
		}
	}
	return nil
}

// replaceDraftCount sustituye el borrador del arqueo: sus líneas, totales y observaciones. Solo se
// modifica un conteo en borrador; el conteo final de un cierre es inmutable.
func (handler *service) replaceDraftCount(countID string, tally countTally, notes string) error {
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_counts SET counted_mxn=$2,counted_usd=$3,extra_mxn=$4,extra_usd=$5,
        extra_note=$6,notes=$7,updated_by=$8,updated_device_id=$9,revision=revision+1,updated_at=now()
        WHERE id=$1 AND state='draft'`,
		countID, money(tally.mxn), money(tally.usd), money(tally.extraMXN), money(tally.extraUSD), tally.extraNote, notes,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return conflict("COUNT_CHANGED", "El conteo ya no está en borrador. Actualiza la pantalla.")
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.cash_count_lines WHERE count_id=$1", countID); err != nil {
		return err
	}
	return handler.insertCountLines(countID, tally)
}

// readCount devuelve el conteo de un turno para un propósito, o nil si no existe.
func (handler *service) readCount(shiftID, purpose string) (*CountView, error) {
	var view CountView
	err := handler.tx.QueryRow(handler.ctx, `SELECT c.id,c.shift_id,c.purpose,c.state,c.extra_mxn::text,c.extra_usd::text,c.extra_note,
        c.notes,c.counted_mxn::text,c.counted_usd::text,c.revision,u.display_name,c.updated_at
        FROM app.cash_counts c JOIN app.users u ON u.id=c.updated_by WHERE c.shift_id=$1 AND c.purpose=$2`, shiftID, purpose).Scan(
		&view.ID, &view.ShiftID, &view.Purpose, &view.State, &view.ExtraMXN, &view.ExtraUSD, &view.ExtraNote, &view.Notes,
		&view.CountedMXN, &view.CountedUSD, &view.Revision, &view.UpdatedByName, &view.UpdatedAt)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT l.denomination_id,d.currency,d.kind,d.value::text,l.quantity,l.amount::text
        FROM app.cash_count_lines l JOIN app.cash_denominations d ON d.id=l.denomination_id
        WHERE l.count_id=$1 ORDER BY d.currency,d.kind,d.value DESC`, view.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	view.Lines = []CountLineView{}
	for rows.Next() {
		var line CountLineView
		if err := rows.Scan(&line.DenominationID, &line.Currency, &line.Kind, &line.Value, &line.Quantity, &line.Amount); err != nil {
			return nil, err
		}
		view.Lines = append(view.Lines, line)
	}
	return &view, rows.Err()
}
