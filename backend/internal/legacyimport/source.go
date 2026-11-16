package legacyimport

import (
	"encoding/json"
	"io"
	"time"
)

// Source es lo que el importador necesita de un paquete abierto: los datos del manifiesto y sus tablas en el orden en
// que vienen, cada una con sus filas. Lo implementa el lector del formato .adelmig (package.go, sobre
// internal/legacypkg) y, en las pruebas unitarias, MemorySource.
//
// NextTable devuelve io.EOF solo cuando todo el paquete se leyó y cuadró (bloques cifrados, huellas y conteos): el
// importador lee el paquete completo antes de escribir nada.
type Source interface {
	Info() PackageInfo
	NextTable() (Table, error)
}

// Table es una tabla del paquete: su nombre, sus columnas en el orden de las filas, cuántas filas dice el manifiesto
// (para el avance) y sus filas, cada una con un valor por columna (nil, bool, string o json.Number con el texto
// decimal exacto). Next devuelve io.EOF al final.
type Table interface {
	Name() string
	Columns() []string
	Rows() int64
	Next() ([]any, error)
}

// PackageInfo son los datos del manifiesto que el lote conserva y el resumen previo muestra. No llevan datos del
// restaurante: solo fechas, versiones, huellas y conteos.
type PackageInfo struct {
	FormatVersion  int
	CreatedAt      time.Time
	Tool           string
	ProductVersion string
	Fingerprint    string
	// BackupTakenAt es la fecha del respaldo de origen tal como la registró SQL Server (hora local del equipo de origen,
	// sin zona; «AAAA-MM-DDTHH:MM:SS.mmm»). Se interpreta con la zona del establecimiento.
	BackupTakenAt string
	// Source es el bloque «source» del manifiesto (tipo de origen, nombre del archivo y de la base, versión de SQL
	// Server, imagen del lector), que el lote guarda tal cual.
	Source json.RawMessage
	// Tables son las filas de cada tabla según el manifiesto; SkippedTables, las tablas que el lector no extrajo con su
	// conteo (por ejemplo personal_detalles, que no viaja por minimización).
	Tables        map[string]int64
	SkippedTables map[string]int64
}

// TotalRows es la suma de las filas de todas las tablas extraídas (el 100 % del avance de la lectura).
func (info PackageInfo) TotalRows() int64 {
	var total int64
	for _, rows := range info.Tables {
		total += rows
	}
	return total
}

// MemoryTable es una tabla en memoria para las pruebas.
type MemoryTable struct {
	TableName string
	Names     []string
	Data      [][]any
}

// MemorySource es un paquete en memoria para las pruebas unitarias (los paquetes sintéticos completos se escriben con
// internal/legacysynth y se leen con el lector real).
type MemorySource struct {
	Package PackageInfo
	Tables  []MemoryTable
	next    int
}

// Info devuelve los datos del paquete; si no traen conteos, los calcula de las tablas.
func (source *MemorySource) Info() PackageInfo {
	info := source.Package
	if info.Tables == nil {
		info.Tables = map[string]int64{}
		for _, table := range source.Tables {
			info.Tables[table.TableName] = int64(len(table.Data))
		}
	}
	return info
}

// NextTable entrega la tabla siguiente.
func (source *MemorySource) NextTable() (Table, error) {
	if source.next >= len(source.Tables) {
		return nil, io.EOF
	}
	table := &memoryTableReader{table: source.Tables[source.next]}
	source.next++
	return table, nil
}

type memoryTableReader struct {
	table MemoryTable
	next  int
}

func (reader *memoryTableReader) Name() string {
	return reader.table.TableName
}

func (reader *memoryTableReader) Columns() []string {
	return reader.table.Names
}

func (reader *memoryTableReader) Rows() int64 {
	return int64(len(reader.table.Data))
}

func (reader *memoryTableReader) Next() ([]any, error) {
	if reader.next >= len(reader.table.Data) {
		return nil, io.EOF
	}
	row := reader.table.Data[reader.next]
	reader.next++
	return row, nil
}
