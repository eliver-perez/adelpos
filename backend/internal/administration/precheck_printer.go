package administration

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/kitchen"
)

// Impresora de precuentas por dispositivo (etapa A2, parte C; §5 de la especificación y decisión A2-D3). Un teléfono o
// tableta de meseros imprime la precuenta solo en la impresora que se le asigna en Sistema › Terminales › Editar
// (devices.precheck_printer_id, migración 0026, parte C): sin ella no imprime en ninguna otra (payments). Se elige
// entre las impresoras que sirven para precuentas y tickets en Cobros: activas y con un transporte que imprime (las de
// red y las USB de una terminal con agente; kitchen.PrinterProblem).

// optionalID es un identificador opcional de un cuerpo PATCH que distingue «no se envió» (Set falso: se conserva lo
// guardado) de null (Set verdadero y Value nil: se quita).
type optionalID struct {
	Set   bool
	Value *string
}

// UnmarshalJSON solo se llama cuando el campo viene en el cuerpo (también con null).
func (field *optionalID) UnmarshalJSON(data []byte) error {
	field.Set = true
	if string(data) == "null" {
		field.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	field.Value = &value
	return nil
}

// validatePrecheckPrinter revisa la impresora de precuentas que se quiere asignar: solo en un dispositivo android y
// solo una impresora que existe y puede imprimir precuentas. Quitarla (null) siempre procede.
func validatePrecheckPrinter(ctx context.Context, tx pgx.Tx, deviceID string, field optionalID) error {
	if !field.Set || field.Value == nil {
		return nil
	}
	if !identifierPattern.MatchString(*field.Value) {
		return invalid("Identificador de impresora inválido.")
	}
	if err := requireAndroidDevice(ctx, tx, deviceID, "La impresora de precuentas solo se asigna a teléfonos y tabletas de meseros."); err != nil {
		return err
	}
	choice, err := kitchen.DocumentPrinterChoice(ctx, tx, *field.Value)
	if err != nil {
		return err
	}
	if choice == nil {
		return invalid("La impresora elegida ya no existe.")
	}
	if choice.Problem != "" {
		return invalid(kitchen.UnavailablePrinterMessage(choice.Problem, choice.Name) +
			" Elige una impresora que pueda imprimir precuentas.")
	}
	return nil
}

// requireAndroidDevice rechaza con el mensaje dado lo que solo se configura en un teléfono o tableta de meseros.
func requireAndroidDevice(ctx context.Context, tx pgx.Tx, deviceID, message string) error {
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.devices WHERE id=$1", deviceID).Scan(&kind); err != nil {
		return err
	}
	if kind != "android" {
		return invalid(message)
	}
	return nil
}

// precheckPrinterConfiguration es lo que «Ver configuración» y «Editar» muestran de la impresora de precuentas de un
// dispositivo android: la asignada con su problema (nil si no tiene) y las que se le pueden elegir. En las demás
// terminales no aplica: nil y una lista vacía.
func precheckPrinterConfiguration(ctx context.Context, tx pgx.Tx, deviceID string) (*kitchen.DocumentPrinterOption,
	[]kitchen.DocumentPrinterOption, error) {
	var kind string
	var printerID *string
	if err := tx.QueryRow(ctx, "SELECT kind,precheck_printer_id::text FROM app.devices WHERE id=$1", deviceID).
		Scan(&kind, &printerID); err != nil {
		return nil, nil, err
	}
	if kind != "android" {
		return nil, []kitchen.DocumentPrinterOption{}, nil
	}
	var assigned *kitchen.DocumentPrinterOption
	if printerID != nil {
		choice, err := kitchen.DocumentPrinterChoice(ctx, tx, *printerID)
		if err != nil {
			return nil, nil, err
		}
		assigned = choice
	}
	options, err := kitchen.DocumentPrinterOptions(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	return assigned, options, nil
}
