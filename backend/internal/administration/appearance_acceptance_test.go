//go:build integration

package administration_test

import (
	"encoding/json"
	"testing"
)

// testNewUserAppearance comprueba la apariencia de los usuarios nuevos (etapa 9C, §4.4): al crear
// un usuario, Go copia la apariencia de Configuración general; cambiarla después no altera a los
// usuarios existentes; y un usuario sin preferencias guardadas usa la de Configuración general como
// respaldo al leer sus preferencias (también en auth/me).
func testNewUserAppearance(t *testing.T, env *delegationEnvironment) {
	t.Run("stage9c_new_user_appearance", func(t *testing.T) {
		settings := map[string]any{}
		if err := json.Unmarshal(env.call(env.admin, "GET", "settings", nil, 200), &settings); err != nil {
			t.Fatal(err)
		}
		original := settings["theme"]
		saveTheme := func(theme string) {
			t.Helper()
			body := map[string]any{}
			for _, key := range []string{"name", "timezone", "currency", "phone", "email", "address"} {
				body[key] = settings[key]
			}
			body["theme"] = theme
			env.call(env.admin, "PUT", "settings", body, 200)
		}
		themeOf := func(actor terminal) string {
			t.Helper()
			return env.text(env.call(actor, "GET", "preferences", nil, 200), "theme")
		}
		identityTheme := func(actor terminal) string {
			t.Helper()
			var identity struct {
				Preferences struct {
					Theme string `json:"theme"`
				} `json:"preferences"`
			}
			if err := json.Unmarshal(env.call(actor, "GET", "auth/me", nil, 200), &identity); err != nil {
				t.Fatal(err)
			}
			return identity.Preferences.Theme
		}
		defer func() {
			if text, ok := original.(string); ok {
				saveTheme(text)
			}
		}()
		role := env.createRole(t, "Apariencia: lectura de cajas", "registers.read")

		saveTheme("dark")
		darkID, darkUser := env.createUser(t, "apariencia_oscura", role)
		if theme := themeOf(darkUser); theme != "dark" {
			t.Fatalf("El usuario nuevo debía recibir la apariencia oscura de Configuración general: %s", theme)
		}
		if theme := identityTheme(darkUser); theme != "dark" {
			t.Fatalf("auth/me debía entregar la apariencia copiada: %s", theme)
		}
		if stored := env.scalar(t, "SELECT count(*) FROM app.user_preferences WHERE user_id=$1 AND theme='dark'", darkID); stored != 1 {
			t.Fatal("La apariencia copiada no quedó guardada en las preferencias del usuario")
		}

		// Cambiar la predeterminada solo afecta a los usuarios que se creen después.
		saveTheme("light")
		if theme := themeOf(darkUser); theme != "dark" {
			t.Fatalf("Cambiar la apariencia predeterminada no debe alterar a un usuario existente: %s", theme)
		}
		_, lightUser := env.createUser(t, "apariencia_clara", role)
		if theme := themeOf(lightUser); theme != "light" {
			t.Fatalf("El usuario nuevo debía recibir la apariencia clara: %s", theme)
		}

		// Un usuario sin preferencias guardadas usa la predeterminada como respaldo al leerlas.
		if _, err := env.pool.Exec(env.ctx, "DELETE FROM app.user_preferences WHERE user_id=$1", darkID); err != nil {
			t.Fatal(err)
		}
		saveTheme("dark")
		if theme := themeOf(darkUser); theme != "dark" {
			t.Fatalf("Sin preferencias guardadas debía usarse la apariencia de Configuración general: %s", theme)
		}
		if theme := identityTheme(darkUser); theme != "dark" {
			t.Fatalf("auth/me debía usar el mismo respaldo: %s", theme)
		}
		// Mis preferencias sigue mandando: guardar la propia apariencia la aplica sobre la predeterminada.
		env.call(darkUser, "PUT", "preferences", map[string]string{"theme": "light", "home_screen": "status"}, 200)
		if theme := themeOf(darkUser); theme != "light" {
			t.Fatalf("La apariencia propia debía prevalecer: %s", theme)
		}
	})
}
