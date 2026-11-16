package legacyimport

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

// El nombre de lo retirado lleva una sola terminación aunque se retire otra vez en cada reemplazo, también para un
// usuario, para el nombre que no admitía la terminación y en el límite de longitud (revisión de la Fase C, M-4).
func TestRetiredNameHasASingleSuffix(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{0x1a, 0x2b, 0x3c, 0x4d, 0x5e}, Valid: true}
	tag := uuidText(id)[:8]
	long := strings.Repeat("Á", 100)
	longUser := strings.Repeat("abcdefghij", 4)
	cases := []struct {
		label, entity, name string
		max                 int
		expected            string
	}{
		{"categoría", MapCategory, "Bebidas", 100, "Bebidas (retirado " + tag + ")"},
		{"categoría ya retirada", MapCategory, "Bebidas (retirado " + tag + ")", 100, "Bebidas (retirado " + tag + ")"},
		{"con dos terminaciones de antes", MapCategory, "Bebidas (retirado 0badc0de) (retirado " + tag + ")", 100,
			"Bebidas (retirado " + tag + ")"},
		{"mesa en el límite", MapTable, strings.Repeat("m", 60), 60, strings.Repeat("m", 40) + " (retirado " + tag + ")"},
		{"nombre largo con acentos", MapComplement, long, 100, strings.Repeat("Á", 80) + " (retirado " + tag + ")"},
		{"nombre largo ya retirado", MapComplement, strings.Repeat("Á", 80) + " (retirado " + tag + ")", 100,
			strings.Repeat("Á", 80) + " (retirado " + tag + ")"},
		{"usuario", MapUser, "u1beto", 40, "u1beto.retirado-" + tag},
		{"usuario ya retirado", MapUser, "u1beto.retirado-" + tag, 40, "u1beto.retirado-" + tag},
		{"usuario con dos terminaciones", MapUser, "u1beto.retirado-0badc0de.retirado-" + tag, 40, "u1beto.retirado-" + tag},
		{"usuario en el límite", MapUser, longUser, 40, longUser[:22] + ".retirado-" + tag},
		{"usuario en el límite ya retirado", MapUser, longUser[:22] + ".retirado-" + tag, 40, longUser[:22] + ".retirado-" + tag},
		{"usuario sin nombre que admita la terminación", MapUser, "retirado-0badc0de", 40, "retirado-" + tag},
	}
	for _, item := range cases {
		got := retiredName(item.entity, item.name, id, item.max)
		if got != item.expected {
			t.Fatalf("%s: %q; se esperaba %q", item.label, got, item.expected)
		}
		if utf8.RuneCountInString(got) > item.max {
			t.Fatalf("%s: %q pasa de %d caracteres", item.label, got, item.max)
		}
		if item.entity == MapUser && !usernamePattern.MatchString(got) {
			t.Fatalf("%s: %q no cumple la regla de nombres de usuario", item.label, got)
		}
		if again := retiredName(item.entity, got, id, item.max); again != got {
			t.Fatalf("%s: retirarlo otra vez da %q", item.label, again)
		}
	}
}
