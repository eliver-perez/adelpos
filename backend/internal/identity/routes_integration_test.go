//go:build integration

package identity_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"net"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/designer"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/identity"
	"local/ad-el-pos/server/internal/kitchen"
)

// Rutas documents/identity* por HTTP, con la autenticación, los permisos efectivos, la auditoría
// y los eventos reales de la administración, y el encabezado de las exportaciones de la 9A con el
// logotipo del restaurante o el de AD-EL POS.

type terminal struct {
	deviceID     string
	deviceToken  string
	sessionToken string
}

func decodeMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	result := map[string]any{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("La respuesta no cumple el contrato: %v: %.200s", err, body)
	}
	return result
}

func (fixture *identityFixture) httpRequest(member terminal, method, path string, body any) (int, []byte) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, "/api/v1/"+path, reader)
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Provisioning-Token", fixture.provisioning)
	request.Header.Set("X-Device-ID", member.deviceID)
	request.Header.Set("X-Device-Token", member.deviceToken)
	request.Header.Set("Authorization", "Bearer "+member.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (fixture *identityFixture) httpCall(t *testing.T, member terminal, method, path string, body any, status int) []byte {
	t.Helper()
	code, response := fixture.httpRequest(member, method, path, body)
	if code != status {
		t.Fatalf("%s %s: esperado %d recibido %d: %.300s", method, path, status, code, response)
	}
	return response
}

// staff crea un rol con los permisos dados, un usuario y su propia terminal con sesión.
func (fixture *identityFixture) staff(t *testing.T, username string, permissions ...string) terminal {
	t.Helper()
	role := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "roles",
		map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.httpCall(t, fixture.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role["id"], "active": true, "password": password,
	}, 200)
	invitation := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "devices/invitations", nil, 200))
	device := decodeMap(t, fixture.httpCall(t, terminal{}, "POST", "devices/enroll",
		map[string]any{"code": invitation["code"], "name": "Terminal " + username}, 200))
	member := terminal{deviceID: device["device_id"].(string), deviceToken: device["device_token"].(string)}
	session := decodeMap(t, fixture.httpCall(t, member, "POST", "auth/login",
		map[string]string{"username": username, "password": password}, 200))
	member.sessionToken = session["session_token"].(string)
	return member
}

func (fixture *identityFixture) httpView(t *testing.T, member terminal) identity.View {
	t.Helper()
	return decodeView(t, fixture.httpCall(t, member, "GET", "documents/identity", nil, 200))
}

func (fixture *identityFixture) scalar(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// noisyPNG es un logotipo de 360 × 360 con ruido: pesa varios cientos de KB (más que el límite
// general de 64 KiB de la API) y menos de 1 MiB.
func noisyPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 360, 360))
	seed := uint32(7)
	for y := 0; y < 360; y++ {
		for x := 0; x < 360; x++ {
			seed = seed*1664525 + 1013904223
			picture.Set(x, y, color.NRGBA{R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255})
		}
	}
	content := encodePNG(t, picture)
	if len(content) < 200*1024 || len(content) > 1<<20 {
		t.Fatalf("El logotipo de prueba debe pesar entre 200 KB y 1 MiB: %d", len(content))
	}
	return content
}

var pdfImagePattern = regexp.MustCompile(`/Subtype /Image\s*/Width (\d+)\s*/Height (\d+)`)

func pdfImageSizes(content []byte) [][2]int {
	sizes := [][2]int{}
	for _, match := range pdfImagePattern.FindAllSubmatch(content, -1) {
		width, _ := strconv.Atoi(string(match[1]))
		height, _ := strconv.Atoi(string(match[2]))
		sizes = append(sizes, [2]int{width, height})
	}
	return sizes
}

// workbookMedia devuelve las imágenes del Excel exportado.
func workbookMedia(t *testing.T, content []byte) map[string][]byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	media := map[string][]byte{}
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "xl/media/") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(reader)
		reader.Close()
		media[file.Name] = data
	}
	return media
}

func (fixture *identityFixture) export(t *testing.T, format string) []byte {
	t.Helper()
	request := httptest.NewRequest("GET", "/api/v1/reports/export/sales.by_product?format="+format, nil)
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Device-ID", fixture.admin.deviceID)
	request.Header.Set("X-Device-Token", fixture.admin.deviceToken)
	request.Header.Set("Authorization", "Bearer "+fixture.admin.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("Exportar %s: %d %.200s", format, response.Code, response.Body.Bytes())
	}
	return append([]byte{}, response.Body.Bytes()...)
}

// routeSubtests corre al final de TestDocumentIdentity, sobre la misma base.
func routeSubtests(t *testing.T, fixture *identityFixture) {
	settingsReader := fixture.staff(t, "lectora", "settings.read")
	outsider := fixture.staff(t, "ajena", "accounts.read")

	t.Run("permisos en Go: leer con settings.read, escribir solo con documents.identity.manage", func(t *testing.T) {
		view := fixture.httpView(t, fixture.admin)
		if view.Establishment.Name != "Fonda de pruebas" {
			t.Fatalf("La identidad reutiliza el nombre del establecimiento: %+v", view.Establishment)
		}
		readerView := fixture.httpView(t, settingsReader)
		if readerView.Revision != view.Revision {
			t.Fatal("settings.read también permite leer")
		}
		fixture.httpCall(t, outsider, "GET", "documents/identity", nil, 403)
		denied := fixture.scalar(t, "SELECT count(*)::int FROM app.audit_events WHERE action = 'access.denied'")
		body := saveBody(view, map[string]any{"footer_text": "No debe guardarse"})
		fixture.httpCall(t, settingsReader, "PUT", "documents/identity", body, 403)
		fixture.httpCall(t, settingsReader, "POST", "documents/identity/logo", upload(logoPNG(t), view.Revision), 403)
		fixture.httpCall(t, settingsReader, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": view.Revision}, 403)
		if fixture.httpView(t, fixture.admin).Revision != view.Revision {
			t.Fatal("Un 403 no escribe nada")
		}
		if fixture.scalar(t, "SELECT count(*)::int FROM app.audit_events WHERE action = 'access.denied'") != denied+3 {
			t.Fatal("Cada rechazo de permisos se audita")
		}
		fixture.httpCall(t, fixture.admin, "DELETE", "documents/identity", nil, 404)
	})

	t.Run("cargar por HTTP un logotipo mayor que el límite general y auditar sin bytes", func(t *testing.T) {
		view := fixture.httpView(t, fixture.admin)
		content := noisyPNG(t)
		saved := decodeView(t, fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo", upload(content, view.Revision), 200))
		if saved.Logo == nil || saved.Logo.SizeBytes != len(content) || saved.Logo.UploadedBy == nil {
			t.Fatalf("Logotipo cargado: %+v", saved.Logo)
		}
		var before, after []byte
		var entity string
		err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(before_state::text, ''), coalesce(after_state::text, ''), entity_type
            FROM app.audit_events WHERE action = 'documents.logo_uploaded' ORDER BY occurred_at DESC LIMIT 1`).
			Scan(&before, &after, &entity)
		if err != nil {
			t.Fatal(err)
		}
		encoded := base64.StdEncoding.EncodeToString(content)
		if entity != "document_logo" || len(after) > 2048 || bytes.Contains(after, []byte(encoded[:64])) ||
			!bytes.Contains(after, []byte(saved.Logo.SHA256)) {
			t.Fatalf("La auditoría identifica el archivo sin incluirlo: %d bytes", len(after))
		}
		// El evento SSE del ámbito documents solo informa el cambio.
		tx, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		batch, err := events.Read(fixture.ctx, tx, 0, []string{"documents.templates.read"}, fixture.admin.deviceID, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, event := range batch.Events {
			if event.Kind == identity.EventChanged && event.Scope == identity.Scope {
				found = true
			}
		}
		if !found {
			t.Fatal("documents.identity.changed llega a quien consulta plantillas")
		}
		hidden, err := events.Read(fixture.ctx, tx, 0, []string{"accounts.read"}, fixture.admin.deviceID, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range hidden.Events {
			if event.Scope == identity.Scope {
				t.Fatal("El ámbito documents no llega a quien no consulta plantillas")
			}
		}
	})

	t.Run("exportaciones: logotipo del restaurante y respaldo de AD-EL POS", func(t *testing.T) {
		current := fixture.httpView(t, fixture.admin)
		if current.Logo == nil || current.Logo.SizeBytes != len(noisyPNG(t)) {
			fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo", upload(noisyPNG(t), current.Revision), 200)
		}
		withLogo := fixture.export(t, "pdf")
		sizes := pdfImageSizes(withLogo)
		// El logotipo de 360 × 360 se reduce a lo más 600 × 168 conservando su proporción.
		if len(sizes) != 1 || sizes[0] != [2]int{168, 168} {
			t.Fatalf("El PDF lleva el logotipo del restaurante: %v", sizes)
		}
		restaurantMedia := workbookMedia(t, fixture.export(t, "xlsx"))
		if len(restaurantMedia) != 1 {
			t.Fatalf("El Excel lleva una sola imagen: %d", len(restaurantMedia))
		}
		view := fixture.httpView(t, fixture.admin)
		fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": view.Revision}, 200)
		brandSizes := pdfImageSizes(fixture.export(t, "pdf"))
		if len(brandSizes) != 1 || brandSizes[0] == sizes[0] {
			t.Fatalf("Sin logotipo el PDF lleva la marca de AD-EL POS: %v", brandSizes)
		}
		brandMedia := workbookMedia(t, fixture.export(t, "xlsx"))
		if len(brandMedia) != 1 {
			t.Fatalf("El Excel lleva la marca: %d", len(brandMedia))
		}
		for name, data := range brandMedia {
			if bytes.Equal(data, restaurantMedia[name]) {
				t.Fatal("La imagen del Excel cambia al retirar el logotipo")
			}
		}
	})

	t.Run("conexión con el renderizador: copia de la identidad, vista previa, impresoras y cola", func(t *testing.T) {
		rendererSubtest(t, fixture)
	})
}

// receiptLayoutWithLogo es el ticket de venta del sistema con el bloque «Logotipo» al inicio.
func (fixture *identityFixture) receiptLayoutWithLogo(t *testing.T) map[string]any {
	t.Helper()
	detail := decodeMap(t, fixture.httpCall(t, fixture.admin, "GET",
		"documents/templates/"+documents.SystemTemplateID(documents.KindReceipt), nil, 200))
	layout := detail["layout"].(map[string]any)
	logo := map[string]any{"id": "logo", "type": "logo", "visible": true, "align": "left", "size": "normal",
		"bold": false, "uppercase": false, "spacing_before": 0, "spacing_after": 0, "options": map[string]any{}}
	layout["blocks"] = append([]any{logo}, layout["blocks"].([]any)...)
	return layout
}

func (fixture *identityFixture) preview(t *testing.T, layout map[string]any, paperWidth int) designer.Preview {
	t.Helper()
	var preview designer.Preview
	body := fixture.httpCall(t, fixture.admin, "POST", "documents/preview", map[string]any{
		"kind": documents.KindReceipt, "layout": layout, "paper_width": paperWidth, "source": map[string]any{"sample": true},
	}, 200)
	if err := json.Unmarshal(body, &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

// escposListener escucha en loopback en un puerto ESC/POS admitido (9100–9109) y entrega lo que
// recibe cada conexión: es el transporte de prueba, nunca una impresora.
func escposListener(t *testing.T) (int, <-chan []byte) {
	t.Helper()
	for port := 9100; port <= 9109; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		t.Cleanup(func() { listener.Close() })
		received := make(chan []byte, 4)
		go func() {
			for {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				buffer := bytes.Buffer{}
				_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
				_, _ = buffer.ReadFrom(connection)
				connection.Close()
				received <- buffer.Bytes()
			}
		}()
		return port, received
	}
	t.Skip("No hay un puerto 9100–9109 libre en loopback para el transporte de prueba")
	return 0, nil
}

// printPending entrega el siguiente trabajo con el transporte de la aplicación hacia el puerto de
// prueba y las conversiones guardadas del logotipo.
func (fixture *identityFixture) printPending(t *testing.T, jobID string, received <-chan []byte) []byte {
	t.Helper()
	worker := &kitchen.Worker{Pool: fixture.pool, Transport: kitchen.HostTransport{}, Rasters: identity.NewRasters(fixture.pool)}
	worked, err := worker.PrintNext(fixture.ctx)
	if err != nil || !worked {
		t.Fatalf("El trabajador no tomó el trabajo %s: %v", jobID, err)
	}
	var state, message string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state, last_error FROM app.kitchen_print_jobs WHERE id = $1", jobID).
		Scan(&state, &message); err != nil {
		t.Fatal(err)
	}
	if state != "submitted" {
		t.Fatalf("El trabajo debe quedar aceptado por el transporte de prueba: %s %s", state, message)
	}
	select {
	case data := <-received:
		return data
	case <-time.After(10 * time.Second):
		t.Fatal("El transporte de prueba no recibió el documento")
	}
	return nil
}

// rasterCommand es la orden GS v 0 de la primera franja de una conversión guardada.
func rasterCommand(t *testing.T, fixture *identityFixture, rasterID string) []byte {
	t.Helper()
	raster, err := identity.NewRasters(fixture.pool).Raster(fixture.ctx, rasterID)
	if err != nil || raster == nil {
		t.Fatalf("Conversión %s: %v", rasterID, err)
	}
	rowBytes := raster.WidthDots / 8
	rows := min(raster.HeightDots, 128)
	command := []byte{0x1d, 0x76, 0x30, 0x00, byte(rowBytes), byte(rowBytes >> 8), byte(rows), byte(rows >> 8)}
	return append(command, raster.Bits[:rows*rowBytes]...)
}

func rendererSubtest(t *testing.T, fixture *identityFixture) {
	view := fixture.httpView(t, fixture.admin)
	if view.Logo != nil {
		fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": view.Revision}, 200)
		view = fixture.httpView(t, fixture.admin)
	}
	layout := fixture.receiptLayoutWithLogo(t)

	// Sin logotipo el bloque no imprime nada y la vista previa lo explica; las impresoras lo dicen.
	empty := fixture.preview(t, layout, 58)
	for _, line := range empty.Lines {
		if line.Image != nil || line.BlockID == "logo" {
			t.Fatal("Sin logotipo el bloque no imprime ninguna línea")
		}
	}
	if !strings.Contains(strings.Join(empty.Warnings, " "), "No hay logotipo cargado") {
		t.Fatalf("La vista previa explica el bloque vacío: %v", empty.Warnings)
	}
	port, received := escposListener(t)
	printer := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "kitchen/printers", map[string]any{
		"operation_id": randomID(), "name": "Tickets ESC/POS de prueba", "transport": "escpos_tcp", "destination": "127.0.0.1", "port": port,
		"active": true, "paper_width": 58, "copies": 1, "cut": false, "supports_cut": false,
	}, 200))
	printerID := printer["id"].(string)
	compatibility := func() designer.PrinterCompatibility {
		var printers []designer.PrinterCompatibility
		_ = json.Unmarshal(fixture.httpCall(t, fixture.admin, "GET", "documents/printers", nil, 200), &printers)
		for _, item := range printers {
			if item.ID == printerID {
				return item
			}
		}
		t.Fatal("La impresora de prueba no aparece en documents/printers")
		return designer.PrinterCompatibility{}
	}
	if logo := compatibility().Logo; logo.Status != "supported" || !strings.HasPrefix(logo.Reason, "No hay logotipo cargado") {
		t.Fatalf("Sin logotipo la compatibilidad lo dice: %+v", logo)
	}

	// Con logotipo: la identidad copiada lleva los textos y las dos conversiones vigentes.
	saved := decodeView(t, fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo", upload(logoPNG(t), view.Revision), 200))
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := documents.LoadIdentity(fixture.ctx, tx)
	_ = tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if copied.Extra != saved.AdditionalInfo || copied.Header != saved.HeaderText || copied.Footer != saved.FooterText ||
		copied.Name != saved.Establishment.Name || len(copied.Logo) != 2 {
		t.Fatalf("Identidad copiada en los documentos: %+v", copied)
	}
	for index, raster := range saved.Logo.Rasters {
		logo := copied.Logo[index]
		if logo.PaperWidth != raster.PaperWidth || logo.RasterID != raster.RasterID || logo.WidthDots != raster.WidthDots ||
			logo.HeightDots != raster.HeightDots {
			t.Fatalf("Conversión copiada %d: %+v frente a %+v", index, logo, raster)
		}
	}
	if !strings.HasPrefix(compatibility().Logo.Reason, "La impresora recibe comandos ESC/POS") {
		t.Fatal("Con logotipo la compatibilidad ya no avisa que falta")
	}
	// La vista previa del diseñador muestra la línea de imagen del ancho elegido con su PNG.
	for index, paperWidth := range []int{58, 80} {
		preview := fixture.preview(t, layout, paperWidth)
		first := preview.Lines[0]
		if first.Image == nil || first.BlockID != "logo" || first.Image.RasterID != saved.Logo.Rasters[index].RasterID {
			t.Fatalf("Línea de logotipo a %d mm: %+v", paperWidth, first)
		}
		decodePreview(t, first.Image.DataURI, first.Image.WidthDots, first.Image.HeightDots)
	}

	// Muestra impresa en el transporte de prueba ESC/POS: GS v 0 con los bits guardados.
	sample := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "documents/samples", map[string]any{
		"operation_id": randomID(), "kind": documents.KindReceipt, "layout": layout, "printer_id": printerID,
	}, 200))
	jobID := sample["job_id"].(string)
	first := fixture.printPending(t, jobID, received)
	command := rasterCommand(t, fixture, saved.Logo.Rasters[0].RasterID)
	if !bytes.Contains(first, command) {
		t.Fatal("El documento con logotipo debe viajar con GS v 0 y los bits de la conversión de 58 mm")
	}

	// Reemplazar el logotipo no cambia lo ya emitido: la copia sale idéntica, con su conversión.
	current := fixture.httpView(t, fixture.admin)
	replaced := decodeView(t, fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo", upload(logoJPEG(t), current.Revision), 200))
	if replaced.Logo.Rasters[0].RasterID == saved.Logo.Rasters[0].RasterID {
		t.Fatal("El logotipo nuevo tiene otras conversiones")
	}
	reprinted := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "kitchen/reprint", map[string]any{
		"operation_id": randomID(), "job_id": jobID, "reason": "Copia tras reemplazar el logotipo",
	}, 200))
	second := fixture.printPending(t, reprinted["id"].(string), received)
	if !bytes.Equal(first, second) {
		t.Fatal("Un documento emitido antes de reemplazar el logotipo se reimprime idéntico")
	}
	if bytes.Contains(second, rasterCommand(t, fixture, replaced.Logo.Rasters[0].RasterID)) {
		t.Fatal("La copia no usa el logotipo nuevo")
	}
	// Retirarlo tampoco: la conversión sigue disponible para las copias.
	current = fixture.httpView(t, fixture.admin)
	fixture.httpCall(t, fixture.admin, "POST", "documents/identity/logo/remove",
		map[string]any{"operation_id": randomID(), "revision": current.Revision}, 200)
	third := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "kitchen/reprint", map[string]any{
		"operation_id": randomID(), "job_id": jobID, "reason": "Copia tras retirar el logotipo",
	}, 200))
	if !bytes.Equal(fixture.printPending(t, third["id"].(string), received), first) {
		t.Fatal("Un documento emitido antes de retirar el logotipo se reimprime idéntico")
	}
}
