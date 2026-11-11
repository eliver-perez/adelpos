package designer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
)

// Las rutas del diseñador son una lista cerrada: cada una exige sus permisos antes de tocar la
// base (Handle se llama aquí sin transacción) y las escrituras exigen administrar plantillas.
func TestRoutesAreClosedAndRequirePermissionsFirst(t *testing.T) {
	cases := []struct {
		method, route string
		permissions   []string
		write         bool
	}{
		{"GET", "documents/catalog", []string{documents.PermissionTemplatesRead}, false},
		{"GET", "documents/templates", []string{documents.PermissionTemplatesRead}, false},
		{"POST", "documents/templates", []string{documents.PermissionTemplatesRead, documents.PermissionTemplatesManage}, true},
		{"PUT", "documents/templates/00000000-0000-4000-8000-000000000001", []string{documents.PermissionTemplatesRead, documents.PermissionTemplatesManage}, true},
		{"POST", "documents/templates/00000000-0000-4000-8000-000000000001/activate", []string{documents.PermissionTemplatesRead, documents.PermissionTemplatesManage}, true},
		{"POST", "documents/restore-default", []string{documents.PermissionTemplatesRead, documents.PermissionTemplatesManage}, true},
		{"POST", "documents/preview", []string{documents.PermissionTemplatesRead}, false},
		{"GET", "documents/recent", []string{documents.PermissionTemplatesRead, documents.PermissionPreviewReal}, false},
		{"POST", "documents/samples", []string{documents.PermissionSamplesPrint}, true},
		{"GET", "documents/printers", []string{documents.PermissionTemplatesRead}, false},
	}
	for _, item := range cases {
		parts := splitRoute(item.route)
		selected := resolve(item.method, parts)
		if selected == nil || !slices.Equal(selected.permissions, item.permissions) || selected.write != item.write {
			t.Fatalf("%s %s no tiene los permisos esperados: %+v", item.method, item.route, selected)
		}
		_, err := Handle(context.Background(), nil, Request{Method: item.method, Route: item.route, Actor: Actor{}}, nil)
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 403 {
			t.Fatalf("%s %s sin permisos no respondió 403 antes de usar la base: %v", item.method, item.route, err)
		}
	}
	for _, unknown := range []struct{ method, route string }{
		{"DELETE", "documents/templates/00000000-0000-4000-8000-000000000001"},
		{"POST", "documents/sql"},
		{"GET", "documents/templates/00000000-0000-4000-8000-000000000001/layout"},
		{"PATCH", "documents/catalog"},
	} {
		if resolve(unknown.method, splitRoute(unknown.route)) != nil {
			t.Fatalf("%s %s no debería existir", unknown.method, unknown.route)
		}
	}
	all := Actor{Permissions: []string{documents.PermissionTemplatesRead}}
	_, err := Handle(context.Background(), nil, Request{Method: "GET", Route: "documents/templates/no-es-uuid", Actor: all}, nil)
	var problem *Error
	if !errors.As(err, &problem) || problem.Status != 400 {
		t.Fatalf("Un identificador inválido no se rechazó antes de usar la base: %v", err)
	}
}

// Un rechazo de validación de la plantilla llega como 422 con el bloque afectado.
func TestTemplateValidationBecomesUnprocessableWithBlock(t *testing.T) {
	_, err := documents.ParseTemplate(documents.KindReceipt, []byte(`{"blocks":[{"id":"esperado","type":"expected_differences"}]}`))
	var problem *Error
	if !errors.As(templateInvalid(err), &problem) || problem.Status != 422 || problem.Code != "TEMPLATE_INVALID" {
		t.Fatalf("Rechazo inesperado: %v", templateInvalid(err))
	}
	if details, ok := problem.Details.(map[string]string); !ok || details["block_id"] != "esperado" {
		t.Fatalf("El rechazo no señala el bloque: %+v", problem.Details)
	}
	if name, err := cleanName("  Ticket \u001b de   mostrador  "); err != nil || name != "Ticket de mostrador" {
		t.Fatalf("El nombre no se saneó: %q %v", name, err)
	}
	if _, err := cleanName("   "); err == nil {
		t.Fatal("Se aceptó un nombre vacío")
	}
}

func splitRoute(route string) []string {
	return strings.Split(strings.TrimPrefix(route, "documents/"), "/")
}
