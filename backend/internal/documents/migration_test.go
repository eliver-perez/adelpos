package documents

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"local/ad-el-pos/server/migrations"
)

// La 0018 siembra exactamente las plantillas predeterminadas de Go con sus identificadores fijos y
// registra los seis permisos con la misma descripción, concedidos solo al administrador. La prueba
// de integración de internal/database compara además lo sembrado en PostgreSQL.
func TestMigrationSeedsGoDefaultsAndPermissions(t *testing.T) {
	content, err := fs.ReadFile(migrations.Files, "0018_document_templates.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, info := range Kinds {
		marker := "('" + SystemVersionID(info.Kind) + "', '" + SystemTemplateID(info.Kind) + "', '"
		start := strings.Index(sql, marker)
		if start < 0 {
			t.Fatalf("La 0018 no siembra la versión del sistema de %s", info.Kind)
		}
		start += len(marker)
		end := strings.Index(sql[start:], "}')")
		if end < 0 {
			t.Fatalf("El layout sembrado de %s no termina", info.Kind)
		}
		var seeded, expected any
		if err := json.Unmarshal([]byte(sql[start:start+end+1]), &seeded); err != nil {
			t.Fatalf("El layout sembrado de %s no es JSON: %v", info.Kind, err)
		}
		if err := json.Unmarshal(Default(info.Kind).Encode(), &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(seeded, expected) {
			t.Fatalf("El layout sembrado de %s difiere de documents.Default", info.Kind)
		}
		if !strings.Contains(sql, "('"+SystemTemplateID(info.Kind)+"', '"+string(info.Kind)+"', 'Predeterminada', true, true, '"+SystemVersionID(info.Kind)+"')") {
			t.Fatalf("La plantilla del sistema de %s no está activa con su versión fija", info.Kind)
		}
	}
	grant := sql[strings.LastIndex(sql, "INSERT INTO app.role_permissions"):]
	if !strings.Contains(grant, "r.code = 'administrator'") || strings.Count(sql, "INSERT INTO app.role_permissions") != 1 {
		t.Fatal("La 0018 debe conceder sus permisos solo al administrador")
	}
	if found := strings.Count(sql, "('documents."); found != len(Permissions) || len(Permissions) != 6 {
		t.Fatalf("La 0018 registra %d permisos y Go %d", found, len(Permissions))
	}
	for _, permission := range Permissions {
		if !strings.Contains(sql, "('"+permission.Key+"', '"+permission.Description+"')") {
			t.Fatalf("La 0018 no registra %s con la misma descripción", permission.Key)
		}
		if !strings.Contains(grant, "'"+permission.Key+"'") {
			t.Fatalf("La 0018 no concede %s al administrador", permission.Key)
		}
	}
}
