package licensing

import "testing"

func TestDevelopmentCannotBecomeCommercialBypass(t *testing.T) {
	if CheckStartup("development", "development") != nil {
		t.Fatal("development blocked")
	}
	for _, environment := range []string{"production", "unknown"} {
		for _, mode := range []string{"development", "commercial"} {
			if CheckStartup(environment, mode) == nil {
				t.Fatal("commercial startup allowed without provider")
			}
		}
	}
}
