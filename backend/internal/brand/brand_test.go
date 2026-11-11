package brand

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// La copia embebida es idéntica byte a byte al original de assets/brand (mismo SHA-256 y mismos
// bytes) y es un PNG de 2048 × 682 con transparencia.
func TestEmbeddedLogoMatchesRepositoryOriginal(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "assets", "brand", LightLogoFile))
	if err != nil {
		t.Fatalf("No se encontró el original de la marca: %v", err)
	}
	embedded := LightLogoPNG()
	originalDigest := sha256.Sum256(original)
	embeddedDigest := sha256.Sum256(embedded)
	if originalDigest != embeddedDigest || !bytes.Equal(original, embedded) {
		t.Fatalf("La copia embebida (%x) difiere del original (%x): vuelve a copiar assets/brand/%s",
			embeddedDigest, originalDigest, LightLogoFile)
	}
	if hex.EncodeToString(originalDigest[:]) != LightLogoSHA256 {
		t.Fatalf("LightLogoSHA256 no corresponde al original: %x", originalDigest)
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(embedded))
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Width != 2048 || configuration.Height != 682 {
		t.Fatalf("Dimensiones de la marca: %d × %d", configuration.Width, configuration.Height)
	}
	if _, _, err := image.Decode(bytes.NewReader(embedded)); err != nil {
		t.Fatalf("La marca embebida no se decodifica: %v", err)
	}
}

// LightLogoPNG entrega copias: modificar una no altera la versión embebida.
func TestLightLogoReturnsCopies(t *testing.T) {
	first := LightLogoPNG()
	first[0] = 0
	second := LightLogoPNG()
	if second[0] != 0x89 {
		t.Fatal("Modificar la copia alteró la marca embebida")
	}
}
