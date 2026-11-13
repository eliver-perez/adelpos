package administration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/maintenance"
)

// «Quitar datos de prueba» (etapa 9F, §3; decisiones D1 y D2 del coordinador), en el Mantenimiento del
// administrador (settings.manage):
//
//   - GET maintenance/test-data: la vista previa (lo que se elimina, se desactiva, se desvincula y se
//     conserva, con el motivo), calculada con locks compartidos.
//   - POST maintenance/test-data/remove {operation_id, plan_id}: la transacción de autenticación solo
//     autoriza, valida y comprueba la idempotencia; después, fuera de ella, se crea el respaldo y solo si
//     quedó bien se aplica el plan en otra transacción con los locks exclusivos, auditoría y eventos. El
//     plan_id debe ser el que se revisó: si los datos cambiaron, no se quita nada.

// maintenanceTimeout es el tiempo de respuesta de maintenance/*: el respaldo completo de la base más la
// transacción que quita los datos. El puente nativo espera un poco más (api.rs).
const maintenanceTimeout = 120 * time.Second

const maintenanceRemoveRoute = "maintenance/test-data/remove"

var planIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// testDataRemoval es la operación autorizada que se completa después de confirmar la autenticación.
type testDataRemoval struct {
	principal   Principal
	operationID string
	planID      string
	fingerprint string
}

func (service *Service) maintenanceRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if err := principal.require("settings.manage"); err != nil {
		return nil, err
	}
	switch request.Method + " " + route {
	case "GET maintenance/test-data":
		if err := maintenanceLocks(ctx, tx, true); err != nil {
			return nil, err
		}
		plan, err := maintenance.Build(ctx, tx)
		if err != nil {
			return nil, err
		}
		return plan.Preview, nil
	case "POST " + maintenanceRemoveRoute:
		return prepareTestDataRemoval(ctx, tx, request, principal)
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

// maintenanceLocks toma los locks de la operación (7820148305), el catálogo (7820148304) y el inventario
// (7820148308), en ese orden: compartidos para leer y exclusivos para quitar.
func maintenanceLocks(ctx context.Context, tx pgx.Tx, shared bool) error {
	function := "pg_advisory_xact_lock"
	if shared {
		function = "pg_advisory_xact_lock_shared"
	}
	for _, key := range []string{"7820148305", "7820148304", "7820148308"} {
		if _, err := tx.Exec(ctx, "SELECT "+function+"("+key+")"); err != nil {
			return err
		}
	}
	return nil
}

func prepareTestDataRemoval(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		PlanID      string `json:"plan_id"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if !identifierPattern.MatchString(input.OperationID) {
		return nil, invalid("La operación requiere un operation_id válido.")
	}
	if !planIDPattern.MatchString(input.PlanID) {
		return nil, invalid("Revisa primero los datos de prueba.")
	}
	pending := &testDataRemoval{
		principal:   principal,
		operationID: strings.ToLower(input.OperationID),
		planID:      input.PlanID,
		fingerprint: removalFingerprint(input.PlanID),
	}
	stored, found, err := replayRemoval(ctx, tx, pending)
	if err != nil || found {
		return stored, err
	}
	return pending, nil
}

// removalFingerprint identifica el contenido de la operación: la ruta y el plan revisado.
func removalFingerprint(planID string) string {
	digest := sha256.Sum256([]byte("POST " + maintenanceRemoveRoute + "\n" + planID))
	return hex.EncodeToString(digest[:])
}

// replayRemoval devuelve la respuesta guardada de la misma operación (mismo actor, terminal y plan) sin
// repetir el respaldo; con otro plan responde 409 IDEMPOTENCY_CONFLICT.
func replayRemoval(ctx context.Context, tx pgx.Tx, pending *testDataRemoval) (json.RawMessage, bool, error) {
	var actorID, deviceID, fingerprint string
	var response json.RawMessage
	err := tx.QueryRow(ctx, `SELECT actor_id::text, device_id::text, fingerprint, response
        FROM app.restaurant_operations WHERE id = $1`, pending.operationID).Scan(&actorID, &deviceID, &fingerprint, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != pending.principal.UserID || deviceID != pending.principal.DeviceID || fingerprint != pending.fingerprint {
		return nil, false, &Fault{
			Status:  409,
			Code:    "IDEMPOTENCY_CONFLICT",
			Message: "Esta operación ya se había enviado con otros datos y no se repitió. Consulta el resultado antes de intentarlo de nuevo.",
		}
	}
	return response, true, nil
}

func testDataChanged() error {
	return &Fault{
		Status:  409,
		Code:    "TEST_DATA_CHANGED",
		Message: "Los datos de prueba cambiaron desde que los revisaste; no se quitó nada. Revísalos de nuevo.",
	}
}

// removeTestData completa la operación fuera de la transacción de autenticación: plan vigente, respaldo
// previo y, solo si el respaldo quedó bien, la transacción que quita los datos.
func (service *Service) removeTestData(ctx context.Context, pending *testDataRemoval) (any, error) {
	current, err := service.currentTestDataPlan(ctx)
	if err != nil {
		return nil, err
	}
	if current.PlanID != pending.planID {
		return nil, testDataChanged()
	}
	backup := ""
	if !current.Empty() {
		backup, err = service.backupBeforeRemoval(ctx, pending.principal)
		if err != nil {
			return nil, err
		}
	}
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if err := maintenanceLocks(ctx, tx, false); err != nil {
		return nil, err
	}
	// Una repetición concurrente de la misma operación espera el lock y recibe la respuesta guardada.
	stored, found, err := replayRemoval(ctx, tx, pending)
	if err != nil || found {
		return stored, err
	}
	plan, err := maintenance.Build(ctx, tx)
	if err != nil {
		return nil, err
	}
	if plan.PlanID != pending.planID {
		return nil, testDataChanged()
	}
	result, err := maintenance.Apply(ctx, tx, plan, backup)
	if err != nil {
		return nil, err
	}
	if !plan.Empty() {
		if err := audit(ctx, tx, pending.principal, "maintenance.test_data_removed", "maintenance", nil, nil, result, true); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id, actor_id, device_id, route, fingerprint, response)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		pending.operationID,
		pending.principal.UserID,
		pending.principal.DeviceID,
		"POST "+maintenanceRemoveRoute,
		pending.fingerprint,
		encoded,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

// currentTestDataPlan calcula el plan vigente en una transacción corta de solo lectura.
func (service *Service) currentTestDataPlan(ctx context.Context) (*maintenance.Plan, error) {
	tx, err := service.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if err := maintenanceLocks(ctx, tx, true); err != nil {
		return nil, err
	}
	return maintenance.Build(ctx, tx)
}

// backupBeforeRemoval crea el respaldo previo (D2). Si no se puede, audita el intento fallido en una
// transacción corta y responde sin haber quitado nada; el detalle técnico solo va a la bitácora.
func (service *Service) backupBeforeRemoval(ctx context.Context, principal Principal) (string, error) {
	failure := &Fault{
		Status:  503,
		Code:    "BACKUP_FAILED",
		Message: "No se pudo crear el respaldo de la base de datos; no se quitó ningún dato de prueba.",
	}
	if service.Backups == nil {
		service.logWarning("test_data_backup_unavailable")
		return "", service.auditRemovalFailure(ctx, principal, failure)
	}
	name, err := service.Backups.Backup(ctx)
	if err != nil {
		service.logWarning("test_data_backup_failed", "reason", err.Error())
		return "", service.auditRemovalFailure(ctx, principal, failure)
	}
	return name, nil
}

func (service *Service) auditRemovalFailure(ctx context.Context, principal Principal, failure *Fault) error {
	tx, err := service.Pool.Begin(ctx)
	if err != nil {
		return failure
	}
	defer tx.Rollback(context.Background())
	details := map[string]string{"backup": "failed"}
	if err := audit(ctx, tx, principal, "maintenance.test_data_removed", "maintenance", nil, nil, details, false); err != nil {
		return failure
	}
	if err := tx.Commit(ctx); err != nil {
		return failure
	}
	return failure
}

func (service *Service) logWarning(message string, arguments ...any) {
	if service.Logger != nil {
		service.Logger.Warn(message, arguments...)
	}
}

// isMaintenanceRoute indica si la ruta usa el tiempo de respuesta del mantenimiento.
func isMaintenanceRoute(path string) bool {
	return strings.HasPrefix(path, "/api/v1/maintenance/")
}
