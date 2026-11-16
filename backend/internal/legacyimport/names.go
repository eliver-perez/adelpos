package legacyimport

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// cleanName prepara un nombre del anterior para AD-EL POS: sin espacios al inicio ni al final, sin caracteres de
// control (se cambian por espacios) y con a lo más max caracteres. Si queda vacío, usa fallback.
func cleanName(text string, max int, fallback string) string {
	var builder strings.Builder
	for _, character := range text {
		if unicode.IsControl(character) {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(character)
	}
	name := strings.Join(strings.Fields(builder.String()), " ")
	if name == "" {
		name = fallback
	}
	return truncateRunes(name, max)
}

func truncateRunes(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:max]))
}

// uniqueName devuelve name o, si ya se usó, «name (2)», «name (3)»…, sin pasar de max caracteres. Registra el
// resultado en used (comparación exacta, como los índices únicos de AD-EL POS).
func uniqueName(name string, used map[string]bool, max int) string {
	candidate := name
	for number := 2; used[candidate]; number++ {
		suffix := fmt.Sprintf(" (%d)", number)
		candidate = truncateRunes(name, max-utf8.RuneCountInString(suffix)) + suffix
	}
	used[candidate] = true
	return candidate
}

// foldText quita acentos y mayúsculas para comparar textos del anterior («Administrador», «administrador»).
func foldText(text string) string {
	var builder strings.Builder
	for _, character := range norm.NFD.String(strings.ToLower(strings.TrimSpace(text))) {
		if unicode.Is(unicode.Mn, character) {
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

// usernamePattern es la regla de AD-EL POS para el nombre de usuario (administration/service.go).
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,39}$`)

// deriveUsername convierte el nombre de usuario del anterior en uno válido de AD-EL POS: en minúsculas y sin acentos;
// lo que no sea letra, número, punto, guion o guion bajo se cambia por un punto; si no empieza con letra se antepone
// «u»; de 2 a 40 caracteres. Si choca con uno reservado o ya usado, agrega «-2», «-3»… El segundo valor dice si cambió
// respecto del original en minúsculas.
func deriveUsername(original string, legacyID int32, used map[string]bool) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(original))
	base := lower
	if !usernamePattern.MatchString(base) {
		var builder strings.Builder
		for _, character := range foldText(lower) {
			switch {
			case character >= 'a' && character <= 'z', character >= '0' && character <= '9', character == '_', character == '.',
				character == '-':
				builder.WriteRune(character)
			default:
				builder.WriteRune('.')
			}
		}
		base = strings.Trim(builder.String(), ".")
		if base == "" {
			base = fmt.Sprintf("usuario%d", legacyID)
		}
		if base[0] < 'a' || base[0] > 'z' {
			base = "u" + base
		}
		if len(base) < 2 {
			base += "0"
		}
		if len(base) > 40 {
			base = strings.TrimRight(base[:40], ".")
		}
	}
	candidate := base
	for number := 2; used[candidate] || candidate == "sa" || !usernamePattern.MatchString(candidate); number++ {
		suffix := fmt.Sprintf("-%d", number)
		trimmed := base
		if len(trimmed)+len(suffix) > 40 {
			trimmed = trimmed[:40-len(suffix)]
		}
		candidate = trimmed + suffix
	}
	used[candidate] = true
	return candidate, candidate != lower
}
