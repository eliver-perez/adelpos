//go:build integration

package identity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/identity"
)

// Aceptación con PostgreSQL de la identidad documental llamando al manejador directamente (cada
// llamada en su propia transacción, como la administración): carga PNG y JPEG, rechazos, reemplazo,
// retiro, conversiones guardadas y reutilizadas, conflicto de revisión, idempotencia, auditoría sin
// bytes, evento SSE y la fuente del logotipo de los reportes. Al final, las rutas por HTTP con
// permisos reales (routes_integration_test.go) sobre la misma base.

type auditEntry struct {
	Action string
	Entity string
	ID     *string
	Before json.RawMessage
	After  json.RawMessage
}

type identityFixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	service      *administration.Service
	provisioning string
	admin        terminal
	manager      identity.Actor
	reader       identity.Actor
	mutex        sync.Mutex
	audits       []auditEntry
}

func randomID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

// newIdentityFixture migra la base temporal y completa el asistente inicial con la API real
// (establecimiento, administrador y terminal). Las llamadas directas al manejador usan ese mismo
// usuario y esa terminal para las referencias de restaurant_operations y updated_by.
func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo abrir el pool temporal")
	}
	t.Cleanup(pool.Close)
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	token := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning")
	if err := os.WriteFile(filename, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := &identityFixture{ctx: ctx, pool: pool, provisioning: token,
		service: &administration.Service{Pool: pool, ProvisioningFile: filename}}
	setup := decodeMap(t, fixture.httpCall(t, terminal{}, "POST", "setup", map[string]string{
		"name": "Fonda de pruebas", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light",
		"terminal_name": "Caja principal",
	}, 200))
	fixture.admin.deviceID = setup["device_id"].(string)
	fixture.admin.deviceToken = setup["device_token"].(string)
	login := decodeMap(t, fixture.httpCall(t, fixture.admin, "POST", "auth/login",
		map[string]string{"username": "admin", "password": "admin"}, 200))
	fixture.admin.sessionToken = login["session_token"].(string)
	_, err = pool.Exec(ctx, `UPDATE app.establishment SET phone = '81 1234 5678', email = 'hola@fonda.mx',
        address = 'Calle 1, Centro'`)
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username = 'admin'").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	deviceID := fixture.admin.deviceID
	fixture.manager = identity.Actor{UserID: userID, DeviceID: deviceID, Permissions: []string{identity.PermissionManage}}
	fixture.reader = identity.Actor{UserID: userID, DeviceID: deviceID, Permissions: []string{identity.PermissionSettingsRead}}
	return fixture
}

// call ejecuta una petición en su propia transacción: se confirma solo si no hubo error.
func (fixture *identityFixture) call(actor identity.Actor, method, route string, body any) (json.RawMessage, error) {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	pending := []auditEntry{}
	audit := func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error {
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		pending = append(pending, auditEntry{Action: action, Entity: entity, ID: id, Before: beforeJSON, After: afterJSON})
		return nil
	}
	result, err := identity.Handle(fixture.ctx, tx, identity.Request{Method: method, Route: route, Body: encoded, Actor: actor}, audit)
	if err != nil {
		return nil, err
	}
	response, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		return nil, err
	}
	fixture.mutex.Lock()
	fixture.audits = append(fixture.audits, pending...)
	fixture.mutex.Unlock()
	return response, nil
}

func (fixture *identityFixture) view(t *testing.T) identity.View {
	t.Helper()
	response, err := fixture.call(fixture.reader, "GET", "documents/identity", nil)
	if err != nil {
		t.Fatal(err)
	}
	return decodeView(t, response)
}

func decodeView(t *testing.T, response json.RawMessage) identity.View {
	t.Helper()
	var view identity.View
	if err := json.Unmarshal(response, &view); err != nil {
		t.Fatalf("La respuesta no cumple el contrato: %v", err)
	}
	return view
}

func (fixture *identityFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var value int
	if err := fixture.pool.QueryRow(fixture.ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func (fixture *identityFixture) counts(t *testing.T) [4]int {
	t.Helper()
	return [4]int{
		fixture.count(t, "SELECT count(*)::int FROM app.document_logos"),
		fixture.count(t, "SELECT count(*)::int FROM app.document_logo_rasters"),
		fixture.count(t, "SELECT revision FROM app.document_identity"),
		fixture.count(t, "SELECT count(*)::int FROM app.realtime_events WHERE kind = $1", identity.EventChanged),
	}
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var problem *identity.Error
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("Se esperaba %s y se obtuvo %v", code, err)
	}
}

func encodePNG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// logoPNG es un logotipo de 200 × 80: un rectángulo negro sobre fondo transparente.
func logoPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 200, 80))
	for y := 10; y < 70; y++ {
		for x := 20; x < 180; x++ {
			picture.Set(x, y, color.NRGBA{R: 10, G: 10, B: 10, A: 255})
		}
	}
	return encodePNG(t, picture)
}

// logoJPEG es un degradado de 300 × 120 (sirve para el difuminado).
func logoJPEG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 300, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 300; x++ {
			level := uint8(x * 255 / 299)
			picture.Set(x, y, color.RGBA{R: level, G: level, B: level, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, picture, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func upload(content []byte, revision int) map[string]any {
	return map[string]any{
		"operation_id":   randomID(),
		"revision":       revision,
		"content_base64": base64.StdEncoding.EncodeToString(content),
	}
}

func saveBody(view identity.View, changes map[string]any) map[string]any {
	body := map[string]any{
		"operation_id":    randomID(),
		"revision":        view.Revision,
		"additional_info": view.AdditionalInfo,
		"header_text":     view.HeaderText,
		"footer_text":     view.FooterText,
		"presentation":    view.Presentation,
	}
	for key, value := range changes {
		body[key] = value
	}
	return body
}

// decodePreview comprueba que un data URI sea un PNG con las medidas anunciadas.
func decodePreview(t *testing.T, uri string, width, height int) {
	t.Helper()
	encoded, found := strings.CutPrefix(uri, "data:image/png;base64,")
	if !found {
		t.Fatalf("La vista previa debe ser un PNG: %.40s", uri)
	}
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(content))
	if err != nil || picture.Bounds().Dx() != width || picture.Bounds().Dy() != height {
		t.Fatalf("Vista previa de %d × %d: %v %v", width, height, picture.Bounds(), err)
	}
}

func TestDocumentIdentity(t *testing.T) {
	fixture := newIdentityFixture(t)

	t.Run("lectura inicial con los datos de Configuración general", func(t *testing.T) {
		view := fixture.view(t)
		if view.Revision != 1 || view.Logo != nil || view.AdditionalInfo != "" || view.HeaderText != "" || view.FooterText != "" {
			t.Fatalf("Identidad inicial: %+v", view)
		}
		if view.Presentation != identity.DefaultPresentation() || view.UpdatedBy != nil || len(view.Warnings) != 0 {
			t.Fatalf("Presentación inicial: %+v", view)
		}
		expected := identity.Establishment{Name: "Fonda de pruebas", Address: "Calle 1, Centro", Phone: "81 1234 5678", Email: "hola@fonda.mx"}
		if view.Establishment != expected {
			t.Fatalf("Los datos del establecimiento se reutilizan: %+v", view.Establishment)
		}
		if view.Limits.TextMaxCharacters != 300 || view.Limits.LogoMaxBytes != 1<<20 {
			t.Fatalf("Límites: %+v", view.Limits)
		}
		templatesReader := identity.Actor{UserID: fixture.reader.UserID, DeviceID: fixture.reader.DeviceID,
			Permissions: []string{identity.PermissionTemplatesRead}}
		if _, err := fixture.call(templatesReader, "GET", "documents/identity", nil); err != nil {
			t.Fatalf("documents.templates.read también permite leer: %v", err)
		}
	})

	t.Run("guardar textos, revisión, idempotencia y conflicto", func(t *testing.T) {
		view := fixture.view(t)
		before := fixture.counts(t)
		body := saveBody(view, map[string]any{
			"additional_info": "RFC: XAXX010101000\r\nPermiso municipal 123\t",
			"header_text":     "¡Bienvenidos!\x1b@",
			"footer_text":     "Gracias por su visita\n\n",
		})
		response, err := fixture.call(fixture.manager, "PUT", "documents/identity", body)
		if err != nil {
			t.Fatal(err)
		}
		saved := decodeView(t, response)
		if saved.Revision != view.Revision+1 || saved.AdditionalInfo != "RFC: XAXX010101000\nPermiso municipal 123" ||
			saved.HeaderText != "¡Bienvenidos!@" || saved.FooterText != "Gracias por su visita" {
			t.Fatalf("Textos guardados y saneados: %+v", saved)
		}
		if saved.UpdatedBy == nil || saved.UpdatedBy.ID != fixture.manager.UserID || saved.UpdatedBy.Name == "" {
			t.Fatalf("Quién guardó: %+v", saved.UpdatedBy)
		}
		after := fixture.counts(t)
		if after[2] != before[2]+1 || after[3] != before[3]+1 {
			t.Fatalf("Una revisión y un evento por cambio: %v → %v", before, after)
		}
		// Reintento con el mismo operation_id y cuerpo: misma respuesta, sin otro cambio.
		replayed, err := fixture.call(fixture.manager, "PUT", "documents/identity", body)
		if err != nil || !bytes.Equal(compact(t, replayed), compact(t, response)) {
			t.Fatalf("El reintento devuelve la respuesta guardada: %v", err)
		}
		if fixture.counts(t) != after {
			t.Fatal("El reintento no repite efectos")
		}
		body["footer_text"] = "Otro pie"
		_, err = fixture.call(fixture.manager, "PUT", "documents/identity", body)
		expectCode(t, err, "IDEMPOTENCY_CONFLICT")
		// Revisión anterior: otra persona guardó antes; nada cambia.
		stale := saveBody(view, map[string]any{"footer_text": "Pie de quien llegó tarde"})
		_, err = fixture.call(fixture.manager, "PUT", "documents/identity", stale)
		expectCode(t, err, "REVISION_CONFLICT")
		if current := fixture.view(t); current.FooterText != "Gracias por su visita" || fixture.counts(t) != after {
			t.Fatal("Un conflicto de revisión no escribe nada")
		}
		// Guardar sin cambios no crea revisión, auditoría ni evento.
		audits := len(fixture.audits)
		if _, err := fixture.call(fixture.manager, "PUT", "documents/identity", saveBody(saved, nil)); err != nil {
			t.Fatal(err)
		}
		if fixture.counts(t) != after || len(fixture.audits) != audits {
			t.Fatal("Guardar sin cambios no debe crear revisión, auditoría ni evento")
		}
		// Textos fuera de límites, claves desconocidas y presentación inválida: 422 sin escribir.
		rejected := []map[string]any{
			saveBody(saved, map[string]any{"header_text": strings.Repeat("x", 301)}),
			saveBody(saved, map[string]any{"footer_text": strings.Repeat("línea\n", 9)}),
			saveBody(saved, map[string]any{"html": "<b>no</b>"}),
			saveBody(saved, map[string]any{"presentation": map[string]any{"width_percent": 10, "align": "center", "method": "threshold", "threshold": 128}}),
			saveBody(saved, map[string]any{"presentation": map[string]any{"width_percent": 60, "align": "center", "method": "threshold", "threshold": 128, "escpos": "1b40"}}),
		}
		for _, candidate := range rejected {
			_, err := fixture.call(fixture.manager, "PUT", "documents/identity", candidate)
			expectCode(t, err, "VALIDATION_ERROR")
		}
		if fixture.counts(t) != after {
			t.Fatal("Un rechazo de validación no escribe nada")
		}
		_, err = fixture.call(fixture.reader, "PUT", "documents/identity", saveBody(saved, map[string]any{"footer_text": "x"}))
		expectCode(t, err, "PERMISSION_DENIED")
	})

	t.Run("cargar PNG: original, conversiones de 58 y 80 mm y vista previa", func(t *testing.T) {
		view := fixture.view(t)
		before := fixture.counts(t)
		content := logoPNG(t)
		response, err := fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(content, view.Revision))
		if err != nil {
			t.Fatal(err)
		}
		saved := decodeView(t, response)
		if saved.Logo == nil || saved.Logo.MediaType != "image/png" || saved.Logo.Width != 200 || saved.Logo.Height != 80 ||
			saved.Logo.SizeBytes != len(content) || len(saved.Logo.SHA256) != 64 || saved.Revision != view.Revision+1 {
			t.Fatalf("Logotipo guardado: %+v", saved.Logo)
		}
		if saved.Logo.DataURI != "" || len(saved.Logo.Rasters) != 2 || saved.Logo.Rasters[0].DataURI != "" {
			t.Fatal("La respuesta de una escritura no lleva vistas previas")
		}
		// 60 % de 384 = 230 puntos de ancho → 92 de alto; de 576 = 346 → 138 de alto.
		first, second := saved.Logo.Rasters[0], saved.Logo.Rasters[1]
		if first.PaperWidth != 58 || first.WidthDots != 384 || first.HeightDots != 92 ||
			second.PaperWidth != 80 || second.WidthDots != 576 || second.HeightDots != 138 {
			t.Fatalf("Conversiones: %+v %+v", first, second)
		}
		after := fixture.counts(t)
		if after[0] != before[0]+1 || after[1] != before[1]+2 || after[3] != before[3]+1 {
			t.Fatalf("Un original y dos conversiones guardadas: %v → %v", before, after)
		}
		current := fixture.view(t)
		if !strings.HasPrefix(current.Logo.DataURI, "data:image/png;base64,") {
			t.Fatal("La lectura muestra el original")
		}
		original, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(current.Logo.DataURI, "data:image/png;base64,"))
		if !bytes.Equal(original, content) {
			t.Fatal("El original se conserva byte a byte")
		}
		for _, raster := range current.Logo.Rasters {
			decodePreview(t, raster.DataURI, raster.WidthDots, raster.HeightDots)
		}
		// Auditoría sin bytes ni base64 del archivo.
		entry := fixture.audits[len(fixture.audits)-1]
		encoded := base64.StdEncoding.EncodeToString(content)
		if entry.Action != "documents.logo_uploaded" || entry.Entity != "document_logo" || entry.ID == nil ||
			*entry.ID != saved.Logo.ID {
			t.Fatalf("Auditoría de la carga: %+v", entry)
		}
		if bytes.Contains(entry.After, []byte(encoded[:40])) || len(entry.After) > 2048 || bytes.Contains(entry.After, []byte("data:")) {
			t.Fatalf("La auditoría no debe llevar el archivo: %s", entry.After)
		}
		if !bytes.Contains(entry.After, []byte(saved.Logo.SHA256)) {
			t.Fatal("La auditoría identifica el archivo por su SHA-256")
		}
		// La operación guardada para reintentos tampoco lleva imágenes.
		var stored int
		_ = fixture.pool.QueryRow(fixture.ctx, `SELECT max(octet_length(response::text))::int FROM app.restaurant_operations
            WHERE route = 'POST documents/identity/logo'`).Scan(&stored)
		if stored > 4096 {
			t.Fatalf("La respuesta guardada no debe llevar vistas previas: %d bytes", stored)
		}
	})

	t.Run("rechazos: SVG, GIF, dañado, demasiado grande o con dimensiones excesivas", func(t *testing.T) {
		view := fixture.view(t)
		before := fixture.counts(t)
		var gifBuffer bytes.Buffer
		_ = gif.Encode(&gifBuffer, image.NewPaletted(image.Rect(0, 0, 10, 10), []color.Color{color.Black, color.White}), nil)
		valid := logoPNG(t)
		oversized := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 1<<20)...)
		wide := encodePNG(t, image.NewGray(image.Rect(0, 0, 2049, 1)))
		cases := []struct {
			name, message string
			content       []byte
		}{
			{"SVG", "no se aceptan SVG", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
			{"GIF", "no se aceptan SVG, GIF", gifBuffer.Bytes()},
			{"PNG truncado", "dañado o incompleto", valid[:len(valid)/2]},
			{"más de 1 MiB", "como máximo 1 MiB", oversized},
			{"2049 píxeles", "entre 1 × 1 y 2048 × 2048", wide},
			{"vacío", "está vacío", []byte{}},
		}
		for _, testCase := range cases {
			_, err := fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(testCase.content, view.Revision))
			expectCode(t, err, "VALIDATION_ERROR")
			if !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("%s: mensaje %q", testCase.name, err.Error())
			}
		}
		body := upload(valid, view.Revision)
		body["content_base64"] = "no es base64!"
		_, err := fixture.call(fixture.manager, "POST", "documents/identity/logo", body)
		expectCode(t, err, "VALIDATION_ERROR")
		body["content_base64"] = strings.Repeat("A", 1398108)
		_, err = fixture.call(fixture.manager, "POST", "documents/identity/logo", body)
		expectCode(t, err, "VALIDATION_ERROR")
		_, err = fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(valid, view.Revision-1))
		expectCode(t, err, "REVISION_CONFLICT")
		if fixture.counts(t) != before {
			t.Fatal("Un archivo rechazado no guarda nada")
		}
	})

	t.Run("reemplazar con JPEG y conservar el anterior", func(t *testing.T) {
		view := fixture.view(t)
		previous := view.Logo
		before := fixture.counts(t)
		response, err := fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(logoJPEG(t), view.Revision))
		if err != nil {
			t.Fatal(err)
		}
		saved := decodeView(t, response)
		if saved.Logo.MediaType != "image/jpeg" || saved.Logo.ID == previous.ID || saved.Logo.Width != 300 {
			t.Fatalf("Reemplazo: %+v", saved.Logo)
		}
		if after := fixture.counts(t); after[0] != before[0]+1 || after[1] != before[1]+2 {
			t.Fatalf("El reemplazo agrega un original y dos conversiones: %v → %v", before, after)
		}
		// El anterior y sus conversiones siguen disponibles para los documentos ya emitidos.
		source := identity.NewRasters(fixture.pool)
		for _, raster := range previous.Rasters {
			stored, err := source.Raster(fixture.ctx, raster.RasterID)
			if err != nil || stored == nil || stored.WidthDots != raster.WidthDots || stored.HeightDots != raster.HeightDots ||
				len(stored.Bits) != raster.WidthDots/8*raster.HeightDots {
				t.Fatalf("La conversión anterior debe seguir disponible: %+v %v", stored, err)
			}
		}
		entry := fixture.audits[len(fixture.audits)-1]
		if !bytes.Contains(entry.After, []byte(previous.ID)) {
			t.Fatal("La auditoría del reemplazo nombra el logotipo anterior")
		}
		// Volver a cargar el mismo archivo reutiliza el original y sus conversiones.
		again := fixture.counts(t)
		response, err = fixture.call(fixture.manager, "POST", "documents/identity/logo", upload(logoPNG(t), saved.Revision))
		if err != nil {
			t.Fatal(err)
		}
		reloaded := decodeView(t, response)
		if reloaded.Logo.ID != previous.ID || reloaded.Logo.Rasters[0].RasterID != previous.Rasters[0].RasterID {
			t.Fatal("El mismo archivo reutiliza su registro y sus conversiones")
		}
		if after := fixture.counts(t); after[0] != again[0] || after[1] != again[1] {
			t.Fatalf("Sin originales ni conversiones nuevas: %v → %v", again, after)
		}
	})

	t.Run("presentación: conversiones nuevas, reutilizadas y vista previa sin guardar", func(t *testing.T) {
		view := fixture.view(t)
		initial := view.Logo.Rasters
		before := fixture.counts(t)
		dither := identity.Presentation{WidthPercent: 100, Align: "left", Method: "dither", Threshold: 77}
		// Vista previa: no guarda nada.
		response, err := fixture.call(fixture.manager, "POST", "documents/identity/logo/preview", map[string]any{"presentation": dither})
		if err != nil {
			t.Fatal(err)
		}
		var preview identity.PreviewResult
		_ = json.Unmarshal(response, &preview)
		if len(preview.Rasters) != 2 || preview.Rasters[0].RasterID != "" || preview.Rasters[0].HeightDots != 154 {
			t.Fatalf("Vista previa sin guardar: %+v", preview.Rasters)
		}
		for _, raster := range preview.Rasters {
			decodePreview(t, raster.DataURI, raster.WidthDots, raster.HeightDots)
		}
		if fixture.counts(t) != before {
			t.Fatal("La vista previa no escribe nada")
		}
		_, err = fixture.call(fixture.reader, "POST", "documents/identity/logo/preview", map[string]any{"presentation": dither})
		expectCode(t, err, "PERMISSION_DENIED")
		// Guardar la presentación genera las dos conversiones nuevas.
		response, err = fixture.call(fixture.manager, "PUT", "documents/identity", saveBody(view, map[string]any{"presentation": dither}))
		if err != nil {
			t.Fatal(err)
		}
		saved := decodeView(t, response)
		if saved.Presentation != dither || saved.Logo.Rasters[0].RasterID == initial[0].RasterID ||
			saved.Logo.Rasters[0].HeightDots != 154 {
			t.Fatalf("Presentación guardada: %+v %+v", saved.Presentation, saved.Logo.Rasters)
		}
		if after := fixture.counts(t); after[1] != before[1]+2 {
			t.Fatalf("Dos conversiones nuevas: %v → %v", before, after)
		}
		// Con difuminado el umbral no cambia la conversión: se reutiliza la guardada.
		withOtherThreshold := dither
		withOtherThreshold.Threshold = 200
		middle := fixture.counts(t)
		response, err = fixture.call(fixture.manager, "PUT", "documents/identity", saveBody(saved, map[string]any{"presentation": withOtherThreshold}))
		if err != nil {
			t.Fatal(err)
		}
		reused := decodeView(t, response)
		if reused.Logo.Rasters[0].RasterID != saved.Logo.Rasters[0].RasterID || fixture.counts(t)[1] != middle[1] {
			t.Fatal("El difuminado con otro umbral reutiliza la conversión")
		}
		// Volver a la presentación inicial reutiliza las conversiones de entonces.
		response, err = fixture.call(fixture.manager, "PUT", "documents/identity",
			saveBody(reused, map[string]any{"presentation": view.Presentation}))
		if err != nil {
			t.Fatal(err)
		}
		restored := decodeView(t, response)
		if restored.Logo.Rasters[0].RasterID != initial[0].RasterID || restored.Logo.Rasters[1].RasterID != initial[1].RasterID {
			t.Fatal("La presentación anterior reutiliza sus conversiones")
		}
		if fixture.counts(t)[1] != middle[1] {
			t.Fatal("Sin conversiones nuevas al volver a una presentación guardada")
		}
	})

	t.Run("fuente del logotipo de los reportes en una transacción de solo lectura", func(t *testing.T) {
		tx, err := fixture.pool.BeginTx(fixture.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		content, err := identity.ReportLogo(fixture.ctx, tx)
		if err != nil || !bytes.Equal(content, logoPNG(t)) {
			t.Fatalf("Los reportes reciben el original vigente: %d %v", len(content), err)
		}
		var advisory int
		_ = tx.QueryRow(fixture.ctx, "SELECT count(*)::int FROM pg_locks WHERE locktype = 'advisory' AND pid = pg_backend_pid()").Scan(&advisory)
		if advisory != 0 {
			t.Fatal("La fuente de los reportes no toma locks consultivos")
		}
	})

	t.Run("retirar: los documentos emitidos conservan su conversión", func(t *testing.T) {
		view := fixture.view(t)
		removed := view.Logo
		before := fixture.counts(t)
		_, err := fixture.call(fixture.manager, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": view.Revision - 1})
		expectCode(t, err, "REVISION_CONFLICT")
		response, err := fixture.call(fixture.manager, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": view.Revision})
		if err != nil {
			t.Fatal(err)
		}
		saved := decodeView(t, response)
		if saved.Logo != nil || saved.Presentation != view.Presentation || saved.Revision != view.Revision+1 {
			t.Fatalf("Retiro: %+v", saved)
		}
		after := fixture.counts(t)
		if after[0] != before[0] || after[1] != before[1] || after[3] != before[3]+1 {
			t.Fatalf("Retirar no borra originales ni conversiones: %v → %v", before, after)
		}
		entry := fixture.audits[len(fixture.audits)-1]
		if entry.Action != "documents.logo_removed" || entry.ID == nil || *entry.ID != removed.ID {
			t.Fatalf("Auditoría del retiro: %+v", entry)
		}
		stored, err := identity.NewRasters(fixture.pool).Raster(fixture.ctx, removed.Rasters[1].RasterID)
		if err != nil || stored == nil || stored.WidthDots != 576 {
			t.Fatal("Un documento emitido antes del retiro sigue encontrando su conversión")
		}
		uri, err := identity.NewRasters(fixture.pool).DataURI(fixture.ctx, removed.Rasters[0].RasterID)
		if err != nil {
			t.Fatal(err)
		}
		decodePreview(t, uri, 384, removed.Rasters[0].HeightDots)
		_, err = fixture.call(fixture.manager, "POST", "documents/identity/logo/remove",
			map[string]any{"operation_id": randomID(), "revision": saved.Revision})
		expectCode(t, err, "NO_LOGO")
		_, err = fixture.call(fixture.manager, "POST", "documents/identity/logo/preview",
			map[string]any{"presentation": identity.DefaultPresentation()})
		expectCode(t, err, "NO_LOGO")
		tx, err := fixture.pool.BeginTx(fixture.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if content, err := identity.ReportLogo(fixture.ctx, tx); content != nil || err != nil {
			t.Fatal("Sin logotipo los reportes usan la marca de AD-EL POS")
		}
		missing, err := identity.NewRasters(fixture.pool).Raster(fixture.ctx, randomID())
		if missing != nil || err != nil {
			t.Fatal("Una conversión inexistente no es un error")
		}
	})

	t.Run("originales y conversiones inmutables", func(t *testing.T) {
		for _, statement := range []string{
			"UPDATE app.document_logos SET media_type = 'image/jpeg'",
			"DELETE FROM app.document_logo_rasters",
			"DELETE FROM app.document_identity",
		} {
			if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
				t.Fatalf("Debe rechazarse: %s", statement)
			}
		}
	})

	t.Run("dos administradores a la vez: uno gana y el otro recibe el conflicto", func(t *testing.T) {
		view := fixture.view(t)
		results := make([]error, 2)
		var wait sync.WaitGroup
		for index := range results {
			wait.Add(1)
			go func() {
				defer wait.Done()
				body := saveBody(view, map[string]any{"footer_text": fmt.Sprintf("Pie del administrador %d", index+1)})
				_, results[index] = fixture.call(fixture.manager, "PUT", "documents/identity", body)
			}()
		}
		wait.Wait()
		winners := 0
		for _, err := range results {
			if err == nil {
				winners++
				continue
			}
			expectCode(t, err, "REVISION_CONFLICT")
		}
		current := fixture.view(t)
		if winners != 1 || current.Revision != view.Revision+1 || !strings.HasPrefix(current.FooterText, "Pie del administrador") {
			t.Fatalf("Exactamente un guardado gana: %v %+v", results, current.FooterText)
		}
		// El mismo operation_id a la vez: la segunda espera el lock de fila y recibe la respuesta
		// guardada de la primera.
		body := saveBody(current, map[string]any{"header_text": "Encabezado concurrente"})
		responses := make([]json.RawMessage, 2)
		for index := range responses {
			wait.Add(1)
			go func() {
				defer wait.Done()
				responses[index], results[index] = fixture.call(fixture.manager, "PUT", "documents/identity", body)
			}()
		}
		wait.Wait()
		if results[0] != nil || results[1] != nil || !bytes.Equal(compact(t, responses[0]), compact(t, responses[1])) {
			t.Fatalf("Un mismo operation_id concurrente se aplica una vez: %v", results)
		}
		if fixture.view(t).Revision != current.Revision+1 {
			t.Fatal("Una sola revisión para el mismo operation_id")
		}
	})

	routeSubtests(t, fixture)
}

// compact devuelve el JSON canónico de una respuesta: la guardada en restaurant_operations (jsonb)
// conserva los valores pero no el orden de las claves.
func compact(t *testing.T, value json.RawMessage) []byte {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
