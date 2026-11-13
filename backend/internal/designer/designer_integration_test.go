//go:build integration

package designer_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/stationscope"
)

// Aceptación del diseñador de documentos (etapa 9B, fase A) contra PostgreSQL: plantillas del
// sistema iguales a Go, crear, duplicar, guardar, renombrar, activar y restaurar, inmutabilidad de
// versiones, conflicto entre dos administradores, permisos antes de escribir, idempotencia,
// auditoría y evento, vista previa con datos de muestra y muestras impresas sin ventas ni pagos con
// el resultado de la cola (incierto incluido). Ninguna prueba imprime: la cola usa un transporte
// controlado y las colas CUPS son simuladas.

type terminal struct {
	deviceID     string
	deviceToken  string
	sessionToken string
}

type fixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	service      *administration.Service
	provisioning string
	admin        terminal
	queues       *queues
}

type queues struct {
	mutex   sync.Mutex
	options map[string]map[string]string
}

func (inspector *queues) InspectQueue(_ context.Context, queue string) kitchen.QueueCapability {
	inspector.mutex.Lock()
	defer inspector.mutex.Unlock()
	options, found := inspector.options[queue]
	if !found {
		return kitchen.QueueCapability{Queue: queue, Reason: "No se pudo consultar la cola " + queue + "."}
	}
	return kitchen.EvaluateRawQueue(queue, options)
}

type controlledTransport struct {
	mutex sync.Mutex
	state string
	jobs  []kitchen.PrintJob
}

func (transport *controlledTransport) Submit(_ context.Context, job kitchen.PrintJob) kitchen.Outcome {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	transport.jobs = append(transport.jobs, job)
	return kitchen.Outcome{State: transport.state, Accepted: transport.state == "submitted", Message: "Transporte controlado; no impresión física."}
}

func randomID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = raw[6]&15 | 64
	raw[8] = raw[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func decodeAs[T any](t *testing.T, body []byte) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("La respuesta no cumple el contrato: %v\n%s", err, body)
	}
	return result
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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
	inspector := &queues{options: map[string]map[string]string{
		"ADEL_TEST_USB": {"device-uri": "usb://Printer/POS-80", "printer-make-and-model": "POS-80", "printer-type": "2150404"},
		"ADEL_TEST_IPP": {"device-uri": "ipp://192.168.1.20/ipp/print", "printer-make-and-model": "Generic PostScript", "printer-type": "2150404"},
	}}
	value := &fixture{ctx: ctx, pool: pool, provisioning: token, queues: inspector,
		service: &administration.Service{Pool: pool, ProvisioningFile: filename, Queues: inspector}}
	setup := decodeAs[map[string]any](t, value.call(t, terminal{}, "POST", "setup", map[string]string{
		"name": "Restaurante del diseñador", "timezone": "America/Monterrey", "currency": "MXN", "theme": "light", "terminal_name": "Diseño",
	}, 200))
	value.admin.deviceID = setup["device_id"].(string)
	value.admin.deviceToken = setup["device_token"].(string)
	login := decodeAs[map[string]any](t, value.call(t, value.admin, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200))
	value.admin.sessionToken = login["session_token"].(string)
	return value
}

func (value *fixture) call(t *testing.T, client terminal, method, route string, body any, status int) []byte {
	t.Helper()
	code, response := value.raw(client, method, route, body)
	if code != status {
		t.Fatalf("%s %s: esperado %d recibido %d: %s", method, route, status, code, response)
	}
	return response
}

func (value *fixture) raw(client terminal, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:52000"
	request.Header.Set("X-Provisioning-Token", value.provisioning)
	request.Header.Set("X-Device-ID", client.deviceID)
	request.Header.Set("X-Device-Token", client.deviceToken)
	request.Header.Set("Authorization", "Bearer "+client.sessionToken)
	response := httptest.NewRecorder()
	value.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

// mutate envía una escritura con un operation_id nuevo.
func (value *fixture) mutate(t *testing.T, client terminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	body["operation_id"] = randomID()
	return value.call(t, client, method, route, body, status)
}

// user crea un usuario con un rol de los permisos indicados en una terminal propia.
func (value *fixture) user(t *testing.T, username string, permissions []string, administrator bool) terminal {
	t.Helper()
	roleID := ""
	if administrator {
		if err := value.pool.QueryRow(value.ctx, "SELECT id FROM app.roles WHERE code='administrator'").Scan(&roleID); err != nil {
			t.Fatal(err)
		}
	} else {
		roleID = decodeAs[struct {
			ID string `json:"id"`
		}](t, value.call(t, value.admin, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200)).ID
	}
	password := "Clave temporal de " + username
	value.call(t, value.admin, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": roleID, "active": true, "password": password,
	}, 200)
	invitation := decodeAs[struct {
		Code string `json:"code"`
	}](t, value.call(t, value.admin, "POST", "devices/invitations", nil, 200))
	device := decodeAs[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, value.call(t, terminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	client := terminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeAs[struct {
		Token string `json:"session_token"`
	}](t, value.call(t, client, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200))
	client.sessionToken = login.Token
	return client
}

func (value *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var total int
	if err := value.pool.QueryRow(value.ctx, query, args...).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

type templateDetail struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Name           string          `json:"name"`
	System         bool            `json:"system"`
	Active         bool            `json:"active"`
	Revision       int             `json:"revision"`
	Layout         json.RawMessage `json:"layout"`
	CurrentVersion struct {
		ID         string `json:"id"`
		Number     int    `json:"number"`
		BlockCount int    `json:"block_count"`
	} `json:"current_version"`
}

type problem struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func errorOf(t *testing.T, body []byte) problem {
	t.Helper()
	return decodeAs[struct {
		Error problem `json:"error"`
	}](t, body).Error
}

// editBlock modifica un bloque de un layout (el JSON de PostgreSQL ordena sus claves: se edita la
// estructura, nunca el texto).
func editBlock(t *testing.T, layout []byte, id string, edit func(block map[string]any)) json.RawMessage {
	t.Helper()
	var template map[string]any
	if err := json.Unmarshal(layout, &template); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range template["blocks"].([]any) {
		block := entry.(map[string]any)
		if block["id"] == id {
			edit(block)
			found = true
		}
	}
	if !found {
		t.Fatalf("El layout no tiene el bloque %s", id)
	}
	encoded, _ := json.Marshal(template)
	return encoded
}

// blockOf devuelve un bloque de un layout.
func blockOf(t *testing.T, layout []byte, id string) map[string]any {
	t.Helper()
	var template struct {
		Blocks []map[string]any `json:"blocks"`
	}
	if err := json.Unmarshal(layout, &template); err != nil {
		t.Fatal(err)
	}
	for _, block := range template.Blocks {
		if block["id"] == id {
			return block
		}
	}
	t.Fatalf("El layout no tiene el bloque %s", id)
	return nil
}

func sameJSON(t *testing.T, left, right []byte) bool {
	t.Helper()
	var first, second any
	if err := json.Unmarshal(left, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(right, &second); err != nil {
		t.Fatal(err)
	}
	firstText, _ := json.Marshal(first)
	secondText, _ := json.Marshal(second)
	return string(firstText) == string(secondText)
}

func TestDocumentDesignerAcceptance(t *testing.T) {
	value := newFixture(t)
	t.Run("semilla igual a Go y catálogo", func(t *testing.T) { testSeedAndCatalog(t, value) })
	t.Run("crear duplicar guardar renombrar activar y restaurar", func(t *testing.T) { testTemplateLifecycle(t, value) })
	t.Run("conflicto entre dos administradores", func(t *testing.T) { testConcurrentAdministrators(t, value) })
	t.Run("validación permisos e idempotencia", func(t *testing.T) { testValidationPermissionsAndIdempotency(t, value) })
	t.Run("vista previa con datos de muestra", func(t *testing.T) { testSamplePreview(t, value) })
	t.Run("importes de la lista de cocina según Cocina", func(t *testing.T) { testKitchenPricesFollowKitchen(t, value) })
	t.Run("muestras sin ventas ni pagos y resultado de la cola", func(t *testing.T) { testSamplePrinting(t, value) })
	// Etapa 9C (§5.3): vista previa y muestras con las columnas de la impresora elegida.
	t.Run("vista previa con la impresora elegida y sus columnas", func(t *testing.T) { testPrinterPreview(t, value) })
}

func testSeedAndCatalog(t *testing.T, value *fixture) {
	for _, info := range documents.Kinds {
		list := decodeAs[[]templateDetail](t, value.call(t, value.admin, "GET", "documents/templates?kind="+string(info.Kind), nil, 200))
		if len(list) != 1 || !list[0].System || !list[0].Active || list[0].ID != documents.SystemTemplateID(info.Kind) ||
			list[0].CurrentVersion.ID != documents.SystemVersionID(info.Kind) || list[0].CurrentVersion.Number != 1 {
			t.Fatalf("La plantilla del sistema de %s no está activa con su versión fija: %+v", info.Kind, list)
		}
		detail := decodeAs[templateDetail](t, value.call(t, value.admin, "GET", "documents/templates/"+list[0].ID, nil, 200))
		if !sameJSON(t, detail.Layout, documents.Default(info.Kind).Encode()) {
			t.Fatalf("La plantilla sembrada de %s no es la predeterminada de Go", info.Kind)
		}
	}
	catalog := decodeAs[documents.Catalog](t, value.call(t, value.admin, "GET", "documents/catalog", nil, 200))
	if len(catalog.Kinds) != 10 || len(catalog.PaperWidths) != 2 || catalog.PaperWidths[0].Columns != 30 {
		t.Fatalf("Catálogo incompleto: %+v", catalog.PaperWidths)
	}
}

func testTemplateLifecycle(t *testing.T, value *fixture) {
	system := documents.SystemTemplateID(documents.KindReceipt)
	created := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "receipt", "name": "  Ticket   con logotipo ", "source_version_id": documents.SystemVersionID(documents.KindReceipt),
	}, 200))
	if created.System || created.Active || created.Name != "Ticket con logotipo" || created.Revision != 1 || created.CurrentVersion.Number != 1 ||
		!sameJSON(t, created.Layout, documents.Default(documents.KindReceipt).Encode()) {
		t.Fatalf("La copia no conservó el layout de origen o quedó activa: %+v", created)
	}
	fresh := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "kitchen_order", "name": "Comanda nueva",
	}, 200))
	if !sameJSON(t, fresh.Layout, documents.Default(documents.KindKitchenOrder).Encode()) {
		t.Fatal("Sin origen, la plantilla nueva no partió de la predeterminada")
	}
	value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{"kind": "receipt", "name": "TICKET CON LOGOTIPO"}, 409)
	value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "precheck", "name": "Otra", "source_version_id": documents.SystemVersionID(documents.KindReceipt),
	}, 409)
	// Guardar crea la versión 2; la 1 sigue intacta y el historial las muestra.
	layout := editBlock(t, created.Layout, "name", func(block map[string]any) {
		block["align"] = "center"
		block["size"] = "double"
		block["bold"] = true
	})
	saved := decodeAs[templateDetail](t, value.mutate(t, value.admin, "PUT", "documents/templates/"+created.ID, map[string]any{
		"revision": created.Revision, "layout": layout, "note": "Nombre centrado",
	}, 200))
	name := blockOf(t, saved.Layout, "name")
	if saved.Revision != 2 || saved.CurrentVersion.Number != 2 || name["align"] != "center" || name["size"] != "double" || name["bold"] != true {
		t.Fatalf("El guardado no creó la versión 2: %+v", saved)
	}
	versions := decodeAs[[]struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		Note   string `json:"note"`
	}](t, value.call(t, value.admin, "GET", "documents/templates/"+created.ID+"/versions", nil, 200))
	if len(versions) != 2 || versions[0].Number != 2 || versions[0].Note != "Nombre centrado" || versions[1].Number != 1 {
		t.Fatalf("Historial incorrecto: %+v", versions)
	}
	first := decodeAs[struct {
		Layout json.RawMessage `json:"layout"`
	}](t, value.call(t, value.admin, "GET", "documents/versions/"+versions[1].ID, nil, 200))
	if !sameJSON(t, first.Layout, documents.Default(documents.KindReceipt).Encode()) {
		t.Fatal("La versión 1 cambió al guardar la 2")
	}
	if _, err := value.pool.Exec(value.ctx, "UPDATE app.document_template_versions SET note='x' WHERE id=$1", versions[1].ID); err == nil {
		t.Fatal("PostgreSQL permitió modificar una versión")
	}
	// Las plantillas del sistema son de solo lectura.
	systemDetail := decodeAs[templateDetail](t, value.call(t, value.admin, "GET", "documents/templates/"+system, nil, 200))
	readOnly := value.mutate(t, value.admin, "PUT", "documents/templates/"+system, map[string]any{"revision": systemDetail.Revision, "layout": layout}, 409)
	if errorOf(t, readOnly).Code != "TEMPLATE_READ_ONLY" {
		t.Fatalf("Se modificó la plantilla del sistema: %s", readOnly)
	}
	value.mutate(t, value.admin, "POST", "documents/templates/"+system+"/rename", map[string]any{"revision": systemDetail.Revision, "name": "Otra"}, 409)
	renamed := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates/"+created.ID+"/rename", map[string]any{
		"revision": saved.Revision, "name": "Ticket centrado",
	}, 200))
	if renamed.Name != "Ticket centrado" || renamed.Revision != 3 {
		t.Fatalf("Renombrar no cambió nombre y revisión: %+v", renamed)
	}
	// Activar deja una sola activa del tipo; restaurar vuelve a la del sistema.
	activated := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates/"+created.ID+"/activate", map[string]any{
		"revision": renamed.Revision,
	}, 200))
	if !activated.Active || value.count(t, "SELECT count(*) FROM app.document_templates WHERE kind='receipt' AND active") != 1 {
		t.Fatal("La activación no dejó exactamente una plantilla activa")
	}
	tx, err := value.pool.Begin(value.ctx)
	if err != nil {
		t.Fatal(err)
	}
	version, err := documents.ActiveVersion(value.ctx, tx, documents.KindReceipt)
	_ = tx.Rollback(value.ctx)
	if err != nil || version.ID != activated.CurrentVersion.ID || version.Template.Blocks[0].Align != documents.AlignCenter {
		t.Fatalf("La versión activa del tipo no es la de la plantilla activada: %+v %v", version, err)
	}
	restored := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/restore-default", map[string]any{"kind": "receipt"}, 200))
	if restored.ID != system || !restored.Active || value.count(t, "SELECT count(*) FROM app.document_templates WHERE kind='receipt' AND active") != 1 {
		t.Fatalf("Restaurar no activó la plantilla del sistema: %+v", restored)
	}
	for _, action := range []string{"documents.template_created", "documents.template_saved", "documents.template_renamed",
		"documents.template_activated", "documents.default_restored"} {
		if value.count(t, "SELECT count(*) FROM app.audit_events WHERE action=$1 AND success", action) == 0 {
			t.Fatalf("Falta la auditoría %s", action)
		}
	}
	if value.count(t, "SELECT count(*) FROM app.audit_events WHERE action='documents.template_saved' AND after_state::text LIKE '%blocks%'") != 0 {
		t.Fatal("La auditoría del guardado volcó el layout completo")
	}
	if value.count(t, "SELECT count(*) FROM app.realtime_events WHERE kind='documents.templates.changed' AND scope='documents'") < 5 {
		t.Fatal("Las escrituras no publicaron el evento de plantillas")
	}
}

// Dos administradores guardan la misma plantilla a la vez: uno gana y el otro recibe 409 sin
// sobrescribir; su borrador no se pierde porque el cliente lo conserva.
func testConcurrentAdministrators(t *testing.T, value *fixture) {
	second := value.user(t, "segunda_admin", nil, true)
	created := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "cash_partial_cut", "name": "Corte compartido",
	}, 200))
	title := func(text string) json.RawMessage {
		return editBlock(t, created.Layout, "title", func(block map[string]any) {
			block["options"] = map[string]any{"text": text}
		})
	}
	first := title("CORTE DE LA TARDE")
	other := title("CORTE DE LA NOCHE")
	type outcome struct {
		code int
		body []byte
	}
	results := make(chan outcome, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, entry := range []struct {
		client terminal
		layout json.RawMessage
	}{{value.admin, first}, {second, other}} {
		go func(client terminal, layout json.RawMessage) {
			start.Wait()
			code, body := value.raw(client, "PUT", "documents/templates/"+created.ID, map[string]any{
				"operation_id": randomID(), "revision": created.Revision, "layout": layout,
			})
			results <- outcome{code, body}
		}(entry.client, entry.layout)
	}
	start.Done()
	one, two := <-results, <-results
	if !((one.code == 200 && two.code == 409) || (one.code == 409 && two.code == 200)) {
		t.Fatalf("Los dos guardados no se resolvieron con un ganador y un conflicto: %d %d", one.code, two.code)
	}
	winner, loser := one, two
	if one.code == 409 {
		winner, loser = two, one
	}
	if errorOf(t, loser.body).Code != "REVISION_CONFLICT" {
		t.Fatalf("El perdedor no recibió REVISION_CONFLICT: %s", loser.body)
	}
	saved := decodeAs[templateDetail](t, winner.body)
	current := decodeAs[templateDetail](t, value.call(t, value.admin, "GET", "documents/templates/"+created.ID, nil, 200))
	if current.CurrentVersion.ID != saved.CurrentVersion.ID || current.CurrentVersion.Number != 2 ||
		value.count(t, "SELECT count(*) FROM app.document_template_versions WHERE template_id=$1", created.ID) != 2 {
		t.Fatal("El conflicto sobrescribió la versión ganadora o creó una versión de más")
	}
}

func testValidationPermissionsAndIdempotency(t *testing.T, value *fixture) {
	created := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "precheck", "name": "Precuenta de prueba",
	}, 200))
	// Un bloque protegido fuera de los cortes y uno incompatible se rechazan en Go con su bloque.
	for _, layout := range []string{
		`{"blocks":[{"id":"esperado","type":"expected_differences"},{"id":"f","type":"fiscal_notice"}]}`,
		`{"blocks":[{"id":"cocina","type":"kitchen_items"},{"id":"f","type":"fiscal_notice"}]}`,
		`{"blocks":[{"id":"f","type":"fiscal_notice","visible":false}]}`,
	} {
		body := value.mutate(t, value.admin, "PUT", "documents/templates/"+created.ID, map[string]any{"revision": created.Revision, "layout": json.RawMessage(layout)}, 422)
		rejection := errorOf(t, body)
		if rejection.Code != "TEMPLATE_INVALID" || rejection.Details["block_id"] == "" {
			t.Fatalf("El rechazo no señaló el bloque: %s", body)
		}
	}
	// Un texto con controles se guarda saneado.
	saved := decodeAs[templateDetail](t, value.mutate(t, value.admin, "PUT", "documents/templates/"+created.ID, map[string]any{
		"revision": created.Revision,
		"layout":   json.RawMessage(`{"blocks":[{"id":"t","type":"fixed_text","options":{"text":"Gracias\u001b@\u001dV\u0000 por su visita"}},{"id":"f","type":"fiscal_notice"}]}`),
	}, 200))
	if strings.Contains(string(saved.Layout), `\u001b`) || !strings.Contains(string(saved.Layout), "Gracias@V por su visita") {
		t.Fatalf("El texto fijo no se saneó al guardar: %s", saved.Layout)
	}
	// Idempotencia: el mismo cuerpo devuelve la respuesta guardada; otro cuerpo, conflicto.
	operation := randomID()
	body := map[string]any{"operation_id": operation, "revision": saved.Revision, "name": "Precuenta idempotente"}
	firstResponse := value.call(t, value.admin, "POST", "documents/templates/"+created.ID+"/rename", body, 200)
	replayed := value.call(t, value.admin, "POST", "documents/templates/"+created.ID+"/rename", body, 200)
	if !sameJSON(t, firstResponse, replayed) {
		t.Fatal("El reintento no devolvió la respuesta guardada")
	}
	body["name"] = "Otro nombre"
	if errorOf(t, value.call(t, value.admin, "POST", "documents/templates/"+created.ID+"/rename", body, 409)).Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("Otro cuerpo con el mismo operation_id no se rechazó")
	}
	// Permisos antes de escribir: sin consultar plantillas no hay catálogo; con consultar pero sin
	// administrar no se crea nada; la vista previa real exige su permiso.
	nobody := value.user(t, "sin_documentos", []string{"settings.read"}, false)
	value.call(t, nobody, "GET", "documents/catalog", nil, 403)
	reader := value.user(t, "lector_documentos", []string{documents.PermissionTemplatesRead}, false)
	before := value.count(t, "SELECT count(*) FROM app.document_templates")
	value.mutate(t, reader, "POST", "documents/templates", map[string]any{"kind": "receipt", "name": "No autorizada"}, 403)
	value.mutate(t, reader, "POST", "documents/restore-default", map[string]any{"kind": "receipt"}, 403)
	if value.count(t, "SELECT count(*) FROM app.document_templates") != before {
		t.Fatal("Un 403 dejó una plantilla creada")
	}
	value.call(t, reader, "POST", "documents/preview", map[string]any{
		"kind": "receipt", "layout": json.RawMessage(documents.Default(documents.KindReceipt).Encode()), "paper_width": 80,
		"source": map[string]any{"ticket_id": randomID()},
	}, 403)
	value.call(t, reader, "GET", "documents/recent?kind=receipt", nil, 403)
	value.mutate(t, reader, "POST", "documents/samples", map[string]any{
		"kind": "receipt", "layout": json.RawMessage(documents.Default(documents.KindReceipt).Encode()), "printer_id": randomID(),
	}, 403)
	// El evento de plantillas solo llega a quien consulta plantillas.
	tx, err := value.pool.Begin(value.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(value.ctx)
	var cursor int64
	if err := tx.QueryRow(value.ctx, "SELECT max(sequence)-1 FROM app.realtime_events WHERE scope='documents'").Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	visible, err := events.Read(value.ctx, tx, cursor, []string{documents.PermissionTemplatesRead}, stationscope.Everything(), "")
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := events.Read(value.ctx, tx, cursor, []string{"settings.read"}, stationscope.Everything(), "")
	if err != nil {
		t.Fatal(err)
	}
	countScope := func(batch *events.Batch) int {
		total := 0
		for _, event := range batch.Events {
			if event.Scope == documents.Scope {
				total++
			}
		}
		return total
	}
	if countScope(visible) == 0 || countScope(hidden) != 0 {
		t.Fatal("El ámbito documents no respeta documents.templates.read")
	}
}

type previewResponse struct {
	Columns          int    `json:"columns"`
	Sample           bool   `json:"sample"`
	IncludesExpected bool   `json:"includes_expected"`
	Text             string `json:"text"`
	Lines            []struct {
		Text    string `json:"text"`
		BlockID string `json:"block_id"`
	} `json:"lines"`
	Warnings    []string `json:"warnings"`
	EmptyBlocks []struct {
		BlockID string `json:"block_id"`
		Reason  string `json:"reason"`
	} `json:"empty_blocks"`
}

func testSamplePreview(t *testing.T, value *fixture) {
	accounts := value.count(t, "SELECT count(*) FROM app.restaurant_accounts")
	layout := `{"blocks":[{"id":"logo","type":"logo"},{"id":"nombre","type":"establishment_name","align":"center"},
		{"id":"esperado","type":"expected_differences"},{"id":"f","type":"fiscal_notice"}]}`
	for _, paper := range []int{58, 80} {
		preview := decodeAs[previewResponse](t, value.call(t, value.admin, "POST", "documents/preview", map[string]any{
			"kind": "cash_final_cut", "layout": json.RawMessage(layout), "paper_width": paper, "source": map[string]any{"sample": true},
		}, 200))
		if !preview.Sample || preview.Columns != documents.ColumnsFor(paper) || !preview.IncludesExpected ||
			!strings.Contains(preview.Text, "Restaurante del diseñador") || !strings.Contains(preview.Text, "EFECTIVO ESPERADO") {
			t.Fatalf("Vista previa de muestra incorrecta a %d mm: %+v", paper, preview)
		}
		if len(preview.EmptyBlocks) != 1 || preview.EmptyBlocks[0].BlockID != "logo" || len(preview.Warnings) != 1 || preview.Lines[0].BlockID != "nombre" {
			t.Fatalf("La vista previa no informó el logotipo vacío o el bloque de cada línea: %+v", preview)
		}
	}
	// Sin cash.expected.read el bloque protegido de la muestra imprime la nota de siempre.
	reader := value.user(t, "lector_vista", []string{documents.PermissionTemplatesRead}, false)
	hidden := decodeAs[previewResponse](t, value.call(t, reader, "POST", "documents/preview", map[string]any{
		"kind": "cash_final_cut", "layout": json.RawMessage(layout), "paper_width": 80, "source": map[string]any{"sample": true},
	}, 200))
	if hidden.IncludesExpected || strings.Contains(hidden.Text, "EFECTIVO ESPERADO") || !strings.Contains(hidden.Text, "no incluidos") {
		t.Fatalf("La muestra mostró el esperado sin permiso:\n%s", hidden.Text)
	}
	invalidBody := value.call(t, value.admin, "POST", "documents/preview", map[string]any{
		"kind": "receipt", "layout": json.RawMessage(`{"blocks":[{"id":"x","type":"opening_float"},{"id":"f","type":"fiscal_notice"}]}`),
		"paper_width": 80, "source": map[string]any{"sample": true},
	}, 422)
	if errorOf(t, invalidBody).Details["block_id"] != "x" {
		t.Fatal("La vista previa no señaló el bloque incompatible")
	}
	if value.count(t, "SELECT count(*) FROM app.restaurant_accounts") != accounts {
		t.Fatal("La vista previa escribió datos")
	}
}

func testSamplePrinting(t *testing.T, value *fixture) {
	printer := decodeAs[kitchen.Printer](t, value.mutate(t, value.admin, "POST", "kitchen/printers", map[string]any{
		"name": "Muestras USB", "transport": "cups", "destination": "ADEL_TEST_USB", "active": true, "paper_width": 58, "copies": 1,
		"accepts_escpos": true,
	}, 200))
	ipp := decodeAs[kitchen.Printer](t, value.mutate(t, value.admin, "POST", "kitchen/printers", map[string]any{
		"name": "Muestras IPP", "transport": "cups", "destination": "ADEL_TEST_IPP", "active": true, "paper_width": 80, "copies": 1,
		"accepts_escpos": true,
	}, 200))
	// Una cola raw de una impresora sin la marca ESC/POS (por omisión) no recibe formato ni logotipo.
	unmarked := decodeAs[kitchen.Printer](t, value.mutate(t, value.admin, "POST", "kitchen/printers", map[string]any{
		"name": "Muestras USB sin marca", "transport": "cups", "destination": "ADEL_TEST_USB", "active": true, "paper_width": 80, "copies": 1,
	}, 200))
	if !printer.AcceptsESCPos || unmarked.AcceptsESCPos {
		t.Fatalf("La marca ESC/POS no se guardó como se pidió: %+v %+v", printer, unmarked)
	}
	compatibility := decodeAs[[]struct {
		ID     string `json:"id"`
		Format struct {
			Status string `json:"status"`
			Code   string `json:"code"`
		} `json:"format"`
	}](t, value.call(t, value.admin, "GET", "documents/printers", nil, 200))
	statuses := map[string]string{}
	for _, item := range compatibility {
		statuses[item.ID] = item.Format.Status + "/" + item.Format.Code
	}
	if statuses[printer.ID] != "supported/queue_raw" || statuses[ipp.ID] != "unsupported/queue_not_raw" ||
		statuses[unmarked.ID] != "unsupported/escpos_disabled" {
		t.Fatalf("Compatibilidad de impresoras incorrecta: %+v", statuses)
	}
	// La marca se cambia con PATCH, se consulta con GET kitchen/printers y solo existe en CUPS.
	marked := unmarked
	marked.AcceptsESCPos = true
	marked.Active = false
	saved := decodeAs[kitchen.Printer](t, value.mutate(t, value.admin, "PATCH", "kitchen/printers/"+unmarked.ID, printerBody(marked), 200))
	listedPrinters := decodeAs[[]kitchen.Printer](t, value.call(t, value.admin, "GET", "kitchen/printers", nil, 200))
	found := false
	for _, item := range listedPrinters {
		found = found || (item.ID == saved.ID && item.AcceptsESCPos && !item.Active)
	}
	if !found {
		t.Fatalf("La marca ESC/POS no se guardó con PATCH: %+v", listedPrinters)
	}
	network := saved
	network.Transport = "escpos_tcp"
	network.Destination = "192.168.1.60"
	rejected := errorOf(t, value.mutate(t, value.admin, "PATCH", "kitchen/printers/"+saved.ID, printerBody(network), 400))
	if !strings.Contains(rejected.Message, "solo se configura en colas CUPS") {
		t.Fatalf("La marca ESC/POS se aceptó en ESC/POS por red: %+v", rejected)
	}
	counts := func() string {
		return fmt.Sprint(
			value.count(t, "SELECT count(*) FROM app.restaurant_accounts"), value.count(t, "SELECT count(*) FROM app.payments"),
			value.count(t, "SELECT count(*) FROM app.cash_movements"), value.count(t, "SELECT count(*) FROM app.cash_operations"),
			value.count(t, "SELECT count(*) FROM app.payment_documents"), value.count(t, "SELECT count(*) FROM app.cash_documents"),
			value.count(t, "SELECT count(*) FROM app.inventory_movements"),
		)
	}
	before := counts()
	layout := editBlock(t, documents.Default(documents.KindReceipt).Encode(), "total", func(block map[string]any) {
		block["size"] = "double_height"
		block["bold"] = true
	})
	result := decodeAs[struct {
		Status   string `json:"status"`
		JobID    string `json:"job_id"`
		TicketID string `json:"ticket_id"`
	}](t, value.mutate(t, value.admin, "POST", "documents/samples", map[string]any{"kind": "receipt", "layout": layout, "printer_id": printer.ID}, 200))
	if result.Status != "pending" || counts() != before {
		t.Fatalf("La muestra no quedó en la cola o registró ventas, pagos o movimientos: %+v %s → %s", result, before, counts())
	}
	var kind, content string
	var stored *string
	if err := value.pool.QueryRow(value.ctx, `SELECT t.kind,j.content,j.layout::text FROM app.kitchen_tickets t JOIN app.kitchen_print_jobs j
        ON j.ticket_id=t.id WHERE j.id=$1`, result.JobID).Scan(&kind, &content, &stored); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(content, "\n")
	if kind != documents.TicketKindSample || !strings.Contains(lines[0], "MUESTRA") || !strings.Contains(lines[len(lines)-1], "VÁLIDO") ||
		stored == nil || !strings.Contains(*stored, `"bold": true`) {
		t.Fatalf("La muestra no se identificó como tal o perdió sus atributos: %s %q", kind, content)
	}
	listed := string(value.call(t, value.admin, "GET", "kitchen/tickets", nil, 200))
	if strings.Contains(listed, result.TicketID) {
		t.Fatal("La muestra apareció entre las comandas")
	}
	if value.count(t, "SELECT count(*) FROM app.audit_events WHERE action='documents.sample_printed'") != 1 {
		t.Fatal("La muestra no se auditó")
	}
	// Con el transporte controlado el resultado incierto queda en la cola y nunca se reenvía solo.
	if _, err := value.pool.Exec(value.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state='pending' AND id<>$1", result.JobID); err != nil {
		t.Fatal(err)
	}
	transport := &controlledTransport{state: "uncertain"}
	worker := kitchen.Worker{Pool: value.pool, Transport: transport}
	if worked, err := worker.PrintNext(value.ctx); err != nil || !worked {
		t.Fatalf("El trabajador no tomó la muestra: %v", err)
	}
	if worked, err := worker.PrintNext(value.ctx); err != nil || worked || len(transport.jobs) != 1 {
		t.Fatal("Se reenvió automáticamente una muestra con resultado incierto")
	}
	sent := transport.jobs[0]
	if sent.Layout == nil || sent.Layout.Plain() || sent.TicketKind != documents.TicketKindSample || sent.Content != content {
		t.Fatalf("El transporte no recibió la IR de la muestra: %+v", sent.Layout)
	}
	if value.count(t, "SELECT count(*) FROM app.kitchen_print_jobs WHERE id=$1 AND state='uncertain'", result.JobID) != 1 {
		t.Fatal("La muestra no quedó incierta en la cola")
	}
	job := decodeAs[struct {
		Job map[string]any `json:"job"`
	}](t, value.call(t, value.admin, "GET", "kitchen/jobs/"+result.JobID, nil, 200))
	if _, found := job.Job["layout"]; found {
		t.Fatal("La consulta de la cola expone la IR del trabajo")
	}
	if counts() != before {
		t.Fatal("Imprimir la muestra registró ventas, pagos o movimientos")
	}
}

// «Importes» de la lista de cocina (revisión de la 9B, hallazgo medio 3): «Sí» se rechaza con 422 al
// guardar y en la vista previa, el catálogo no lo ofrece y la vista previa con datos de muestra
// aplica la configuración vigente de Cocina como una comanda real.
func testKitchenPricesFollowKitchen(t *testing.T, value *fixture) {
	yes := json.RawMessage(`{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"yes"}}]}`)
	preview := func(layout json.RawMessage, status int) []byte {
		return value.call(t, value.admin, "POST", "documents/preview", map[string]any{
			"kind": "kitchen_order", "layout": layout, "paper_width": 80, "source": map[string]any{"sample": true},
		}, status)
	}
	if rejected := errorOf(t, preview(yes, 422)); rejected.Code != "TEMPLATE_INVALID" || rejected.Details["block_id"] != "lista" ||
		!strings.Contains(rejected.Message, "«Incluir importes comerciales en comandas»") {
		t.Fatalf("«Importes: Sí» no se rechazó en la vista previa: %+v", rejected)
	}
	created := decodeAs[templateDetail](t, value.mutate(t, value.admin, "POST", "documents/templates", map[string]any{
		"kind": "kitchen_order", "name": "Comanda con importes",
	}, 200))
	saved := errorOf(t, value.mutate(t, value.admin, "PUT", "documents/templates/"+created.ID, map[string]any{
		"revision": created.Revision, "layout": yes,
	}, 422))
	if saved.Code != "TEMPLATE_INVALID" || saved.Details["block_id"] != "lista" {
		t.Fatalf("«Importes: Sí» no se rechazó al guardar: %+v", saved)
	}
	catalog := string(value.call(t, value.admin, "GET", "documents/catalog", nil, 200))
	if !strings.Contains(catalog, `"key":"show_prices","label":"Importes","type":"enum","values":[{"value":"inherit"`) ||
		!strings.Contains(catalog, `{"value":"no","label":"Ocultar"}]`) || !strings.Contains(catalog, "activa «Incluir importes comerciales en comandas» en Cocina") {
		t.Fatal("El catálogo debe ofrecer «Importes» solo con «Según Cocina» u «Ocultar» y explicar cómo imprimirlos")
	}
	inherit := json.RawMessage(`{"blocks":[{"id":"lista","type":"kitchen_items"}]}`)
	// Cocina con «Mostrar precios» apagado (por omisión): ni la vista previa ni la comanda los llevan.
	if text := decodeAs[previewResponse](t, preview(inherit, 200)).Text; strings.Contains(text, "Importe:") {
		t.Fatalf("La vista previa mostró importes con «Mostrar precios» apagado:\n%s", text)
	}
	settings := decodeAs[kitchen.Settings](t, value.call(t, value.admin, "GET", "kitchen/settings", nil, 200))
	settings.ShowPrices = true
	settings.ShowNotes = false
	updated := decodeAs[kitchen.Settings](t, value.mutate(t, value.admin, "PUT", "kitchen/settings", settingsBody(settings), 200))
	text := decodeAs[previewResponse](t, preview(inherit, 200)).Text
	if !strings.Contains(text, "Importe: $250.00") || strings.Contains(text, "NOTA:") {
		t.Fatalf("La vista previa no siguió la configuración de Cocina:\n%s", text)
	}
	hidden := json.RawMessage(`{"blocks":[{"id":"lista","type":"kitchen_items","options":{"show_prices":"no"}}]}`)
	if text := decodeAs[previewResponse](t, preview(hidden, 200)).Text; strings.Contains(text, "Importe:") {
		t.Fatalf("«Ocultar» imprimió importes:\n%s", text)
	}
	updated.ShowPrices = false
	updated.ShowNotes = true
	value.mutate(t, value.admin, "PUT", "kitchen/settings", settingsBody(updated), 200)
}

// settingsBody es el cuerpo de PUT kitchen/settings con la configuración indicada.
func settingsBody(settings kitchen.Settings) map[string]any {
	var body map[string]any
	encoded, _ := json.Marshal(settings)
	_ = json.Unmarshal(encoded, &body)
	return body
}

// printerBody es el cuerpo de POST o PATCH kitchen/printers con la impresora indicada.
func printerBody(printer kitchen.Printer) map[string]any {
	var body map[string]any
	encoded, _ := json.Marshal(printer)
	_ = json.Unmarshal(encoded, &body)
	return body
}
