//go:build !darwin && !windows

package licensing

import "os"

// readMachineID lee /etc/machine-id (o la copia de D-Bus). AD-EL POS solo se distribuye para macOS y
// Windows: esto sirve para compilar y probar en otros sistemas.
func readMachineID() (string, error) {
	for _, filename := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		raw, err := os.ReadFile(filename)
		if err == nil {
			return normalizeMachineID(string(raw))
		}
	}
	return "", errMachineID
}
