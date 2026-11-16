package legacypkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"unicode/utf8"
)

// FormatName es el valor de «format» del manifiesto.
const FormatName = "adelmig"

// ManifestFile es la primera entrada del tar.
const ManifestFile = "manifest.json"

// Manifest es manifest.json: qué hay en el paquete, de dónde salió y sus huellas. Los conteos de cada tabla quedan
// aquí para la validación del importador (§4.4).
type Manifest struct {
	Format        string          `json:"format"`
	FormatVersion uint16          `json:"format_version"`
	CreatedAt     string          `json:"created_at"`
	CreatedBy     Creator         `json:"created_by"`
	Source        Source          `json:"source"`
	Summary       *Summary        `json:"summary,omitempty"`
	Tables        []TableManifest `json:"tables"`
	SkippedTables []SkippedTable  `json:"skipped_tables"`
	// ContentFingerprint es el SHA-256 de las huellas de las tablas en orden (ContentFingerprint): dos paquetes con
	// los mismos datos tienen la misma huella aunque se hayan cifrado con otra frase o en otro momento.
	ContentFingerprint string `json:"content_fingerprint"`
}

// Creator es la versión de AD-EL POS y la herramienta que crearon el paquete.
type Creator struct {
	Product string `json:"product"`
	Version string `json:"version"`
	// Tool es legacy-prepare (respaldo real), legacy-synthetic o el nombre de una prueba.
	Tool string `json:"tool"`
}

// Fuentes de un paquete.
const (
	SourceSQLServerBackup = "sqlserver_backup"
	SourceSynthetic       = "synthetic"
)

// Source describe el respaldo de origen. Las fechas del respaldo son las que registró SQL Server con la hora local del
// equipo de origen (sin zona horaria), en el formato «AAAA-MM-DDTHH:MM:SS.mmm».
type Source struct {
	Kind               string `json:"kind"`
	BackupFile         string `json:"backup_file,omitempty"`
	BackupBytes        int64  `json:"backup_bytes,omitempty"`
	BackupSHA256       string `json:"backup_sha256,omitempty"`
	DatabaseName       string `json:"database_name,omitempty"`
	BackupStartedAt    string `json:"backup_started_at,omitempty"`
	BackupFinishedAt   string `json:"backup_finished_at,omitempty"`
	ServerVersion      string `json:"server_version,omitempty"`
	DatabaseVersion    int    `json:"database_version,omitempty"`
	CompatibilityLevel int    `json:"compatibility_level,omitempty"`
	Collation          string `json:"collation,omitempty"`
	BackupSetPosition  int    `json:"backup_set_position,omitempty"`
	ReaderImage        string `json:"reader_image,omitempty"`
	ReaderImageID      string `json:"reader_image_id,omitempty"`
	ReaderVersion      string `json:"reader_version,omitempty"`
}

// Summary es el resumen que el asistente muestra sin leer las tablas: el rango de fechas de las ventas.
type Summary struct {
	SalesTable string `json:"sales_table,omitempty"`
	SalesFirst string `json:"sales_first,omitempty"`
	SalesLast  string `json:"sales_last,omitempty"`
}

// TableManifest describe una tabla extraída: su archivo dentro del tar, sus filas, su SHA-256 y sus columnas.
type TableManifest struct {
	Name           string          `json:"name"`
	File           string          `json:"file"`
	Rows           int64           `json:"rows"`
	Bytes          int64           `json:"bytes"`
	SHA256         string          `json:"sha256"`
	PrimaryKey     []string        `json:"primary_key"`
	Columns        []Column        `json:"columns"`
	OmittedColumns []OmittedColumn `json:"omitted_columns"`
}

// Column es una columna tal como aparece en las filas del paquete, con su tipo de SQL Server («int», «money»,
// «numeric(18,3)», «varchar(50)», «datetime»…).
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Identity bool   `json:"identity,omitempty"`
	// Replaces es la columna de contraseña que este booleano sustituye (solo en <columna>_present).
	Replaces string `json:"replaces,omitempty"`
}

// OmittedColumn es una columna que no viaja en el paquete (binaria o de un tipo no admitido), con cuántas filas
// tenían un valor.
type OmittedColumn struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Reason        string `json:"reason"`
	RowsWithValue int64  `json:"rows_with_value"`
}

// SkippedTable es una tabla que no se extrajo, con su conteo.
type SkippedTable struct {
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
	Reason string `json:"reason"`
}

var (
	safeTableName = regexp.MustCompile(`^[A-Za-z0-9_]{1,128}$`)
	tableFileName = regexp.MustCompile(`^tables/[A-Za-z0-9_]{1,128}\.json$`)
	sha256Hex     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// tableFile es el nombre de la entrada del tar de una tabla: tables/<tabla>.json cuando el nombre es seguro; si no
// (espacios, puntos, acentos), uno por posición. El nombre real siempre está en el manifiesto.
func tableFile(name string, index int) string {
	if safeTableName.MatchString(name) {
		return "tables/" + name + ".json"
	}
	return fmt.Sprintf("tables/_t%04d.json", index)
}

// ContentFingerprint calcula la huella de contenido: SHA-256 de, por cada tabla en orden, la longitud y el nombre, el
// número de filas y el SHA-256 de su archivo.
func ContentFingerprint(tables []TableManifest) string {
	hash := sha256.New()
	for _, table := range tables {
		fmt.Fprintf(hash, "%d:%s\n%d\n%s\n", len(table.Name), table.Name, table.Rows, table.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Table busca una tabla extraída por nombre.
func (manifest *Manifest) Table(name string) (TableManifest, bool) {
	for _, table := range manifest.Tables {
		if table.Name == name {
			return table, true
		}
	}
	return TableManifest{}, false
}

// validateManifest comprueba la estructura y la huella del manifiesto contra los límites.
func validateManifest(manifest *Manifest, version uint16, limits Limits) error {
	if manifest.Format != FormatName || manifest.FormatVersion != version {
		return damaged("formato o versión del manifiesto", nil)
	}
	if len(manifest.Tables) > limits.MaxTables || len(manifest.SkippedTables) > limits.MaxTables {
		return limitExceeded("número de tablas")
	}
	names := map[string]bool{}
	files := map[string]bool{}
	for _, table := range manifest.Tables {
		if err := validateTableManifest(table, limits); err != nil {
			return err
		}
		if names[table.Name] || files[table.File] {
			return damaged("tabla repetida en el manifiesto", nil)
		}
		names[table.Name] = true
		files[table.File] = true
	}
	for _, skipped := range manifest.SkippedTables {
		if !validName(skipped.Name) || skipped.Rows < 0 || names[skipped.Name] {
			return damaged("tabla omitida inválida en el manifiesto", nil)
		}
		names[skipped.Name] = true
	}
	if manifest.ContentFingerprint != ContentFingerprint(manifest.Tables) {
		return damaged("la huella de contenido no coincide", nil)
	}
	return nil
}

func validateTableManifest(table TableManifest, limits Limits) error {
	if !validName(table.Name) || !tableFileName.MatchString(table.File) {
		return damaged("nombre de tabla o de archivo inválido en el manifiesto", nil)
	}
	if table.Rows < 0 || table.Bytes < 2 || !sha256Hex.MatchString(table.SHA256) {
		return damaged("conteo o huella de tabla inválidos en el manifiesto", nil)
	}
	if table.Bytes > limits.MaxTableBytes {
		return limitExceeded("tamaño de una tabla")
	}
	if len(table.Columns) > limits.MaxColumns || len(table.OmittedColumns) > limits.MaxColumns {
		return limitExceeded("número de columnas")
	}
	columns := map[string]bool{}
	for _, column := range table.Columns {
		if !validName(column.Name) || column.Type == "" || columns[column.Name] {
			return damaged("columna inválida o repetida en el manifiesto", nil)
		}
		columns[column.Name] = true
	}
	for _, key := range table.PrimaryKey {
		if !columns[key] {
			return damaged("llave primaria con una columna que no existe", nil)
		}
	}
	for _, omitted := range table.OmittedColumns {
		if !validName(omitted.Name) || omitted.RowsWithValue < 0 {
			return damaged("columna omitida inválida en el manifiesto", nil)
		}
	}
	return nil
}

func validName(name string) bool {
	return name != "" && len(name) <= 256 && utf8.ValidString(name)
}
