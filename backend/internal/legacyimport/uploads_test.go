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
