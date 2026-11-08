package operations

import (
	"reflect"
	"time"
)

func compatibleAccounts(source, target *Account) error {
	if source.ID == target.ID {
		return invalid("Selecciona una cuenta de destino diferente.")
	}
	if source.State != "open" || target.State != "open" {
		return invalid("Ambas cuentas deben estar abiertas.")
	}
	if source.Mode != target.Mode || source.PriceListID != target.PriceListID {
		return invalid("Las cuentas deben compartir modalidad y lista de precios para conservar una operación coherente.")
	}
	if source.Mode == "takeaway" {
		sameCustomer := source.CustomerName == target.CustomerName && source.Phone == target.Phone
		sameSchedule := reflect.DeepEqual(source.ScheduledAt, target.ScheduledAt) && reflect.DeepEqual(source.ScheduleKind, target.ScheduleKind)
		if !sameCustomer || !sameSchedule {
			return invalid("Para llevar requiere el mismo cliente, teléfono y programación en ambas cuentas.")
		}
	}
	return nil
}

func (handler *service) selectedUnits(accountID string, selections []Selection) ([]Unit, error) {
	if len(selections) < 1 || len(selections) > 100 {
		return nil, invalid("Selecciona entre 1 y 100 partidas para transferir.")
	}
	seen := map[string]bool{}
	units := []Unit{}
	for _, selection := range selections {
		if seen[selection.VersionID] {
			return nil, invalid("Una partida está repetida en la transferencia.")
		}
		seen[selection.VersionID] = true
		available, err := handler.heldUnits(accountID, selection.VersionID)
		if err != nil {
			return nil, err
		}
		if selection.Quantity < 1 || selection.Quantity > len(available) {
			return nil, invalid("La cantidad seleccionada excede las unidades de una partida en la cuenta de origen.")
		}
		units = append(units, available[:selection.Quantity]...)
	}
	return units, nil
}

func (handler *service) validateTransferTarget(target *Account) error {
	if err := handler.authorizeAccount(target, true); err != nil {
		return err
	}
	if target.TableID != nil {
		return handler.validateTableTarget(target.TableID)
	}
	return nil
}

func (handler *service) splitInput(source *Account, input *AccountInput) (*time.Time, error) {
	if err := handler.request.Actor.Require("accounts.open"); err != nil {
		return nil, err
	}
	if input.Revision != 0 || input.OperationID != "" {
		return nil, invalid("La cuenta resultante se crea dentro de esta operación; no lleva otra operación ni revisión.")
	}
	input.Mode = source.Mode
	input.TableID = source.TableID
	input.CustomerName, input.Phone, input.Origin = source.CustomerName, source.Phone, source.Origin
	input.ScheduledLocal = source.ScheduledLocal
	if source.ScheduleKind != nil {
		input.ScheduleKind = *source.ScheduleKind
	}
	input.AssignedUserID = &source.AssignedUserID
	input.PriceListID = &source.PriceListID
	// Se hereda la lista, sin exigir permiso de seleccionar otra lista ni cambiar importes históricos.
	inherited := *source
	inherited.ID = ""
	scheduled, err := handler.validateAccountInput(input, &inherited)
	if err != nil {
		return nil, err
	}
	if err := handler.validateTableTarget(input.TableID); err != nil {
		return nil, err
	}
	return scheduled, nil
}

func (handler *service) transfer(id string, split bool) (any, error) {
	var input TransferInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	source, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(source, true); err != nil {
		return nil, err
	}
	if err := ensureOpen(source, input.Revision); err != nil {
		return nil, err
	}
	units, err := handler.selectedUnits(id, input.Selections)
	if err != nil {
		return nil, err
	}
	var target *Account
	var scheduled *time.Time
	if split {
		if input.NewAccount == nil || input.TargetAccountID != nil {
			return nil, invalid("Dividir requiere nombre de cuenta nueva y no acepta un destino existente.")
		}
		scheduled, err = handler.splitInput(source, input.NewAccount)
		if err != nil {
			return nil, err
		}
	} else {
		if input.TargetAccountID == nil || input.NewAccount != nil {
			return nil, invalid("Transferir requiere una cuenta de destino existente.")
		}
		if err := validateID(*input.TargetAccountID); err != nil {
			return nil, err
		}
		target, err = handler.readAccount(*input.TargetAccountID)
		if err != nil {
			return nil, err
		}
		if err := ensureOpen(target, input.TargetRevision); err != nil {
			return nil, err
		}
		if err := compatibleAccounts(source, target); err != nil {
			return nil, err
		}
		if err := handler.validateTransferTarget(target); err != nil {
			return nil, err
		}
	}
	// Toda autorización y validación termina antes de crear una cuenta o mover la primera unidad.
	if split {
		target, err = handler.insertAccount(*input.NewAccount, scheduled)
		if err != nil {
			return nil, err
		}
	}
	if err := handler.touch(source.ID, source.Revision); err != nil {
		return nil, err
	}
	if err := handler.touch(target.ID, target.Revision); err != nil {
		return nil, err
	}
	identifiers := make([]string, 0, len(units))
	for _, unit := range units {
		if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_item_units SET account_id=$1 WHERE id=$2", target.ID, unit.ID); err != nil {
			return nil, err
		}
		identifiers = append(identifiers, unit.ID)
	}
	kind := "items.transferred"
	if split {
		kind = "account.split"
	}
	if err := handler.event(source.ID, &target.ID, kind, map[string]any{"selections": input.Selections}, map[string]any{
		"source_id": source.ID, "target_id": target.ID, "unit_ids": identifiers, "totals": unitTotals(units),
	}); err != nil {
		return nil, err
	}
	return handler.accountPair(source.ID, target.ID)
}

func (handler *service) accountPair(sourceID, targetID string) (any, error) {
	source, err := handler.readBill(sourceID)
	if err != nil {
		return nil, err
	}
	target, err := handler.readBill(targetID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": source, "target": target}, nil
}
