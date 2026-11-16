package legacypkg

// Limits acota lo que la lectura acepta de un paquete, para que uno manipulado no agote la memoria, el disco ni el
// tiempo de la computadora que lo importa. Los valores predeterminados dejan mucho margen sobre un paquete real (el del
// restaurante, unas 52 tablas y unos cientos de MB de JSON).
type Limits struct {
	// MaxArgonMemoryKiB y MaxArgonTime acotan el costo de derivar la llave (Argon2id) antes de saber si la frase es
	// correcta; MaxArgonThreads, los hilos.
	MaxArgonMemoryKiB uint32
	MaxArgonTime      uint32
	MaxArgonThreads   uint8
	// MaxChunkSize acota el tamaño de bloque del cifrado (memoria de un bloque).
	MaxChunkSize int
	// MaxUncompressedBytes acota el total descomprimido del tar (bomba de descompresión).
	MaxUncompressedBytes int64
	// MaxManifestBytes acota manifest.json, que se lee completo en memoria.
	MaxManifestBytes int64
	// MaxTableBytes acota cada tables/<tabla>.json.
	MaxTableBytes int64
	// MaxTables y MaxColumns acotan el número de tablas y de columnas por tabla del manifiesto.
	MaxTables  int
	MaxColumns int
	// MaxRowBytes acota el JSON de una fila (la lectura es en flujo, pero una fila se decodifica completa).
	MaxRowBytes int64
}

// Parámetros del formato y sus límites.
const (
	// MinPassphraseLength es el mínimo de caracteres de la frase de paso que elige el propietario.
	MinPassphraseLength = 8
	// MaxPassphraseBytes acota la frase de paso (se lee de la entrada estándar).
	MaxPassphraseBytes = 1024
	// DefaultChunkSize es el tamaño de bloque del cifrado (1 MiB).
	DefaultChunkSize = 1 << 20
	// MinChunkSize es el bloque más chico que se acepta.
	MinChunkSize = 4 << 10
	// minArgonMemoryKiB y minArgonTime son los mínimos que se aceptan (las pruebas usan los mínimos para ir rápido).
	minArgonMemoryKiB = 8 << 10
	minArgonTime      = 1
	// maxHeaderParamsBytes acota el JSON de parámetros del encabezado.
	maxHeaderParamsBytes = 4 << 10
)

// DefaultLimits son los límites de la lectura.
func DefaultLimits() Limits {
	return Limits{
		MaxArgonMemoryKiB:    256 << 10,
		MaxArgonTime:         10,
		MaxArgonThreads:      16,
		MaxChunkSize:         8 << 20,
		MaxUncompressedBytes: 16 << 30,
		MaxManifestBytes:     16 << 20,
		MaxTableBytes:        8 << 30,
		MaxTables:            1000,
		MaxColumns:           1024,
		MaxRowBytes:          4 << 20,
	}
}

// KDFParams son los parámetros de Argon2id con que se deriva la llave de la frase de paso.
type KDFParams struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultKDF son los parámetros con que se escribe un paquete: 3 pasadas, 64 MiB y 4 hilos (unas décimas de segundo).
func DefaultKDF() KDFParams {
	return KDFParams{Time: 3, MemoryKiB: 64 << 10, Threads: 4}
}
