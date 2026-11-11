package inventory

import (
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
	taken := func(codes ...string) map[string]bool {
		set := map[string]bool{}
		for _, code := range codes {
			set[strings.ToLower(code)] = true
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
	}
	for _, entry := range cases {
		if got := suggestFreeCode(entry.base, entry.taken, maximumItemCode); got != entry.expected {
			t.Errorf("suggestFreeCode(%q) = %q; se esperaba %q", entry.base, got, entry.expected)
		}
	}
	if got := trimCode("pan-dulce-", 40); got != "pan-dulce" {
		t.Errorf("trimCode quita los guiones finales: %q", got)
	}
}
