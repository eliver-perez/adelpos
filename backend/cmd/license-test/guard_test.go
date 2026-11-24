//go:build !adelpos_production

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Las herramientas de prueba de las licencias nunca llegan a un instalador ni a una compilación de producción:
//
//  1. el servidor (./cmd/ad-el-pos-server) no depende de internal/licensetest ni de esta orden;
//  2. los guiones que arman el .pkg, el instalador de Windows y el binario de desarrollo solo compilan
//     ./cmd/ad-el-pos-server y no nombran estas herramientas;
//  3. con la etiqueta de producción no compilan (todas sus fuentes llevan //go:build !adelpos_production).
func TestTestToolsNeverShip(t *testing.T) {
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBinary += ".exe"
	}
	if _, err := os.Stat(goBinary); err != nil {
		t.Skipf("no está el compilador de Go: %v", err)
	}
	backend := filepath.Join("..", "..")
	command := func(arguments ...string) (string, error) {
		process := exec.Command(goBinary, arguments...)
		process.Dir = backend
		process.Env = os.Environ()
		output, err := process.CombinedOutput()
		return string(output), err
	}

	deps, err := command("list", "-deps", "./cmd/ad-el-pos-server")
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, deps)
	}
	for _, forbidden := range []string{"internal/licensetest", "cmd/license-test"} {
		if strings.Contains(deps, forbidden) {
			t.Fatalf("el servidor depende de %s", forbidden)
		}
	}

	repository := filepath.Join(backend, "..")
	build := regexp.MustCompile(`build[^\n]*\./cmd/[a-z-]+`)
	for _, script := range []string{
		"scripts/build-macos-pkg.sh", "scripts/build-windows.ps1", "scripts/windows/dev.mjs", "scripts/dev.sh",
	} {
		raw, err := os.ReadFile(filepath.Join(repository, script))
		if err != nil {
			t.Fatalf("%s: %v", script, err)
		}
		text := string(raw)
		if strings.Contains(text, "license-test") || strings.Contains(text, "licensetest") {
			t.Fatalf("%s nombra las herramientas de prueba de las licencias", script)
		}
		lines := build.FindAllString(text, -1)
		if len(lines) == 0 {
			t.Fatalf("%s: no se encontró la compilación del servidor (¿cambió el guion?)", script)
		}
		for _, line := range lines {
			if !strings.HasSuffix(line, "./cmd/ad-el-pos-server") {
				t.Fatalf("%s compila otro programa: %s", script, line)
			}
		}
	}

	for _, target := range []string{"./cmd/license-test", "./internal/licensetest"} {
		output, err := command("build", "-tags", "adelpos_production", target)
		if err == nil || !strings.Contains(output, "build constraints exclude all Go files") {
			t.Fatalf("%s compiló con la etiqueta de producción:\n%s", target, output)
		}
	}
}
