package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/logoraster"
)

// Pruebas sin base de datos: saneamiento y límites de los textos, presentación, rutas y permisos
// (que se comprueban antes de tocar la transacción) y huella idempotente.

func publicCode(t *testing.T, err error) string {
	t.Helper()
	var problem *Error
	if !errors.As(err, &problem) {
		t.Fatalf("Se esperaba un rechazo público y se obtuvo %v", err)
	}
	return problem.Code
}

func TestNormalizeTextCleansControlsAndSpacing(t *testing.T) {
	cases := []struct {
		name, input, expected string
	}{
		{"saltos de Windows", "Línea 1\r\nLínea 2\rLínea 3", "Línea 1\nLínea 2\nLínea 3"},
		{"tabulador", "RFC:\tXAXX010101000", "RFC: XAXX010101000"},
		{"comandos ESC/POS", "Gracias\x1b@\x1dV\x00 por su visita", "Gracias@V por su visita"},
		{"invisibles", "Hola​‮mundo­", "Holamundo"},
		{"controles C1", "Pie\u0085\u009b", "Pie"},
		{"espacios finales y líneas vacías", "\n\n  Encabezado   \n  segunda  \n\n", "  Encabezado\n  segunda"},
		{"acentos y eñes", "Ñandú · café", "Ñandú · café"},
		{"vacío", "   \n \t ", ""},
	}
	for _, testCase := range cases {
		result, err := normalizeText("El texto", testCase.input)
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if result != testCase.expected {
			t.Fatalf("%s: esperado %q y se obtuvo %q", testCase.name, testCase.expected, result)
		}
	}
}

func TestNormalizeTextLimits(t *testing.T) {
	if _, err := normalizeText("El texto de pie", strings.Repeat("é", MaxTextCharacters)); err != nil {
		t.Fatalf("300 caracteres deben aceptarse: %v", err)
	}
	_, err := normalizeText("El texto de pie", strings.Repeat("é", MaxTextCharacters+1))
	if publicCode(t, err) != "VALIDATION_ERROR" || !strings.Contains(err.Error(), "El texto de pie admite como máximo 300 caracteres.") {
		t.Fatalf("Mensaje de longitud: %v", err)
	}
	eight := strings.TrimSuffix(strings.Repeat("línea\n", MaxTextLines), "\n")
	if _, err := normalizeText("La información adicional", eight); err != nil {
		t.Fatalf("Ocho líneas deben aceptarse: %v", err)
	}
	_, err = normalizeText("La información adicional", eight+"\nnovena")
	if err == nil || err.Error() != "La información adicional admite como máximo 8 líneas." {
		t.Fatalf("Mensaje de líneas: %v", err)
	}
	// Las líneas vacías al final no cuentan: se recortan antes de aplicar el límite.
	if _, err := normalizeText("El texto", eight+"\n\n\n"); err != nil {
		t.Fatalf("Las líneas vacías finales no deben contar: %v", err)
	}
	if _, err := normalizeText("El texto", "a\xffb"); err == nil {
		t.Fatal("Un texto con UTF-8 inválido debe rechazarse")
	}
}

func TestPresentationValidationAndRasterKey(t *testing.T) {
	presentation := DefaultPresentation()
	if presentation != (Presentation{WidthPercent: 60, Align: "center", Method: "threshold", Threshold: 128}) {
		t.Fatalf("La presentación inicial debe coincidir con la 0019: %+v", presentation)
	}
	if err := presentation.validate(); err != nil {
		t.Fatal(err)
	}
	invalidCases := []Presentation{
		{WidthPercent: 24, Align: "center", Method: "threshold", Threshold: 128},
		{WidthPercent: 101, Align: "center", Method: "threshold", Threshold: 128},
		{WidthPercent: 60, Align: "justify", Method: "threshold", Threshold: 128},
		{WidthPercent: 60, Align: "center", Method: "halftone", Threshold: 128},
		{WidthPercent: 60, Align: "center", Method: "threshold", Threshold: 0},
		{WidthPercent: 60, Align: "center", Method: "dither", Threshold: 256},
	}
	for _, candidate := range invalidCases {
		if publicCode(t, candidate.validate()) != "VALIDATION_ERROR" {
			t.Fatalf("La presentación %+v debe rechazarse", candidate)
		}
	}
	dither := Presentation{WidthPercent: 80, Align: "left", Method: "dither", Threshold: 40}
	if dither.rasterThreshold() != 128 {
		t.Fatal("El difuminado se guarda con su punto de decisión fijo (128)")
	}
	threshold := Presentation{WidthPercent: 80, Align: "left", Method: "threshold", Threshold: 40}
	if threshold.rasterThreshold() != 40 {
		t.Fatal("El umbral se guarda con el valor elegido")
	}
	options := threshold.options(logoraster.Paper80)
	if options.PaperWidth != 80 || options.WidthPercent != 80 || options.Align != "left" || options.Threshold != 40 {
		t.Fatalf("Opciones de conversión: %+v", options)
	}
}

func TestRoutesAndPermissionsBeforeTouchingTheTransaction(t *testing.T) {
	ctx := context.Background()
	reader := Actor{UserID: "u", DeviceID: "d", Permissions: []string{PermissionSettingsRead}}
	templatesReader := Actor{Permissions: []string{PermissionTemplatesRead}}
	nobody := Actor{Permissions: []string{"accounts.read"}}
	cases := []struct {
		method, route string
		actor         Actor
		code          string
	}{
		{"GET", "documents/identity", nobody, "PERMISSION_DENIED"},
		{"PUT", "documents/identity", reader, "PERMISSION_DENIED"},
		{"PUT", "documents/identity", templatesReader, "PERMISSION_DENIED"},
		{"POST", "documents/identity/logo", reader, "PERMISSION_DENIED"},
		{"POST", "documents/identity/logo/remove", templatesReader, "PERMISSION_DENIED"},
		{"POST", "documents/identity/logo/preview", reader, "PERMISSION_DENIED"},
		{"DELETE", "documents/identity", reader, "NOT_FOUND"},
		{"GET", "documents/identity/logo", reader, "NOT_FOUND"},
		{"POST", "documents/identity/rasters", reader, "NOT_FOUND"},
	}
	for _, testCase := range cases {
		// Sin transacción: un rechazo de permisos o de ruta nunca llega a la base.
		_, err := Handle(ctx, nil, Request{Method: testCase.method, Route: testCase.route, Actor: testCase.actor}, nil)
		if publicCode(t, err) != testCase.code {
			t.Fatalf("%s %s: esperado %s y se obtuvo %v", testCase.method, testCase.route, testCase.code, err)
		}
	}
	manager := Actor{Permissions: []string{PermissionManage}}
	for _, body := range []string{``, `{}`, `{"operation_id":"no-es-uuid"}`, `[1]`} {
		_, err := Handle(ctx, nil, Request{Method: "PUT", Route: "documents/identity", Body: []byte(body), Actor: manager}, nil)
		if publicCode(t, err) != "VALIDATION_ERROR" {
			t.Fatalf("Sin operation_id válido (%q) la escritura debe rechazarse antes del lock: %v", body, err)
		}
	}
	for _, route := range Routes {
		if !Owns(route) || !Owns("/"+route+"/") {
			t.Fatalf("La identidad debe atender %s", route)
		}
	}
	if Owns("documents/templates") || Owns("documents/identity/x") {
		t.Fatal("La identidad no debe atender rutas ajenas")
	}
}

func TestFingerprintIncludesTheContent(t *testing.T) {
	request := Request{Method: "POST", Route: "documents/identity/logo",
		Body: []byte(`{"operation_id":"a","revision":1,"content_base64":"AAAA"}`)}
	first, err := requestFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	reordered := request
	reordered.Body = []byte(`{"content_base64":"AAAA","revision":1,"operation_id":"a"}`)
	second, _ := requestFingerprint(reordered)
	if first != second {
		t.Fatal("La huella es la del JSON canónico: el orden de las claves no importa")
	}
	other := request
	other.Body = []byte(`{"operation_id":"a","revision":1,"content_base64":"AAAB"}`)
	third, _ := requestFingerprint(other)
	if third == first {
		t.Fatal("Otro archivo con el mismo operation_id debe tener otra huella")
	}
	if _, err := requestFingerprint(Request{Body: []byte(`{`)}); err == nil {
		t.Fatal("Un cuerpo inválido no tiene huella")
	}
}

func TestContractConstants(t *testing.T) {
	if Scope != documents.Scope {
		t.Fatalf("El ámbito SSE debe ser el de documentos: %s", documents.Scope)
	}
	if PermissionManage != documents.PermissionIdentityManage || PermissionTemplatesRead != documents.PermissionTemplatesRead {
		t.Fatal("Los permisos deben ser los registrados por documents")
	}
	if maxEncodedLogo != 1398104 {
		t.Fatalf("El base64 de 1 MiB mide 1 398 104 caracteres: %d", maxEncodedLogo)
	}
	limits := currentLimits()
	if limits.TextMaxCharacters != 300 || limits.TextMaxLines != 8 || limits.LogoMaxBytes != 1<<20 ||
		limits.LogoMaxDimension != 2048 || limits.MaxHeightDots != 320 || len(limits.PaperWidths) != 2 ||
		limits.PaperWidths[0] != (PaperWidth{58, 384}) || limits.PaperWidths[1] != (PaperWidth{80, 576}) {
		t.Fatalf("Límites publicados: %+v", limits)
	}
}

func TestRasterSourceIgnoresInvalidIdentifiers(t *testing.T) {
	// Un identificador que no es UUID nunca llega a la base (fuente sin conexión).
	source := NewRasters(nil)
	raster, err := source.Raster(context.Background(), "../../etc/passwd")
	if raster != nil || err != nil {
		t.Fatalf("Identificador inválido: %v %v", raster, err)
	}
	uri, err := source.DataURI(context.Background(), "")
	if uri != "" || err != nil {
		t.Fatalf("Identificador vacío: %q %v", uri, err)
	}
}

func TestRasterPreviewIsTheStoredBits(t *testing.T) {
	bits := make([]byte, 48*2)
	bits[0] = 0x80
	content, err := rasterPNG(384, 2, bits)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pngDataURI(content), "data:image/png;base64,iVBORw0KGgo") {
		t.Fatal("La vista previa debe ser un PNG en data URI")
	}
	if _, err := rasterPNG(384, 2, bits[:10]); err == nil {
		t.Fatal("Bits incompletos no son una conversión válida")
	}
}
