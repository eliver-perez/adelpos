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

// removalAuditTimeout es el plazo propio para auditar un intento fallido después del plazo de la petición.
const removalAuditTimeout = 10 * time.Second

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
//
// Revisión independiente de la 9F (B2): una sola eliminación a la vez en este servidor, de modo que el
// reenvío de la misma operación (una terminal que perdió la conexión mientras la primera respaldaba)
// espera a que la primera termine y, antes de comparar el plan y de respaldar, encuentra su respuesta
// guardada: no crea otro respaldo ni responde TEST_DATA_CHANGED («no se quitó nada») cuando sí se quitó.
func (service *Service) removeTestData(ctx context.Context, pending *testDataRemoval) (any, error) {
	release, err := service.waitRemovalTurn(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	current, stored, err := service.currentTestDataPlan(ctx, pending)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return stored, nil
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
	// Otro servidor sobre la misma base (no es una instalación prevista) no comparte el turno: la
	// respuesta guardada se vuelve a buscar con los locks exclusivos.
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

// waitRemovalTurn espera el turno de «Quitar datos de prueba» en este servidor (o a que venza el plazo de
// la petición) y devuelve la función que lo libera.
func (service *Service) waitRemovalTurn(ctx context.Context) (func(), error) {
	service.removalOnce.Do(func() {
		service.removalTurn = make(chan struct{}, 1)
	})
	select {
	case service.removalTurn <- struct{}{}:
		return func() {
			<-service.removalTurn
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// currentTestDataPlan calcula el plan vigente en una transacción corta de solo lectura. Antes, con los
// locks compartidos (que esperan a una eliminación en curso), busca la respuesta guardada de la misma
// operación: si existe, la devuelve (stored) con el plan nulo, sin calcularlo.
func (service *Service) currentTestDataPlan(
	ctx context.Context,
	pending *testDataRemoval,
) (*maintenance.Plan, json.RawMessage, error) {
	tx, err := service.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(context.Background())
	if err := maintenanceLocks(ctx, tx, true); err != nil {
		return nil, nil, err
	}
	stored, found, err := replayRemoval(ctx, tx, pending)
	if err != nil {
		return nil, nil, err
	}
	if found {
		return nil, stored, nil
	}
	plan, err := maintenance.Build(ctx, tx)
	return plan, nil, err
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

// auditRemovalFailure registra el intento fallido. Usa un contexto sin la cancelación de la petición
// (con un plazo corto propio): si el respaldo agotó el plazo de 120 s, el intento queda registrado igual
// (revisión independiente de la 9F, B2). Si ni así se puede registrar, queda en la bitácora.
func (service *Service) auditRemovalFailure(ctx context.Context, principal Principal, failure *Fault) error {
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), removalAuditTimeout)
	defer cancel()
	tx, err := service.Pool.Begin(auditContext)
	if err != nil {
		service.logWarning("test_data_failure_audit_failed", "reason", err.Error())
		return failure
	}
	defer tx.Rollback(context.Background())
	details := map[string]string{"backup": "failed"}
	err = audit(auditContext, tx, principal, "maintenance.test_data_removed", "maintenance", nil, nil, details, false)
	if err != nil {
		service.logWarning("test_data_failure_audit_failed", "reason", err.Error())
		return failure
	}
	if err := tx.Commit(auditContext); err != nil {
		service.logWarning("test_data_failure_audit_failed", "reason", err.Error())
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
