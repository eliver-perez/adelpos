package legacyread

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	readerImport  = "local/ad-el-pos/server/internal/legacyread"
	allowedImport = "cmd/ad-el-pos-server/legacy.go"
)

// El lector (Docker y SQL Server) está separado del servidor (§3.2): el único archivo que lo importa es el del
// subcomando legacy-prepare. Si ningún otro paquete lo importa directamente, ningún módulo del servidor (serve, host,
// la API) puede depender de él ni siquiera de forma indirecta, porque el paquete main no se puede importar.
func TestOnlyTheSubcommandImportsTheReader(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	importers := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(files, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			name, _ := strconv.Unquote(imported.Path.Value)
			if name == readerImport {
				relative, _ := filepath.Rel(root, path)
				importers = append(importers, filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(importers) != 1 || importers[0] != allowedImport {
		t.Fatalf("archivos que importan el lector: %v (solo debe ser %s)", importers, allowedImport)
	}
}
