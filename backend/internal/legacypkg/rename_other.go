//go:build !darwin && !linux && !windows

package legacypkg

// renameNoReplace usa el enlace duro (falla si el destino existe) o, sin enlaces, la comprobación.
func renameNoReplace(temporary, final string) error {
	return renameLinkOrCheck(temporary, final)
}
