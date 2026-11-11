package kitchen

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/logoraster"
)

// Etapa 9B, fase B2: el logotipo real (convertido con logoraster, como lo guarda la identidad) por
// los transportes de la cola, sin ejecutar programas ni llegar a una impresora: ejecutor simulado de
// CUPS. Por una cola que admite datos raw viaja con GS v 0; por una que no, se imprime el texto con
// aviso y el trabajo queda aceptado; sin logotipo el bloque no imprime nada.

// realLogoRaster convierte un logotipo PNG de 120 × 48 a 80 mm con la presentación por omisión.
func realLogoRaster(t *testing.T) *logoraster.Raster {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 120, 48))
	for y := 6; y < 42; y++ {
		for x := 6; x < 114; x++ {
			picture.Set(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	logo, err := logoraster.Validate(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	raster, err := logoraster.Convert(logo.Image, logoraster.DefaultOptions(logoraster.Paper80))
	if err != nil {
		t.Fatal(err)
	}
	return raster
}

// receiptWithLogo renderiza el ticket de muestra con el bloque «Logotipo» al inicio y la identidad
// dada (con o sin conversión de 80 mm).
func receiptWithLogo(t *testing.T, identity documents.Identity) *documents.Layout {
	t.Helper()
	var template map[string]any
	if err := json.Unmarshal(documents.Default(documents.KindReceipt).Encode(), &template); err != nil {
		t.Fatal(err)
	}
	logo := map[string]any{"id": "logo", "type": "logo", "align": "left"}
	template["blocks"] = append([]any{logo}, template["blocks"].([]any)...)
	encoded, _ := json.Marshal(template)
	parsed, err := documents.ParseTemplate(documents.KindReceipt, encoded)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := documents.Render(documents.KindReceipt, parsed, documents.SampleData(documents.KindReceipt, identity),
		documents.RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func logoJob(layout *documents.Layout, raster *logoraster.Raster) PrintJob {
	job := formattedJob("cups")
	job.Layout = layout
	job.Content = layout.Text()
	job.Rasters = map[string]*documents.Raster{}
	if raster != nil {
		job.Rasters["logo-real-80"] = &documents.Raster{WidthDots: raster.Width(), HeightDots: raster.Height(), Bits: raster.Bits()}
	}
	return job
}

func TestRealLogoThroughCUPSQueues(t *testing.T) {
	raster := realLogoRaster(t)
	identity := documents.Identity{Name: "Fonda", Logo: []documents.LogoRaster{
		{PaperWidth: 80, RasterID: "logo-real-80", WidthDots: raster.Width(), HeightDots: raster.Height()},
	}}
	layout := receiptWithLogo(t, identity)
	if layout.Plain() || !layout.HasImages() || layout.Lines[0].Image == nil || layout.Lines[0].Image.RasterID != "logo-real-80" {
		t.Fatalf("El bloque «Logotipo» produce la línea de imagen de 80 mm: %+v", layout.Lines[0])
	}

	// Cola con datos raw: los bytes de lp son el ESC/POS del layout con GS v 0 y los bits exactos.
	raw := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-usb-pos80.txt")},
		"/usr/bin/lp":        {output: []byte("la id. de petición es Printer_POS_80-401 (1 archivo(s))\n")},
	}}
	job := logoJob(layout, raster)
	outcome := HostTransport{commands: raw}.Submit(context.Background(), job)
	sent := raw.calls[len(raw.calls)-1].pulse
	if outcome.State != "submitted" || !bytes.Contains(sent, raster.ESCPos()) {
		t.Fatalf("Por CUPS raw el logotipo viaja con GS v 0 y sus bits: %+v", outcome)
	}

	// Cola sin datos raw: se imprime el texto (sin la imagen) con aviso y el trabajo queda aceptado.
	textOnly := &recordedCommands{t: t, replies: map[string]recordedReply{
		"/usr/bin/lpstat":    {output: recorded(t, "lpstat-p-usb-pos80.txt")},
		"/usr/bin/lpoptions": {output: recorded(t, "lpoptions-dnssd-airprint.txt")},
	}}
	var printed string
	transport := HostTransport{commands: textOnly, textCUPS: func(_ context.Context, sent PrintJob) Outcome {
		printed = sent.Content
		return Outcome{State: "submitted", Accepted: true, Reference: "Q-2", Message: "Aceptado por CUPS; no confirma salida física de papel."}
	}}
	outcome = transport.Submit(context.Background(), logoJob(layout, raster))
	if outcome.State != "submitted" || !outcome.Accepted || !strings.Contains(outcome.Message, "sin formato ni logotipo") {
		t.Fatalf("Una cola sin raw omite el logotipo con aviso y sin fallar: %+v", outcome)
	}
	if printed != layout.Text() || strings.ContainsAny(printed, "\x1b\x1d") {
		t.Fatal("La cola sin raw recibe solo el texto, sin comandos ni imagen")
	}
	for _, call := range textOnly.calls {
		if call.program == "/usr/bin/lp" {
			t.Fatal("No se envían bytes raw a una cola que no los admite")
		}
	}

	// Sin logotipo cargado el bloque no imprime nada: el texto es el del ticket sin el bloque.
	withoutLogo := receiptWithLogo(t, documents.Identity{Name: "Fonda"})
	plainReceipt, err := documents.Render(documents.KindReceipt, documents.Default(documents.KindReceipt),
		documents.SampleData(documents.KindReceipt, documents.Identity{Name: "Fonda"}), documents.RenderOptions{PaperWidth: 80})
	if err != nil {
		t.Fatal(err)
	}
	if withoutLogo.HasImages() || withoutLogo.Text() != plainReceipt.Text() ||
		!strings.Contains(strings.Join(withoutLogo.Warnings, " "), "No hay logotipo cargado") {
		t.Fatalf("Sin logotipo el bloque no imprime nada y la vista previa lo explica: %v", withoutLogo.Warnings)
	}
}
