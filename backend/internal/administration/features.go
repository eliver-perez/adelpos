package administration

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
)

// establishmentFeatures lista las funciones activables a cualquier sesión autenticada (la interfaz
// decide qué mostrar) y permite cambiarlas con settings.features.manage. El código viaja en el
// cuerpo porque el puente nativo solo admite identificadores UUID en la ruta.
func establishmentFeatures(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if request.Method == "GET" {
		return features.List(ctx, tx)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para las funciones del establecimiento.")
	}
	// Se autoriza antes de cualquier lectura con bloqueo o escritura.
	if err := principal.require("settings.features.manage"); err != nil {
		return nil, err
	}
	var input struct {
		Code     string `json:"code"`
		Enabled  bool   `json:"enabled"`
		Revision int    `json:"revision"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	code := strings.TrimSpace(input.Code)
	if input.Revision < 0 {
		return nil, invalid("La revisión de la función no es válida.")
	}
	previous, saved, err := features.Set(ctx, tx, code, input.Enabled, input.Revision, principal.UserID)
	if errors.Is(err, features.ErrUnknown) {
		return nil, &Fault{Status: 404, Code: "FEATURE_NOT_FOUND", Message: "La función del establecimiento no existe."}
	}
	if errors.Is(err, features.ErrRevision) {
		return nil, &Fault{
			Status:  409,
			Code:    "REVISION_CONFLICT",
			Message: "La función cambió en otra terminal. Actualiza la configuración y vuelve a intentarlo.",
		}
	}
	if err != nil {
		return nil, err
	}
	// Cambiar el estado no borra ni altera registros históricos: solo decide qué se acepta después.
	if err := events.Append(ctx, tx, "settings.features.changed", "configuration", nil, nil); err != nil {
		return nil, err
	}
	return saved, audit(ctx, tx, principal, "settings.feature_changed", "establishment_feature", nil, previous, saved, true)
}
