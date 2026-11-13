//go:build windows

package standalone

import (
	"path/filepath"
	"strings"
	"testing"
)

// Etapa 9F (D1): el modo de pruebas rechaza la carpeta de datos real aunque LOCALAPPDATA apunte a otra
// parte, y acepta cualquier otra carpeta.
func TestRefuseRealDataDirUsesTheKnownFolder(t *testing.T) {
	real, err := RealDataDir("AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(real, `\AD-EL POS`) {
		t.Fatalf("carpeta real inesperada: %s", real)
	}
	// La variable no cambia la carpeta conocida.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	again, err := RealDataDir("AD-EL POS")
	if err != nil || !samePath(again, real) {
		t.Fatalf("la carpeta real dependió de LOCALAPPDATA: %s / %s", again, real)
	}
	if err := RefuseRealDataDir(Layout{DataDir: real}, "AD-EL POS"); err == nil {
		t.Fatal("el modo de pruebas aceptó la carpeta de datos real")
	}
	if err := RefuseRealDataDir(Layout{DataDir: strings.ToUpper(real)}, "AD-EL POS"); err == nil {
		t.Fatal("el modo de pruebas aceptó la carpeta real escrita con otras mayúsculas")
	}
	temporary := filepath.Join(t.TempDir(), "AD-EL POS")
	if err := RefuseRealDataDir(Layout{DataDir: temporary}, "AD-EL POS"); err != nil {
		t.Fatalf("el modo de pruebas rechazó una carpeta temporal: %v", err)
	}
}
