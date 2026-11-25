package administration

import "testing"

// Etapa K1 (Fase A): la lista cerrada de la pantalla de cocina está en la tabla de la licencia (ninguna ruta suya cae
// en la regla de lo desconocido) y solo connect toca el cupo, en su propia ruta.
func TestKitchenDisplayRoutesAreClassified(t *testing.T) {
	for entry := range kitchenDisplayRoutes {
		method, route, _ := cutRoute(entry)
		operation := ClassifyGateOperation(method, route)
		if !operation.Known {
			t.Errorf("%s no está en la tabla de la licencia", entry)
		}
		if route == "kitchen-display/connect" && operation.SeatEffect != "kitchen_display" {
			t.Errorf("connect debe ocupar el cupo de cocina: %+v", operation)
		}
		if route != "kitchen-display/connect" && operation.SeatEffect != "none" {
			t.Errorf("%s no toca cupos: %+v", entry, operation)
		}
	}
	for route := range kitchenDisplayWrites {
		if !kitchenDisplayRoutes["POST "+route] || !ClassifyGateOperation("POST", route).Dated {
			t.Errorf("%s: una escritura de la pantalla registra con fecha y está en la lista", route)
		}
	}
}

func cutRoute(entry string) (string, string, bool) {
	for index := range entry {
		if entry[index] == ' ' {
			return entry[:index], entry[index+1:], true
		}
	}
	return entry, "", false
}

// El X-Client-Kind de cada tipo de dispositivo (K1-D3): la aplicación de cocina solo en los kitchen.
func TestMatchClientKindByDeviceKind(t *testing.T) {
	for _, check := range []struct {
		kind, header string
		ok           bool
	}{
		{"desktop", "desktop", true}, {"desktop", "android", false}, {"desktop", "kitchen", false},
		{"android", "android", true}, {"android", "desktop", false}, {"android", "kitchen", false},
		{"kitchen", "kitchen", true}, {"kitchen", "desktop", false}, {"kitchen", "android", false},
		{"kitchen", "", true}, {"desktop", "", true},
	} {
		err := matchClientKind(check.kind, Request{ClientKind: check.header})
		if (err == nil) != check.ok {
			t.Errorf("dispositivo %s con X-Client-Kind %q: %v", check.kind, check.header, err)
		}
	}
}

// La versión instalada solo se guarda si es una versión válida de 40 caracteres o menos (K1-D9).
func TestInstalledVersion(t *testing.T) {
	for text, expected := range map[string]string{
		"0.1.0": "0.1.0", "0.16.0-beta.1": "0.16.0-beta.1", "": "", "versión": "", "1.2": "", "1.2.3.4": "",
		"0.1.0-" + string(make([]byte, 40)): "",
	} {
		if got := installedVersion(text); got != expected {
			t.Errorf("%q: %q, se esperaba %q", text, got, expected)
		}
	}
}
