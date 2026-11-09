package administration

import "testing"

func TestPasswordHashUsesIndependentSaltAndRejectsWrongPasswords(t *testing.T) {
	password := "solo-para-esta-prueba"
	first := HashPassword(password)
	second := HashPassword(password)
	if first == second {
		t.Fatal("La sal debe ser diferente en cada hash")
	}
	if !VerifyPassword(password, first) || VerifyPassword("incorrecta", first) || VerifyPassword(password, "formato inválido") {
		t.Fatal("La verificación de contraseña no funciona")
	}
	if len(NewToken()) != 43 || len(TokenHash(NewToken())) != 64 {
		t.Fatal("La credencial aleatoria no tiene el formato esperado")
	}
}

func TestHomeScreenFallsBackWhenPermissionIsMissing(t *testing.T) {
	if allowedHome("inventories", []string{}) != "status" || allowedHome("users", []string{}) != "status" {
		t.Fatal("Se permitió una pantalla no implementada o sin permiso")
	}
	if allowedHome("users", []string{"users.read"}) != "users" {
		t.Fatal("No se respetó una pantalla permitida")
	}
	// Etapa 7: la pantalla «Caja» se admite como inicio con el permiso de consultar turnos.
	if allowedHome("cash", []string{"cash.shifts.read"}) != "cash" || allowedHome("cash", []string{"payments.read"}) != "status" {
		t.Fatal("La pantalla de caja no respeta su permiso como pantalla de inicio")
	}
}

func TestLegacyPreviewHomeOpensCatalog(t *testing.T) {
	// Etapa 6.1 §2.6: «Probar personalización» salió del menú principal y vive en Catálogo.
	if allowedHome("preview", []string{"catalog.read", "catalog.prices.read"}) != "catalog" {
		t.Fatal("La pantalla de inicio heredada debe abrir Catálogo")
	}
	if allowedHome("preview", []string{"catalog.prices.read"}) != "status" {
		t.Fatal("Sin permiso de catálogo la pantalla heredada debe volver a Estado del sistema")
	}
	if allowedHome("catalog", []string{"catalog.read"}) != "catalog" {
		t.Fatal("Catálogo debe seguir siendo una pantalla de inicio válida")
	}
}

func TestTimezoneMustBeAnExistingIANAIdentifier(t *testing.T) {
	// Etapa 6.1 §2.7: la zona se elige de una lista; Go confirma que exista al guardar.
	for _, name := range []string{
		"America/Monterrey",
		"America/Mexico_City",
		"America/Tijuana",
		"America/Hermosillo",
		"America/Chihuahua",
		"America/Ciudad_Juarez",
		"America/Matamoros",
		"America/Mazatlan",
		"America/Bahia_Banderas",
		"America/Merida",
		"America/Cancun",
		"America/Ojinaga",
		"Europe/Madrid",
		"UTC",
	} {
		if !validTimezone(name) {
			t.Fatalf("Se rechazó la zona válida %q", name)
		}
	}
	for _, name := range []string{
		"",
		"Local",
		"America/Monterey",
		"Hora del centro",
		" America/Monterrey",
		"America/Monterrey ",
		"../etc/passwd",
	} {
		if validTimezone(name) {
			t.Fatalf("Se aceptó la zona inexistente %q", name)
		}
	}
	input := EstablishmentInput{Name: "ADEL POS", Timezone: "America/Monterey", Currency: "MXN", Theme: "light"}
	if err := input.validate(); err == nil {
		t.Fatal("El guardado de ajustes aceptó una zona inexistente")
	}
	input.Timezone = "America/Monterrey"
	if err := input.validate(); err != nil {
		t.Fatalf("El guardado de ajustes rechazó una zona válida: %v", err)
	}
}
