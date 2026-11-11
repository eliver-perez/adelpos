//go:build integration

package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/identity"
)

// Correcciones de la revisión independiente de la etapa 9B en la identidad documental, con
// PostgreSQL:
//   - hallazgo medio 2: un JPEG con demasiados barridos se rechaza al momento y la validación del
//     archivo ocurre antes del lock de la identidad (no espera a otra transacción que tenga la fila);
//   - observación de la 0019: cada conversión vigente es de su ancho de papel (llave compuesta).

// manyScansJPEG arma un JPEG progresivo válido de 2048 × 2048 (un componente) con un barrido DC y
// scans barridos AC vacíos de 17 bytes cada uno.
func manyScansJPEG(scans int) []byte {
	var buffer bytes.Buffer
	buffer.Write([]byte{0xFF, 0xD8, 0xFF, 0xDB, 0x00, 67, 0x00})
	buffer.Write(bytes.Repeat([]byte{1}, 64))
	buffer.Write([]byte{0xFF, 0xC2, 0x00, 11, 8, 0x08, 0x00, 0x08, 0x00, 1, 1, 0x11, 0})
	dc := []byte{0x00, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x00}
	buffer.Write([]byte{0xFF, 0xC4, 0x00, byte(2 + len(dc))})
	buffer.Write(dc)
	ac := []byte{0x10, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xE0, 0x10}
	buffer.Write([]byte{0xFF, 0xC4, 0x00, byte(2 + len(ac))})
	buffer.Write(ac)
	buffer.Write([]byte{0xFF, 0xDA, 0x00, 8, 1, 1, 0x00, 0, 0, 0x00})
	buffer.Write(make([]byte, 65536/8))
	for index := 0; index < scans; index++ {
		buffer.Write([]byte{0xFF, 0xDA, 0x00, 8, 1, 1, 0x00, 1, 63, 0x00})
		buffer.Write([]byte{0x3F, 0xFF, 0x00, 0x3F, 0xFF, 0x00, 0x5F})
	}
	buffer.Write([]byte{0xFF, 0xD9})
	return buffer.Bytes()
}

// reviewFixSubtests se ejecuta al final de TestDocumentIdentity, sobre la misma base.
func reviewFixSubtests(t *testing.T, fixture *identityFixture) {
	t.Run("un JPEG con demasiados barridos se rechaza sin esperar el lock de la identidad", func(t *testing.T) {
		view := fixture.view(t)
		before := fixture.counts(t)
		// Otra transacción tiene bloqueada la fila de la identidad (otro administrador guardando).
		holder, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(context.Background())
		if _, err := holder.Exec(fixture.ctx, "SELECT revision FROM app.document_identity FOR UPDATE"); err != nil {
			t.Fatal(err)
		}
		megabyte := manyScansJPEG((1<<20 - len(manyScansJPEG(0))) / 17)
		for name, content := range map[string][]byte{"101 barridos": manyScansJPEG(100), "1 MiB de barridos": megabyte} {
			ctx, cancel := context.WithTimeout(fixture.ctx, 3*time.Second)
			started := time.Now()
			err := callWithContext(ctx, fixture, "POST", "documents/identity/logo", upload(content, view.Revision))
			elapsed := time.Since(started)
			cancel()
			expectCode(t, err, "VALIDATION_ERROR")
			if !strings.Contains(err.Error(), "demasiados barridos") || elapsed > time.Second {
				t.Fatalf("%s: el rechazo debe llegar al momento sin esperar el lock (%v): %v", name, elapsed, err)
			}
		}
		if err := holder.Rollback(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if fixture.counts(t) != before {
			t.Fatal("Un archivo rechazado no guarda nada")
		}
		// Un JPEG progresivo con un número normal de barridos sí se carga.
		response, err := fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(manyScansJPEG(9), view.Revision))
		if err != nil {
			t.Fatalf("Un JPEG progresivo con 10 barridos debía cargarse: %v", err)
		}
		if saved := decodeView(t, response); saved.Logo == nil || saved.Logo.Width != 2048 || saved.Logo.MediaType != "image/jpeg" {
			t.Fatalf("Logotipo progresivo cargado: %+v", saved.Logo)
		}
	})

	t.Run("cada conversión vigente es de su ancho de papel", func(t *testing.T) {
		view := fixture.view(t)
		if view.Logo == nil {
			t.Fatal("La prueba necesita un logotipo cargado")
		}
		for _, statement := range []string{
			"UPDATE app.document_identity SET logo_raster_58_id = logo_raster_80_id",
			"UPDATE app.document_identity SET logo_raster_80_id = logo_raster_58_id",
		} {
			_, err := fixture.pool.Exec(fixture.ctx, statement)
			if err == nil || !strings.Contains(err.Error(), "document_identity_logo_raster_") {
				t.Fatalf("Se aceptó una conversión de otro ancho: %s: %v", statement, err)
			}
		}
		for _, statement := range []string{
			"UPDATE app.document_identity SET logo_raster_58_paper = 80",
			"UPDATE app.document_identity SET logo_raster_80_paper = 58",
		} {
			if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
				t.Fatalf("Se cambió el ancho constante de una referencia: %s", statement)
			}
		}
		if after := fixture.view(t); after.Revision != view.Revision || after.Logo.Rasters[0].RasterID != view.Logo.Rasters[0].RasterID {
			t.Fatal("Los intentos rechazados no cambian la identidad")
		}
	})
}

// callWithContext llama al manejador con un contexto propio (con plazo) en su propia transacción.
func callWithContext(ctx context.Context, fixture *identityFixture, method, route string, body any) error {
	encoded, _ := json.Marshal(body)
	tx, err := fixture.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	audit := func(context.Context, pgx.Tx, string, string, *string, any, any) error {
		return nil
	}
	_, err = identity.Handle(ctx, tx, identity.Request{Method: method, Route: route, Body: encoded, Actor: fixture.manager}, audit)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
