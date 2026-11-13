//go:build integration

package operations_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/identity"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa 9B, fase B2, encadenada en testStage9BDocuments: el logotipo del restaurante en una comanda
// real. La comanda copia la identidad con las conversiones vigentes; el trabajador carga los bits
// guardados y el transporte recibe la línea de imagen (GS v 0 con esos bits); una comanda emitida
// antes de reemplazar o retirar el logotipo se reimprime idéntica con su propia conversión; sin
// logotipo el bloque no imprime nada. Ningún documento llega a una impresora: el transporte es el
// adaptador controlado de la cola.

// logoForKitchen es un logotipo PNG de 160 × 64 (un rectángulo negro sobre transparente).
func logoForKitchen(t *testing.T, shade uint8) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 160, 64))
	for y := 8; y < 56; y++ {
		for x := 8; x < 152; x++ {
			picture.Set(x, y, color.NRGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// addLogo agrega el bloque «Logotipo» al inicio de la plantilla.
func addLogo(blocks []any) []any {
	logo := map[string]any{"id": "logo", "type": "logo", "align": "left"}
	return append([]any{logo}, blocks...)
}

func (fixture *stage9BFixture) identityView(t *testing.T) identity.View {
	t.Helper()
	return decodeResponse[identity.View](t, fixture.adminCall(t, "GET", "documents/identity", nil, 200))
}

// printWithRasters entrega el trabajo al adaptador controlado con el trabajador de la aplicación y
// su fuente de conversiones guardadas; devuelve el trabajo tal como lo recibió el transporte.
func (fixture *stage9BFixture) printWithRasters(t *testing.T, jobID string) kitchen.PrintJob {
	t.Helper()
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.kitchen_print_jobs SET state='cancelled',lease_until=NULL
        WHERE state IN ('pending','error','processing') AND id<>$1`, jobID); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingTransport{state: "submitted"}
	worker := &kitchen.Worker{Pool: fixture.pool, Transport: adapter, Rasters: identity.NewRasters(fixture.pool)}
	if worked, err := worker.PrintNext(fixture.ctx); err != nil || !worked || len(adapter.jobs) != 1 {
		t.Fatalf("El trabajador no entregó el trabajo %s: %v", jobID, err)
	}
	return adapter.jobs[0]
}

func testStage9BLogo(t *testing.T, fixture *stage9BFixture) {
	var original kitchen.PrintJob
	var originalLayout string
	var originalRaster string
	t.Run("9B_comanda_con_logotipo_copia_la_conversion_y_viaja_con_GS_v_0", func(t *testing.T) {
		view := fixture.identityView(t)
		uploaded := decodeResponse[identity.View](t, fixture.mutate(t, "POST", "documents/identity/logo", map[string]any{
			"revision": view.Revision, "content_base64": base64.StdEncoding.EncodeToString(logoForKitchen(t, 10)),
		}, 200))
		fixture.customize(t, documents.KindKitchenOrder, "Comanda con logotipo", addLogo)
		original = fixture.releasedJob(t, "Comanda con logotipo")
		job := fixture.job(t, original.ID)
		if job.layout == nil {
			t.Fatal("Una comanda con logotipo guarda su IR en la cola")
		}
		originalLayout = *job.layout
		var document kitchen.Document
		if err := json.Unmarshal(fixture.ticketDocument(t, job.ticketID), &document); err != nil {
			t.Fatal(err)
		}
		if document.Identity == nil || len(document.Identity.Logo) != 2 {
			t.Fatalf("La comanda copia las dos conversiones vigentes: %+v", document.Identity)
		}
		// La impresora de comandas es de 80 mm: la línea de imagen es la conversión de 80 mm.
		originalRaster = uploaded.Logo.Rasters[1].RasterID
		if uploaded.Logo.Rasters[1].PaperWidth != 80 || !strings.Contains(originalLayout, originalRaster) {
			t.Fatalf("La IR de la comanda usa la conversión de 80 mm: %s", originalLayout)
		}
		received := fixture.printWithRasters(t, original.ID)
		raster := received.Rasters[originalRaster]
		if raster == nil || raster.WidthDots != 576 || received.Layout == nil {
			t.Fatalf("El trabajador cargó los bits guardados: %+v", raster)
		}
		// Primera franja de GS v 0 (a lo más 128 filas de 72 bytes) con los bits guardados.
		encoded := documents.EncodeESCPos(received.Layout, received.Rasters, documents.NoPaperCut)
		rows := min(raster.HeightDots, 128)
		band := append([]byte{0x1d, 0x76, 0x30, 0x00, 72, 0, byte(rows), 0}, raster.Bits[:rows*72]...)
		if !bytes.Contains(encoded, band) || len(documents.MissingRasters(received.Layout, received.Rasters)) != 0 {
			t.Fatal("El logotipo viaja con GS v 0 y los bits de la conversión guardada")
		}
		if strings.Contains(received.Content, "\x1d") || received.Content != job.content {
			t.Fatal("El texto de la cola no lleva comandos ni la imagen")
		}
	})
	t.Run("9B_comanda_emitida_antes_de_reemplazar_o_retirar_el_logotipo_se_reimprime_identica", func(t *testing.T) {
		view := fixture.identityView(t)
		replaced := decodeResponse[identity.View](t, fixture.mutate(t, "POST", "documents/identity/logo", map[string]any{
			"revision": view.Revision, "content_base64": base64.StdEncoding.EncodeToString(logoForKitchen(t, 60)),
		}, 200))
		if replaced.Logo.Rasters[1].RasterID == originalRaster {
			t.Fatal("El logotipo nuevo tiene otra conversión")
		}
		reprint := func(reason string) kitchen.PrintJob {
			copied := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{
				"job_id": original.ID, "reason": reason,
			}, 200))
			job := fixture.job(t, copied.ID)
			if job.layout == nil || *job.layout != originalLayout || job.content != fixture.job(t, original.ID).content {
				t.Fatalf("La copia no es idéntica a la comanda original: %v", job.layout)
			}
			return fixture.printWithRasters(t, copied.ID)
		}
		first := reprint("Copia tras reemplazar el logotipo")
		if first.Rasters[originalRaster] == nil || first.Rasters[replaced.Logo.Rasters[1].RasterID] != nil {
			t.Fatal("La copia usa la conversión con la que se emitió, no la del logotipo nuevo")
		}
		current := fixture.identityView(t)
		fixture.mutate(t, "POST", "documents/identity/logo/remove", map[string]any{"revision": current.Revision}, 200)
		second := reprint("Copia tras retirar el logotipo")
		if !bytes.Equal(documents.EncodeESCPos(second.Layout, second.Rasters, documents.NoPaperCut), documents.EncodeESCPos(first.Layout, first.Rasters, documents.NoPaperCut)) {
			t.Fatal("Retirar el logotipo no cambia la copia de una comanda ya emitida")
		}
	})
	t.Run("9B_sin_logotipo_el_bloque_no_imprime_nada", func(t *testing.T) {
		fresh := fixture.job(t, fixture.releasedJob(t, "Comanda sin logotipo").ID)
		if fresh.layout != nil && strings.Contains(*fresh.layout, "raster_id") {
			t.Fatalf("Sin logotipo la comanda no lleva línea de imagen: %s", *fresh.layout)
		}
		fixture.settle(t)
	})
}
