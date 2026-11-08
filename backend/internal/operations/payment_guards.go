package operations

import (
	"local/ad-el-pos/server/internal/payments"
)

// Reglas sobre partidas con pago (etapa 6): las unidades con pago asignado o descuento manual
// permanecen en su cuenta y no se transfieren, dividen, combinan, cancelan ni editan; las demás
// siguen las reglas de las etapas 4 y 5 siempre que la cuenta no quede con total menor que lo pagado.

func unitIDs(units []Unit) []string {
	identifiers := make([]string, 0, len(units))
	for _, unit := range units {
		identifiers = append(identifiers, unit.ID)
	}
	return identifiers
}

// splitFreeUnits separa, conservando su orden, las unidades libres de las bloqueadas por pagos.
func (handler *service) splitFreeUnits(units []Unit) ([]Unit, []Unit, error) {
	blocked, err := payments.BlockedUnits(handler.ctx, handler.tx, unitIDs(units))
	if err != nil {
		return nil, nil, err
	}
	free, held := []Unit{}, []Unit{}
	for _, unit := range units {
		if _, found := blocked[unit.ID]; found {
			held = append(held, unit)
		} else {
			free = append(free, unit)
		}
	}
	return free, held, nil
}

// freeUnitsFirst ordena primero las unidades libres para que transferir o dividir una cantidad
// tome unidades sin pago cuando existen.
func (handler *service) freeUnitsFirst(units []Unit) ([]Unit, error) {
	free, held, err := handler.splitFreeUnits(units)
	if err != nil {
		return nil, err
	}
	return append(free, held...), nil
}

func (handler *service) requireUnitsFree(units []Unit) error {
	return payments.RequireUnitsFree(handler.ctx, handler.tx, unitIDs(units))
}

func (handler *service) requireAccountCovered(accountID string) error {
	return payments.RequireAccountCovered(handler.ctx, handler.tx, accountID)
}
