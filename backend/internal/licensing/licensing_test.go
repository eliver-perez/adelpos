package licensing

import "testing"

// Una compilación de desarrollo solo arranca con environment y license_mode «development»: ninguna
// configuración la convierte en comercial (L1-D18).
func TestDevelopmentCannotBecomeCommercialBypass(t *testing.T) {
	if BuildEnvironment != "development" {
		t.Skip("solo en la compilación de desarrollo")
	}
	if err := CheckStartup("development", "development"); err != nil {
		t.Fatalf("desarrollo bloqueado: %v", err)
	}
	for _, environment := range []string{"production", "unknown", "development"} {
		for _, mode := range []string{"development", "commercial"} {
			if environment == "development" && mode == "development" {
				continue
			}
			if CheckStartup(environment, mode) == nil {
				t.Fatalf("%s/%s arrancó en una compilación de desarrollo", environment, mode)
			}
		}
	}
}
