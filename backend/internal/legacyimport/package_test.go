package legacyimport

import (
	"errors"
	"testing"

	"local/ad-el-pos/server/internal/legacypkg"
)

// Los errores del formato se traducen a los de la migración; uno de lectura (USB desconectada, archivo que no se pudo
// leer) tiene su propio código que se puede reintentar, distinto de «dañado» (revisión de la Fase C, M-5).
func TestPackageErrorCodes(t *testing.T) {
	cases := map[legacypkg.ErrorKind]string{
		legacypkg.KindNotPackage:         CodeNotPackage,
		legacypkg.KindUnsupportedVersion: CodeUnsupportedVersion,
		legacypkg.KindWrongPassphrase:    CodeWrongPassphrase,
		legacypkg.KindDamaged:            CodeDamaged,
		legacypkg.KindLimit:              CodeLimit,
		legacypkg.KindUnreadable:         CodeUnreadable,
	}
	for kind, code := range cases {
		err := packageError(&legacypkg.Error{Kind: kind, Message: "mensaje del formato", Detail: "detalle"})
		if CodeOf(err) != code || MessageOf(err) != "mensaje del formato" {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	if err := packageError(errors.New("error de entrada y salida")); CodeOf(err) != CodeUnreadable {
		t.Fatalf("un error de lectura que no es del formato: %v", err)
	}
}
