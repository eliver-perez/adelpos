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
	if !strings.Contains(output, "llaves-production.json") {
		t.Fatalf("la compilación de producción falló por otra causa:\n%s", output)
	}

	// Con llaves de producción (simuladas, fuera de la carpeta) compila: el único faltante son las llaves.
	directory := t.TempDir()
	keys := filepath.Join(directory, "llaves-production.json")
	public := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	body := `{"environment":"production","keys":[{"kid":"prueba-de-compilacion","environment":"production","public_key":"` + public + `"}]}`
	if err := os.WriteFile(keys, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.Abs("llaves-production.json")
	if err != nil {
		t.Fatal(err)
	}
	overlay, _ := json.Marshal(map[string]map[string]string{"Replace": {target: keys}})
	overlayFile := filepath.Join(directory, "overlay.json")
	if err := os.WriteFile(overlayFile, overlay, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := build("-overlay", overlayFile, "./internal/licensing", "./cmd/ad-el-pos-server"); err != nil {
		t.Fatalf("con llaves de producción, la compilación de producción debe compilar:\n%s", output)
	}
}
