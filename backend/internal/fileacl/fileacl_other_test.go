//go:build !windows

package fileacl

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Etapa 9G (revisión de la fase F): un respaldo nace privado (0600 aunque la máscara del proceso sea 0), sin seguir un
// enlace ni reemplazar un archivo existente.
func TestCreatePrivateIsPrivateExclusiveAndDoesNotFollowLinks(t *testing.T) {
	previous := syscall.Umask(0)
	defer syscall.Umask(previous)
	directory := t.TempDir()
	path := filepath.Join(directory, "respaldo.dump.partial")
	file, err := CreatePrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permisos desde el primer byte: %v %v", info.Mode().Perm(), err)
	}
	if _, err := file.WriteString("PGDMP"); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := CreatePrivate(path); err == nil {
		t.Fatal("se reemplazó un archivo existente")
	}
	victim := filepath.Join(directory, "otro.txt")
	if err := os.WriteFile(victim, []byte("intacto"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "enlace.dump.partial")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePrivate(link); err == nil {
		t.Fatal("se siguió un enlace")
	}
	dangling := filepath.Join(directory, "colgante.dump.partial")
	if err := os.Symlink(filepath.Join(directory, "no-existe"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePrivate(dangling); err == nil {
		t.Fatal("se siguió un enlace colgante")
	}
	if _, err := os.Stat(filepath.Join(directory, "no-existe")); err == nil {
		t.Fatal("se creó el destino del enlace colgante")
	}
	content, err := os.ReadFile(victim)
	if err != nil || string(content) != "intacto" {
		t.Fatal("se modificó el archivo al que apuntaba el enlace")
	}
}
