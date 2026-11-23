package licensing

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// §9.1 de la especificación: ninguna prueba automática llama al servidor de licencias real. Se comprueba de dos
// formas, igual que la protección contra el servicio real de la Mac:
//
//  1. con test_mode el cliente rechaza cualquier destino que no sea loopback, empezando por la dirección del entorno
//     compilado, sin conectarse (TestClientRefusesRemoteServersInTestMode y TestTestModeDialRefusesRemoteAddresses);
//  2. ningún archivo de pruebas del repositorio (Go, el escritorio, Playwright, Android ni los guiones de las pruebas)
//     nombra el servidor real, y las configuraciones de ejemplo no lo fijan como license_server_url.
//
// La Fase G, la única autorizada, corre a mano con ADEL_LICENSE_ALLOW_REMOTE=1 y no es una prueba automática.
func TestNoAutomatedTestCallsTheRealLicenseServer(t *testing.T) {
	// El nombre se arma por partes para que este archivo tampoco lo contenga.
	forbidden := strings.Join([]string{"licensing", "sicrat", "com"}, ".")
	if !strings.Contains(DefaultServerURL, forbidden) && BuildEnvironment == "development" {
		t.Fatal("la búsqueda debe usar el mismo nombre que la dirección del entorno de pruebas")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	skipped := map[string]bool{"node_modules": true, ".git": true, ".gradle": true, "build": true, "target": true, "dist": true, ".tools": true, ".cache": true, ".local": true}
	checked := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if skipped[entry.Name()] || strings.HasPrefix(relative, "contracts/") || relative == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !isAutomatedTestFile(relative) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("%s nombra el servidor de licencias real: ninguna prueba automática puede llamarlo", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Si el recorrido no encontrara las pruebas (otra carpeta de trabajo), no probaría nada.
	if checked < 200 {
		t.Fatalf("solo se revisaron %d archivos de pruebas en %s", checked, root)
	}
}

// isAutomatedTestFile: las pruebas de Go, del escritorio (vitest y Playwright), de Android, de Rust y los guiones y
// configuraciones con que corren.
func isAutomatedTestFile(relative string) bool {
	name := filepath.Base(relative)
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return true
	case strings.HasSuffix(name, ".test.ts"), strings.HasSuffix(name, ".test.tsx"), strings.HasSuffix(name, ".spec.ts"):
		return true
	case strings.HasPrefix(relative, "desktop/e2e"), strings.HasPrefix(relative, "tests/"):
		return true
	case strings.HasPrefix(relative, "android/") && (strings.Contains(relative, "/src/test/") || strings.Contains(relative, "/src/androidTest/")):
		return true
	case strings.HasPrefix(relative, "desktop/src-tauri/") && strings.Contains(relative, "/tests/"):
		return true
	case strings.HasPrefix(relative, "desktop/playwright"):
		return true
	case relative == "scripts/browser-tests.py", relative == "scripts/android-e2e.sh", relative == "scripts/database.py",
		relative == "scripts/server-process.py", relative == "scripts/macos-pkg/test-scripts.sh":
		return true
	case strings.HasPrefix(relative, "config/") && strings.HasSuffix(name, ".json") && !strings.HasPrefix(relative, "config/licencias/"):
		return true
	}
	return false
}
