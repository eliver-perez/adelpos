package payments

import (
	"fmt"
	"testing"
)

// Etapa 9F (encargo del coordinador, como la corrección B3 de operations): la huella idempotente de cobros
// nunca depende de la contraseña de la autorización de un supervisor, la escriba el cliente con las
// mayúsculas que sea (decode las acepta).
func TestRequestFingerprintExcludesAuthorizationPassword(t *testing.T) {
	fingerprint := func(body string) string {
		t.Helper()
		value, err := requestFingerprint(Request{Method: "POST", Route: "payments/accounts/x/discounts", Body: []byte(body)})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	body := func(container, username, passwordKey, password string) string {
		authorization := fmt.Sprintf(`{"username":%q}`, username)
		if passwordKey != "" {
			authorization = fmt.Sprintf(`{"username":%q,%q:%q}`, username, passwordKey, password)
		}
		return fmt.Sprintf(`{"operation_id":"00000000-0000-4000-8000-000000000001","scope":"account","value":"10",%q:%s}`,
			container, authorization)
	}
	// «ſ» (s larga) se pliega a «s» como en encoding/json.
	passwordKeys := []string{"password", "Password", "PASSWORD", "pAsSwOrD", "paſsword"}
	for _, container := range []string{"authorization", "Authorization", "AUTHORIZATION"} {
		withoutPassword := fingerprint(body(container, "supervisor", "", ""))
		for _, key := range passwordKeys {
			for _, password := range []string{"clave-uno", "otra-clave-distinta"} {
				if got := fingerprint(body(container, "supervisor", key, password)); got != withoutPassword {
					t.Fatalf("la huella depende de la contraseña con %q/%q", container, key)
				}
			}
		}
		if fingerprint(body(container, "otro-supervisor", "Password", "clave-uno")) == withoutPassword {
			t.Fatalf("la huella debe seguir dependiendo del resto de la autorización (%q)", container)
		}
	}
	// En cualquier nivel del cuerpo, también dentro de arreglos.
	nested := `{"items":[{"Authorization":{"username":"supervisor","PASSWORD":"%s"}}]}`
	if fingerprint(fmt.Sprintf(nested, "clave-uno")) != fingerprint(fmt.Sprintf(nested, "otra-clave")) {
		t.Fatal("la huella depende de una contraseña anidada")
	}
	// Una clave «password» fuera de una autorización no se toca (no es la del supervisor).
	if fingerprint(`{"note":{"password":"a"}}`) == fingerprint(`{"note":{"password":"b"}}`) {
		t.Fatal("solo se quita la contraseña de las autorizaciones")
	}

	// La premisa: decode acepta las mismas claves con otras mayúsculas.
	var input struct {
		Authorization *AuthorizationInput `json:"authorization"`
	}
	if err := decode([]byte(`{"AUTHORIZATION":{"username":"supervisor","paſsword":"clave-uno"}}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.Authorization == nil || input.Authorization.Password != "clave-uno" {
		t.Fatalf("decode no aceptó la autorización con otras mayúsculas: %+v", input.Authorization)
	}
}
