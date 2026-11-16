package legacyread

import (
	"encoding/json"
	"io"
	"sync"

	"local/ad-el-pos/server/internal/legacypkg"
)

// Avance de legacy-prepare: una línea JSON por evento en la salida estándar (la aplicación de escritorio lanza el
// subcomando y la muestra; docs/MIGRACION_FORMATO.md, «Avance»).

// Pasos de la preparación.
const (
	StepImage   = "image"
	StepStart   = "start"
	StepRestore = "restore"
	StepRead    = "read"
	StepWrite   = "write"
	StepCleanup = "cleanup"
)

// stepMessages son los textos de cada paso para el propietario.
var stepMessages = map[string]string{
	StepImage:   "Preparando el lector del sistema anterior",
	StepStart:   "Iniciando el lector temporal",
	StepRestore: "Abriendo el respaldo",
	StepRead:    "Leyendo los datos del sistema anterior",
	StepWrite:   "Guardando el paquete cifrado",
	StepCleanup: "Borrando el lector temporal",
}

// StepEvent marca el inicio («started») o el final («finished», con sus segundos) de un paso.
type StepEvent struct {
	Event   string   `json:"event"`
	Step    string   `json:"step"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Seconds *float64 `json:"seconds,omitempty"`
}

// ProgressEvent es el avance dentro de un paso: capas descargadas de la imagen o porcentaje de la restauración.
type ProgressEvent struct {
	Event   string `json:"event"`
	Step    string `json:"step"`
	Message string `json:"message"`
	Current int64  `json:"current"`
	Total   int64  `json:"total"`
}

// TableEvent es el avance de la lectura: tabla actual, cuántas van de cuántas y sus filas leídas.
type TableEvent struct {
	Event     string `json:"event"`
	Table     string `json:"table"`
	Status    string `json:"status"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Rows      int64  `json:"rows"`
	TableRows int64  `json:"table_rows"`
}

// ResultEvent es el resumen final.
type ResultEvent struct {
	Event   string  `json:"event"`
	Summary Summary `json:"summary"`
}

// ErrorEvent es el error final.
type ErrorEvent struct {
	Event   string `json:"event"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Summary es el resumen de un paquete preparado (lo muestra el asistente y lo usa la Fase E para medir).
type Summary struct {
	Output             string                   `json:"output"`
	PackageBytes       int64                    `json:"package_bytes"`
	ContentFingerprint string                   `json:"content_fingerprint"`
	Backup             BackupSummary            `json:"backup"`
	Sales              *SalesRange              `json:"sales,omitempty"`
	Tables             []TableCount             `json:"tables"`
	SkippedTables      []legacypkg.SkippedTable `json:"skipped_tables"`
	OmittedColumns     []OmittedColumnSummary   `json:"omitted_columns"`
	Timings            map[string]float64       `json:"timings"`
}

// BackupSummary describe el respaldo leído.
type BackupSummary struct {
	File          string `json:"file"`
	Bytes         int64  `json:"bytes"`
	Database      string `json:"database"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	ServerVersion string `json:"server_version"`
}

// SalesRange es la fecha de la primera y la última venta.
type SalesRange struct {
	First string `json:"first"`
	Last  string `json:"last"`
}

// TableCount es el conteo de una tabla extraída.
type TableCount struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

// OmittedColumnSummary es una columna que no viajó.
type OmittedColumnSummary struct {
	Table         string `json:"table"`
	Column        string `json:"column"`
	Reason        string `json:"reason"`
	RowsWithValue int64  `json:"rows_with_value"`
}

// Reporter recibe los eventos.
type Reporter interface {
	Emit(event any)
}

// JSONReporter escribe cada evento como una línea JSON. Si la salida se cierra (la aplicación que lo lanzó terminó),
// llama a Broken una sola vez para cancelar la preparación.
type JSONReporter struct {
	output io.Writer
	broken func()
	mutex  sync.Mutex
	failed bool
}

// NewJSONReporter crea el informador de la salida estándar.
func NewJSONReporter(output io.Writer, broken func()) *JSONReporter {
	return &JSONReporter{output: output, broken: broken}
}

func (reporter *JSONReporter) Emit(event any) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()
	if reporter.failed {
		return
	}
	if _, err := reporter.output.Write(append(encoded, '\n')); err != nil {
		reporter.failed = true
		if reporter.broken != nil {
			reporter.broken()
		}
	}
}

func stepStarted(step string) StepEvent {
	return StepEvent{Event: "step", Step: step, Status: "started", Message: stepMessages[step]}
}

func stepFinished(step string, seconds float64) StepEvent {
	return StepEvent{Event: "step", Step: step, Status: "finished", Message: stepMessages[step], Seconds: &seconds}
}

func errorEvent(failure *Failure) ErrorEvent {
	return ErrorEvent{Event: "error", Code: failure.Code, Message: failure.Message, Detail: failure.Detail}
}
