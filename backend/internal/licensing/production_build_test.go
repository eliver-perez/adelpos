package licensing

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// §1.2 de la especificación: una compilación de producción sin llaves de producción falla al compilar, así
// que no puede salir un instalador de producción por accidente. Se comprueba de verdad con `go build -tags
// adelpos_production`, que debe fallar por la falta de llaves-production.json y por nada más: con un archivo
// de llaves de producción simulado (un overlay de go build, que no toca la carpeta) compila.
func TestProductionBuildFailsWithoutProductionKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("compila el servidor")
	}
	if _, err := os.Stat("llaves-production.json"); err == nil {
		t.Skip("ya hay llaves de producción: esta prueba se retira en el paso a producción")
	}
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBinary += ".exe"
	}
	if _, err := os.Stat(goBinary); err != nil {
		t.Skipf("no está el compilador de Go: %v", err)
	}
	build := func(arguments ...string) (string, error) {
		command := exec.Command(goBinary, append([]string{"build", "-tags", "adelpos_production"}, arguments...)...)
		command.Dir = filepath.Join("..", "..")
		command.Env = os.Environ()
		output, err := command.CombinedOutput()
		return string(output), err
	}

	output, err := build("./internal/licensing", "./cmd/ad-el-pos-server")
	if err == nil {
		t.Fatal("la compilación de producción compiló sin llaves de producción")
	}
	if !strings.Contains(output, "llaves-production.json") && !strings.Contains(output, "soporte-production.pub.json") {
		t.Fatalf("la compilación de producción falló por otra causa:\n%s", output)
	}

	// Con llaves de producción (simuladas, fuera de la carpeta) compila: el único faltante son las llaves. Desde la
	// Fase C también la pública de soporte (§6.4): sin ella tampoco compila.
	directory := t.TempDir()
	public := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	simulated := map[string]string{
		"llaves-production.json":      `{"environment":"production","keys":[{"kid":"prueba-de-compilacion","environment":"production","public_key":"` + public + `"}]}`,
		"soporte-production.pub.json": `{"environment":"production","keys":[{"kid":"prueba-de-compilacion","public_key":"` + public + `"}]}`,
	}
	overlayWith := func(names ...string) string {
		replace := map[string]string{}
		for _, name := range names {
			source := filepath.Join(directory, name)
			if err := os.WriteFile(source, []byte(simulated[name]), 0600); err != nil {
				t.Fatal(err)
			}
			target, err := filepath.Abs(name)
			if err != nil {
				t.Fatal(err)
			}
			replace[target] = source
		}
		overlay, _ := json.Marshal(map[string]map[string]string{"Replace": replace})
		overlayFile := filepath.Join(directory, strings.Join(names, "+")+".overlay.json")
		if err := os.WriteFile(overlayFile, overlay, 0600); err != nil {
			t.Fatal(err)
		}
		return overlayFile
	}
	output, err = build("-overlay", overlayWith("llaves-production.json"), "./internal/licensing", "./cmd/ad-el-pos-server")
	if err == nil || !strings.Contains(output, "soporte-production.pub.json") {
		t.Fatalf("sin la llave pública de soporte de producción no debe compilar:\n%s", output)
	}
	if output, err := build("-overlay", overlayWith("llaves-production.json", "soporte-production.pub.json"),
		"./internal/licensing", "./cmd/ad-el-pos-server"); err != nil {
		t.Fatalf("con llaves de producción, la compilación de producción debe compilar:\n%s", output)
	}
}
