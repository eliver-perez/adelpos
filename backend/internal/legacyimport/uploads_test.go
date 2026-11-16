package legacyimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLegacyUploadsInParts(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	manager := &Manager{WorkDir: t.TempDir(), MaxUploadBytes: 3 << 20, Now: func() time.Time {
		return now
	}}
	actor := Actor{UserID: "u", DeviceID: "d"}
	other := Actor{UserID: "otro", DeviceID: "d"}
	content := bytes.Repeat([]byte("paquete sintético "), 150000)
	digest := sha256.Sum256(content)
	if _, err := manager.CreateUpload(actor, 4<<20, hex.EncodeToString(digest[:])); CodeOf(err) != CodeUploadTooLarge {
		t.Fatalf("tope de tamaño: %v", err)
	}
	view, err := manager.CreateUpload(actor, int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil || view.Parts != 3 {
		t.Fatalf("subida: %+v %v", view, err)
	}
	part := func(index int) ([]byte, string) {
		end := (index + 1) * PartSize
		if end > len(content) {
			end = len(content)
		}
		data := content[index*PartSize : end]
		sum := sha256.Sum256(data)
		return data, hex.EncodeToString(sum[:])
	}
	data, sum := part(0)
	if _, err := manager.AddPart(other, view.ID, 0, data, sum); CodeOf(err) != CodeUploadMissing {
		t.Fatalf("otro usuario: %v", err)
	}
	if _, err := manager.AddPart(actor, view.ID, 0, data, strings.Repeat("0", 64)); CodeOf(err) != CodeUploadHash {
		t.Fatalf("huella mala: %v", err)
	}
	second, secondSum := part(1)
	if _, err := manager.AddPart(actor, view.ID, 1, second, secondSum); CodeOf(err) != CodeUploadOrder {
		t.Fatalf("fuera de orden: %v", err)
	}
	if view, err = manager.AddPart(actor, view.ID, 0, data, sum); err != nil || view.ReceivedParts != 1 {
		t.Fatalf("primera parte: %+v %v", view, err)
	}
	if view, err = manager.AddPart(actor, view.ID, 0, data, sum); err != nil || view.ReceivedParts != 1 {
		t.Fatalf("reenviar la última parte no cambia nada: %+v %v", view, err)
	}
	if _, err := manager.AddPart(actor, view.ID, 1, second[:100], hex.EncodeToString(sha256.New().Sum(nil))); CodeOf(err) != CodeUploadHash {
		t.Fatalf("parte corta con huella mala: %v", err)
	}
	if _, err = manager.AddPart(actor, view.ID, 1, second, secondSum); err != nil {
		t.Fatal(err)
	}
	last, lastSum := part(2)
	if view, err = manager.AddPart(actor, view.ID, 2, last, lastSum); err != nil || view.Status != UploadComplete {
		t.Fatalf("última parte: %+v %v", view, err)
	}
	// Una subida cuyo contenido completo no coincide con la huella anunciada se descarta.
	wrong, err := manager.CreateUpload(actor, int64(len(last)), strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddPart(actor, wrong.ID, 0, last, lastSum); CodeOf(err) != CodeUploadHash {
		t.Fatalf("huella del paquete: %v", err)
	}
	if _, err := manager.GetUpload(actor, wrong.ID); CodeOf(err) != CodeUploadMissing {
		t.Fatal("la subida con huella mala debía descartarse")
	}
	// Vencimiento y limpieza fuera de la petición.
	if removed := manager.cleanExpired(); removed != 0 {
		t.Fatalf("nada vencido todavía: %d", removed)
	}
	now = now.Add(DefaultUploadTTL + time.Minute)
	if removed := manager.cleanExpired(); removed != 1 {
		t.Fatalf("subidas vencidas: %d", removed)
	}
	entries, _ := os.ReadDir(manager.WorkDir)
	if len(entries) != 0 {
		t.Fatalf("quedaron archivos en la carpeta de trabajo: %v", entries)
	}
}

// Una escritura que falló a medias (disco lleno) deja bytes de más en el archivo parcial; al reenviar la parte, se
// escribe en su lugar y el paquete completo tiene el tamaño y la huella del original. La huella final sale del archivo:
// si el disco quedó con otros bytes, la subida se descarta (revisión de la Fase C, M-3).
func TestLegacyUploadWritesPartsInPlace(t *testing.T) {
	manager := &Manager{WorkDir: t.TempDir(), MaxUploadBytes: 3 << 20}
	actor := Actor{UserID: "u", DeviceID: "d"}
	content := bytes.Repeat([]byte("paquete sintético en su lugar "), 80000)
	digest := sha256.Sum256(content)
	part := func(index int) ([]byte, string) {
		end := (index + 1) * PartSize
		if end > len(content) {
			end = len(content)
		}
		data := content[index*PartSize : end]
		sum := sha256.Sum256(data)
		return data, hex.EncodeToString(sum[:])
	}
	appendGarbage := func(path string, size int) {
		t.Helper()
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(bytes.Repeat([]byte{0xEE}, size)); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	view, err := manager.CreateUpload(actor, int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil || view.Parts != 3 {
		t.Fatalf("subida: %+v %v", view, err)
	}
	first, firstSum := part(0)
	if view, err = manager.AddPart(actor, view.ID, 0, first, firstSum); err != nil {
		t.Fatal(err)
	}
	// Un intento anterior de la parte 1 que escribió solo una parte de sus bytes antes de fallar.
	second, secondSum := part(1)
	appendGarbage(manager.partialPath(view.ID), 777)
	if view, err = manager.AddPart(actor, view.ID, 1, second, secondSum); err != nil {
		t.Fatal(err)
	}
	// Y basura después de la parte 1, antes de la última.
	appendGarbage(manager.partialPath(view.ID), PartSize/2)
	last, lastSum := part(2)
	if view, err = manager.AddPart(actor, view.ID, 2, last, lastSum); err != nil || view.Status != UploadComplete {
		t.Fatalf("última parte: %+v %v", view, err)
	}
	stored, err := os.ReadFile(manager.completePath(view.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(content) || sha256.Sum256(stored) != digest {
		t.Fatalf("el paquete guardado tiene %d bytes (se esperaban %d) o su huella no coincide", len(stored), len(content))
	}
	// Si el disco quedó con otros bytes que los recibidos, la huella del archivo no coincide y la subida se descarta.
	damaged, err := manager.CreateUpload(actor, int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.AddPart(actor, damaged.ID, 0, first, firstSum); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.AddPart(actor, damaged.ID, 1, second, secondSum); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(manager.partialPath(damaged.ID), os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{0x00, 0x01}, 10); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddPart(actor, damaged.ID, 2, last, lastSum); CodeOf(err) != CodeUploadHash {
		t.Fatalf("archivo con otros bytes en el disco: %v", err)
	}
	if _, err := manager.GetUpload(actor, damaged.ID); CodeOf(err) != CodeUploadMissing {
		t.Fatal("la subida con el archivo dañado debía descartarse")
	}
}
