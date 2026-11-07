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
}
