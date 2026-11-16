package legacypkg

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Comprobación de datos reales del repositorio (scripts/check-legacy-data.sh y su equivalente de Windows en
// scripts/windows/dev.mjs): con un índice de git temporal, falla con un paquete de verdad escrito por Writer (aunque
// se llame de otra forma), con un respaldo de SQL Server (firma TAPE de MTF) y con los nombres que .gitignore excluye;
// pasa con archivos de texto que solo se parecen y sin .git.

type repositoryCheck struct {
	name    string
	command func(root string) *exec.Cmd
}

func repositoryChecks(t *testing.T) []repositoryCheck {
	t.Helper()
	project, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
	checks := []repositoryCheck{}
	if runtime.GOOS != "windows" {
		if bash, err := exec.LookPath("bash"); err == nil {
			script := filepath.Join(project, "scripts", "check-legacy-data.sh")
			checks = append(checks, repositoryCheck{name: "check-legacy-data.sh", command: func(root string) *exec.Cmd {
				return exec.Command(bash, script, root)
			}})
		}
	}
	if node, err := exec.LookPath("node"); err == nil {
		script := filepath.Join(project, "scripts", "windows", "dev.mjs")
		checks = append(checks, repositoryCheck{name: "dev.mjs", command: func(root string) *exec.Cmd {
			return exec.Command(node, script, "check-legacy-data", root)
		}})
	} else {
		t.Log("sin node en el PATH: no se prueba el equivalente de Windows")
	}
	if len(checks) == 0 {
		t.Skip("sin bash ni node")
	}
	return checks
}

func gitEnvironment(home string) []string {
	return append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig"))
}

func newIndex(t *testing.T, files map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, arguments := range [][]string{{"init", "-q"}, {"add", "--force", "."}} {
		command := exec.Command("git", arguments...)
		command.Dir = root
		command.Env = gitEnvironment(home)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", arguments, err, output)
		}
	}
	return root
}

func runCheck(t *testing.T, check repositoryCheck, root string) (bool, string) {
	t.Helper()
	command := check.command(root)
	command.Env = gitEnvironment(t.TempDir())
	output, err := command.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("%s: %v", check.name, err)
		}
		return false, string(output)
	}
	return true, string(output)
}

func TestRepositoryCheckRejectsLegacyData(t *testing.T) {
	checks := repositoryChecks(t)
	encoded, _ := buildPackage(t, []testTable{{spec: salesSpec(), rows: salesRows(5)}}, nil)
	backup := append([]byte("TAPE\x00\x00\x03\x00"), make([]byte, 120)...)
	clean := map[string][]byte{
		"README.md":           []byte("# Proyecto\n"),
		"notas/cinta.txt":     []byte("TAPE en un texto que solo se parece\n"),
		"docs/formato.md":     []byte("Encabezado\nADELMIG seguido de un byte NUL\n"),
		"datos/imagen.png":    {0x89, 'P', 'N', 'G', 0, 0},
		"legacy-data-ok.md":   []byte("no es la carpeta\n"),
		"scripts/respaldo.sh": []byte("#!/bin/sh\necho TAPE\n"),
	}
	cases := map[string]struct {
		file     string
		content  []byte
		expected string
	}{
		"paquete con otro nombre":  {"datos/respaldo-del-mes.bin", encoded, "paquete de migración"},
		"respaldo con otro nombre": {"copias/possys.dat", backup, "respaldo de SQL Server"},
		"nombre .BAK":              {"Notas.BAK", []byte("texto"), "(nombre)"},
		"nombre .adelmig":          {"exportes/migracion.adelmig", []byte("texto"), "(nombre)"},
		"carpeta legacy-data":      {"herramientas/legacy-data/tabla.json", []byte("[]"), "(nombre)"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if ok, output := runCheck(t, check, newIndex(t, clean)); !ok {
				t.Fatalf("falló con archivos limpios: %s", output)
			}
			for name, testCase := range cases {
				files := map[string][]byte{testCase.file: testCase.content}
				for path, content := range clean {
					files[path] = content
				}
				ok, output := runCheck(t, check, newIndex(t, files))
				if ok || !strings.Contains(output, testCase.file) || !strings.Contains(output, testCase.expected) {
					t.Fatalf("%s: aprobado=%v, salida %q", name, ok, output)
				}
				if bytes.Contains([]byte(output), encoded[:16]) {
					t.Fatal("la salida repite el contenido del archivo")
				}
			}
			ok, output := runCheck(t, check, t.TempDir())
			if !ok || !strings.Contains(output, "sin .git") {
				t.Fatalf("sin .git: aprobado=%v, salida %q", ok, output)
			}
		})
	}
}
