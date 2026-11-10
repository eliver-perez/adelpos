// Package features registra las funciones activables del establecimiento y consulta su estado
// en PostgreSQL. No depende de ningún módulo de negocio: administración lo usa para listar y
// cambiar funciones, y cada módulo (por ejemplo, payments) lo consulta para hacer cumplir el
// estado aunque alguien llame a la API directamente.
//
// Agregar una función nueva:
//  1. Declararla en Known con su código estable, nombre y explicación en español.
//  2. Sembrar su fila desactivada en una migración nueva (una fila ausente también cuenta como
//     desactivada, pero la semilla deja el estado explícito y auditable).
//  3. En el módulo dueño, llamar a Enabled dentro de la transacción de la operación y rechazar
//     con 409 FEATURE_DISABLED los datos de la función cuando está desactivada.
//  4. En la interfaz, leer settings/features y ocultar lo que no aplica.
//
// Desactivar una función nunca borra ni altera los registros históricos que generó.
package features

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// USDCash habilita recibir dólares en efectivo con tipo de cambio fijo por turno.
const USDCash = "usd_cash"

// CashLimit habilita el tope de caja (etapa 7): un aviso, nunca un bloqueo, cuando el efectivo
// esperado de una caja supera el importe configurado.
const CashLimit = "cash_limit"

// InventoryControl habilita el consumo de inventario al liberar partidas a cocina (etapa 8). La
// administración de inventario (insumos, almacenes, existencias iniciales) funciona aunque esté
// desactivada; solo el consumo depende de ella y solo a partir de enabled_at.
const InventoryControl = "inventory_control"

// Definition describe una función conocida por el sistema.
type Definition struct {
	Code        string
	Name        string
	Description string
}

// Known es la lista cerrada de funciones activables. El orden es el de la interfaz.
var Known = []Definition{
	{
		Code: USDCash,
		Name: "Pago en dólares en efectivo",
		Description: "Permite recibir dólares estadounidenses en efectivo, solos o junto con pesos, con un tipo de cambio " +
			"fijo que se captura al abrir cada turno. El cambio se muestra en pesos y en dólares. Las tarjetas, " +
			"transferencias y devoluciones siguen en pesos.",
	},
	{
		Code: CashLimit,
		Name: "Tope de efectivo en caja",
		Description: "Avisa a quienes pueden consultar el efectivo esperado cuando el de una caja supera el tope configurado, " +
			"para sugerir un retiro. Nunca bloquea cobros. Los importes del tope (en pesos y, con el pago en dólares, en " +
			"dólares) se configuran en Caja → Configuración con el permiso de administrar la caja.",
	},
	{
		Code: InventoryControl,
		Name: "Control de inventario",
		Description: "Descuenta de las existencias los insumos de cada partida cuando se libera a cocina, según sus recetas " +
			"y porciones. Solo consume lo que se libere después de activarla: los pedidos ya liberados nunca consumen. " +
			"Insumos, almacenes y existencias iniciales pueden prepararse antes de activarla. Desactivarla deja de " +
			"generar consumos nuevos y conserva todo el historial de existencias y movimientos.",
	},
}

// Feature es el estado público de una función.
type Feature struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	// EnabledAt es la fecha y hora en que la función pasó de desactivada a activada por última vez
	// (etapa 8); desactivarla la conserva. Nil si nunca se activó desde la migración 0010.
	EnabledAt     *time.Time `json:"enabled_at"`
	Revision      int        `json:"revision"`
	UpdatedAt     *time.Time `json:"updated_at"`
	UpdatedByName *string    `json:"updated_by_name"`
}

// ErrUnknown indica un código que no pertenece al registro.
var ErrUnknown = errors.New("función desconocida")

// ErrRevision indica que la función cambió en otra terminal.
var ErrRevision = errors.New("revisión obsoleta")

// Lookup devuelve la definición de una función conocida.
func Lookup(code string) (Definition, bool) {
	for _, definition := range Known {
		if definition.Code == code {
			return definition, true
		}
	}
	return Definition{}, false
}

// Enabled consulta el estado vigente de una función en la transacción de la operación. Una
// función desconocida o sin fila se considera desactivada.
func Enabled(ctx context.Context, tx pgx.Tx, code string) (bool, error) {
	if _, known := Lookup(code); !known {
		return false, nil
	}
	var enabled bool
	err := tx.QueryRow(ctx, "SELECT enabled FROM app.establishment_features WHERE code=$1", code).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

// Read devuelve el estado de una función conocida; sin fila, desactivada con revisión 0.
func Read(ctx context.Context, tx pgx.Tx, code string) (*Feature, error) {
	definition, known := Lookup(code)
	if !known {
		return nil, ErrUnknown
	}
	feature := &Feature{Code: definition.Code, Name: definition.Name, Description: definition.Description}
	err := tx.QueryRow(
		ctx,
		`SELECT f.enabled,f.enabled_at,f.revision,f.updated_at,u.display_name FROM app.establishment_features f
        LEFT JOIN app.users u ON u.id=f.updated_by WHERE f.code=$1`,
		code,
	).Scan(&feature.Enabled, &feature.EnabledAt, &feature.Revision, &feature.UpdatedAt, &feature.UpdatedByName)
	if errors.Is(err, pgx.ErrNoRows) {
		return feature, nil
	}
	if err != nil {
		return nil, err
	}
	return feature, nil
}

// List devuelve todas las funciones conocidas con su estado vigente.
func List(ctx context.Context, tx pgx.Tx) ([]Feature, error) {
	result := make([]Feature, 0, len(Known))
	for _, definition := range Known {
		feature, err := Read(ctx, tx, definition.Code)
		if err != nil {
			return nil, err
		}
		result = append(result, *feature)
	}
	return result, nil
}

// Set cambia el estado de una función con revisión optimista y devuelve el estado anterior y el
// nuevo. La fila se bloquea para que dos terminales no cambien la misma función a la vez; una
// función sin fila se crea con revisión 1 (la revisión esperada debe ser 0). Al pasar de
// desactivada a activada se fija enabled_at con la hora de la transacción; activarla otra vez sin
// haberla desactivado o desactivarla conserva la fecha anterior.
func Set(ctx context.Context, tx pgx.Tx, code string, enabled bool, revision int, actorID string) (*Feature, *Feature, error) {
	if _, known := Lookup(code); !known {
		return nil, nil, ErrUnknown
	}
	if _, err := tx.Exec(ctx, "SELECT 1 FROM app.establishment_features WHERE code=$1 FOR UPDATE", code); err != nil {
		return nil, nil, err
	}
	previous, err := Read(ctx, tx, code)
	if err != nil {
		return nil, nil, err
	}
	if previous.Revision != revision {
		return nil, nil, ErrRevision
	}
	var actor *string
	if actorID != "" {
		actor = &actorID
	}
	if previous.Revision == 0 {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO app.establishment_features(code,enabled,enabled_at,updated_by)
            VALUES ($1,$2,CASE WHEN $2 THEN now() END,$3)`,
			code,
			enabled,
			actor,
		)
	} else {
		_, err = tx.Exec(
			ctx,
			`UPDATE app.establishment_features SET enabled=$2,
                enabled_at=CASE WHEN $2 AND NOT enabled THEN now() ELSE enabled_at END,
                updated_by=$3,revision=revision+1,updated_at=now()
            WHERE code=$1`,
			code,
			enabled,
			actor,
		)
	}
	if err != nil {
		return nil, nil, err
	}
	saved, err := Read(ctx, tx, code)
	if err != nil {
		return nil, nil, err
	}
	return previous, saved, nil
}
