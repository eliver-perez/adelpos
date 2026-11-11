package inventory

import (
	"encoding/json"
	"strings"
	"testing"
)

// Etapa 9A, §3.3: el segundo teléfono y el celular admiten formatos habituales con 7 a 20 dígitos
// (sin contar la extensión); vacío es «sin teléfono».
func TestValidatePhone(t *testing.T) {
	accepted := map[string]string{
		"":                           "",
		"  55 1234 5678  ":           "55 1234 5678",
		"(33) 3615-0000":             "(33) 3615-0000",
		"+52 1 55 1234 5678":         "+52 1 55 1234 5678",
		"81.8123.4567":               "81.8123.4567",
		"(33) 3615-0000 ext. 12":     "(33) 3615-0000 ext. 12",
		"5512345678 EXT 4":           "5512345678 EXT 4",
		"812 345 6789 x 123":         "812 345 6789 x 123",
		"01 800 123 4567":            "01 800 123 4567",
		"55/1234/5678":               "55/1234/5678",
		"+1 (212) 555-0100 ext.9001": "+1 (212) 555-0100 ext.9001",
	}
	for input, expected := range accepted {
		value, err := validatePhone(input, "El segundo teléfono")
		if err != nil || value != expected {
			t.Errorf("validatePhone(%q) = %q, %v; se esperaba %q", input, value, err, expected)
		}
	}
	rejected := []string{
		"123456",                   // menos de 7 dígitos
		"llamar en la tarde",       // sin dígitos
		"55 1234 5678 o 55 8765",   // texto entre números
		"5512345678901234567890",   // 22 dígitos
		"55 1234 5678 ext. abc",    // extensión no numérica
		"tel: 55 1234 5678",        // prefijo de texto
		"55 1234 5678 ext 1234567", // extensión demasiado larga
		strings.Repeat("5", 41),    // excede 40 caracteres
	}
	for _, input := range rejected {
		if _, err := validatePhone(input, "El teléfono celular"); err == nil {
			t.Errorf("validatePhone(%q) debía rechazarse", input)
		}
	}
	if _, err := validatePhone("55\n1234 5678", "El teléfono celular"); err == nil {
		t.Error("Un salto de línea interior debía rechazarse")
	}
}

// La página web es un dominio con ruta opcional, con o sin http(s)://; ningún otro esquema.
func TestValidateWebsite(t *testing.T) {
	accepted := []string{
		"", "restaurante.mx", "www.proveedor.com.mx", "https://www.proveedor.com.mx/catalogo", "HTTP://Tienda.Example.com",
		"distribuidora-norte.com:8080/pedidos?sucursal=2", "abarrotes.mx/",
	}
	for _, input := range accepted {
		if value, err := validateWebsite("  " + input + " "); err != nil || value != input {
			t.Errorf("validateWebsite(%q) = %q, %v", input, value, err)
		}
	}
	rejected := []string{
		"javascript:alert(1)", "mailto:ventas@proveedor.mx", "ftp://proveedor.mx", "proveedor", "www.proveedor .mx",
		"https://", "http://localhost", "proveedor.mx/ruta con espacio", "192.168.0.10", "https://-proveedor.mx",
		"https://proveedor\n.mx", "https://" + strings.Repeat("a", 190) + ".com",
	}
	for _, input := range rejected {
		if _, err := validateWebsite(input); err == nil {
			t.Errorf("validateWebsite(%q) debía rechazarse", input)
		}
	}
}

// §3.4: la sugerencia de código libre sigue la regla de suggestProductCode (productCode.ts) con el
// largo de 40 caracteres de los insumos: base si está libre o base-2, base-3…, recortando la base
// para no pasar del máximo y sin guiones al final.
func TestSuggestFreeCode(t *testing.T) {
	// cLower es lower() de PostgreSQL con la configuración regional C de las instalaciones del
	// proyecto: solo pasa a minúsculas las letras ASCII («Ñ» y las vocales acentuadas no cambian).
	cLower := func(text string) string {
		return strings.Map(func(character rune) rune {
			if character >= 'A' && character <= 'Z' {
				return character + ('a' - 'A')
			}
			return character
		}, text)
	}
	taken := func(codes ...string) map[string]bool {
		set := map[string]bool{}
		for _, code := range codes {
			set[cLower(code)] = true
		}
		return set
	}
	cases := []struct {
		base     string
		taken    map[string]bool
		expected string
	}{
		{"harina-de-trigo", taken(), "harina-de-trigo"},
		{"harina-de-trigo", taken("HARINA-DE-TRIGO"), "harina-de-trigo-2"},
		{"harina-de-trigo", taken("harina-de-trigo", "harina-de-trigo-2"), "harina-de-trigo-3"},
		{"harina-de-trigo", taken("harina-de-trigo-2"), "harina-de-trigo"},
		{strings.Repeat("x", 40), taken(strings.Repeat("x", 40)), strings.Repeat("x", 38) + "-2"},
		// Recortar a 38 deja un guion al final: se quita antes del sufijo.
		{strings.Repeat("a", 37) + "-bc", taken(strings.Repeat("a", 37) + "-bc"), strings.Repeat("a", 37) + "-2"},
		{"C-HAR", taken("c-har", "c-har-2", "c-har-3"), "C-HAR-4"},
		// Revisión independiente de la 9A: con la misma clave que el índice único, «PAÑO» ocupado
		// nunca se sugiere a sí mismo (strings.ToLower daba «paño», otra clave) y «paño» sí está
		// libre para el índice.
		{"PAÑO", taken("PAÑO"), "PAÑO-2"},
		{"PAÑO", taken("PAÑO", "paÑo-2"), "PAÑO-3"},
		{"paño", taken("PAÑO"), "paño"},
		{"CAFÉ-" + strings.Repeat("X", 35), taken("CAFÉ-" + strings.Repeat("X", 35)), "CAFÉ-" + strings.Repeat("X", 33) + "-2"},
	}
	for _, entry := range cases {
		if got := suggestFreeCode(entry.base, cLower, entry.taken, maximumItemCode); got != entry.expected {
			t.Errorf("suggestFreeCode(%q) = %q; se esperaba %q", entry.base, got, entry.expected)
		}
		if entry.taken[cLower(entry.base)] && entry.taken[cLower(suggestFreeCode(entry.base, cLower, entry.taken, maximumItemCode))] {
			t.Errorf("suggestFreeCode(%q) sugirió un código ocupado", entry.base)
		}
	}
	// Las bases recortadas que necesita la sugerencia: la base y una por cada largo de sufijo.
	stems := codeStems(strings.Repeat("a", 37)+"-bc", maximumItemCode)
	if len(stems) != 5 || stems[0] != strings.Repeat("a", 37)+"-bc" || stems[1] != strings.Repeat("a", 37) || stems[4] != strings.Repeat("a", 35) {
		t.Errorf("codeStems: %q", stems)
	}
	if got := trimCode("pan-dulce-", 40); got != "pan-dulce" {
		t.Errorf("trimCode quita los guiones finales: %q", got)
	}
}

// Revisión independiente de la 9A: el segundo teléfono, el celular y la página web ausentes del
// cuerpo (un cliente anterior a la 9A no los envía) se conservan al editar; presentes se validan
// y la cadena vacía los borra.
func TestSupplierContactFieldsAreOptional(t *testing.T) {
	var legacy supplierInput
	if err := json.Unmarshal([]byte(`{"revision":3,"trade_name":"Abarrotes del Centro","phone":"Pedir a Juan"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSupplier(&legacy); err != nil {
		t.Fatalf("Un cuerpo sin los campos nuevos debía aceptarse: %v", err)
	}
	if legacy.PhoneSecondary != nil || legacy.MobilePhone != nil || legacy.Website != nil {
		t.Fatalf("Los campos ausentes debían seguir ausentes: %+v", legacy)
	}
	if contactOr(legacy.PhoneSecondary, "55 1234 5678") != "55 1234 5678" || contactOr(legacy.Website, "") != "" {
		t.Fatal("Un campo ausente debía conservar el valor guardado (o vacío al crear)")
	}
	var current supplierInput
	body := `{"trade_name":"Abarrotes del Centro","phone_secondary":"","mobile_phone":" 81 8123 4567 ","website":"abarrotes.mx"}`
	if err := json.Unmarshal([]byte(body), &current); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSupplier(&current); err != nil {
		t.Fatal(err)
	}
	if contactOr(current.PhoneSecondary, "55 1234 5678") != "" || contactOr(current.MobilePhone, "") != "81 8123 4567" ||
		contactOr(current.Website, "otro.mx") != "abarrotes.mx" {
		t.Fatalf("Los campos presentes se validan y vacío los borra: %+v", current)
	}
	for _, rejected := range []string{`{"trade_name":"X","mobile_phone":"12345"}`, `{"trade_name":"X","website":"javascript:alert(1)"}`} {
		var input supplierInput
		if err := json.Unmarshal([]byte(rejected), &input); err != nil {
			t.Fatal(err)
		}
		if _, err := validateSupplier(&input); err == nil {
			t.Errorf("Debía rechazarse %s", rejected)
		}
	}
}
