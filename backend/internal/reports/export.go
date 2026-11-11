package reports

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Exportación de reportes a PDF y Excel (etapa 9A, §17, §18 y §28).
//
// GET reports/export/{id}?format=pdf|xlsx&… admite los mismos parámetros que reports/run (periodo,
// comparación, orden y los filtros que declara el reporte), salvo la paginación, más paper=letter|a4
// para el PDF. La exportación usa exactamente la misma definición, la misma consulta y el mismo
// Result que la pantalla (Prepare y Execute): columnas visibles según los permisos, todos los
// renglones del conjunto filtrado (Result.AllRows), los totales y las cifras calculados en Go y los
// metadatos de la misma fotografía. Nada se recalcula con reglas distintas.
//
// Permisos: además de los del reporte, reports.export.pdf o reports.export.excel según el formato,
// validados en la transacción de autenticación (una denegación se audita como access.denied). Las
// columnas protegidas (costos, efectivo físico, esperado de caja) no existen en el Result de quien
// no tiene su permiso, así que tampoco en el archivo, aunque se llame a la API directamente.
//
// El archivo se genera en memoria dentro de la misma petición y se entrega como respuesta binaria
// (Content-Type exacto y nombre sugerido en Content-Disposition): no se guarda en el servidor ni
// existe un identificador de descarga que otra persona pudiera adivinar. La consulta corre en la
// transacción de solo lectura del Runner y el lugar del semáforo se conserva mientras se genera el
// archivo, para que dos exportaciones grandes no compitan con más reportes. Después de generar el
// archivo, administration registra report.exported (reporte, formato, filtros y número de
// renglones; sin contenido ni datos personales) en una transacción corta aparte.

const (
	FormatPDF  = "pdf"
	FormatXLSX = "xlsx"

	PaperLetter = "letter"
	PaperA4     = "a4"

	ContentTypePDF  = "application/pdf"
	ContentTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	// MaxExportRows es el máximo de renglones de un archivo exportado. Si el conjunto filtrado lo
	// excede, la exportación se rechaza con 422 sin generar nada.
	MaxExportRows = 50000
	// MaxExportBytes es el tamaño máximo de un archivo exportado (el mismo tope que aplica el puente).
	MaxExportBytes = 32 << 20
)

// File es un archivo exportado listo para enviarse. Audit es lo que se registra en report.exported.
type File struct {
	ContentType string
	FileName    string
	Content     []byte
	Audit       ExportAudit
}

// ExportAudit describe una exportación para la auditoría: qué reporte, en qué formato, con qué
// periodo, filtros (claves y valores tal como se enviaron, sin nombres de personas) y cuántos
// renglones. Nunca lleva importes ni el contenido del archivo.
type ExportAudit struct {
	Report      string            `json:"report"`
	Format      string            `json:"format"`
	Paper       string            `json:"paper,omitempty"`
	Orientation string            `json:"orientation,omitempty"`
	Period      *ExportPeriod     `json:"period,omitempty"`
	Comparison  bool              `json:"comparison"`
	Filters     map[string]string `json:"filters"`
	Sort        SortInfo          `json:"sort"`
	Rows        int               `json:"rows"`
	Bytes       int               `json:"bytes"`
}

// ExportPeriod es el periodo resuelto de una exportación (días comerciales).
type ExportPeriod struct {
	Kind     string `json:"kind"`
	FirstDay string `json:"first_day"`
	LastDay  string `json:"last_day"`
}

// exportOptions son los parámetros propios de la exportación.
type exportOptions struct {
	Format string
	Paper  string
}

// exportDocument es lo que reciben los generadores: el Result de la pantalla, la identidad del
// establecimiento leída en la misma fotografía (nombre, dirección, teléfono y correo, vacíos si no
// existen, y la imagen del encabezado: el logotipo del restaurante o el de AD-EL POS), la zona
// horaria y el papel.
type exportDocument struct {
	Establishment string
	Address       string
	Phone         string
	Email         string
	Logo          *exportLogo
	Result        *Result
	Location      *time.Location
	Paper         string
}

func exportTooLarge(rows int) error {
	return &Error{
		Status: http.StatusUnprocessableEntity,
		Code:   "EXPORT_TOO_LARGE",
		Message: "El reporte tiene " + groupDigits(strconv.Itoa(rows)) + " renglones y la exportación admite hasta " +
			groupDigits(strconv.Itoa(MaxExportRows)) + ". Reduce el periodo o agrega filtros.",
	}
}

func exportFileTooLarge() error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "EXPORT_FILE_TOO_LARGE",
		Message: "El archivo excede 32 MiB. Reduce el periodo o agrega filtros.",
	}
}

func exportCancelled() error {
	return &Error{
		Status:  http.StatusServiceUnavailable,
		Code:    "REPORT_TIMEOUT",
		Message: "La exportación tardó demasiado. Reduce el periodo o agrega filtros y vuelve a intentarlo.",
	}
}

// exportPermission es el permiso de exportación de cada formato.
func exportPermission(format string) string {
	if format == FormatPDF {
		return PermissionExportPDF
	}
	return PermissionExportExcel
}

// parseExportOptions valida format y paper y devuelve los demás parámetros, que son los de
// reports/run sin la paginación.
func parseExportOptions(values url.Values) (exportOptions, url.Values, error) {
	options := exportOptions{}
	format, err := single(values, "format")
	if err != nil {
		return options, nil, err
	}
	if format != FormatPDF && format != FormatXLSX {
		return options, nil, invalid("El formato de exportación debe ser pdf o xlsx.")
	}
	options.Format = format
	paper, err := single(values, "paper")
	if err != nil {
		return options, nil, err
	}
	switch {
	case paper == "" && format == FormatPDF:
		options.Paper = PaperLetter
	case paper == PaperLetter || paper == PaperA4:
		if format != FormatPDF {
			return options, nil, invalid("El tamaño de papel solo aplica al PDF.")
		}
		options.Paper = paper
	case paper != "":
		return options, nil, invalid("El tamaño de papel debe ser letter o a4.")
	}
	rest := url.Values{}
	for key, list := range values {
		switch key {
		case "format", "paper":
			continue
		case "page", "page_size":
			return options, nil, invalid("La exportación incluye todos los renglones del reporte; no admite page ni page_size.")
		}
		rest[key] = list
	}
	return options, rest, nil
}

// prepareExport valida la exportación en la transacción de autenticación: formato, permiso de
// exportación, reporte, permisos del reporte y forma de los parámetros. La generación queda en un
// plan diferido que corre en el Runner.
func prepareExport(actor Actor, id string, values url.Values) (*Plan, error) {
	options, rest, err := parseExportOptions(values)
	if err != nil {
		return nil, err
	}
	if !actor.Can(exportPermission(options.Format)) {
		return nil, &Error{Status: http.StatusForbidden, Code: "PERMISSION_DENIED",
			Message: "No tienes permiso para exportar reportes en este formato."}
	}
	definition, raw, err := Prepare(actor, id, rest)
	if err != nil {
		return nil, err
	}
	return &Plan{deferred: func(ctx context.Context, runner *Runner) (any, error) {
		return runExport(ctx, runner, actor, definition, raw, options)
	}}, nil
}

// runExport consulta el reporte en la transacción de solo lectura y genera el archivo conservando
// el lugar del semáforo hasta terminar.
func runExport(ctx context.Context, runner *Runner, actor Actor, definition *Definition, raw *rawParams,
	options exportOptions) (*File, error) {
	release, ok := runner.Acquire()
	if !ok {
		return nil, busy()
	}
	defer release()
	document := &exportDocument{Paper: options.Paper}
	var identity exportIdentity
	err := runner.read(ctx, func(tx pgx.Tx) error {
		result, err := Execute(ctx, tx, actor, definition, raw)
		if err != nil {
			return err
		}
		document.Result = result
		identity, err = readExportIdentity(ctx, tx, runner.logoSource())
		return err
	})
	if err != nil {
		return nil, err
	}
	// La imagen del encabezado se prepara ya cerrada la transacción de solo lectura.
	document.applyIdentity(identity)
	result := document.Result
	if len(result.AllRows) > MaxExportRows {
		return nil, exportTooLarge(len(result.AllRows))
	}
	document.Location = exportLocation(result.Meta.Timezone)
	var content []byte
	orientation := ""
	if options.Format == FormatPDF {
		content, orientation, err = renderPDF(ctx, document)
	} else {
		content, err = renderXLSX(ctx, document)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, exportCancelled()
		}
		return nil, err
	}
	if len(content) > MaxExportBytes {
		return nil, exportFileTooLarge()
	}
	file := &File{
		Content:  content,
		FileName: exportFileName(result, options.Format, document.Location),
		Audit: ExportAudit{
			Report: result.Report.ID, Format: options.Format, Paper: options.Paper, Orientation: orientation,
			Comparison: result.Meta.Comparison != nil, Filters: map[string]string{}, Sort: result.Sort,
			Rows: len(result.AllRows), Bytes: len(content),
		},
	}
	file.ContentType = ContentTypeXLSX
	if options.Format == FormatPDF {
		file.ContentType = ContentTypePDF
	}
	if period := result.Meta.Period; period != nil {
		file.Audit.Period = &ExportPeriod{Kind: period.Kind, FirstDay: period.FirstDay, LastDay: period.LastDay}
	}
	for _, filter := range result.Meta.Filters {
		file.Audit.Filters[filter.Key] = filter.Value
	}
	return file, nil
}

// exportLocation es la zona horaria del restaurante para mostrar fechas y horas (UTC si no se
// reconoce, igual que businessday).
func exportLocation(timezone string) *time.Location {
	location, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		return time.UTC
	}
	return location
}
