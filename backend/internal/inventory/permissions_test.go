package inventory

import (
	"io/fs"
	"strings"
	"testing"

	"local/ad-el-pos/server/migrations"
)

// Todo permiso de inventario registrado en Go existe en la migración de su fase (0010 la fase A,
// 0011 la fase B, 0013 la fase C, 0014 la fase D) con la misma descripción y se concede al
// administrador (para que siga contando como administrador completo); ninguna migración registra
// permisos de inventario que Go no conozca ni los concede a otros roles.
func TestInventoryPermissionsAreMigratedAndGrantedToAdministrator(t *testing.T) {
	expected := map[string]int{
		"0010_inventory_core.sql": 10, "0011_inventory_recipes_and_consumption.sql": 4, "0013_inventory_purchasing.sql": 6,
		"0014_inventory_control.sql": 4,
	}
	registered := map[string]bool{}
	total := 0
	for name, count := range expected {
		content, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		sql := string(content)
		grant := sql[strings.LastIndex(sql, "INSERT INTO app.role_permissions"):]
		if !strings.Contains(grant, "r.code = 'administrator'") {
			t.Fatalf("la %s no concede sus permisos al administrador", name)
		}
		if strings.Count(sql, "INSERT INTO app.role_permissions") != 1 || strings.Contains(sql, "'inventory', '") {
			t.Fatalf("la %s no debe conceder permisos a otros roles", name)
		}
		found := strings.Count(sql, "('inventory.")
		if found != count {
			t.Fatalf("la %s registra %d permisos de inventario; se esperaban %d", name, found, count)
		}
		total += found
		for _, permission := range Permissions {
			entry := "('" + permission.Key + "', '" + permission.Description + "')"
			if !strings.Contains(sql, entry) {
				continue
			}
			if registered[permission.Key] {
				t.Fatalf("%s se registra en más de una migración", permission.Key)
			}
			registered[permission.Key] = true
			if !strings.Contains(grant, "'"+permission.Key+"'") {
				t.Fatalf("la %s no concede %s al administrador", name, permission.Key)
			}
		}
	}
	for _, permission := range Permissions {
		if !registered[permission.Key] {
			t.Fatalf("ninguna migración registra %s con la misma descripción", permission.Key)
		}
	}
	if total != len(Permissions) || len(Permissions) != 24 {
		t.Fatalf("se esperaban 24 permisos de inventario en Go y en las migraciones: %d y %d", len(Permissions), total)
	}
}
