package administration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

func registers(ctx context.Context, tx pgx.Tx, request Request, principal Principal, parts []string) (any, error) {
	if request.Method == "GET" && len(parts) == 1 {
		if err := principal.require("registers.read"); err != nil {
			return nil, err
		}
		return list(ctx, tx, "SELECT id,name,description,active,created_at FROM app.registers ORDER BY name")
	}
	if err := principal.require("registers.manage"); err != nil {
		return nil, err
	}
	if (request.Method != "POST" || len(parts) != 1) && (request.Method != "PATCH" || len(parts) != 2) {
		return nil, invalid("Operación de caja no válida.")
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Active      bool   `json:"active"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 || len(input.Description) > 500 {
		return nil, invalid("Revisa el nombre (hasta 80 caracteres) y descripción (hasta 500).")
	}
	var registerID string
	var previous json.RawMessage
	action := "register.created"
	if len(parts) == 1 {
		if err := tx.QueryRow(ctx, "INSERT INTO app.registers(name,description,active) VALUES ($1,$2,$3) RETURNING id", input.Name, input.Description, input.Active).Scan(&registerID); err != nil {
			return nil, err
		}
	} else {
		registerID = parts[1]
		var err error
		previous, err = object(ctx, tx, "SELECT id,name,description,active FROM app.registers WHERE id=$1 FOR UPDATE", registerID)
		if err != nil {
			return nil, err
		}
		if !input.Active {
			var inUse bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.devices WHERE register_id=$1 AND revoked_at IS NULL)", registerID).Scan(&inUse); err != nil {
				return nil, err
			}
			if inUse {
				return nil, &Fault{Status: 409, Code: "REGISTER_IN_USE", Message: "Desasocia las terminales autorizadas antes de desactivar la caja."}
			}
			// Una caja con turno activo (abierto o en arqueo de cierre) conserva su identidad
			// financiera hasta el cierre.
			var shiftOpen bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.cash_shifts WHERE register_id=$1 AND state IN ('open','closing'))", registerID).Scan(&shiftOpen); err != nil {
				return nil, err
			}
			if shiftOpen {
				return nil, &Fault{Status: 409, Code: "SHIFT_OPEN", Message: "La caja tiene un turno abierto o en arqueo de cierre. No puede desactivarse hasta cerrar ese turno."}
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE app.registers SET name=$1,description=$2,active=$3 WHERE id=$4", input.Name, input.Description, input.Active, registerID); err != nil {
			return nil, err
		}
		action = "register.updated"
	}
	next, err := object(ctx, tx, "SELECT id,name,description,active,created_at FROM app.registers WHERE id=$1", registerID)
	if err != nil {
		return nil, err
	}
	return next, audit(ctx, tx, principal, action, "register", &registerID, previous, next, true)
}
