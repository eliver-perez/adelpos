package apicontract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Regla de textos de la etapa A3 (§1 de docs/etapas/ETAPA_A3_AJUSTES.md): ningún texto provisional ni de desarrollo
// llega a la pantalla. El escritorio y la aplicación Android muestran tal cual el «message» de los errores de Go (y
// muchos textos más, como los avisos y los nombres de los estados), así que esta prueba recorre las cadenas de todo
// backend/internal, sin las pruebas, y busca las frases típicas. Las cadenas se leen con go/parser: los comentarios no
// cuentan. Las mismas frases las buscan desktop/src/pages/plainLanguage.test.tsx en el escritorio y la prueba de textos
// de la aplicación Android.

// developmentPhrases son las frases que delatan un texto provisional, una nota de etapas o de desarrollo.
var developmentPhrases = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{name: "«todavía no»", pattern: regexp.MustCompile(`(?i)todav[ií]a no\b`)},
	{name: "«aún no»", pattern: regexp.MustCompile(`(?i)\ba[uú]n no\b`)},
	{name: "«etapa»", pattern: regexp.MustCompile(`(?i)\betapa\b`)},
	{name: "«fase»", pattern: regexp.MustCompile(`(?i)\bfase [0-9a-z]\b`)},
	{name: "«próximamente»", pattern: regexp.MustCompile(`(?i)pr[oó]ximamente`)},
	{name: "«provisional»", pattern: regexp.MustCompile(`(?i)provisional`)},
	{name: "«por ahora»", pattern: regexp.MustCompile(`(?i)\bpor ahora\b`)},
	{name: "«más adelante»", pattern: regexp.MustCompile(`(?i)\bm[aá]s adelante\b`)},
	{name: "«en desarrollo»", pattern: regexp.MustCompile(`(?i)\ben desarrollo\b`)},
	{name: "«versión de prueba»", pattern: regexp.MustCompile(`(?i)\bversi[oó]n de prueba\b`)},
	{name: "«(muestra)»", pattern: regexp.MustCompile(`(?i)\(muestra\)`)},
}

// allowedDevelopmentPhrases son las únicas excepciones, por archivo (relativo a backend/internal) y frase, cada una con
// su porqué: agregar una exige escribir el motivo. Una excepción que deja de hacer falta hace fallar la prueba, para
// que la lista no crezca sin control.
var allowedDevelopmentPhrases = []struct {
	file   string
	phrase string
	reason string
}{
	{
		file:   "inventory/costing.go",
		phrase: "«provisional»",
		reason: "«provisional» es el código de la base del costo que se guarda en la base de datos (costo estimado " +
			"de un faltante); la interfaz nunca lo muestra crudo",
	},
	{
		file:   "inventory/movements.go",
		phrase: "«provisional»",
		reason: "«Valor provisional del faltante» es el término del costo de inventario (el valor estimado de un " +
			"faltante), no una nota de desarrollo",
	},
	{
		file:   "reports/costs.go",
		phrase: "«provisional»",
		reason: "estado del costo de una unidad en las consultas y «Costo provisional»: el término del costo " +
			"estimado de inventario",
	},
	{
		file:   "kitchen/coverage.go",
		phrase: "«por ahora»",
		reason: "aviso unrouted_station_inactive de la cobertura, con el texto fijado en el contrato de la etapa K1.1 " +
			"(«…que está desactivada: por ahora no necesitan preparación.»): dice qué pasa mientras la estación de " +
			"«Productos sin ruta» siga desactivada, no es una nota de desarrollo",
	},
	{
		file:   "standalone/adopt.go",
		phrase: "«en desarrollo»",
		reason: "mensaje de la orden adopt-legacy en la terminal de la Mac, que nunca llega a la interfaz; «en " +
			"desarrollo» nombra el entorno de desarrollo y su orden para detener el servidor",
	},
}

func TestPlainLanguageMessages(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	findings, used, files := developmentPhraseFindings(t, root)
	// Si el recorrido no encontrara las fuentes (por ejemplo, otra carpeta de trabajo), la prueba no probaría nada.
	if files < 300 {
		t.Fatalf("solo se leyeron %d archivos de Go en %s", files, root)
	}
	for _, finding := range findings {
		t.Errorf("texto provisional o de desarrollo: %s", finding)
	}
	for index, allowed := range allowedDevelopmentPhrases {
		if !used[index] {
			t.Errorf("la excepción %s %s ya no hace falta: quítala de allowedDevelopmentPhrases", allowed.file,
				allowed.phrase)
		}
	}
}

// developmentPhraseFindings devuelve cada cadena con una frase de desarrollo que no está en las excepciones
// («archivo:línea (frase): texto»), qué excepciones se usaron y cuántos archivos se leyeron.
func developmentPhraseFindings(t *testing.T, root string) ([]string, map[int]bool, int) {
	t.Helper()
	fileSet := token.NewFileSet()
	used := map[int]bool{}
	var findings []string
	files := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		// Sin parser.ParseComments: los comentarios no forman parte del árbol y no se revisan.
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Errorf("%s: no se pudo leer la cadena %s: %v", relative, literal.Value, err)
				return true
			}
			for _, phrase := range developmentPhrases {
				if !phrase.pattern.MatchString(text) {
					continue
				}
				if index := allowedDevelopmentPhrase(relative, phrase.name); index >= 0 {
					used[index] = true
					continue
				}
				line := fileSet.Position(literal.Pos()).Line
				findings = append(findings, fmt.Sprintf("%s:%d (%s): %s", relative, line, phrase.name, text))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return findings, used, files
}

func allowedDevelopmentPhrase(file, phrase string) int {
	for index, allowed := range allowedDevelopmentPhrases {
		if allowed.file == file && allowed.phrase == phrase {
			return index
		}
	}
	return -1
}
