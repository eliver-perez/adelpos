package administration

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/backupschedule"
)

// Respaldos programados (etapa 9G, fase F; §7 de la especificación), en Sistema › Mantenimiento con settings.manage
// (el permiso de Mantenimiento: quien puede cambiar la configuración general es quien puede corregir la carpeta o la
// hora de los respaldos, y por eso también es quien recibe el aviso de un respaldo fallido):
//
//   - GET maintenance/backups: configuración, próximo respaldo, último respaldo, aviso vigente y los últimos
//     respaldos con fecha, tamaño, verificación y carpeta.
//   - GET maintenance/backups/status: último respaldo y aviso (pantalla de inicio y estado del sistema).
//   - PUT maintenance/backups/settings {enabled, time, keep}: activo, hora (null = la «Hora de inicio del día») y
//     cuántos respaldos automáticos conservar. Auditada.
//   - PUT maintenance/backups/directory {directory}: la carpeta que devolvió el diálogo nativo de la computadora
//     principal. Solo desde una conexión local; Go valida que sea una ruta absoluta, existente y con permiso de
//     escritura para el servidor. Auditada. No está en la lista cerrada de operaciones de la interfaz: solo la llama
//     Rust (backup_folder.rs) con la ruta que eligió el administrador en el diálogo.
//   - POST maintenance/backups/directory/default: vuelve a la carpeta predeterminada. Auditada.
//   - POST maintenance/backups/run {operation_id}: «Respaldar ahora» por el mismo camino del automático. Una clave
//     repetida devuelve la misma ejecución (en curso o terminada) sin respaldar otra vez; con otro usuario o terminal,
//     409 IDEMPOTENCY_CONFLICT. Se audita al terminar, correcto o no.
//
// Estas rutas no son de la API de clientes (Android): no van en el contrato OpenAPI de internal/apicontract.

// backupRunMargin es lo que se reserva del plazo de la petición para responder: si el respaldo tarda más, la
// respuesta dice que sigue en curso y la pantalla lo consulta después.
const backupRunMargin = 5 * time.Second

// backupNow es «Respaldar ahora» autorizado, que se completa después de confirmar la autenticación.
type backupNow struct {
	principal   Principal
	operationID string
}

type backupRunResponse struct {
	Run backupschedule.RunView `json:"run"`
}

func backupsUnavailable() error {
	return &Fault{
		Status:  503,
		Code:    "BACKUPS_UNAVAILABLE",
		Message: "Los respaldos programados no están disponibles en este servidor.",
	}
}

func (service *Service) backupRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if err := principal.require("settings.manage"); err != nil {
		return nil, err
	}
	runner := service.ScheduledBackups
	if runner == nil {
		return nil, backupsUnavailable()
	}
	switch request.Method + " " + route {
	case "GET maintenance/backups":
		return runner.State(ctx, tx, request.Local)
	case "GET maintenance/backups/status":
		return backupschedule.SummaryOf(ctx, tx)
	case "PUT maintenance/backups/settings":
		return saveBackupSettings(ctx, tx, request, principal, runner)
	case "PUT maintenance/backups/directory":
		return saveBackupDirectory(ctx, tx, request, principal, runner)
	case "POST maintenance/backups/directory/default":
		return resetBackupDirectory(ctx, tx, request, principal, runner)
	case "POST maintenance/backups/run":
		return prepareBackupNow(ctx, tx, request, principal)
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

// backupSettingsAudit es lo que queda en el Registro de movimientos de un cambio de configuración.
type backupSettingsAudit struct {
	Enabled bool    `json:"enabled"`
	Time    *string `json:"time"`
	Keep    int     `json:"keep"`
}

func settingsAudit(settings backupschedule.Settings) backupSettingsAudit {
	audit := backupSettingsAudit{Enabled: settings.Enabled, Keep: settings.Keep}
	if settings.Time != "" {
		value := settings.Time
		audit.Time = &value
	}
	return audit
}

func saveBackupSettings(ctx context.Context, tx pgx.Tx, request Request, principal Principal, runner *backupschedule.Runner) (any, error) {
	var input struct {
		Enabled *bool   `json:"enabled"`
		Time    *string `json:"time"`
		Keep    *int    `json:"keep"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Enabled == nil || input.Keep == nil {
		return nil, invalid("Indica si los respaldos automáticos están activos y cuántos conservar.")
	}
	next := backupschedule.Settings{Enabled: *input.Enabled, Keep: *input.Keep}
	if input.Time != nil {
		next.Time = *input.Time
		if next.Time == "" || !backupschedule.ValidTime(next.Time) {
			return nil, invalid("Elige una hora válida para el respaldo automático.")
		}
	}
	if next.Keep < backupschedule.MinKeep || next.Keep > backupschedule.MaxKeep {
		return nil, invalid("Se pueden conservar de 1 a 365 respaldos automáticos.")
	}
	// Una sola configuración: el lock de la fila evita que dos administradores se pisen el registro.
	current, err := lockedBackupSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	next.Directory = current.Directory
	if next.Enabled != current.Enabled || next.Time != current.Time || next.Keep != current.Keep {
		if err := backupschedule.SaveSchedule(ctx, tx, next, principal.UserID); err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "maintenance.backup_settings_changed", "maintenance", nil,
			settingsAudit(current), settingsAudit(next), true); err != nil {
			return nil, err
		}
	}
	return runner.State(ctx, tx, request.Local)
}

// lockedBackupSettings lee la configuración con la fila bloqueada hasta el final de la transacción.
func lockedBackupSettings(ctx context.Context, tx pgx.Tx) (backupschedule.Settings, error) {
	if _, err := tx.Exec(ctx, "SELECT 1 FROM app.backup_settings WHERE singleton FOR UPDATE"); err != nil {
		return backupschedule.Settings{}, err
	}
	return backupschedule.LoadSettings(ctx, tx)
}

// requireLocal exige que la petición venga de la propia computadora principal (como el asistente inicial).
func requireLocal(request Request) error {
	if !request.Local {
		return &Fault{
			Status:  403,
			Code:    "LOCAL_REQUIRED",
			Message: "La carpeta de los respaldos solo se puede cambiar desde la computadora principal.",
		}
	}
	return nil
}

// directoryMessages explica en palabras del restaurante por qué no se acepta la carpeta.
var directoryMessages = map[string]string{
	backupschedule.ReasonMissing:      "No se encontró esa carpeta. Si está en un disco o una USB, conéctalo y vuelve a elegirla.",
	backupschedule.ReasonNotDirectory: "Elige una carpeta, no un archivo.",
	backupschedule.ReasonNotWritable:  "El servidor no puede guardar archivos en esa carpeta. Elige otra o revisa sus permisos.",
	backupschedule.ReasonRelative:     "Elige una carpeta de esta computadora.",
	backupschedule.ReasonPostgresData: "Esa carpeta guarda la base de datos; elige otra para los respaldos.",
	backupschedule.ReasonInvalid:      "Elige una carpeta de esta computadora.",
}

type backupDirectoryAudit struct {
	Directory *string `json:"directory"`
}

func directoryAudit(directory string) backupDirectoryAudit {
	if directory == "" {
		return backupDirectoryAudit{}
	}
	return backupDirectoryAudit{Directory: &directory}
}

func saveBackupDirectory(ctx context.Context, tx pgx.Tx, request Request, principal Principal, runner *backupschedule.Runner) (any, error) {
	if err := requireLocal(request); err != nil {
		return nil, err
	}
	var input struct {
		Directory string `json:"directory"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	// Elegir la carpeta predeterminada es lo mismo que volver a ella (aunque todavía no exista: se crea con el primer
	// respaldo).
	if filepath.IsAbs(input.Directory) && backupschedule.SameDirectory(input.Directory, runner.Target.DefaultDirectory()) {
		return changeBackupDirectory(ctx, tx, request, principal, runner, "")
	}
	directory, err := backupschedule.ValidateDirectory(input.Directory)
	var problem *backupschedule.DirectoryError
	if errors.As(err, &problem) {
		return nil, &Fault{Status: 400, Code: "BACKUP_DIRECTORY_INVALID", Message: directoryMessages[problem.Reason]}
	}
	if err != nil {
		return nil, err
	}
	return changeBackupDirectory(ctx, tx, request, principal, runner, directory)
}

func resetBackupDirectory(ctx context.Context, tx pgx.Tx, request Request, principal Principal, runner *backupschedule.Runner) (any, error) {
	if len(request.Body) > 0 {
		var input struct{}
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
	}
	return changeBackupDirectory(ctx, tx, request, principal, runner, "")
}

func changeBackupDirectory(ctx context.Context, tx pgx.Tx, request Request, principal Principal, runner *backupschedule.Runner, directory string) (any, error) {
	current, err := lockedBackupSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	if current.Directory != directory {
		if err := backupschedule.SaveDirectory(ctx, tx, directory, principal.UserID); err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "maintenance.backup_directory_changed", "maintenance", nil,
			directoryAudit(current.Directory), directoryAudit(directory), true); err != nil {
			return nil, err
		}
	}
	return runner.State(ctx, tx, request.Local)
}

func prepareBackupNow(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !identifierPattern.MatchString(input.OperationID) {
		return nil, invalid("La operación requiere un operation_id válido.")
	}
	operationID := strings.ToLower(input.OperationID)
	// Un reenvío de la misma clave devuelve la ejecución registrada sin respaldar otra vez.
	existing, err := backupschedule.FindOperation(ctx, tx, operationID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Kind != backupschedule.KindManual || existing.RequestedBy != principal.UserID ||
			existing.RequestedDevice != principal.DeviceID {
			return nil, backupConflict()
		}
		return backupRunResponse{Run: backupschedule.View(*existing, true)}, nil
	}
	return &backupNow{principal: principal, operationID: operationID}, nil
}

func backupConflict() error {
	return &Fault{
		Status:  409,
		Code:    "IDEMPOTENCY_CONFLICT",
		Message: "Esta operación ya se había enviado con otros datos y no se repitió. Consulta el resultado antes de intentarlo de nuevo.",
	}
}

// runBackupNow empieza el respaldo fuera de la transacción de autenticación y espera su resultado mientras dure el
// plazo de la petición; si tarda más, responde con la ejecución en curso (la pantalla la consulta después).
func (service *Service) runBackupNow(ctx context.Context, pending *backupNow) (any, error) {
	runner := service.ScheduledBackups
	if runner == nil {
		return nil, backupsUnavailable()
	}
	started, err := runner.StartManual(ctx, backupschedule.ManualRequest{
		OperationID: pending.operationID,
		UserID:      pending.principal.UserID,
		DeviceID:    pending.principal.DeviceID,
	}, func(run backupschedule.Run) {
		service.auditManualBackup(pending.principal, run)
	})
	if errors.Is(err, backupschedule.ErrBusy) {
		return nil, &Fault{
			Status:  409,
			Code:    "BACKUP_IN_PROGRESS",
			Message: "Ya se está creando un respaldo. Espera a que termine y revisa la lista de respaldos.",
		}
	}
	if errors.Is(err, backupschedule.ErrOperationConflict) {
		return nil, backupConflict()
	}
	if err != nil {
		return nil, err
	}
	if started.Replayed {
		return backupRunResponse{Run: backupschedule.View(started.Run, true)}, nil
	}
	wait := 2 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		wait = time.Until(deadline) - backupRunMargin
	}
	if wait < time.Second {
		wait = time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case final := <-started.Done:
		return backupRunResponse{Run: backupschedule.View(final, true)}, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	return backupRunResponse{Run: backupschedule.View(started.Run, false)}, nil
}

// manualBackupAudit es lo que queda en el Registro de movimientos de «Respaldar ahora».
type manualBackupAudit struct {
	FileName       string `json:"file_name,omitempty"`
	SizeBytes      int64  `json:"size_bytes,omitempty"`
	Directory      string `json:"directory,omitempty"`
	UsedDefault    bool   `json:"used_default_directory"`
	FallbackReason string `json:"fallback_reason,omitempty"`
	Verified       bool   `json:"verified"`
	FailureReason  string `json:"failure_reason,omitempty"`
}

// auditManualBackup registra el resultado de «Respaldar ahora» en una transacción corta propia (el respaldo terminó
// fuera de la petición). Si no se puede registrar, queda en la bitácora.
func (service *Service) auditManualBackup(principal Principal, run backupschedule.Run) {
	ctx, cancel := context.WithTimeout(context.Background(), removalAuditTimeout)
	defer cancel()
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		service.logWarning("backup_audit_failed", "reason", err.Error())
		return
	}
	defer tx.Rollback(context.Background())
	details := manualBackupAudit{
		FileName:       run.FileName,
		SizeBytes:      run.SizeBytes,
		Directory:      run.Directory,
		UsedDefault:    run.UsedDefault,
		FallbackReason: run.FallbackReason,
		Verified:       run.Verified,
		FailureReason:  run.FailureReason,
	}
	id := run.ID
	success := run.Status == backupschedule.StatusSucceeded
	if err := audit(ctx, tx, principal, "maintenance.backup_created", "backup", &id, nil, details, success); err != nil {
		service.logWarning("backup_audit_failed", "reason", err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		service.logWarning("backup_audit_failed", "reason", err.Error())
	}
}
