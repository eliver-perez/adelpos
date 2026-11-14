//go:build !windows

package standalone

// RealDataDir es la carpeta de datos del servicio (SystemDataDir), la misma aunque ADEL_DATA_DIR o
// --data-dir apunten a otra parte.
func RealDataDir(productName string) (string, error) {
	return canonicalPath(SystemDataDir(productName)), nil
}

// testModeHint explica cómo usar otra carpeta de datos en macOS.
const testModeHint = "indica otra carpeta con --data-dir o ADEL_DATA_DIR"
