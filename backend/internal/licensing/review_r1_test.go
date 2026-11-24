package licensing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// Revisión independiente R1 de la etapa L1 (verificación de la §10 del contrato y su almacén). Estas pruebas
// intentan engañar al verificador y confirman lo que ya está bien: quedan como pruebas de regresión. Las que
// demuestran un hallazgo de la revisión se llaman «…Defect…» (review_r1_defects*_test.go) y fallan hasta que se
// corrija.

// signedWithHeader firma un contenido con una llave de los archivos de prueba, con el encabezado que se pida
// (tal cual, byte por byte): sirve para comprobar que solo el encabezado exacto del contrato se acepta, aunque
// la firma sea válida.
func signedWithHeader(t *testing.T, loaded fixtures, kid, header string, content []byte) string {
	t.Helper()
	private, known := loaded.signingKeys[kid]
	if !known {
		t.Fatalf("no existe la llave de prueba %s", kid)
	}
	headerSegment := encodeBase64URL([]byte(header))
	contentSegment := encodeBase64URL(content)
	signature := ed25519.Sign(private, []byte(headerSegment+"."+contentSegment))
	return headerSegment + "." + contentSegment + "." + encodeBase64URL(signature)
}

// fixtureContent es el contenido publicado de una licencia de prueba, como mapa con números exactos.
func fixtureContent(t *testing.T, loaded fixtures, name string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(loaded.license(t, name).Content))
	decoder.UseNumber()
	var content map[string]any
	if err := decoder.Decode(&content); err != nil {
		t.Fatal(err)
	}
	return content
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// R1 · Encabezado exacto y base64url canónico: relleno, espacios, saltos de línea, alfabeto estándar, bits de
// relleno distintos de cero, mayúsculas, otro typ, campos extra o repetidos, otro orden y escapes del kid. Todo
// se rechaza antes de mirar la firma o el contenido, aunque la firma sea válida.
func TestReviewR1HeaderAndEncodingAreExact(t *testing.T) {
	loaded := loadFixtures(t)
	verifier := loaded.verifier("test", "installation-fixture-a")
	valid := loaded.license(t, "subscription-monthly").JWS
	if _, err := verifier.Verify(valid, nil); err != nil {
		t.Fatalf("la licencia de partida es válida: %v", err)
	}
	segments := strings.Split(valid, ".")

	// La firma ocupa 86 caracteres: los últimos 4 bits son relleno. Un último carácter con esos bits encendidos
	// decodifica a los mismos bytes con un decodificador permisivo, pero no es la forma canónica.
	signature := segments[2]
	last := strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", signature[len(signature)-1])
	nonCanonical := signature[:len(signature)-1] + string("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"[last|1])
	lenient, _ := base64.RawURLEncoding.DecodeString(nonCanonical)
	canonical, _ := base64.RawURLEncoding.DecodeString(signature)
	if nonCanonical == signature || !bytes.Equal(lenient, canonical) {
		t.Fatal("el caso no canónico debe decodificar a los mismos bytes con un decodificador permisivo")
	}

	malformed := map[string]string{
		"relleno en el encabezado":     segments[0] + "=." + segments[1] + "." + segments[2],
		"relleno en la firma":          segments[0] + "." + segments[1] + "." + segments[2] + "==",
		"salto de línea final":         valid + "\n",
		"espacio inicial":              " " + valid,
		"salto de línea en medio":      segments[0] + "." + segments[1][:10] + "\n" + segments[1][10:] + "." + segments[2],
		"alfabeto estándar":            segments[0] + "." + segments[1] + "." + strings.NewReplacer("-", "+", "_", "/").Replace(segments[2]),
		"bits de relleno no canónicos": segments[0] + "." + segments[1] + "." + nonCanonical,
		"cuatro segmentos":             valid + ".",
		"todo en mayúsculas":           strings.ToUpper(valid),
	}
	for name, jws := range malformed {
		if name == "alfabeto estándar" && !strings.ContainsAny(segments[2], "-_") {
			continue
		}
		_, err := verifier.Verify(jws, nil)
		if code := codeOf(err); code != ErrMalformed && code != ErrInvalidHeader {
			t.Errorf("%s: se esperaba malformed_jws o invalid_header y resultó %s", name, code)
		}
	}

	// Encabezados distintos del exacto, con la firma válida de una llave de confianza.
	content := loaded.license(t, "subscription-monthly").Content
	headers := map[string]string{
		"espacios":                `{"alg": "EdDSA", "kid": "adelpos-test-fixture-a", "typ": "adelpos-lic+jws"}`,
		"alg en mayúsculas":       `{"alg":"EDDSA","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}`,
		"typ JWT":                 `{"alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"JWT"}`,
		"typ en mayúsculas":       `{"alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"ADELPOS-LIC+JWS"}`,
		"campo crit":              `{"alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws","crit":["exp"]}`,
		"kid repetido":            `{"alg":"EdDSA","kid":"otro","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}`,
		"alg repetido":            `{"alg":"none","alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}`,
		"otro orden":              `{"kid":"adelpos-test-fixture-a","alg":"EdDSA","typ":"adelpos-lic+jws"}`,
		"kid con escape":          `{"alg":"EdDSA","kid":"adelpos` + string(rune(92)) + `u002dtest-fixture-a","typ":"adelpos-lic+jws"}`,
		"KID en mayúsculas":       `{"alg":"EdDSA","KID":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}`,
		"salto de línea al final": `{"alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}` + "\n",
		"sin typ":                 `{"alg":"EdDSA","kid":"adelpos-test-fixture-a"}`,
	}
	for name, header := range headers {
		_, err := verifier.Verify(signedWithHeader(t, loaded, "adelpos-test-fixture-a", header, content), nil)
		requireCodeNamed(t, name, err, ErrInvalidHeader)
	}
	// El encabezado exacto con la misma llave sí vale (la prueba de arriba no falla por la firma).
	exact := `{"alg":"EdDSA","kid":"adelpos-test-fixture-a","typ":"adelpos-lic+jws"}`
	if _, err := verifier.Verify(signedWithHeader(t, loaded, "adelpos-test-fixture-a", exact, content), nil); err != nil {
		t.Fatalf("el encabezado exacto con la misma firma vale: %v", err)
	}
}

// R1 · Firma maleable: Ed25519 con S + L (el orden del grupo) es la misma ecuación para un verificador que no
// exige S canónico. Go la rechaza, y por eso la regla «igual solo si es idéntica byte por byte» no se puede
// burlar con otra firma de los mismos bytes.
func TestReviewR1SignatureIsNotMalleable(t *testing.T) {
	loaded := loadFixtures(t)
	verifier := loaded.verifier("test", "installation-fixture-a")
	valid := loaded.license(t, "subscription-monthly").JWS
	segments := strings.Split(valid, ".")
	signature, ok := decodeBase64URL(segments[2])
	if !ok {
		t.Fatal("firma ilegible")
	}
	order, _ := new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
	scalar := new(big.Int).SetBytes(reversed(signature[32:]))
	malleated := new(big.Int).Add(scalar, order)
	if malleated.BitLen() > 256 {
		t.Fatal("S + L no cabe en 32 bytes")
	}
	raw := make([]byte, 32)
	malleated.FillBytes(raw)
	altered := append(append([]byte{}, signature[:32]...), reversed(raw)...)
	jws := segments[0] + "." + segments[1] + "." + encodeBase64URL(altered)
	_, err := verifier.Verify(jws, nil)
	requireCode(t, err, ErrInvalidSignature)

	// Y como revisión: la misma revisión con otra firma de los mismos bytes no pasa como «idéntica».
	accepted := &Accepted{LicenseID: "11111111-1111-4111-8111-111111111111", Revision: 1, SHA256: licenseSHA256(valid)}
	_, err = verifier.Verify(jws, accepted)
	requireCode(t, err, ErrInvalidSignature)
}

func reversed(raw []byte) []byte {
	copied := make([]byte, len(raw))
	for index := range raw {
		copied[len(raw)-1-index] = raw[index]
	}
	return copied
}

// R1 · Contenido estricto de la §4, firmado con una llave de confianza: cupos fuera de 0 a 999 o que no son
// enteros JSON, instantes con desfase o sin «Z», schema_version incompatible, server_id en mayúsculas, llave no
// canónica y tipos incorrectos. Ninguno se acepta.
func TestReviewR1ContentIsStrict(t *testing.T) {
	loaded := loadFixtures(t)
	verifier := loaded.verifier("test", "installation-fixture-a")
	mutate := func(change func(map[string]any)) string {
		content := fixtureContent(t, loaded, "subscription-monthly")
		change(content)
		return loaded.sign(t, "adelpos-test-fixture-a", mustJSON(t, content))
	}
	setLimit := func(value any) func(map[string]any) {
		return func(content map[string]any) {
			content["limits"].(map[string]any)["order_takers"] = value
		}
	}
	set := func(field string, value any) func(map[string]any) {
		return func(content map[string]any) {
			content[field] = value
		}
	}
	invalid := map[string]func(map[string]any){
		"cupo 1000":                    setLimit(json.Number("1000")),
		"cupo negativo":                setLimit(json.Number("-1")),
		"cupo con decimales":           setLimit(json.Number("3.0")),
		"cupo con exponente":           setLimit(json.Number("3e0")),
		"cupo como texto":              setLimit("3"),
		"cupo nulo":                    setLimit(nil),
		"revisión cero":                set("license_revision", json.Number("0")),
		"revisión como texto":          set("license_revision", "7"),
		"instante con desfase":         set("expires_at", "2026-12-16T00:30:00-06:00"),
		"instante sin zona":            set("expires_at", "2026-12-16T06:30:00"),
		"instante con z minúscula":     set("expires_at", "2026-12-16T06:30:00.000000z"),
		"instante con 10 decimales":    set("issued_at", "2026-11-16T06:30:00.0000000000Z"),
		"día inexistente":              set("issued_at", "2026-02-30T06:30:00Z"),
		"segundo 60":                   set("issued_at", "2026-11-16T06:30:60Z"),
		"server_id en mayúsculas":      set("server_id", "AAAAAAAA-2222-4222-8222-222222222222"),
		"llave con relleno":            set("installation_public_key", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="),
		"plan como lista":              set("plan", []any{}),
		"features como lista":          set("features", []any{}),
		"feature en falso":             set("features", map[string]any{"cocina": false}),
		"grace_days 3":                 set("grace_days", json.Number("3")),
		"activa con motivo":            set("status_reason", "revoked"),
		"licensee_name en blanco":      set("licensee_name", "   "),
		"licensee_name con control":    set("licensee_name", "Fonda\u0007"),
		"product_id distinto":          set("product_id", "adel_pos2"),
		"environment de otro entorno":  set("environment", "production"),
		"schema_version sin punto":     set("schema_version", "1"),
		"schema_version 2.0":           set("schema_version", "2.0"),
		"schema_version 01.0":          set("schema_version", "01.0"),
		"license_status desconocido":   set("license_status", "suspended"),
		"perpetua con vencimiento":     set("license_type", "perpetual"),
		"tipo de licencia desconocido": set("license_type", "lifetime"),
	}
	for name, change := range invalid {
		if _, err := verifier.Verify(mutate(change), nil); err == nil {
			t.Errorf("%s: se aceptó", name)
		}
	}
	// Una versión 1.x y los campos desconocidos sí se aceptan (§4: «ignora los campos adicionales»).
	accepted := mutate(func(content map[string]any) {
		content["schema_version"] = "1.7"
		content["campo_nuevo"] = map[string]any{"x": json.Number("1")}
	})
	if _, err := verifier.Verify(accepted, nil); err != nil {
		t.Fatalf("schema_version 1.7 con un campo desconocido: %v", err)
	}
}

// R1 · Entorno: una compilación normal confía solo en sus llaves embebidas, y ninguna licencia de vectors.json
// (llaves de semilla pública) se acepta con ellas. Un verificador de producción no acepta ninguna llave del
// entorno «test», aunque la tuviera entre sus llaves por error.
func TestReviewR1FixtureKeysAreNotTrustedByTheBuild(t *testing.T) {
	loaded := loadFixtures(t)
	compiled, err := CompiledTrust()
	if err != nil {
		t.Fatal(err)
	}
	for kid := range loaded.trusted["test"] {
		if _, known := compiled.Keys[kid]; known {
			t.Fatalf("la compilación confía en la llave de los archivos de prueba %s", kid)
		}
	}
	verifier := Verifier{Environment: CompiledEnvironment(), Keys: compiled.Keys, Installation: loaded.installations["installation-fixture-a"]}
	for _, item := range loaded.set.Licenses {
		installation := loaded.installations[item.Installation]
		verifier.Installation = installation
		_, err := verifier.Verify(item.JWS, nil)
		requireCodeNamed(t, item.Name, err, ErrUnknownKid)
	}
	// Un verificador de producción con las llaves «test» (las de los archivos de prueba y la embebida) por error.
	mixed := map[string]TrustedKey{}
	for kid, key := range loaded.trusted["test"] {
		mixed[kid] = key
	}
	for kid, key := range compiled.Keys {
		mixed[kid] = key
	}
	production := Verifier{Environment: "production", Keys: mixed}
	for _, item := range loaded.set.Licenses {
		production.Installation = loaded.installations[item.Installation]
		_, err := production.Verify(item.JWS, nil)
		requireCodeNamed(t, item.Name, err, ErrUnknownKid)
	}
}

func requireCodeNamed(t *testing.T, name string, err error, code string) {
	t.Helper()
	if codeOf(err) != code {
		t.Errorf("%s: se esperaba %s y resultó %v", name, code, err)
	}
}
