package catalog

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Error struct {
	Status  int
	Code    string
	Message string
	// ProductID es el producto que no se pudo calcular (agotado o no disponible), cuando se conoce. No se
	// publica: lo usa la validación de los pedidos capturados sin conexión (etapa 9G, §5.5) para distinguir el
	// producto de la partida de un complemento o un componente de combo.
	ProductID string
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: 400, Code: "CATALOG_VALIDATION", Message: message}
}

func conflict() error {
	return &Error{
		Status:  409,
		Code:    "REVISION_CONFLICT",
		Message: "Otro administrador modificó este registro. Recarga antes de guardar para conservar sus cambios.",
	}
}

func unavailable(message string) error {
	return &Error{Status: 409, Code: "PRODUCT_UNAVAILABLE", Message: message}
}

// unavailableProduct es unavailable con el producto que lo causó.
func unavailableProduct(productID, message string) error {
	return &Error{Status: 409, Code: "PRODUCT_UNAVAILABLE", Message: message, ProductID: productID}
}

// soldOut es el rechazo de un producto agotado (etapa 9G, §5.4): no se puede capturar ni confirmar hasta que
// alguien lo marque como disponible otra vez o empiece el día comercial siguiente.
func soldOut(productID, message string) error {
	return &Error{Status: 409, Code: "PRODUCT_SOLD_OUT", Message: message, ProductID: productID}
}

type Permission struct {
	Key         string
	Description string
}

var Permissions = []Permission{
	{"catalog.read", "Consultar catálogo y configuraciones"},
	{"catalog.prices.read", "Consultar precios y previsualizar cálculos"},
	{"catalog.prices.manage", "Modificar precios y valores comerciales"},
	{"catalog.products.manage", "Crear y editar productos"},
	{"catalog.products.state", "Activar, desactivar y archivar productos"},
	{"catalog.categories.manage", "Administrar categorías"},
	{"catalog.ingredients.manage", "Administrar ingredientes"},
	{"catalog.complements.manage", "Administrar complementos y porciones"},
	{"catalog.groups.manage", "Administrar grupos de personalización"},
	{"catalog.combos.manage", "Administrar combos y componentes"},
	{"catalog.price_lists.manage", "Administrar listas de precios"},
	{"catalog.promotions.manage", "Administrar promociones"},
	{"catalog.availability.manage", "Modificar disponibilidad y horarios"},
	{"catalog.availability.override", "Autorizar excepciones de disponibilidad en previsualización"},
	{"catalog.images.manage", "Administrar imágenes del catálogo"},
	// Etapa 9G (§5.4, decisión D8): «Agotado» y «Disponible otra vez» desde la captura. Es distinto de
	// catalog.availability.manage («Pausar» y horarios), que no cambia. La migración 0023 lo concede solo al
	// administrador, con esta misma descripción.
	{"catalog.sold_out.manage", "Marcar productos como agotados y disponibles otra vez desde la captura"},
}

type Actor struct {
	Permissions []string
	// UserID y DeviceID identifican a quien marca «Agotado» (etapa 9G): su clave de operación se guarda con el
	// usuario y la terminal, como en pedidos y cobros.
	UserID   string
	DeviceID string
}

func (actor Actor) Can(permission string) bool {
	return slices.Contains(actor.Permissions, permission)
}

func (actor Actor) Require(permission string) error {
	if !actor.Can(permission) {
		return &Error{
			Status:  403,
			Code:    "PERMISSION_DENIED",
			Message: "No tienes permiso para esta operación del catálogo.",
		}
	}
	return nil
}

type Request struct {
	Method string
	Route  string
	Query  url.Values
	Body   json.RawMessage
	Actor  Actor
	// IfNoneMatch es la cabecera del mismo nombre: la imagen binaria responde «sin cambios» si coincide con su
	// etiqueta (etapa 9G, §5.3).
	IfNoneMatch string
}

type Audit func(context.Context, pgx.Tx, string, string, *string, any, any) error

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func newID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6] = (value[6] & 15) | 64
	value[8] = (value[8] & 63) | 128
	return fmt.Sprintf(

		"%x-%x-%x-%x-%x",
		value[:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:],
	)
}

func decode(body []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("Los campos del catálogo no tienen un formato válido.")
	}
	return nil
}

func validateMetadata(metadata *Metadata) error {
	metadata.Name = strings.TrimSpace(metadata.Name)
	if metadata.Name == "" || len(metadata.Name) > 100 || len(metadata.Description) > 2000 {
		return invalid("Indica un nombre de hasta 100 caracteres y una descripción de hasta 2000.")
	}
	return nil
}

func validateID(id string) error {
	if !idPattern.MatchString(id) {
		return invalid("Identificador de catálogo inválido.")
	}
	return nil
}

func validateOptionalID(id *string) error {
	if id != nil {
		return validateID(*id)
	}
	return nil
}

func jsonValue(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}
