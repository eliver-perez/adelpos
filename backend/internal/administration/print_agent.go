package administration

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/kitchen"
)

// printAgent atiende las rutas print-agent/* del agente de impresión de una terminal (etapa 9G, fase E;
// kitchen/agent.go). La terminal ya está autenticada por su credencial (authenticateDevice); no hay sesión de
// usuario, así que la auditoría la firma la terminal. Cada terminal solo informa sus propias impresoras y solo
// reclama y confirma los trabajos de las impresoras que tiene conectadas.
func (service *Service) printAgent(ctx context.Context, tx pgx.Tx, request Request, route string) (any, error) {
	principal := Principal{DeviceID: request.DeviceID}
	result, err := kitchen.HandleAgent(ctx, tx, kitchen.AgentRequest{
		Method: request.Method, Route: route, Body: request.Body, DeviceID: request.DeviceID, Rasters: service.documentRasters(),
	}, func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
		return audit(ctx, tx, principal, action, entity, id, before, after, true)
	})
	var problem *kitchen.Error
	if errors.As(err, &problem) {
		return nil, &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message, Details: problem.Details}
	}
	return result, err
}
