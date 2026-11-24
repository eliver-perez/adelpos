//go:build darwin

package licensing

import (
	"context"
	"os/exec"
	"time"
)

// readMachineID lee el IOPlatformUUID con ioreg, por su ruta absoluta y sin cgo. Es el identificador de la
// tarjeta madre que macOS usa como «UUID del hardware».
func readMachineID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", errMachineID
	}
	return parseIOPlatformUUID(output)
}
