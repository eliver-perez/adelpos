package legacyimport

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"local/ad-el-pos/server/internal/legacypkg"
)

// Lectura del paquete .adelmig con internal/legacypkg (formato en docs/MIGRACION_FORMATO.md). El lector es en
// flujo: entrega las tablas en el orden del manifiesto y confirma la integridad completa (bloques cifrados, huellas
// y conteos) solo cuando NextTable devuelve io.EOF; Load lee todo antes de que se escriba nada.

// openMutex serializa las aperturas de paquetes (la derivación de la llave).
var openMutex sync.Mutex

// packageSource adapta un *legacypkg.Reader a Source.
type packageSource struct {
	reader *legacypkg.Reader
	info   PackageInfo
}

// packageTable adapta un *legacypkg.TableReader a Table.
type packageTable struct {
	table   *legacypkg.TableReader
	columns []string
}

// OpenPackage abre el paquete del archivo path con la frase de paso. Devuelve la fuente para Load y el archivo, que
// el llamador cierra. Los errores del formato se traducen a los de la migración (frase incorrecta, paquete dañado,
// versión desconocida, no es un paquete, límites).
func OpenPackage(path, passphrase string) (Source, io.Closer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, &Error{Code: CodeUploadMissing, Message: "El paquete subido ya no está disponible. Vuelve a elegirlo.", cause: err}
	}
	// Una apertura a la vez en todo el servidor: derivar la llave puede pedir hasta 256 MiB de Argon2 (límite del
	// formato). El trabajador ya corre una corrida a la vez; esto lo asegura aunque otra parte del servidor abra un
	// paquete. OpenOptions vacío = los límites predeterminados completos del formato.
	openMutex.Lock()
	reader, err := legacypkg.Open(bufio.NewReaderSize(file, 1<<20), passphrase, legacypkg.OpenOptions{})
	openMutex.Unlock()
	if err != nil {
		file.Close()
		return nil, nil, packageError(err)
	}
	return newPackageSource(reader), file, nil
}

func newPackageSource(reader *legacypkg.Reader) *packageSource {
	manifest := reader.Manifest()
	info := PackageInfo{
		FormatVersion:  int(manifest.FormatVersion),
		Tool:           manifest.CreatedBy.Tool,
		ProductVersion: manifest.CreatedBy.Version,
		Fingerprint:    manifest.ContentFingerprint,
		BackupTakenAt:  manifest.Source.BackupFinishedAt,
		Tables:         map[string]int64{},
		SkippedTables:  map[string]int64{},
	}
	if info.BackupTakenAt == "" {
		info.BackupTakenAt = manifest.Source.BackupStartedAt
	}
	if created, err := time.Parse(time.RFC3339, manifest.CreatedAt); err == nil {
		info.CreatedAt = created
	}
	if encoded, err := json.Marshal(manifest.Source); err == nil {
		info.Source = encoded
	}
	for _, table := range manifest.Tables {
		info.Tables[table.Name] = table.Rows
	}
	for _, table := range manifest.SkippedTables {
		info.SkippedTables[table.Name] = table.Rows
	}
	return &packageSource{reader: reader, info: info}
}

func (source *packageSource) Info() PackageInfo {
	return source.info
}

func (source *packageSource) NextTable() (Table, error) {
	table, err := source.reader.NextTable()
	if errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if err != nil {
		return nil, packageError(err)
	}
	columns := make([]string, len(table.Columns()))
	for index, column := range table.Columns() {
		columns[index] = column.Name
	}
	return &packageTable{table: table, columns: columns}, nil
}

func (table *packageTable) Name() string {
	return table.table.Info().Name
}

func (table *packageTable) Columns() []string {
	return table.columns
}

func (table *packageTable) Rows() int64 {
	return table.table.Info().Rows
}

func (table *packageTable) Next() ([]any, error) {
	values, err := table.table.Next()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, packageError(err)
	}
	return values, err
}

// packageError traduce un error del formato a un error de la migración con el mensaje del formato (ya en español
// sencillo y sin datos del paquete). Detail lleva solo el detalle técnico del formato (Error() de legacypkg, que nunca
// cita contenido); la causa interna (Unwrap) se conserva para errors.Is pero nunca se registra.
func packageError(err error) error {
	var typed *legacypkg.Error
	if !errors.As(err, &typed) {
		return &Error{Code: CodeDamaged, Message: "El paquete está dañado o no se pudo leer. Vuelve a copiarlo o prepáralo de nuevo.",
			Detail: "error de lectura del paquete", cause: err}
	}
	codes := map[legacypkg.ErrorKind]string{
		legacypkg.KindNotPackage:         CodeNotPackage,
		legacypkg.KindUnsupportedVersion: CodeUnsupportedVersion,
		legacypkg.KindWrongPassphrase:    CodeWrongPassphrase,
		legacypkg.KindDamaged:            CodeDamaged,
		legacypkg.KindLimit:              CodeLimit,
		legacypkg.KindInvalid:            CodeWrongPassphrase,
	}
	code, found := codes[typed.Kind]
	if !found {
		code = CodeDamaged
	}
	return &Error{Code: code, Message: typed.Message, Detail: typed.Detail, cause: err}
}
