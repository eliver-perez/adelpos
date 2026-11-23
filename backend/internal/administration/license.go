package administration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
)

// Rutas de la licencia de la etapa L1, Fase B (§3 de la especificación; contrato §8 a §10). Las de la hora (Fase C:
// license/time-check, license/support-request y license/support-unlock) van en su propio archivo.
//
//   - GET license/status: el estado, con cualquier sesión (ver el estado no pide permiso, L1-D21). Con solo la
//     credencial del dispositivo (Android en «¿Quién eres?»), el resumen: estado, entorno y si es una copia.
//   - Con license.manage («Administrar la licencia», solo el rol Administrador de fábrica):
//   - POST license/activate {license_key}: activar en línea (§8.3). ACTIVATION_LIMIT llega como
//     409 LICENSE_ACTIVATION_LIMIT con details.transfer_available.
//   - POST license/transfer {license_key, confirmed: true}: transferir a esta computadora (§8.4), solo después de
//     ACTIVATION_LIMIT con transfer_available y la confirmación del usuario. TRANSFER_LIMIT: «Comunícate con
//     soporte».
//   - POST license/refresh: «Buscar actualización» (§8.5).
//   - POST license/deactivate {confirmed: true}: «Liberar esta licencia» (§8.6).
//   - POST license/offline-request {action, license_key?}: la solicitud .licreq de activate (código opcional),
//     renew o deactivate (§9.1), como archivo para guardar.
//   - POST license/import {license}: importar un .lic (§9.2), verificado como una respuesta en línea.
//   - POST license/identity/regenerate {confirmed: true}: «Crear una identidad nueva para esta computadora» en una
//     instalación copiada (L1-D11); además, solo en la computadora principal (escucha local).
//
// Las operaciones en línea corren después de confirmar la autenticación (completeDeferred), fuera de la transacción:
// la red nunca retiene una conexión de la base ni un lock. Los textos salen de los códigos (L1-D17), nunca del
// message del servidor de licencias. El código comercial nunca se registra ni se audita.

// licenseManagePermission es «Administrar la licencia» (migración 0028, sección «Fase B»).
const licenseManagePermission = "license.manage"

// licenseTimeout es el plazo de las rutas de la licencia: un desafío y una operación, con sus reintentos.
const licenseTimeout = 75 * time.Second

func isLicenseRoute(path string) bool {
	return strings.HasPrefix(path, "/api/v1/license/")
}

// licenseCoreRoutes son las rutas de la Fase B (routes.go las envía a licenseRoutes).
var licenseCoreRoutes = []string{
	"license/status", "license/activate", "license/transfer", "license/refresh", "license/deactivate",
	"license/offline-request", "license/import", "license/identity/regenerate",
}

func isLicenseCoreRoute(route string) bool {
	return slices.Contains(licenseCoreRoutes, route)
}

// licenseOperation es una operación en línea autorizada, que se completa después de confirmar la autenticación.
type licenseOperation struct {
	principal  Principal
	action     string
	licenseKey string
}

// --- Vistas --------------------------------------------------------------------------------------------------------

// licenseStatusView es la respuesta de GET license/status.
type licenseStatusView struct {
	// State: active, grace, expired, revoked, not_activated o not_activated_with_data (contrato §6).
	State string `json:"state"`
	// Environment: test (servidor de licencias de pruebas) o production.
	Environment string `json:"environment"`
	// Copied: la instalación se copió de otra computadora; MachineUnverified: no se pudo comprobar la computadora;
	// IdentityUnavailable: falta la identidad del servidor o su llave de licencias.
	Copied              bool `json:"copied"`
	MachineUnverified   bool `json:"machine_unverified"`
	IdentityUnavailable bool `json:"identity_unavailable"`
	// RevocationReason: deactivated, transferred o revoked en una licencia revocada.
	RevocationReason *string `json:"revocation_reason"`
	// License: la licencia vigente, o null sin licencia válida.
	License *licenseView `json:"license"`
	// ServerID es el de la identidad de esta computadora (para soporte), o null.
	ServerID *string `json:"server_id"`
	// EffectiveNow es máximo(reloj, última hora confiable) con que se calculó el estado (§5.6).
	EffectiveNow string `json:"effective_now"`
	// CanManage: quien consulta tiene «Administrar la licencia».
	CanManage bool `json:"can_manage"`
	// IdentityRegenerationAvailable: una copia que se puede corregir desde aquí (la computadora principal, con el
	// permiso).
	IdentityRegenerationAvailable bool `json:"identity_regeneration_available"`
}

type licenseView struct {
	LicenseID      string            `json:"license_id"`
	ActivationID   string            `json:"activation_id"`
	LicenseeName   string            `json:"licensee_name"`
	LicenseType    string            `json:"license_type"`
	BillingPeriod  *string           `json:"billing_period"`
	Revision       int64             `json:"license_revision"`
	Plan           licensePlanView   `json:"plan"`
	Limits         licenseLimitsView `json:"limits"`
	Features       map[string]bool   `json:"features"`
	IssuedAt       string            `json:"issued_at"`
	PeriodStartsAt string            `json:"period_starts_at"`
	UpdatesUntil   string            `json:"updates_until"`
	Perpetual      bool              `json:"perpetual"`
	ExpiresAt      *string           `json:"expires_at"`
	GraceDays      int               `json:"grace_days"`
	GraceEndsAt    *string           `json:"grace_ends_at"`
}

type licensePlanView struct {
	PlanID string `json:"plan_id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

type licenseLimitsView struct {
	CashRegisters   int `json:"cash_registers"`
	OrderTakers     int `json:"order_takers"`
	KitchenDisplays int `json:"kitchen_displays"`
}

// licenseDeviceView es el resumen con solo la credencial del dispositivo.
type licenseDeviceView struct {
	State       string `json:"state"`
	Environment string `json:"environment"`
	Copied      bool   `json:"copied"`
}

func formatLicenseTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func optionalLicenseTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := formatLicenseTime(*value)
	return &text
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// licenseStatus es el estado del gestor, o «Sin activar» sin identidad si el servidor no tiene gestor (pruebas).
func (service *Service) licenseStatus() licensing.Status {
	if service.License == nil {
		return licensing.Status{State: licensing.StateNotActivated, Environment: licensing.CompiledEnvironment(), IdentityUnavailable: true, EffectiveNow: time.Now()}
	}
	return service.License.Status()
}

func (service *Service) licenseStatusView(principal Principal, local bool) licenseStatusView {
	status := service.licenseStatus()
	canManage := slices.Contains(principal.Permissions, licenseManagePermission)
	view := licenseStatusView{
		State:                         string(status.State),
		Environment:                   status.Environment,
		Copied:                        status.Copied,
		MachineUnverified:             status.MachineUnverified,
		IdentityUnavailable:           status.IdentityUnavailable,
		RevocationReason:              optionalText(status.RevocationReason),
		EffectiveNow:                  formatLicenseTime(status.EffectiveNow),
		CanManage:                     canManage,
		IdentityRegenerationAvailable: status.Copied && canManage && local,
	}
	if service.License != nil {
		view.ServerID = optionalText(service.License.ServerID())
	}
	if status.HasLicense() {
		view.License = &licenseView{
			LicenseID:      status.LicenseID,
			ActivationID:   status.ActivationID,
			LicenseeName:   status.LicenseeName,
			LicenseType:    status.LicenseType,
			BillingPeriod:  optionalText(status.BillingPeriod),
			Revision:       status.Revision,
			Plan:           licensePlanView{PlanID: status.Plan.PlanID, Name: status.Plan.Name, Custom: status.Plan.Custom},
			Limits:         licenseLimitsView{CashRegisters: status.Limits.CashRegisters, OrderTakers: status.Limits.OrderTakers, KitchenDisplays: status.Limits.KitchenDisplays},
			Features:       status.Features,
			IssuedAt:       formatLicenseTime(status.IssuedAt),
			PeriodStartsAt: formatLicenseTime(status.PeriodStartsAt),
			UpdatesUntil:   formatLicenseTime(status.UpdatesUntil),
			Perpetual:      status.Perpetual,
			ExpiresAt:      optionalLicenseTime(status.ExpiresAt),
			GraceDays:      status.GraceDays,
			GraceEndsAt:    optionalLicenseTime(status.GraceEndsAt),
		}
	}
	return view
}

// licenseDeviceSummary es GET license/status con solo la credencial del dispositivo.
func (service *Service) licenseDeviceSummary() licenseDeviceView {
	status := service.licenseStatus()
	return licenseDeviceView{State: string(status.State), Environment: status.Environment, Copied: status.Copied}
}

// --- Rutas ---------------------------------------------------------------------------------------------------------

func (service *Service) licenseRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if request.Method == "GET" && route == "license/status" {
		return service.licenseStatusView(principal, request.Local), nil
	}
	if request.Method != "POST" || route == "license/status" {
		return nil, &Fault{Status: 405, Code: "METHOD_NOT_ALLOWED", Message: "Operación no disponible."}
	}
	if err := principal.require(licenseManagePermission); err != nil {
		return nil, err
	}
	if service.License == nil {
		return nil, licenseFault(licensing.ErrIdentityUnavailable)
	}
	// Sin identidad, con la instalación copiada o sin huella no se va a la red (ni se firma una solicitud): primero
	// hay que resolver eso (L1-D11). Importar un .lic y la identidad nueva tienen sus propias comprobaciones.
	switch route {
	case "license/activate", "license/transfer", "license/refresh", "license/deactivate", "license/offline-request":
		if err := service.License.SigningProblem(); err != nil {
			return nil, licenseFault(err)
		}
	}
	switch route {
	case "license/activate":
		var input struct {
			LicenseKey string `json:"license_key"`
		}
		if err := decodeLicenseBody(request.Body, &input); err != nil {
			return nil, err
		}
		if _, err := licensing.NormalizeLicenseKey(input.LicenseKey); err != nil {
			return nil, licenseFault(err)
		}
		return &licenseOperation{principal: principal, action: licensing.ActionActivate, licenseKey: input.LicenseKey}, nil
	case "license/transfer":
		var input struct {
			LicenseKey string `json:"license_key"`
			Confirmed  bool   `json:"confirmed"`
		}
		if err := decodeLicenseBody(request.Body, &input); err != nil {
			return nil, err
		}
		if !input.Confirmed {
			return nil, invalid("Confirma que quieres mover la licencia a esta computadora.")
		}
		if _, err := licensing.NormalizeLicenseKey(input.LicenseKey); err != nil {
			return nil, licenseFault(err)
		}
		return &licenseOperation{principal: principal, action: licensing.ActionTransfer, licenseKey: input.LicenseKey}, nil
	case "license/refresh":
		return &licenseOperation{principal: principal, action: licensing.ActionRefresh}, nil
	case "license/deactivate":
		var input struct {
			Confirmed bool `json:"confirmed"`
		}
		if err := decodeLicenseBody(request.Body, &input); err != nil {
			return nil, err
		}
		if !input.Confirmed {
			return nil, invalid("Confirma que quieres liberar la licencia de esta computadora.")
		}
		return &licenseOperation{principal: principal, action: licensing.ActionDeactivate}, nil
	case "license/offline-request":
		return service.licenseOfflineRequest(ctx, tx, request, principal)
	case "license/import":
		return service.licenseImport(ctx, tx, request, principal)
	case "license/identity/regenerate":
		return service.regenerateIdentity(ctx, tx, request, principal)
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

// decodeLicenseBody lee el cuerpo sin campos desconocidos.
func decodeLicenseBody(body json.RawMessage, into any) error {
	if len(body) == 0 {
		return invalid("Faltan los datos de la operación.")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return invalid("Los datos de la operación no son válidos.")
	}
	return nil
}

// completeLicenseOperation hace la operación en línea, ya fuera de la transacción de autenticación, y la audita en
// una transacción corta aparte (correcta o no; nunca con el código comercial).
func (service *Service) completeLicenseOperation(ctx context.Context, operation *licenseOperation) (any, error) {
	if service.LicenseServer == nil {
		return nil, licenseFault(&licensing.TransportError{Reason: licensing.ReasonNotConfigured})
	}
	var status licensing.Status
	var err error
	success, failure := "", ""
	switch operation.action {
	case licensing.ActionActivate:
		status, err = service.LicenseServer.Activate(ctx, operation.licenseKey)
		success, failure = "license.activated", "license.activation_failed"
	case licensing.ActionTransfer:
		status, err = service.LicenseServer.Transfer(ctx, operation.licenseKey)
		success, failure = "license.transferred", "license.transfer_failed"
	case licensing.ActionRefresh:
		status, err = service.LicenseServer.Refresh(ctx)
		success, failure = "license.refreshed", "license.refresh_failed"
	case licensing.ActionDeactivate:
		status, err = service.LicenseServer.Deactivate(ctx)
		success, failure = "license.deactivated", "license.deactivation_failed"
	default:
		return nil, errors.New("operación de licencia desconocida")
	}
	fault := (*Fault)(nil)
	if err != nil {
		fault = licenseFault(err)
	}
	if auditErr := service.auditLicense(ctx, operation.principal, success, failure, status, fault); auditErr != nil && service.Logger != nil {
		service.Logger.Error("license_audit_failed", "reason", auditErr.Error())
	}
	if fault != nil {
		return nil, fault
	}
	return service.licenseStatusView(operation.principal, false), nil
}

// licenseAuditDetails son los datos de la auditoría de una operación de la licencia: nunca el código comercial.
func licenseAuditDetails(status licensing.Status, fault *Fault) map[string]any {
	details := map[string]any{"state": string(status.State)}
	if status.LicenseID != "" {
		details["license_id"] = status.LicenseID
		details["license_revision"] = status.Revision
	}
	if fault != nil {
		details["code"] = fault.Code
	}
	return details
}

// auditLicense registra la operación en una transacción corta aparte.
func (service *Service) auditLicense(ctx context.Context, principal Principal, success, failure string, status licensing.Status, fault *Fault) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	tx, err := service.Pool.Begin(auditCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	action := success
	if fault != nil {
		action = failure
	}
	if err := audit(auditCtx, tx, principal, action, "license", nil, nil, licenseAuditDetails(status, fault), fault == nil); err != nil {
		return err
	}
	return tx.Commit(auditCtx)
}

// licenseOfflineRequest arma la solicitud .licreq (sin red): el archivo vuelve en la respuesta para guardarlo.
func (service *Service) licenseOfflineRequest(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		Action     string `json:"action"`
		LicenseKey string `json:"license_key"`
	}
	if err := decodeLicenseBody(request.Body, &input); err != nil {
		return nil, err
	}
	switch input.Action {
	case licensing.OfflineActivate, licensing.OfflineRenew, licensing.OfflineDeactivate:
	default:
		return nil, invalid("Elige activar, renovar o liberar.")
	}
	if input.Action != licensing.OfflineActivate && strings.TrimSpace(input.LicenseKey) != "" {
		return nil, invalid("El código de la licencia solo va en la solicitud de activación.")
	}
	offline, err := service.License.OfflineRequest(input.Action, input.LicenseKey, product.Read().Version)
	if err != nil {
		return nil, licenseFault(err)
	}
	details := map[string]string{"action": offline.Action, "request_id": offline.RequestID}
	if err := audit(ctx, tx, principal, "license.offline_request_created", "license", nil, nil, details, true); err != nil {
		return nil, err
	}
	return map[string]string{
		"action":     offline.Action,
		"request_id": offline.RequestID,
		"filename":   offline.Filename,
		"content":    offline.Content,
	}, nil
}

// licenseImport importa un .lic (sin red), verificado con la §10.
func (service *Service) licenseImport(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		License string `json:"license"`
	}
	if err := decodeLicenseBody(request.Body, &input); err != nil {
		return nil, err
	}
	status, err := service.License.ImportLicense(input.License)
	if err != nil {
		fault := licenseFault(err)
		// El rechazo se audita aunque la operación no cambie nada (Rejection: se conserva la auditoría).
		fault.Rejection = &RejectionAudit{Action: "license.import_rejected", Entity: "license", Details: licenseAuditDetails(status, fault)}
		return nil, fault
	}
	if err := audit(ctx, tx, principal, "license.imported", "license", nil, nil, licenseAuditDetails(status, nil), true); err != nil {
		return nil, err
	}
	return service.licenseStatusView(principal, request.Local), nil
}

// regenerateIdentity es «Crear una identidad nueva para esta computadora» (L1-D11): solo en una instalación copiada,
// en la computadora principal y con confirmación. Revoca las terminales emparejadas por red (con sus sesiones y
// credenciales de envío), aparta la identidad anterior, crea la nueva (server_id, certificado ECDSA y llave Ed25519)
// y pasa el gestor a ella conservando la última hora confiable (L1-D25). Ningún dato del restaurante cambia. La
// escucha de red sigue con el certificado anterior hasta reiniciar el servidor (restart_required).
func (service *Service) regenerateIdentity(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if !request.Local {
		return nil, &Fault{Status: 403, Code: "LICENSE_IDENTITY_LOCAL_ONLY", Message: "La identidad nueva solo se crea en la computadora principal."}
	}
	var input struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := decodeLicenseBody(request.Body, &input); err != nil {
		return nil, err
	}
	if !input.Confirmed {
		return nil, invalid("Confirma que quieres crear una identidad nueva para esta computadora.")
	}
	status := service.License.Status()
	if status.MachineUnverified {
		return nil, licenseFault(licensing.ErrMachineUnverified)
	}
	if !status.Copied {
		return nil, &Fault{Status: 409, Code: "IDENTITY_REGENERATION_NOT_NEEDED", Message: "Esta instalación no se copió de otra computadora: su identidad no se cambia."}
	}
	previousServerID := service.License.ServerID()
	previousFingerprint := ""
	if service.Identity != nil {
		previousFingerprint = hex.EncodeToString(service.Identity.Fingerprint[:])
	}
	// Las terminales emparejadas por red fijaron el certificado anterior: se revocan, con sus sesiones y sus
	// credenciales de envío. La de la propia computadora (sin huella) sigue igual.
	rows, err := tx.Query(ctx, `UPDATE app.devices SET revoked_at=now()
        WHERE server_fingerprint IS NOT NULL AND revoked_at IS NULL RETURNING id`)
	if err != nil {
		return nil, err
	}
	var revoked []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		revoked = append(revoked, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, deviceID := range revoked {
		if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='identity_regenerated' WHERE device_id=$1 AND revoked_at IS NULL", deviceID); err != nil {
			return nil, err
		}
		if _, err := revokeOfflineCredentials(ctx, tx, "device_id", deviceID, "device_revoked"); err != nil {
			return nil, err
		}
	}
	machine, ok := service.License.CurrentMachine()
	if !ok {
		return nil, licenseFault(licensing.ErrMachineUnverified)
	}
	identityDir := filepath.Dir(service.License.Directory())
	identity, key, aside, err := localsetup.RegenerateIdentity(identityDir, time.Now(), func() (string, error) {
		return machine, nil
	})
	if err != nil {
		return nil, &Fault{Status: 503, Code: "IDENTITY_UNAVAILABLE", Message: "No se pudo crear la identidad nueva de esta computadora. No se cambió nada.", Details: map[string]string{"reason": "regeneration_failed"}}
	}
	installation := licensing.Installation{ServerID: identity.ServerID, PublicKey: key.PublicKey}
	if err := service.License.SwitchInstallation(filepath.Join(identityDir, localsetup.LicenseStateFolder), installation, key.PrivateKey); err != nil {
		return nil, licenseFault(err)
	}
	details := map[string]any{
		"previous_server_id":   previousServerID,
		"previous_fingerprint": previousFingerprint,
		"server_id":            identity.ServerID,
		"fingerprint":          identity.FingerprintHex(),
		"revoked_terminals":    len(revoked),
		"previous_identity":    filepath.Base(aside),
	}
	if err := audit(ctx, tx, principal, "license.identity_regenerated", "license", nil, nil, details, true); err != nil {
		return nil, err
	}
	return map[string]any{
		"server_id":         identity.ServerID,
		"revoked_terminals": len(revoked),
		"restart_required":  true,
		"status":            service.licenseStatusView(principal, request.Local),
	}, nil
}

// --- Errores (L1-D17, avance §0.6.2) -------------------------------------------------------------------------------

// licenseFault traduce un error de la licencia a su código estable, con un texto propio en español.
func licenseFault(err error) *Fault {
	var serverErr *licensing.ServerError
	var transport *licensing.TransportError
	var rejected *licensing.ResponseRejectedError
	var verify *licensing.VerifyError
	switch {
	case errors.Is(err, licensing.ErrLicenseKeyFormat):
		return &Fault{Status: 400, Code: "LICENSE_KEY_INVALID_FORMAT", Message: "El código de la licencia no tiene el formato correcto: son 15 letras y números en grupos de 3, 4, 4 y 4."}
	case errors.Is(err, licensing.ErrCopiedInstallation):
		return &Fault{Status: 409, Code: "LICENSE_INSTALLATION_COPIED", Message: "Esta instalación se copió de otra computadora. Crea una identidad nueva para esta computadora antes de activar."}
	case errors.Is(err, licensing.ErrMachineUnverified):
		return &Fault{Status: 503, Code: "IDENTITY_UNAVAILABLE", Message: "No se pudo comprobar esta computadora. Reinicia el servidor; si sigue igual, comunícate con soporte.", Details: map[string]string{"reason": "machine_unverified"}}
	case errors.Is(err, licensing.ErrIdentityUnavailable):
		return &Fault{Status: 503, Code: "IDENTITY_UNAVAILABLE", Message: "La identidad de esta computadora no está disponible. Comunícate con soporte.", Details: map[string]string{"reason": "identity"}}
	case errors.Is(err, licensing.ErrNotActivated):
		return &Fault{Status: 409, Code: "LICENSE_NOT_ACTIVATED", Message: "Esta computadora no tiene una licencia activada.", Details: map[string]any{"state": "not_activated"}}
	case errors.Is(err, licensing.ErrStateNotSaved):
		return &Fault{Status: 503, Code: "LICENSE_STATE_NOT_SAVED", Message: "No se pudo guardar la licencia en esta computadora. No se cambió nada; inténtalo de nuevo."}
	case errors.As(err, &serverErr):
		return licenseServerFault(serverErr)
	case errors.As(err, &transport):
		if transport.Reason == licensing.ReasonInvalidResponse {
			return &Fault{Status: 502, Code: "LICENSE_SERVER_ERROR", Message: "El servidor de licencias respondió algo inesperado. Inténtalo más tarde; si sigue igual, comunícate con soporte.", Details: map[string]string{"reason": transport.Reason}}
		}
		return &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE", Message: "No se pudo conectar con el servidor de licencias. Revisa la conexión a internet o usa la activación sin internet.", Details: map[string]string{"reason": transport.Reason}}
	case errors.As(err, &rejected):
		return &Fault{Status: 502, Code: "LICENSE_RESPONSE_INVALID", Message: "La licencia que envió el servidor de licencias no es válida y no se instaló. Tu licencia actual no cambió.", Details: map[string]string{"reason": rejected.Reason}}
	case errors.As(err, &verify):
		return &Fault{Status: 400, Code: "LICENSE_FILE_INVALID", Message: "El archivo de licencia no es válido para esta computadora y no se instaló. Tu licencia actual no cambió.", Details: map[string]string{"reason": verify.Code}}
	}
	return &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE", Message: "No se pudo completar la operación de la licencia. Inténtalo de nuevo.", Details: map[string]string{"reason": "unexpected"}}
}

// licenseServerFault es el código de un error del servidor de licencias (decidido por error.code).
func licenseServerFault(serverErr *licensing.ServerError) *Fault {
	switch serverErr.Code {
	case licensing.CodeLicenseNotFound:
		return &Fault{Status: 404, Code: "LICENSE_KEY_NOT_FOUND", Message: "No existe una licencia con ese código. Revisa que esté bien escrito."}
	case licensing.CodeRevoked:
		return &Fault{Status: 409, Code: "LICENSE_SERVER_REVOKED", Message: "Esa licencia fue revocada. Comunícate con soporte."}
	case licensing.CodeActivationLimit:
		available := serverErr.TransferAvailable != nil && *serverErr.TransferAvailable
		message := "La licencia ya está activa en otra computadora. Puedes transferirla a esta computadora."
		if !available {
			message = "La licencia ya está activa en otra computadora y tiene una transferencia pendiente. Comunícate con soporte."
		}
		return &Fault{Status: 409, Code: "LICENSE_ACTIVATION_LIMIT", Message: message, Details: map[string]bool{"transfer_available": available}}
	case licensing.CodeTransferLimit:
		return &Fault{Status: 409, Code: "LICENSE_TRANSFER_LIMIT", Message: "La licencia ya tiene una transferencia pendiente. Comunícate con soporte."}
	case licensing.CodeRateLimited:
		seconds := int(serverErr.RetryAfter / time.Second)
		if seconds <= 0 {
			seconds = 60
		}
		return &Fault{Status: 429, Code: "LICENSE_SERVER_BUSY", Message: "El servidor de licencias recibió demasiados intentos. Espera un momento e inténtalo de nuevo.", Details: map[string]int{"retry_after_seconds": seconds}}
	case licensing.CodeTemporaryUnavailable:
		return &Fault{Status: 503, Code: "LICENSE_SERVER_UNAVAILABLE", Message: "El servidor de licencias no está disponible en este momento. Tu licencia actual no cambió; inténtalo más tarde.", Details: map[string]string{"reason": "temporary"}}
	}
	return &Fault{Status: 502, Code: "LICENSE_SERVER_ERROR", Message: "El servidor de licencias rechazó la operación. Inténtalo de nuevo; si sigue igual, comunícate con soporte.", Details: map[string]string{"server_code": serverErr.Code}}
}
