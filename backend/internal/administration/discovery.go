package administration

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// EstablishmentName es el nombre del establecimiento para el anuncio del servidor en la red local (etapa 9G,
// fase C; internal/discovery). Antes del asistente inicial no hay establecimiento: devuelve "" y el anuncio usa
// el nombre de la computadora.
func (service *Service) EstablishmentName(ctx context.Context) (string, error) {
	var name string
	err := service.Pool.QueryRow(ctx, "SELECT name FROM app.establishment").Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}
