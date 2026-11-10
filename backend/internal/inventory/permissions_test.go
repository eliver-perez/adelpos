package inventory

import (
	"io/fs"
	"strings"
	"testing"

	"local/ad-el-pos/server/migrations"
)

// Todo permiso de inventario registrado en Go existe en la 0010 con la misma descripción y se
// concede al administrador (para que siga contando como administrador completo); la 0010 no
// registra permisos de inventario que Go no conozca.
func TestInventoryPermissionsAreMigratedAndGrantedToAdministrator(t *testing.T) {
	content, err := fs.ReadFile(migrations.Files, "0010_inventory_core.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	grant := sql[strings.LastIndex(sql, "INSERT INTO app.role_permissions"):]
	if !strings.Contains(grant, "r.code = 'administrator'") {
		t.Fatal("la 0010 no concede sus permisos al administrador")
	}
	if strings.Count(sql, "INSERT INTO app.role_permissions") != 1 || strings.Contains(sql, "'inventory', '") {
		t.Fatal("la 0010 no debe conceder permisos a otros roles")
	}
	for _, permission := range Permissions {
		entry := "('" + permission.Key + "', '" + permission.Description + "')"
		if !strings.Contains(sql, entry) {
			t.Fatalf("la 0010 no registra %s con la misma descripción", permission.Key)
		}
		if !strings.Contains(grant, "'"+permission.Key+"'") {
			t.Fatalf("la 0010 no concede %s al administrador", permission.Key)
		}
	}
	if registered := strings.Count(sql, "('inventory."); registered != len(Permissions) || len(Permissions) != 10 {
		t.Fatalf("se esperaban 10 permisos de inventario en Go y en la 0010: %d y %d", len(Permissions), registered)
	}
}
