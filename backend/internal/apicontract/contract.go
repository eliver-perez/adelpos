// Package apicontract incrusta el contrato OpenAPI 3.1 de la API v1 para clientes (etapa 9G, §5.1): la API que usan
// la aplicación de escritorio y las terminales y que usará la aplicación Android (versión, sesión, catálogo
// descargable, imágenes, cuentas y pedidos —incluido el pedido capturado sin conexión—, estados de preparación,
// recuperación del estado y «Agotado»). El documento (openapi.json) es la fuente del contrato: el servidor lo publica
// en GET /api/v1/openapi.json, docs/API_V1.md lo explica y las pruebas de contrato lo comparan con las rutas y las
// respuestas reales.
//
// Para validar respuestas sin dependencias nuevas, el paquete trae un validador de un subconjunto de JSON Schema
// (el que usa el documento). Una palabra clave fuera del subconjunto hace fallar las pruebas en lugar de ignorarse.
package apicontract

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed openapi.json
var document []byte

// Document devuelve el contrato tal como está en el repositorio.
func Document() []byte {
	return document
}

// Operation es una operación documentada: método, ruta relativa a /api/v1 y sus respuestas.
type Operation struct {
	ID        string
	Method    string
	Path      string
	Responses map[string]any
}

// Contract es el documento interpretado.
type Contract struct {
	Raw        map[string]any
	Operations []Operation
	schemas    map[string]any
}

// Load interpreta el documento incrustado.
func Load() (*Contract, error) {
	return Parse(document)
}

// Parse interpreta un documento OpenAPI.
func Parse(source []byte) (*Contract, error) {
	var raw map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(source)))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("el contrato no es JSON válido: %w", err)
	}
	contract := &Contract{Raw: raw, schemas: map[string]any{}}
	if components, ok := raw["components"].(map[string]any); ok {
		if schemas, ok := components["schemas"].(map[string]any); ok {
			contract.schemas = schemas
		}
	}
	paths, _ := raw["paths"].(map[string]any)
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	sort.Strings(names)
	for _, path := range names {
		item, ok := paths[path].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("la ruta %s no es un objeto", path)
		}
		for _, method := range []string{"get", "put", "post", "patch", "delete"} {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			id, _ := operation["operationId"].(string)
			responses, _ := operation["responses"].(map[string]any)
			contract.Operations = append(contract.Operations, Operation{
				ID:        id,
				Method:    strings.ToUpper(method),
				Path:      path,
				Responses: responses,
			})
		}
	}
	return contract, nil
}

// Match encuentra la operación documentada de un método y una ruta concreta relativa a /api/v1 («/catalog/sync»).
func (contract *Contract) Match(method, path string) (*Operation, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for index := range contract.Operations {
		operation := &contract.Operations[index]
		if operation.Method != method {
			continue
		}
		template := strings.Split(strings.Trim(operation.Path, "/"), "/")
		if len(template) != len(segments) {
			continue
		}
		matches := true
		for position, part := range template {
			parameter := strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")
			if (parameter && segments[position] == "") || (!parameter && part != segments[position]) {
				matches = false
				break
			}
		}
		if matches {
			return operation, true
		}
	}
	return nil, false
}

// ResponseSchema devuelve el esquema JSON documentado para un estado de la operación (o su respuesta «default»), o
// nil si la respuesta no lleva JSON (por ejemplo, una imagen o 304). documented es falso si el estado no está
// documentado ni hay «default».
func (contract *Contract) ResponseSchema(operation *Operation, status int) (any, bool, error) {
	response, exists := operation.Responses[strconv.Itoa(status)]
	if !exists {
		response, exists = operation.Responses["default"]
	}
	if !exists {
		return nil, false, nil
	}
	resolved, err := contract.resolve(response)
	if err != nil {
		return nil, true, err
	}
	object, _ := resolved.(map[string]any)
	content, _ := object["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	if media == nil {
		return nil, true, nil
	}
	return media["schema"], true, nil
}

// MediaTypes devuelve los tipos de contenido documentados para un estado (sin «default»).
func (contract *Contract) MediaTypes(operation *Operation, status int) ([]string, error) {
	response, exists := operation.Responses[strconv.Itoa(status)]
	if !exists {
		return nil, nil
	}
	resolved, err := contract.resolve(response)
	if err != nil {
		return nil, err
	}
	object, _ := resolved.(map[string]any)
	content, _ := object["content"].(map[string]any)
	types := []string{}
	for name := range content {
		types = append(types, name)
	}
	sort.Strings(types)
	return types, nil
}

// RequestSchema devuelve el esquema JSON del cuerpo de la operación, o nil si no lleva cuerpo.
func (contract *Contract) RequestSchema(operation *Operation) (any, error) {
	paths, _ := contract.Raw["paths"].(map[string]any)
	item, _ := paths[operation.Path].(map[string]any)
	raw, _ := item[strings.ToLower(operation.Method)].(map[string]any)
	body, exists := raw["requestBody"]
	if !exists {
		return nil, nil
	}
	resolved, err := contract.resolve(body)
	if err != nil {
		return nil, err
	}
	object, _ := resolved.(map[string]any)
	content, _ := object["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	return media["schema"], nil
}

// resolve sigue una referencia local «#/components/…».
func (contract *Contract) resolve(value any) (any, error) {
	for depth := 0; depth < 16; depth++ {
		object, ok := value.(map[string]any)
		if !ok {
			return value, nil
		}
		reference, ok := object["$ref"].(string)
		if !ok {
			return value, nil
		}
		if !strings.HasPrefix(reference, "#/") {
			return nil, fmt.Errorf("referencia externa no admitida: %s", reference)
		}
		var target any = contract.Raw
		for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
			container, ok := target.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("referencia rota: %s", reference)
			}
			target, ok = container[part]
			if !ok {
				return nil, fmt.Errorf("referencia rota: %s", reference)
			}
		}
		value = target
	}
	return nil, errors.New("referencias encadenadas en exceso")
}

// supportedKeywords es el subconjunto de JSON Schema que el validador entiende.
var supportedKeywords = []string{
	"$ref", "type", "properties", "required", "items", "enum", "const", "additionalProperties", "anyOf", "oneOf",
	"format", "pattern", "minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength",
	"description", "title", "examples", "default", "deprecated", "readOnly",
}

// CheckSchemaKeywords recorre un esquema y falla con cualquier palabra clave fuera del subconjunto.
func CheckSchemaKeywords(schema any, location string) error {
	object, ok := schema.(map[string]any)
	if !ok {
		if _, isBool := schema.(bool); isBool {
			return nil
		}
		return fmt.Errorf("%s: el esquema no es un objeto", location)
	}
	for key, value := range object {
		if !slices.Contains(supportedKeywords, key) {
			return fmt.Errorf("%s: palabra clave no admitida por el validador: %s", location, key)
		}
		switch key {
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: properties no es un objeto", location)
			}
			for name, child := range properties {
				if err := CheckSchemaKeywords(child, location+"."+name); err != nil {
					return err
				}
			}
		case "items", "additionalProperties":
			if err := CheckSchemaKeywords(value, location+"/"+key); err != nil {
				return err
			}
		case "anyOf", "oneOf":
			options, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s: %s no es una lista", location, key)
			}
			for index, child := range options {
				if err := CheckSchemaKeywords(child, fmt.Sprintf("%s/%s[%d]", location, key, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Validate comprueba un valor JSON (decodificado con UseNumber) contra un esquema del documento.
func (contract *Contract) Validate(schema any, value any) error {
	return contract.validate(schema, value, "$", 0)
}

// ValidateJSON decodifica un cuerpo y lo valida.
func (contract *Contract) ValidateJSON(schema any, body []byte) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("la respuesta no es JSON: %w", err)
	}
	return contract.Validate(schema, value)
}

var formats = map[string]*regexp.Regexp{
	"uuid":    regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
	"date":    regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
	"decimal": regexp.MustCompile(`^-?\d+\.\d{2}$`),
}

func (contract *Contract) validate(schema any, value any, location string, depth int) error {
	if depth > 64 {
		return fmt.Errorf("%s: el esquema anida demasiado", location)
	}
	if allowed, ok := schema.(bool); ok {
		if !allowed {
			return fmt.Errorf("%s: no se admite ningún valor", location)
		}
		return nil
	}
	resolved, err := contract.resolve(schema)
	if err != nil {
		return err
	}
	object, ok := resolved.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: esquema inválido", location)
	}
	if err := CheckSchemaKeywords(object, location); err != nil {
		return err
	}
	if options, ok := object["anyOf"].([]any); ok {
		failures := []string{}
		for _, option := range options {
			problem := contract.validate(option, value, location, depth+1)
			if problem == nil {
				failures = nil
				break
			}
			failures = append(failures, problem.Error())
		}
		if failures != nil {
			return fmt.Errorf("%s: no cumple ninguna alternativa (%s)", location, strings.Join(failures, "; "))
		}
	}
	if options, ok := object["oneOf"].([]any); ok {
		matches := 0
		for _, option := range options {
			if contract.validate(option, value, location, depth+1) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: cumple %d alternativas de oneOf (debe ser exactamente una)", location, matches)
		}
	}
	if types, exists := object["type"]; exists {
		if err := checkType(types, value, location); err != nil {
			return err
		}
	}
	if values, ok := object["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			if equalJSON(candidate, value) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s: %v no está entre los valores permitidos", location, value)
		}
	}
	if constant, exists := object["const"]; exists && !equalJSON(constant, value) {
		return fmt.Errorf("%s: debe ser %v", location, constant)
	}
	switch typed := value.(type) {
	case string:
		return contract.validateString(object, typed, location)
	case json.Number:
		return validateNumber(object, typed, location)
	case []any:
		return contract.validateArray(object, typed, location, depth)
	case map[string]any:
		return contract.validateObject(object, typed, location, depth)
	}
	return nil
}

func (contract *Contract) validateString(schema map[string]any, value, location string) error {
	if format, ok := schema["format"].(string); ok {
		switch format {
		case "date-time":
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				return fmt.Errorf("%s: %q no es una fecha y hora RFC 3339", location, value)
			}
		case "binary", "byte", "uri-reference":
		default:
			pattern, known := formats[format]
			if !known {
				return fmt.Errorf("%s: formato no admitido por el validador: %s", location, format)
			}
			if !pattern.MatchString(value) {
				return fmt.Errorf("%s: %q no tiene el formato %s", location, value, format)
			}
		}
	}
	if pattern, ok := schema["pattern"].(string); ok {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("%s: patrón inválido: %w", location, err)
		}
		if !expression.MatchString(value) {
			return fmt.Errorf("%s: %q no cumple el patrón %s", location, value, pattern)
		}
	}
	length := len([]rune(value))
	if minimum, ok := integerKeyword(schema, "minLength"); ok && length < minimum {
		return fmt.Errorf("%s: texto más corto que %d", location, minimum)
	}
	if maximum, ok := integerKeyword(schema, "maxLength"); ok && length > maximum {
		return fmt.Errorf("%s: texto más largo que %d", location, maximum)
	}
	return nil
}

func validateNumber(schema map[string]any, value json.Number, location string) error {
	number, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return fmt.Errorf("%s: número inválido", location)
	}
	if minimum, exists := schema["minimum"].(json.Number); exists {
		limit, _ := new(big.Rat).SetString(minimum.String())
		if limit != nil && number.Cmp(limit) < 0 {
			return fmt.Errorf("%s: %s es menor que %s", location, value, minimum)
		}
	}
	if maximum, exists := schema["maximum"].(json.Number); exists {
		limit, _ := new(big.Rat).SetString(maximum.String())
		if limit != nil && number.Cmp(limit) > 0 {
			return fmt.Errorf("%s: %s es mayor que %s", location, value, maximum)
		}
	}
	return nil
}

func (contract *Contract) validateArray(schema map[string]any, value []any, location string, depth int) error {
	if minimum, ok := integerKeyword(schema, "minItems"); ok && len(value) < minimum {
		return fmt.Errorf("%s: se esperaban al menos %d elementos", location, minimum)
	}
	if maximum, ok := integerKeyword(schema, "maxItems"); ok && len(value) > maximum {
		return fmt.Errorf("%s: se esperaban como máximo %d elementos", location, maximum)
	}
	items, exists := schema["items"]
	if !exists {
		return nil
	}
	for index, element := range value {
		if err := contract.validate(items, element, fmt.Sprintf("%s[%d]", location, index), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (contract *Contract) validateObject(schema map[string]any, value map[string]any, location string, depth int) error {
	if required, ok := schema["required"].([]any); ok {
		for _, entry := range required {
			name, _ := entry.(string)
			if _, exists := value[name]; !exists {
				return fmt.Errorf("%s: falta la propiedad obligatoria %s", location, name)
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, child := range value {
		if definition, exists := properties[name]; exists {
			if err := contract.validate(definition, child, location+"."+name, depth+1); err != nil {
				return err
			}
			continue
		}
		if additional, exists := schema["additionalProperties"]; exists {
			if err := contract.validate(additional, child, location+"."+name, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func integerKeyword(schema map[string]any, key string) (int, bool) {
	number, ok := schema[key].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	return int(value), err == nil
}

// checkType comprueba «type», que puede ser un nombre o una lista (por ejemplo ["string", "null"]).
func checkType(types any, value any, location string) error {
	names := []string{}
	switch typed := types.(type) {
	case string:
		names = append(names, typed)
	case []any:
		for _, entry := range typed {
			if name, ok := entry.(string); ok {
				names = append(names, name)
			}
		}
	}
	for _, name := range names {
		if typeMatches(name, value) {
			return nil
		}
	}
	return fmt.Errorf("%s: se esperaba %s y llegó %s", location, strings.Join(names, " o "), describe(value))
}

func typeMatches(name string, value any) bool {
	switch name {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseInt(number.String(), 10, 64)
		return err == nil
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

func describe(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", value)
}

func equalJSON(left, right any) bool {
	encodedLeft, errLeft := json.Marshal(left)
	encodedRight, errRight := json.Marshal(right)
	return errLeft == nil && errRight == nil && string(encodedLeft) == string(encodedRight)
}
