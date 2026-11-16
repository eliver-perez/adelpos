//go:build windows

package legacypkg

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"local/ad-el-pos/server/internal/fileacl"
)

// createPartial crea el temporal del paquete con la ACL privada. Una USB con FAT32 o exFAT no guarda listas de control
// de acceso (fileacl.CreatePrivate fallaría al protegerlo y el propietario vería «no se puede escribir ahí»): en esos
// discos el temporal se crea sin ACL propia, sin reemplazar nada. El paquete va cifrado de principio a fin.
func createPartial(path string) (*os.File, error) {
	if !volumeKeepsACLs(path) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	}
	return fileacl.CreatePrivate(path)
}

// volumeKeepsACLs dice si el disco de la carpeta guarda listas de control de acceso (NTFS sí; FAT32 y exFAT no). Ante
// la duda responde que sí (se intenta con la ACL privada).
func volumeKeepsACLs(path string) bool {
	directory, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return true
	}
	volume := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(directory, &volume[0], uint32(len(volume))); err != nil {
		return true
	}
	var flags uint32
	if err := windows.GetVolumeInformation(&volume[0], nil, 0, nil, nil, &flags, nil, 0); err != nil {
		return true
	}
	return flags&windows.FILE_PERSISTENT_ACLS != 0
}
