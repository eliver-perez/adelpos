package administration

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
)

// «Restablecer permisos predeterminados» de un rol de fábrica (etapa A3, §3.1; decisión A3-D6 del coordinador):
//
//   - GET roles/{id}/defaults es la vista previa: los permisos actuales del rol, los de fábrica y qué se agregará y qué
//     se quitará, sin escribir nada.
//   - POST roles/{id}/defaults con {expected} aplica el conjunto de fábrica si los permisos del rol siguen siendo los que
//     mostró la vista previa (expected); si cambiaron mientras tanto, 409 ROLE_CHANGED y no se escribe nada. Si el rol
//     ya tiene exactamente los de fábrica (por ejemplo, el reintento de una confirmación cuya respuesta se perdió),
//     responde 200 con changed false, sin escribir ni auditar.
//
// Las dos aplican las mismas reglas que PATCH roles/{id} (roles.go): exigen roles.manage, permissions.manage y
// permissions.read; un rol reservado no se toca; nadie restablece su propio rol; el actor debe tener todos los permisos
// actuales del rol y todos los de fábrica (requireWithin: nadie concede así lo que no tiene) y un límite de descuento
// no menor que el del rol; y el cambio nunca deja el sistema sin un administrador completo (lockAdministration y
// preserveAdministration). Solo cambia role_permissions: el límite de descuento del rol y las excepciones por usuario
// (user_permissions) quedan igual. Queda en el Registro de movimientos como role.defaults_restored. Solo los roles de
// fábrica con código (factoryRoleCodes) tienen conjunto; un rol del usuario, el Administrador y el soporte, no.

// roleChangedMessage explica el 409 ROLE_CHANGED de la confirmación.
const roleChangedMessage = "Los permisos de este rol cambiaron mientras revisabas los cambios. Revisa otra vez qué se agregará y qué se quitará."

// noDefaultsMessage explica por qué un rol no tiene permisos predeterminados.
const noDefaultsMessage = "Solo los roles de fábrica (Mesero, Cajero, Cocina, Supervisor, Inventarios y Finanzas) tienen permisos predeterminados."

// roleDefaultsPlan es lo que muestra la vista previa y aplica la confirmación.
type roleDefaultsPlan struct {
	RoleID  string   `json:"-"`
	Role    roleRef  `json:"role"`
	Current []string `json:"current"`
	Factory []string `json:"factory"`
	Add     []string `json:"add"`
	Remove  []string `json:"remove"`
}

// roleRef identifica el rol en la vista previa.
type roleRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

func roleDefaults(ctx context.Context, tx pgx.Tx, request Request, principal Principal, roleID string) (any, error) {
	for _, key := range []string{"roles.manage", "permissions.manage", "permissions.read"} {
		if err := principal.require(key); err != nil {
			return nil, err
		}
	}
	switch request.Method {
	case "GET":
		return planRoleDefaults(ctx, tx, principal, roleID, false)
	case "POST":
		return restoreRoleDefaults(ctx, tx, request, principal, roleID)
	}
	return nil, invalid("Operación de rol no válida.")
}

// planRoleDefaults lee el rol con las reglas de PATCH roles/{id} y calcula qué cambiaría. Con forUpdate bloquea la fila
// del rol hasta el final de la transacción (la confirmación).
func planRoleDefaults(ctx context.Context, tx pgx.Tx, principal Principal, roleID string, forUpdate bool) (*roleDefaultsPlan, error) {
	query := "SELECT reserved,coalesce(code,''),name FROM app.roles WHERE id=$1"
	if forUpdate {
		query += " FOR UPDATE"
	}
	var reserved bool
	var code, name string
	if err := tx.QueryRow(ctx, query, roleID).Scan(&reserved, &code, &name); err != nil {
		return nil, err
	}
	if reserved {
		return nil, denied()
	}
	// Primero lo que es del rol: un rol del usuario o el Administrador no tienen conjunto, lo pida quien lo pida.
	factory, found := factoryPermissions(code)
	if !found {
		return nil, invalid(noDefaultsMessage)
	}
	// Nadie modifica el rol que le otorga sus propios permisos.
	if roleID == principal.RoleID {
		return nil, delegationDenied("No puedes modificar tu propio rol.")
	}
	current, err := rolePermissionKeys(ctx, tx, roleID)
	if err != nil {
		return nil, err
	}
	// Delegación segura, como al editar: el actor debe tener lo que el rol tiene hoy y lo que recibirá.
	if err := principal.requireWithin(current, "No puedes editar un rol con permisos que tú no tienes."); err != nil {
		return nil, err
	}
	if err := principal.requireWithin(factory, "No puedes conceder a un rol permisos que tú no tienes."); err != nil {
		return nil, err
	}
	if err := requireLimitWithin(ctx, tx, principal, roleID); err != nil {
		return nil, err
	}
	return &roleDefaultsPlan{
		RoleID:  roleID,
		Role:    roleRef{ID: roleID, Code: code, Name: name},
		Current: current,
		Factory: factory,
		Add:     missingFrom(factory, current),
		Remove:  missingFrom(current, factory),
	}, nil
}

func restoreRoleDefaults(ctx context.Context, tx pgx.Tx, request Request, principal Principal, roleID string) (any, error) {
	var input struct {
		// Expected son los permisos del rol que mostró la vista previa.
		Expected *[]string `json:"expected"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Expected == nil {
		return nil, invalid("Falta lo que mostró la vista previa de los permisos del rol.")
	}
	// Serializa cambios que pueden alterar quién administra el sistema (como PATCH roles/{id}).
	if err := lockAdministration(ctx, tx); err != nil {
		return nil, err
	}
	plan, err := planRoleDefaults(ctx, tx, principal, roleID, true)
	if err != nil {
		return nil, err
	}
	if sameKeys(plan.Current, plan.Factory) {
		saved, err := object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"role": saved, "changed": false, "added": []string{}, "removed": []string{}}, nil
	}
	// Carrera entre la vista previa y la confirmación: si el rol cambió, se vuelve a mostrar la vista previa.
	if !sameKeys(plan.Current, *input.Expected) {
		return nil, &Fault{Status: 409, Code: "ROLE_CHANGED", Message: roleChangedMessage}
	}
	previous, err := object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
	if err != nil {
		return nil, err
	}
	// El último administrador se determina por los permisos efectivos de todos los usuarios, como al editar.
	err = preserveAdministration(ctx, tx, func() error {
		if _, err := tx.Exec(ctx, "DELETE FROM app.role_permissions WHERE role_id=$1", roleID); err != nil {
			return err
		}
		return insertRolePermissions(ctx, tx, roleID, plan.Factory)
	})
	if err != nil {
		return nil, err
	}
	next, err := object(ctx, tx, rolesQuery+" AND r.id=$1", roleID)
	if err != nil {
		return nil, err
	}
	after := map[string]any{"role": json.RawMessage(next), "added": plan.Add, "removed": plan.Remove}
	if err := audit(ctx, tx, principal, "role.defaults_restored", "role", &roleID, previous, after, true); err != nil {
		return nil, err
	}
	return map[string]any{"role": next, "changed": true, "added": plan.Add, "removed": plan.Remove}, nil
}

// missingFrom devuelve, ordenadas, las claves de wanted que no están en present.
func missingFrom(wanted, present []string) []string {
	missing := []string{}
	for _, key := range wanted {
		if !slices.Contains(present, key) && !slices.Contains(missing, key) {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// sameKeys compara dos listas de permisos como conjuntos (sin orden ni repetidos).
func sameKeys(first, second []string) bool {
	return len(missingFrom(first, second)) == 0 && len(missingFrom(second, first)) == 0
}
