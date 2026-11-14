package administration

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/identity"
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

// exportTimeout es el tiempo de respuesta de reports/export/* (fase C1): la consulta (hasta 30 s) más
// la generación del archivo de hasta 50 000 renglones. El puente nativo espera 120 s.
const exportTimeout = 110 * time.Second

// reportPlan conserva quién pidió el reporte para auditar la exportación después de generar el
// archivo, fuera de la transacción de autenticación.
type reportPlan struct {
	plan      *reports.Plan
	principal Principal
}

// reportRunner devuelve el ejecutor de reportes del servicio (se crea al primer uso sobre el pool).
func (service *Service) reportRunner() *reports.Runner {
	service.reportsOnce.Do(func() {
		if service.Reports == nil {
			// Etapa 9B: el encabezado de las exportaciones usa el logotipo del restaurante.
			service.Reports = reports.NewRunner(service.Pool, reports.Slots).UseLogoSource(identity.ReportLogo)
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
	return &reportPlan{plan: plan, principal: principal}, nil
}

// completeDeferred ejecuta, ya confirmada la transacción de autenticación, la consulta diferida de un
// reporte. Cualquier otro resultado se devuelve tal cual. Un archivo exportado se audita antes de
// entregarlo: si la auditoría no se puede registrar, el archivo no se entrega.
func (service *Service) completeDeferred(ctx context.Context, result any) (any, error) {
	// «Quitar datos de prueba» (etapa 9F): el respaldo y la transacción que quita los datos corren
	// después de confirmar la autenticación (maintenance.go).
	if removal, ok := result.(*testDataRemoval); ok {
		return service.removeTestData(ctx, removal)
	}
	// Reinicio del servicio (etapa 9G, fase A): se programa ya auditado (service_host.go).
	if _, ok := result.(*serviceRestart); ok {
		return service.completeRestart()
	}
	pending, ok := result.(*reportPlan)
	if !ok {
		return result, nil
	}
	value, err := pending.plan.Execute(ctx, service.reportRunner())
	if err != nil {
		return nil, reportFault(err)
	}
	if file, ok := value.(*reports.File); ok {
		if err := service.auditExport(ctx, pending.principal, file); err != nil {
			return nil, err
		}
	}
	return value, nil
}

// auditExport registra report.exported en una transacción corta aparte, sin locks operativos: el
// reporte, el formato, el periodo, los filtros y el número de renglones; nunca el contenido.
func (service *Service) auditExport(ctx context.Context, principal Principal, file *reports.File) error {
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := audit(ctx, tx, principal, "report.exported", "report", nil, nil, file.Audit, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// writeExport envía un archivo exportado como respuesta binaria con su tipo exacto y el nombre
// sugerido. Devuelve false si el resultado no es un archivo.
func writeExport(writer http.ResponseWriter, result any) bool {
	file, ok := result.(*reports.File)
	if !ok {
		return false
	}
	header := writer.Header()
	header.Set("Content-Type", file.ContentType)
	header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.FileName}))
	header.Set("Content-Length", strconv.Itoa(len(file.Content)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(file.Content)
	return true
}

func reportFault(err error) error {
	var problem *reports.Error
	if errors.As(err, &problem) {
		return &Fault{Status: problem.Status, Code: problem.Code, Message: problem.Message}
	}
	return err
}
