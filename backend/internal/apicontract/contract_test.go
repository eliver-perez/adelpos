package apicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Pruebas del documento sin base de datos (etapa 9G, §5.1): estructura OpenAPI 3.1, referencias, palabras clave que
// entiende el validador, parámetros de ruta y cabeceras de cliente en cada operación, y que docs/API_V1.md explique
// cada operación. Las rutas y las respuestas reales se comparan en contract_integration_test.go.

func TestDocumentStructure(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if contract.Raw["openapi"] != "3.1.0" {
		t.Fatalf("versión de OpenAPI: %v", contract.Raw["openapi"])
	}
	servers, _ := contract.Raw["servers"].([]any)
	if len(servers) != 1 || servers[0].(map[string]any)["url"] != "/api/v1" {
		t.Fatalf("servidores: %v", servers)
	}
	if len(contract.Operations) < 20 {
		t.Fatalf("el contrato documenta solo %d operaciones", len(contract.Operations))
	}
	seen := map[string]bool{}
	parameter := regexp.MustCompile(`\{([a-z_]+)\}`)
	paths := contract.Raw["paths"].(map[string]any)
	for _, operation := range contract.Operations {
		if operation.ID == "" || seen[operation.ID] {
			t.Fatalf("%s %s: operationId vacío o repetido %q", operation.Method, operation.Path, operation.ID)
		}
		seen[operation.ID] = true
		if !strings.HasPrefix(operation.Path, "/") || strings.HasPrefix(operation.Path, "/api/") {
			t.Fatalf("%s: la ruta debe ser relativa a /api/v1", operation.ID)
		}
		raw := paths[operation.Path].(map[string]any)[strings.ToLower(operation.Method)].(map[string]any)
		for _, field := range []string{"summary", "description"} {
			if text, _ := raw[field].(string); strings.TrimSpace(text) == "" {
				t.Fatalf("%s: falta %s", operation.ID, field)
			}
		}
		success := false
		for status := range operation.Responses {
			if strings.HasPrefix(status, "2") {
				success = true
			}
		}
		if !success {
			t.Fatalf("%s: sin respuesta correcta documentada", operation.ID)
		}
		declared := map[string]bool{}
		headers := map[string]bool{}
		parameters, _ := raw["parameters"].([]any)
		for _, entry := range parameters {
			resolved, err := contract.resolve(entry)
			if err != nil {
				t.Fatalf("%s: %v", operation.ID, err)
			}
			definition := resolved.(map[string]any)
			if definition["in"] == "path" {
				declared[definition["name"].(string)] = true
			}
			if definition["in"] == "header" {
				headers[definition["name"].(string)] = true
			}
		}
		for _, match := range parameter.FindAllStringSubmatch(operation.Path, -1) {
			if !declared[match[1]] {
				t.Fatalf("%s: falta el parámetro de ruta %s", operation.ID, match[1])
			}
		}
		if !headers["X-Client-Version"] || !headers["X-Client-Kind"] {
			t.Fatalf("%s: faltan las cabeceras de cliente", operation.ID)
		}
		if raw["requestBody"] != nil && operation.Method == "GET" {
			t.Fatalf("%s: una consulta no lleva cuerpo", operation.ID)
		}
	}
	// Toda referencia apunta a algo y todo esquema usa solo palabras clave que el validador entiende.
	var walk func(value any, location string)
	walk = func(value any, location string) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "$ref" {
					if _, err := contract.resolve(typed); err != nil {
						t.Fatalf("%s: %v", location, err)
					}
				}
				if key == "schema" {
					if err := CheckSchemaKeywords(child, location+"/schema"); err != nil {
						t.Fatal(err)
					}
				}
				walk(child, location+"/"+key)
			}
		case []any:
			for _, child := range typed {
				walk(child, location)
			}
		}
	}
	walk(contract.Raw, "#")
	for name, schema := range contract.schemas {
		if err := CheckSchemaKeywords(schema, "#/components/schemas/"+name); err != nil {
			t.Fatal(err)
		}
	}
}

// docs/API_V1.md es la explicación del contrato para la etapa de Android: menciona cada operación y cada ruta.
func TestDocumentedInAPIV1(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "API_V1.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(guide)
	for _, operation := range contract.Operations {
		if !strings.Contains(text, operation.ID) || !strings.Contains(text, operation.Path) {
			t.Fatalf("docs/API_V1.md no explica %s (%s %s)", operation.ID, operation.Method, operation.Path)
		}
	}
}

func TestMatchAndValidator(t *testing.T) {
	contract, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	operation, found := contract.Match("POST", "/catalog/products/0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00/sold-out")
	if !found || operation.ID != "setProductSoldOut" {
		t.Fatalf("no se encontró la operación de «Agotado»: %v", operation)
	}
	if _, found := contract.Match("DELETE", "/catalog/sync"); found {
		t.Fatal("se encontró una operación no documentada")
	}
	if _, found := contract.Match("GET", "/catalog/images//content"); found {
		t.Fatal("un parámetro vacío no debe coincidir")
	}
	schema, documented, err := contract.ResponseSchema(operation, 200)
	if err != nil || !documented || schema == nil {
		t.Fatalf("esquema de respuesta: %v %v", documented, err)
	}
	valid := `{"product_id":"0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00","name":"Hamburguesa","sold_out":true,"indefinite":false,
        "sold_out_at":"2026-11-13T19:00:00Z","sold_out_until":"2026-11-14T11:00:00Z","catalog_version":4,"extra":1}`
	if err := contract.ValidateJSON(schema, []byte(valid)); err != nil {
		t.Fatalf("una respuesta válida no pasó: %v", err)
	}
	for name, body := range map[string]string{
		"falta una propiedad":  `{"product_id":"0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00","name":"x","sold_out":true,"indefinite":false,"sold_out_at":null,"sold_out_until":null}`,
		"tipo incorrecto":      strings.Replace(valid, `"sold_out":true`, `"sold_out":"sí"`, 1),
		"uuid inválido":        strings.Replace(valid, `0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00`, `producto`, 1),
		"fecha inválida":       strings.Replace(valid, `2026-11-13T19:00:00Z`, `13/11/2026`, 1),
		"entero con decimales": strings.Replace(valid, `"catalog_version":4`, `"catalog_version":4.5`, 1),
		"mínimo":               strings.Replace(valid, `"catalog_version":4`, `"catalog_version":0`, 1),
	} {
		if contract.ValidateJSON(schema, []byte(body)) == nil {
			t.Fatalf("%s: se aceptó una respuesta inválida", name)
		}
	}
	money := map[string]any{"$ref": "#/components/schemas/Money"}
	for _, value := range []string{`"12.50"`, `"0.00"`, `"-3.10"`} {
		if err := contract.ValidateJSON(money, []byte(value)); err != nil {
			t.Fatalf("importe %s: %v", value, err)
		}
	}
	for _, value := range []string{`12.5`, `"12.5"`, `"12"`, `"$12.00"`} {
		if contract.ValidateJSON(money, []byte(value)) == nil {
			t.Fatalf("se aceptó el importe %s", value)
		}
	}
	if err := CheckSchemaKeywords(map[string]any{"type": "string", "allOf": []any{}}, "#"); err == nil {
		t.Fatal("se aceptó una palabra clave que el validador no entiende")
	}
	recursive := map[string]any{"$ref": "#/components/schemas/Customization"}
	nested := `{"ingredients":[],"groups":null,"components":[{"slot_id":"0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00",
        "product_id":"0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f01","quantity":1,"customization":{"components":[{"quantity":1}]}}],"notes":""}`
	if contract.ValidateJSON(recursive, []byte(nested)) == nil {
		t.Fatal("no se validó la personalización anidada")
	}
	var anything any
	if err := json.Unmarshal([]byte(`{"a":1}`), &anything); err != nil || contract.Validate(true, anything) != nil {
		t.Fatal("el esquema true debe aceptar cualquier valor")
	}
}
