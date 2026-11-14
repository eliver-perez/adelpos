package httpapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Compatibilidad de clientes (etapa 9G, §5.1). Cada cliente de la API v1 (la aplicación de escritorio y, en su
// etapa, la de Android) se identifica con dos cabeceras: su versión (la del producto, «0.10.0») y su tipo. El
// servidor rechaza con 426 CLIENT_UPDATE_REQUIRED a un cliente anterior a la versión mínima que acepta, con un
// mensaje claro en lugar de errores raros; el cliente, a su vez, compara la versión del servidor que informa
// /api/v1/version con la mínima que necesita (product.json, min_server_version) y avisa si el servidor es más viejo.
//
// Una petición sin X-Client-Version se acepta como «cliente sin identificar»: herramientas de diagnóstico (curl),
// las pruebas automáticas y las aplicaciones anteriores a la 9G, que solo existen en la propia computadora
// principal y se actualizan junto con el servidor. Toda aplicación desde la 9G envía siempre las dos cabeceras.
const (
	ClientVersionHeader = "X-Client-Version"
	ClientKindHeader    = "X-Client-Kind"
)

// ClientKinds son los tipos de cliente que conoce la API v1.
var ClientKinds = []string{"desktop", "android"}

// unversionedRoutes responden a cualquier cliente: con ellas un cliente viejo averigua qué versión necesita.
var unversionedRoutes = []string{
	"/api/v1/health/live",
	"/api/v1/health/ready",
	"/api/v1/version",
	"/api/v1/openapi.json",
}

// version es una versión MAJOR.MINOR.PATCH; prerelease indica un sufijo «-beta…», que va antes de la versión
// final con los mismos números (semver). Los metadatos «+…» no cuentan.
type version struct {
	parts      [3]int
	prerelease bool
}

// parseVersion interpreta una versión del producto. Solo admite dígitos (hasta 6 por número) para no aceptar
// textos arbitrarios en una cabecera.
func parseVersion(text string) (version, bool) {
	result := version{}
	core, _, _ := strings.Cut(text, "+")
	core, suffix, hasSuffix := strings.Cut(core, "-")
	if hasSuffix {
		if suffix == "" || len(suffix) > 40 {
			return result, false
		}
		result.prerelease = true
	}
	numbers := strings.Split(core, ".")
	if len(numbers) != 3 {
		return result, false
	}
	for index, number := range numbers {
		if number == "" || len(number) > 6 {
			return result, false
		}
		for _, character := range number {
			if character < '0' || character > '9' {
				return result, false
			}
		}
		value, err := strconv.Atoi(number)
		if err != nil {
			return result, false
		}
		result.parts[index] = value
	}
	return result, true
}

// CompareVersions devuelve -1, 0 o 1 si a es anterior, igual o posterior a b; ok es falso si alguna no es válida.
func CompareVersions(a, b string) (int, bool) {
	left, validLeft := parseVersion(a)
	right, validRight := parseVersion(b)
	if !validLeft || !validRight {
		return 0, false
	}
	for index := range left.parts {
		if left.parts[index] < right.parts[index] {
			return -1, true
		}
		if left.parts[index] > right.parts[index] {
			return 1, true
		}
	}
	if left.prerelease && !right.prerelease {
		return -1, true
	}
	if !left.prerelease && right.prerelease {
		return 1, true
	}
	return 0, true
}

// clientProblem es el rechazo de un cliente que se identificó con una versión o un tipo que el servidor no acepta.
type clientProblem struct {
	status  int
	code    string
	message string
	details map[string]any
}

// checkClient revisa las cabeceras de identificación del cliente en una ruta de la aplicación. Sin cabecera de
// versión no hay nada que revisar (ver el comentario del paquete).
func checkClient(request *http.Request, minimum, serverVersion string) *clientProblem {
	if slices.Contains(unversionedRoutes, request.URL.Path) {
		return nil
	}
	kind := request.Header.Get(ClientKindHeader)
	if kind != "" && !slices.Contains(ClientKinds, kind) {
		return &clientProblem{
			status:  400,
			code:    "CLIENT_KIND_INVALID",
			message: "El servidor no reconoce el tipo de esta terminal. Actualiza esta terminal.",
		}
	}
	text := request.Header.Get(ClientVersionHeader)
	if text == "" {
		return nil
	}
	comparison, valid := CompareVersions(text, minimum)
	if !valid {
		return &clientProblem{
			status:  400,
			code:    "CLIENT_VERSION_INVALID",
			message: "El servidor no reconoce la versión de esta terminal. Actualiza esta terminal.",
		}
	}
	if comparison < 0 {
		return &clientProblem{
			status: 426,
			code:   "CLIENT_UPDATE_REQUIRED",
			message: "Actualiza esta terminal: el servidor del restaurante necesita una versión más reciente de " +
				"AD-EL POS en este equipo.",
			details: map[string]any{
				"client_version":     text,
				"min_client_version": minimum,
				"server_version":     serverVersion,
			},
		}
	}
	return nil
}
