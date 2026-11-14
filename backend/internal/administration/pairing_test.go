package administration

import (
	"net"
	"strings"
	"testing"
)

func pairingVector() ([32]byte, []byte, []byte) {
	var fingerprint [32]byte
	terminal := make([]byte, 32)
	server := make([]byte, 32)
	for index := 0; index < 32; index++ {
		fingerprint[index] = byte(index)
		terminal[index] = byte(32 + index)
		server[index] = byte(64 + index)
	}
	return fingerprint, terminal, server
}

// Etapa 9G, fase B: vector fijo de la derivación del código, calculado aparte (Python, hashlib) y repetido
// en las pruebas de Rust (pairing.rs) para que Go, Rust y el puente de pruebas deriven lo mismo.
func TestPairingCodeMatchesTheReferenceVector(t *testing.T) {
	fingerprint, terminal, server := pairingVector()
	code, err := PairingCode(fingerprint, terminal, server, "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if code != "166101" {
		t.Fatalf("código %s, se esperaba 166101", code)
	}
	if commitment := PairingCommitment(terminal); commitment != "72dbb7336c76780023f83da4c355f2eeea85733b13d3477697917790c1229084" {
		t.Fatalf("compromiso inesperado: %s", commitment)
	}
	// El identificador en mayúsculas es el mismo UUID.
	upper, err := PairingCode(fingerprint, terminal, server, strings.ToUpper("00000000-0000-4000-8000-000000000001"))
	if err != nil || upper != code {
		t.Fatal("el identificador se compara como UUID")
	}
}

// Cualquier cambio en la huella, en un número o en la solicitud cambia el código: un intermediario con otro
// certificado obtiene otro código.
func TestPairingCodeDependsOnEveryInput(t *testing.T) {
	fingerprint, terminal, server := pairingVector()
	base, _ := PairingCode(fingerprint, terminal, server, "00000000-0000-4000-8000-000000000001")
	other := fingerprint
	other[0] = 0xff
	changed, _ := PairingCode(other, terminal, server, "00000000-0000-4000-8000-000000000001")
	if changed != "089301" || changed == base {
		t.Fatalf("otra huella debe dar otro código: %s", changed)
	}
	variants := map[string]func() (string, error){
		"número de la terminal": func() (string, error) {
			value := append([]byte{}, terminal...)
			value[31] ^= 1
			return PairingCode(fingerprint, value, server, "00000000-0000-4000-8000-000000000001")
		},
		"número del servidor": func() (string, error) {
			value := append([]byte{}, server...)
			value[0] ^= 1
			return PairingCode(fingerprint, terminal, value, "00000000-0000-4000-8000-000000000001")
		},
		"solicitud": func() (string, error) {
			return PairingCode(fingerprint, terminal, server, "00000000-0000-4000-8000-000000000002")
		},
	}
	for name, variant := range variants {
		code, err := variant()
		if err != nil || code == base {
			t.Errorf("cambiar %s debe cambiar el código (%s, %v)", name, code, err)
		}
	}
	for _, code := range []string{base, changed} {
		if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
			t.Errorf("código de 6 dígitos esperado: %q", code)
		}
	}
	if _, err := PairingCode(fingerprint, terminal[:31], server, "00000000-0000-4000-8000-000000000001"); err == nil {
		t.Error("los números deben tener 32 bytes")
	}
	if _, err := PairingCode(fingerprint, terminal, server, "no-es-un-uuid"); err == nil {
		t.Error("la solicitud debe ser un UUID")
	}
}

func TestPairingLabelsRejectControlCharactersAndLongText(t *testing.T) {
	if value, ok := cleanLabel("  Caja 2  ", 80); !ok || value != "Caja 2" {
		t.Fatalf("recorte esperado: %q", value)
	}
	for _, text := range []string{"", "   ", "Caja\n2", "Caja\x002", strings.Repeat("ñ", 81), string([]byte{0xff})} {
		if _, ok := cleanLabel(text, 80); ok {
			t.Errorf("%q debe rechazarse", text)
		}
	}
	if _, ok := cleanLabel(strings.Repeat("ñ", 80), 80); !ok {
		t.Error("80 caracteres con acentos caben")
	}
	if _, ok := decodeNonce(strings.Repeat("A", 64)); ok {
		t.Error("el número va en hexadecimal en minúsculas")
	}
	if _, ok := decodeNonce(strings.Repeat("a", 64)); !ok {
		t.Error("64 caracteres hexadecimales son válidos")
	}
}

// Hallazgo 1 de la revisión: el límite por origen agrupa la IPv6 por su /64 y todo el enlace local en un solo
// grupo, para que una terminal no multiplique sus orígenes.
func TestSourceGroups(t *testing.T) {
	cases := map[string]string{
		"192.168.1.20":         "v4:192.168.1.20",
		"::ffff:192.168.1.20":  "v4:192.168.1.20",
		"169.254.10.20":        "link-local",
		"fe80::1":              "link-local",
		"fe80::abcd:1234":      "link-local",
		"fd12:3456:789a:1::20": "v6:fd12:3456:789a:1::/64",
	}
	for text, expected := range cases {
		address := net.ParseIP(text)
		if address == nil {
			continue
		}
		if group := SourceGroup(address); group != expected {
			t.Errorf("%s → %q, se esperaba %q", text, group, expected)
		}
	}
	if SourceGroup(net.ParseIP("fd12:3456:789a:1::1")) != SourceGroup(net.ParseIP("fd12:3456:789a:1:ffff:ffff:ffff:ffff")) {
		t.Error("todo el /64 es el mismo origen")
	}
	if SourceGroup(net.ParseIP("fd12:3456:789a:1::1")) == SourceGroup(net.ParseIP("fd12:3456:789a:2::1")) {
		t.Error("otro /64 es otro origen")
	}
}
