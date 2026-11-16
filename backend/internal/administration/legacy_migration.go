package administration

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/backupschedule"
	"local/ad-el-pos/server/internal/legacyimport"
)

// Migración del sistema anterior (etapa 9H, fase C; internal/legacyimport), en Sistema › Mantenimiento con
// settings.manage (el permiso de Mantenimiento y de los respaldos: la migración reemplaza catálogo, mesas y usuarios y
// hace antes un respaldo) y solo desde la computadora principal (Request.Local: el paquete tiene datos reales del
// restaurante y la aplicación lo lee del disco de esa computadora). La interfaz es de la Fase D:
//
//   - GET  maintenance/legacy: límites de la subida, lote vigente, corrida activa, últimas corridas y subidas abiertas.
//   - POST maintenance/legacy/uploads {size, sha256}: empieza una subida (tope de 1 GiB).
//   - GET  maintenance/legacy/uploads/{id}: avance de la subida.
//   - POST maintenance/legacy/uploads/{id}/parts {index, data, sha256}: una parte de 1 MiB en base64 con su SHA-256,
//     en orden (reenviar la última no cambia nada); con la última se comprueba la huella del paquete.
//   - POST maintenance/legacy/uploads/{id}/cancel: borra la subida.
//   - POST maintenance/legacy/analyze {upload_id, passphrase}: análisis en segundo plano (resumen previo).
//   - POST maintenance/legacy/import {upload_id, passphrase, analysis_run_id, confirm, replace_batch_id}:
//     importación en segundo plano (respaldo verificado previo y una sola transacción).
//   - GET  maintenance/legacy/runs/{id}: avance y resultado de una corrida (resumen previo o informe final).
//   - GET  maintenance/legacy/batches/{id}: un lote con su informe y su validación.
//   - GET  maintenance/legacy/batches/{id}/report.pdf: el informe en PDF (con la lista de datos incoherentes).
//   - POST maintenance/legacy/preparations {…}: registra en el Registro de movimientos el resumen de una preparación
//     del paquete hecha por la aplicación (legacy-prepare), sin datos del restaurante.
//
// La frase de paso nunca se guarda: viaja en la petición y vive en memoria mientras corre el trabajo. No son rutas de
// la API de clientes (Android): no van en el contrato OpenAPI.

// legacyPartBodyLimit es el cuerpo de una parte: 1 MiB en base64 (1 398 104 caracteres) más el JSON que la envuelve.
const legacyPartBodyLimit = 1398104 + 1024

var legacyPartPath = regexp.MustCompile(`^/api/v1/maintenance/legacy/uploads/[0-9a-fA-F-]{36}/parts$`)

// legacyBodyLimit es el límite de cuerpo de una ruta de la migración (0 si no es una de ellas).
func legacyBodyLimit(path string) int64 {
	if legacyPartPath.MatchString(path) {
		return legacyPartBodyLimit
	}
	return 0
}

// legacyStart es un análisis o una importación ya registrados que se entregan al trabajador después de confirmar la
// transacción de la petición.
type legacyStart struct {
	pending *legacyimport.Pending
}

// legacyStatusCodes traduce los errores de la migración a su estado HTTP.
var legacyStatusCodes = map[string]int{
	legacyimport.CodeUploadMissing:      404,
	legacyimport.CodeUploadInvalid:      400,
	legacyimport.CodeUploadHash:         400,
	legacyimport.CodeWrongPassphrase:    400,
	legacyimport.CodeUploadTooLarge:     413,
	legacyimport.CodeUploadOrder:        409,
	legacyimport.CodeUploadBusy:         409,
	legacyimport.CodeUploadLimit:        409,
	legacyimport.CodeBusy:               409,
	legacyimport.CodeBatchChanged:       409,
	legacyimport.CodeNotClean:           409,
	legacyimport.CodeNotPackage:         400,
	legacyimport.CodeUnsupportedVersion: 400,
	legacyimport.CodeDamaged:            400,
}

func legacyFault(err error) error {
	var problem *legacyimport.Error
	if !errors.As(err, &problem) {
		return err
	}
	status, found := legacyStatusCodes[problem.Code]
	if !found {
		status = 503
	}
	fault := &Fault{Status: status, Code: problem.Code, Message: problem.Message}
	if len(problem.Blockers) > 0 {
		fault.Details = map[string]any{"blockers": problem.Blockers}
	}
	return fault
}

func legacyUnavailable() error {
	return &Fault{Status: 503, Code: "LEGACY_UNAVAILABLE", Message: "La migración del sistema anterior no está disponible en este servidor."}
}

func legacyActor(principal Principal) legacyimport.Actor {
	return legacyimport.Actor{UserID: principal.UserID, Username: principal.Username, DeviceID: principal.DeviceID}
}

func (service *Service) legacyRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if err := principal.require("settings.manage"); err != nil {
		return nil, err
	}
	if !request.Local {
		return nil, &Fault{Status: 403, Code: "LOCAL_REQUIRED",
			Message: "La migración del sistema anterior solo se hace desde la computadora principal."}
	}
	manager := service.Legacy
	if manager == nil {
		return nil, legacyUnavailable()
	}
	result, err := service.legacyRoute(ctx, tx, request, principal, route, manager)
	return result, legacyFault(err)
}

func (service *Service) legacyRoute(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string,
	manager *legacyimport.Manager) (any, error) {
	actor := legacyActor(principal)
	parts := strings.Split(route, "/")
	switch {
	case request.Method == "GET" && route == "maintenance/legacy":
		return manager.Status(ctx, tx, actor)
	case request.Method == "POST" && route == "maintenance/legacy/uploads":
		return createLegacyUpload(ctx, tx, request, principal, manager)
	case request.Method == "POST" && route == "maintenance/legacy/analyze":
		return startLegacyAnalysis(ctx, tx, request, actor, manager)
	case request.Method == "POST" && route == "maintenance/legacy/import":
		return startLegacyImport(ctx, tx, request, actor, manager)
	case request.Method == "POST" && route == "maintenance/legacy/preparations":
		return recordLegacyPreparation(ctx, tx, request, principal)
	}
	if len(parts) < 4 || !identifierPattern.MatchString(parts[3]) {
		return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
	}
	id := strings.ToLower(parts[3])
	switch {
	case parts[2] == "uploads" && len(parts) == 4 && request.Method == "GET":
		return manager.GetUpload(actor, id)
	case parts[2] == "uploads" && len(parts) == 5 && parts[4] == "parts" && request.Method == "POST":
		return addLegacyPart(ctx, tx, request, principal, manager, id)
	case parts[2] == "uploads" && len(parts) == 5 && parts[4] == "cancel" && request.Method == "POST":
		if err := manager.CancelUpload(actor, id); err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "legacy.upload_cancelled", "legacy_import", &id, nil, nil, true); err != nil {
			return nil, err
		}
		return map[string]bool{"cancelled": true}, nil
	case parts[2] == "runs" && len(parts) == 4 && request.Method == "GET":
		run, err := legacyimport.FindRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if run == nil {
			return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "La corrida no existe."}
		}
		return run, nil
	case parts[2] == "batches" && len(parts) == 4 && request.Method == "GET":
		return legacyimport.BatchDetail(ctx, tx, id)
	case parts[2] == "batches" && len(parts) == 5 && parts[4] == "report.pdf" && request.Method == "GET":
		file, err := legacyimport.RenderReport(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "legacy.report_downloaded", "legacy_import", &id, nil,
			map[string]any{"bytes": len(file.Content)}, true); err != nil {
			return nil, err
		}
		return file, nil
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

func createLegacyUpload(ctx context.Context, tx pgx.Tx, request Request, principal Principal, manager *legacyimport.Manager) (any, error) {
	var input struct {
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	view, err := manager.CreateUpload(legacyActor(principal), input.Size, input.SHA256)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, principal, "legacy.upload_created", "legacy_import", &view.ID, nil,
		map[string]any{"size": view.Size, "sha256": view.SHA256}, true); err != nil {
		return nil, err
	}
	return view, nil
}

func addLegacyPart(ctx context.Context, tx pgx.Tx, request Request, principal Principal, manager *legacyimport.Manager, id string) (any, error) {
	var input struct {
		Index  *int   `json:"index"`
		Data   []byte `json:"data"`
		SHA256 string `json:"sha256"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Index == nil || *input.Index < 0 {
		return nil, invalid("Indica el número de la parte.")
	}
	before, err := manager.GetUpload(legacyActor(principal), id)
	if err != nil {
		return nil, err
	}
	view, err := manager.AddPart(legacyActor(principal), id, *input.Index, input.Data, input.SHA256)
	if err != nil {
		return nil, err
	}
	if before.Status != legacyimport.UploadComplete && view.Status == legacyimport.UploadComplete {
		if err := audit(ctx, tx, principal, "legacy.upload_completed", "legacy_import", &view.ID, nil,
			map[string]any{"size": view.Size, "sha256": view.SHA256, "parts": view.Parts}, true); err != nil {
			return nil, err
		}
	}
	return view, nil
}

func startLegacyAnalysis(ctx context.Context, tx pgx.Tx, request Request, actor legacyimport.Actor, manager *legacyimport.Manager) (any, error) {
	var input struct {
		UploadID   string `json:"upload_id"`
		Passphrase string `json:"passphrase"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	pending, err := manager.StartAnalysis(ctx, tx, actor, input.UploadID, input.Passphrase)
	if err != nil {
		return nil, err
	}
	return &legacyStart{pending: pending}, nil
}

func startLegacyImport(ctx context.Context, tx pgx.Tx, request Request, actor legacyimport.Actor, manager *legacyimport.Manager) (any, error) {
	var input struct {
		UploadID       string  `json:"upload_id"`
		Passphrase     string  `json:"passphrase"`
		AnalysisRunID  string  `json:"analysis_run_id"`
		Confirm        bool    `json:"confirm"`
		ReplaceBatchID *string `json:"replace_batch_id"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !identifierPattern.MatchString(input.AnalysisRunID) {
		return nil, invalid("Analiza el paquete y revisa el resumen antes de importarlo.")
	}
	replace := ""
	if input.ReplaceBatchID != nil {
		if !identifierPattern.MatchString(*input.ReplaceBatchID) {
			return nil, invalid("El lote que se reemplaza no es válido.")
		}
		replace = strings.ToLower(*input.ReplaceBatchID)
	}
	pending, err := manager.StartImport(ctx, tx, actor, legacyimport.ImportInput{
		UploadID: input.UploadID, Passphrase: input.Passphrase, AnalysisRunID: strings.ToLower(input.AnalysisRunID),
		Confirm: input.Confirm, ReplaceBatchID: replace,
	})
	if err != nil {
		return nil, err
	}
	return &legacyStart{pending: pending}, nil
}

// legacyPreparation es el resumen de una preparación del paquete (legacy-prepare, Fase D): solo fechas, conteos,
// tiempos y tamaño; nunca datos del restaurante ni rutas de archivos.
type legacyPreparation struct {
	Succeeded     bool             `json:"succeeded"`
	ErrorCode     string           `json:"error_code"`
	BackupTakenAt string           `json:"backup_taken_at"`
	SalesFirst    string           `json:"sales_first"`
	SalesLast     string           `json:"sales_last"`
	PackageBytes  int64            `json:"package_bytes"`
	FormatVersion int              `json:"format_version"`
	Fingerprint   string           `json:"fingerprint"`
	Tables        map[string]int64 `json:"tables"`
	TimingsMS     map[string]int64 `json:"timings_ms"`
}

var (
	preparationText = regexp.MustCompile(`^[A-Za-z0-9_:.+ -]{0,40}$`)
	preparationKey  = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	fingerprintText = regexp.MustCompile(`^([0-9a-f]{64})?$`)
)

func recordLegacyPreparation(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input legacyPreparation
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	valid := preparationText.MatchString(input.ErrorCode) && preparationText.MatchString(input.BackupTakenAt) &&
		preparationText.MatchString(input.SalesFirst) && preparationText.MatchString(input.SalesLast) &&
		fingerprintText.MatchString(input.Fingerprint) && input.PackageBytes >= 0 && input.FormatVersion >= 0 &&
		len(input.Tables) <= 200 && len(input.TimingsMS) <= 50
	for key, value := range input.Tables {
		valid = valid && preparationKey.MatchString(key) && value >= 0
	}
	for key, value := range input.TimingsMS {
		valid = valid && preparationKey.MatchString(key) && value >= 0
	}
	if !valid {
		return nil, invalid("El resumen de la preparación no es válido.")
	}
	if err := audit(ctx, tx, principal, "legacy.preparation_recorded", "legacy_import", nil, nil, input, input.Succeeded); err != nil {
		return nil, err
	}
	return map[string]bool{"recorded": true}, nil
}

// completeLegacyStart entrega la corrida al trabajador después de confirmar la petición y responde con ella.
func (service *Service) completeLegacyStart(start *legacyStart) (any, error) {
	if service.Legacy == nil {
		return nil, legacyUnavailable()
	}
	service.Legacy.Dispatch(start.pending)
	return start.pending.Run, nil
}

// writeLegacyFile envía un archivo de la migración (el PDF del informe). Devuelve false si no es un archivo.
func writeLegacyFile(writer http.ResponseWriter, result any) bool {
	file, ok := result.(*legacyimport.File)
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

// legacyBackupWait es lo más que la importación espera a que termine otro respaldo en curso antes de hacer el suyo.
const legacyBackupWait = 5 * time.Minute

// EnableLegacyMigration arma la migración del sistema anterior con la auditoría y el respaldo previo de este
// servicio; workDir es la carpeta de trabajo privada dentro de la carpeta de datos (la de la configuración).
func (service *Service) EnableLegacyMigration(workDir string) {
	service.Legacy = &legacyimport.Manager{
		Pool:    service.Pool,
		WorkDir: workDir,
		Logger:  service.Logger,
		Audit: func(ctx context.Context, tx pgx.Tx, actor legacyimport.Actor, action, entityID string, details any, success bool) error {
			principal := Principal{UserID: actor.UserID, Username: actor.Username, DeviceID: actor.DeviceID}
			var id *string
			if entityID != "" {
				id = &entityID
			}
			return audit(ctx, tx, principal, action, "legacy_import", id, nil, details, success)
		},
		Backup: service.legacyBackup,
	}
}

// legacyBackup hace el respaldo verificado previo a una importación por el mismo camino de «Respaldar ahora»
// (backupschedule: el lock 7820148309 se toma solo, en su propia conexión y sin transacción abierta). Si hay otro
// respaldo en curso, espera a que termine (hasta 5 minutos). Queda en la lista de respaldos y en el Registro de
// movimientos como cualquier respaldo manual. Si falla, la importación no se hace.
func (service *Service) legacyBackup(ctx context.Context, actor legacyimport.Actor, runID string) (legacyimport.BackupResult, error) {
	runner := service.ScheduledBackups
	if runner == nil {
		return legacyimport.BackupResult{}, errors.New("los respaldos no están disponibles en este servidor")
	}
	principal := Principal{UserID: actor.UserID, Username: actor.Username, DeviceID: actor.DeviceID}
	deadline := time.Now().Add(legacyBackupWait)
	for {
		started, err := runner.StartManual(ctx, backupschedule.ManualRequest{
			OperationID: runID, UserID: actor.UserID, DeviceID: actor.DeviceID,
		}, func(run backupschedule.Run) {
			service.auditManualBackup(principal, run)
		})
		if errors.Is(err, backupschedule.ErrBusy) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return legacyimport.BackupResult{}, ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if err != nil {
			return legacyimport.BackupResult{}, err
		}
		final := started.Run
		if !started.Replayed {
			select {
			case final = <-started.Done:
			case <-ctx.Done():
				return legacyimport.BackupResult{}, ctx.Err()
			}
		}
		if final.Status != backupschedule.StatusSucceeded || !final.Verified {
			return legacyimport.BackupResult{}, errors.New("el respaldo previo falló: " + final.FailureReason)
		}
		return legacyimport.BackupResult{RunID: final.ID, FileName: final.FileName}, nil
	}
}
