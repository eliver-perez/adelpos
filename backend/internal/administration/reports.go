package administration

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/reports"
)

// Reportes (etapa 9A). La petición se autentica y autoriza en la transacción corta habitual
// (Execute), que se confirma con su auditoría; reports.Handle devuelve entonces un plan y la consulta
// corre después, fuera de esa transacción, en el ejecutor de reportes: transacción REPEATABLE READ
// READ ONLY con límites de tiempo sobre un semáforo propio de reports.Slots conexiones. Así un
// reporte largo no retiene los locks de fila del dispositivo y la sesión, ni ningún lock consultivo.

// reportTimeout es el tiempo de respuesta HTTP de reports/* (las demás rutas tienen 10 s): cubre el
// statement_timeout de 30 s de la consulta más la autenticación.
const reportTimeout = 40 * time.Second

// reportRunner devuelve el ejecutor de reportes del servicio (se crea al primer uso sobre el pool).
func (service *Service) reportRunner() *reports.Runner {
	service.reportsOnce.Do(func() {
		if service.Reports == nil {
			service.Reports = reports.NewRunner(service.Pool, reports.Slots)
		}
	})
	return service.Reports
}

// handleReports atiende reports/* dentro de la transacción de autenticación (routes.go).
func handleReports(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	plan, err := reports.Handle(ctx, tx, reports.Request{
		Method: request.Method, Route: route, Query: request.Query,
		Actor: reports.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
	})
	if err != nil {
		return nil, reportFault(err)
	}
	return plan, nil
}

// completeDeferred ejecuta, ya confirmada la transacción de autenticación, la consulta diferida de un
// reporte. Cualquier otro resultado se devuelve tal cual.
func (service *Service) completeDeferred(ctx context.Context, result any) (any, error) {
	plan, ok := result.(*reports.Plan)
	if !ok {
		return result, nil
	}
	value, err := plan.Execute(ctx, service.reportRunner())
	if err != nil {
		return nil, reportFault(err)
	}
	return value, nil
}

func reportFault(err error) error {
	var problem *reports.Error
	if errors.As(err, &problem) {
		return &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message}
	}
	return err
}
