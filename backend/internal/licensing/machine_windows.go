//go:build windows

package licensing

import "golang.org/x/sys/windows/registry"

// readMachineID lee MachineGuid de HKLM\SOFTWARE\Microsoft\Cryptography en la vista de 64 bits del registro
// (WOW64_64KEY), para leer el mismo valor aunque el proceso fuera de 32 bits. Lo crea Windows al instalarse y
// no cambia salvo que se reinstale el sistema.
func readMachineID() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", errMachineID
	}
	defer key.Close()
	value, valueType, err := key.GetStringValue("MachineGuid")
	if err != nil || valueType != registry.SZ {
		return "", errMachineID
	}
	return normalizeMachineID(value)
}
