package administration

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/identity"
)

// handleDocumentIdentity atiende documents/identity* (etapa 9B, fase B2: identidad del
// establecimiento y logotipo) dentro de la transacción autenticada, con el mismo Actor y la misma
// auditoría que los demás módulos. identity.Handle comprueba sus permisos antes de leer el cuerpo o
// escribir; un 403 revierte la operación con el savepoint de runProtected.
func handleDocumentIdentity(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	result, err := identity.Handle(ctx, tx, identity.Request{
		Method: request.Method,
		Route:  route,
		Body:   request.Body,
		Actor: identity.Actor{
			UserID:      principal.UserID,
			DeviceID:    principal.DeviceID,
			Permissions: principal.Permissions,
		},
	}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
		return audit(ctx, tx, principal, action, entity, id, before, after, true)
	})
	var problem *identity.Error
	if errors.As(err, &problem) {
		return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message}
	}
	return result, err
}

// documentRasters es la fuente de las conversiones del logotipo para la vista previa del diseñador:
// la configurada en el servicio (main.go) o, sin valor (pruebas de aceptación), la de su pool. Las
// conversiones son inmutables, así que leerlas fuera de la transacción de la petición es seguro.
func (service *Service) documentRasters() documents.RasterSource {
	if service.Rasters != nil {
		return service.Rasters
	}
	return identity.NewRasters(service.Pool)
}
