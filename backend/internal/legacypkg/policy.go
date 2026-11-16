package legacypkg

import "strings"

// Reglas de lo que el lector del respaldo extrae (etapa 9H, «Datos reales»). Viven aquí, en el paquete neutral,
// porque las comparten el lector (internal/legacyread), los paquetes sintéticos (internal/legacysynth) y el
// importador, que debe saber qué columnas y tablas no viajan.

// PresentSuffix es la terminación de la columna booleana que sustituye a una columna de contraseña: el paquete solo
// dice si había un valor, nunca el valor.
const PresentSuffix = "_present"

// passwordFragments son los fragmentos de nombre que marcan una columna de contraseña (sin distinguir mayúsculas).
// «pass» ya cubre «password» y «passwd»; se listan todos para que la regla se lea completa.
var passwordFragments = []string{"password", "contrasena", "contraseña", "passwd", "pwd", "pass"}

// IsPasswordColumn dice si una columna se trata como contraseña: su valor nunca sale de SQL Server.
func IsPasswordColumn(name string) bool {
	lower := strings.ToLower(name)
	for _, fragment := range passwordFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// PresentColumnName es el nombre de la columna booleana que sustituye a una columna de contraseña.
func PresentColumnName(name string) string {
	return name + PresentSuffix
}

// IsBinaryType dice si un tipo de SQL Server es binario. Las columnas binarias (imágenes de productos, diagramas) no
// se extraen: el manifiesto registra la columna omitida y cuántas filas tenían valor.
func IsBinaryType(sqlType string) bool {
	switch strings.ToLower(sqlType) {
	case "binary", "varbinary", "image", "timestamp", "rowversion":
		return true
	}
	return false
}

// Motivos de las tablas y columnas que no viajan en el paquete.
const (
	ReasonTool         = "tool"
	ReasonPersonalData = "personal_data"
	ReasonBinary       = "binary"
	ReasonUnsupported  = "unsupported_type"
)

// excludedTables son las tablas que no se extraen; el manifiesto las registra como omitidas con su conteo.
//   - sysdiagrams: diagramas de SQL Server Management Studio (la herramienta, no el sistema anterior).
//   - personal_detalles: domicilio, teléfonos y correo de los empleados. No se importan y, por minimización, tampoco
//     viajan en el paquete.
//
// La Fase 0 no encontró tablas de licencia en la base (la licencia del sistema anterior vivía en archivos).
var excludedTables = map[string]string{
	"sysdiagrams":       ReasonTool,
	"personal_detalles": ReasonPersonalData,
}

// ExcludedTableReason dice si una tabla no se extrae y por qué ("" si se extrae). La comparación no distingue
// mayúsculas, como SQL Server con la intercalación del sistema anterior.
func ExcludedTableReason(name string) string {
	return excludedTables[strings.ToLower(name)]
}

// ColumnTreatment es lo que el lector hace con una columna.
type ColumnTreatment int

const (
	// TreatKeep: la columna viaja con su valor.
	TreatKeep ColumnTreatment = iota
	// TreatPassword: se sustituye por <columna>_present (booleano); el valor nunca sale de SQL Server.
	TreatPassword
	// TreatBinary: se omite y se cuentan las filas con valor.
	TreatBinary
	// TreatUnsupported: un tipo que el formato no lleva (geografía, sql_variant…); se omite como la binaria.
	TreatUnsupported
)

// keptTypes son los tipos de SQL Server que viajan en el paquete (text, ntext y xml, convertidos a texto).
var keptTypes = map[string]bool{
	"bigint": true, "int": true, "smallint": true, "tinyint": true, "bit": true,
	"decimal": true, "numeric": true, "money": true, "smallmoney": true, "float": true, "real": true,
	"date": true, "datetime": true, "datetime2": true, "smalldatetime": true, "datetimeoffset": true, "time": true,
	"char": true, "varchar": true, "nchar": true, "nvarchar": true, "text": true, "ntext": true,
	"uniqueidentifier": true, "xml": true,
}

// TreatmentFor decide qué se hace con una columna por su nombre y su tipo base de SQL Server («varchar», «money»…).
// La regla de contraseñas va primero: una columna de contraseña nunca se lee, sea texto o binaria.
func TreatmentFor(name, baseType string) ColumnTreatment {
	if IsPasswordColumn(name) {
		return TreatPassword
	}
	if IsBinaryType(baseType) {
		return TreatBinary
	}
	if !keptTypes[strings.ToLower(baseType)] {
		return TreatUnsupported
	}
	return TreatKeep
}

// PresentColumn es la columna booleana que sustituye a una columna de contraseña.
func PresentColumn(name string) Column {
	return Column{Name: PresentColumnName(name), Type: "bit", Nullable: false, Replaces: name}
}

// BaseType es el tipo sin longitud ni precisión: «varchar(50)» → «varchar».
func BaseType(sqlType string) string {
	if index := strings.IndexByte(sqlType, '('); index >= 0 {
		return strings.ToLower(sqlType[:index])
	}
	return strings.ToLower(sqlType)
}

// La tabla y la columna de las que sale el rango de fechas de las ventas del resumen (Summary).
const (
	SalesTable      = "ventas"
	SalesDateColumn = "fecha"
)
