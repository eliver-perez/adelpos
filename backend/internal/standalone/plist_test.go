//go:build !windows

package standalone

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// decodePlist convierte un plist XML en valores de Go (map, slice, string, int y bool): basta para
// comprobar el servicio sin depender de plutil.
func decodePlist(t *testing.T, text string) map[string]any {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			t.Fatalf("plist sin diccionario: %v", err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "dict" {
			value, err := plistValue(decoder, start)
			if err != nil {
				t.Fatal(err)
			}
			return value.(map[string]any)
		}
	}
}

func plistValue(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "true", "false":
		if err := decoder.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	case "string", "key":
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			return nil, err
		}
		return text, nil
	case "integer":
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			return nil, err
		}
		return strconv.Atoi(strings.TrimSpace(text))
	case "array":
		values := []any{}
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch element := token.(type) {
			case xml.StartElement:
				value, err := plistValue(decoder, element)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			case xml.EndElement:
				return values, nil
			}
		}
	case "dict":
		values := map[string]any{}
		key := ""
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch element := token.(type) {
			case xml.StartElement:
				value, err := plistValue(decoder, element)
				if err != nil {
					return nil, err
				}
				if element.Name.Local == "key" {
					key = value.(string)
					continue
				}
				values[key] = value
			case xml.EndElement:
				return values, nil
			}
		}
	}
	return nil, errors.New("elemento de plist inesperado: " + start.Name.Local)
}

// TestLaunchDaemonTemplateMatchesTheServer comprueba el plist real del servicio (ops/launchd) con los
// valores que pone el postinstall: el usuario que instala, el servidor dentro de la aplicación con el
// comando host y la carpeta de datos, RunAtLoad, KeepAlive solo por el archivo de habilitación que vigila
// el servidor (mode.go), el intervalo de relanzamiento y las bitácoras en logs.
func TestLaunchDaemonTemplateMatchesTheServer(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "ops", "launchd", "local.adel.pos.server.plist"))
	if err != nil {
		t.Fatal(err)
	}
	dataDir := "/Library/Application Support/AD-EL POS"
	server := "/Applications/AD-EL POS.app/Contents/MacOS/ad-el-pos-server"
	text := strings.NewReplacer("__ADEL_USER__", "cajero", "__ADEL_SERVER__", server, "__ADEL_DATA_DIR__", dataDir).Replace(string(body))
	if strings.Contains(text, "__ADEL_") {
		t.Fatal("la plantilla tiene marcadores desconocidos")
	}
	plist := decodePlist(t, text)
	if plist["Label"] != "local.adel.pos.server" || plist["UserName"] != "cajero" {
		t.Fatalf("etiqueta o usuario inesperados: %v", plist)
	}
	arguments, _ := plist["ProgramArguments"].([]any)
	expected := []any{server, "host", "--data-dir", dataDir}
	if len(arguments) != len(expected) {
		t.Fatalf("argumentos inesperados: %v", arguments)
	}
	for index := range expected {
		if arguments[index] != expected[index] {
			t.Fatalf("argumentos inesperados: %v", arguments)
		}
	}
	// El servidor arranca con la computadora y launchd lo mantiene vivo solo mientras exista el archivo de
	// habilitación que crea el instalador o la aplicación (Layout.EnableFile).
	layout := Layout{DataDir: dataDir}
	keepAlive, _ := plist["KeepAlive"].(map[string]any)
	pathState, _ := keepAlive["PathState"].(map[string]any)
	if plist["RunAtLoad"] != true || len(keepAlive) != 1 || len(pathState) != 1 || pathState[layout.EnableFile()] != true {
		t.Fatalf("KeepAlive debe depender solo del archivo de habilitación: %v", keepAlive)
	}
	if plist["ThrottleInterval"] != 10 || plist["WorkingDirectory"] != dataDir {
		t.Fatalf("intervalo o carpeta de trabajo inesperados: %v", plist)
	}
	if exit, _ := plist["ExitTimeOut"].(int); exit < 60 {
		t.Fatalf("el cierre ordenado de Go y PostgreSQL necesita tiempo antes de SIGKILL: %v", plist["ExitTimeOut"])
	}
	if plist["Umask"] != 63 {
		t.Fatal("los archivos del servicio deben quedar solo para su cuenta (umask 077)")
	}
	for _, key := range []string{"StandardOutPath", "StandardErrorPath"} {
		if filepath.Dir(plist[key].(string)) != layout.Logs() {
			t.Fatalf("%s fuera de la carpeta de bitácoras: %v", key, plist[key])
		}
	}
}
