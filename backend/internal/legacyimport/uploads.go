package legacyimport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// Subida del paquete por partes (nota técnica 7 de la Fase 0): el servicio de macOS no puede leer las carpetas del
// usuario, así que la aplicación lee el archivo (Rust, Fase D) y lo envía a Go en partes de 1 MiB, cada una con su
// SHA-256, en orden. El archivo vive en una carpeta de trabajo privada dentro de la carpeta de datos (0700; cada
// archivo 0600 desde el primer byte con fileacl.CreatePrivate) y se borra al importar, al cancelar o por vencimiento
// (limpieza del trabajador, fuera de la petición). El estado de las subidas vive en memoria: al arrancar el servidor
// se borra lo que haya quedado en la carpeta.

const (
	// PartSize es el tamaño de cada parte (la última puede ser menor).
	PartSize = 1 << 20
	// DefaultMaxUploadBytes es el tope del paquete: 1 GiB (el paquete real cifrado y comprimido ocupa decenas de MB).
	DefaultMaxUploadBytes = 1 << 30
	// DefaultUploadTTL es cuánto vive una subida sin actividad.
	DefaultUploadTTL = 2 * time.Hour
	// maxUploads es cuántas subidas puede haber a la vez en el servidor.
	maxUploads = 3
	// WorkFolder es el nombre de la carpeta de trabajo dentro de la carpeta de datos.
	WorkFolder = "legacy-import"
)

// Estados de una subida.
const (
	UploadReceiving = "receiving"
	UploadComplete  = "complete"
)

// Códigos de error de las subidas.
const (
	CodeUploadInvalid  = "LEGACY_UPLOAD_INVALID"
	CodeUploadTooLarge = "LEGACY_UPLOAD_TOO_LARGE"
	CodeUploadOrder    = "LEGACY_UPLOAD_OUT_OF_ORDER"
	CodeUploadHash     = "LEGACY_UPLOAD_HASH_MISMATCH"
	CodeUploadBusy     = "LEGACY_UPLOAD_BUSY"
	CodeUploadLimit    = "LEGACY_UPLOAD_LIMIT"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// UploadView es una subida tal como la ve el asistente.
type UploadView struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	PartSize      int64     `json:"part_size"`
	Parts         int       `json:"parts"`
	ReceivedParts int       `json:"received_parts"`
	ReceivedBytes int64     `json:"received_bytes"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type upload struct {
	view       UploadView
	userID     string
	deviceID   string
	updatedAt  time.Time
	lastDigest string
	busy       int
}

func (manager *Manager) workDir() string {
	return manager.WorkDir
}

func (manager *Manager) maxUploadBytes() int64 {
	if manager.MaxUploadBytes > 0 {
		return manager.MaxUploadBytes
	}
	return DefaultMaxUploadBytes
}

func (manager *Manager) uploadTTL() time.Duration {
	if manager.UploadTTL > 0 {
		return manager.UploadTTL
	}
	return DefaultUploadTTL
}

// partialPath y completePath son los archivos de una subida en la carpeta de trabajo.
func (manager *Manager) partialPath(id string) string {
	return filepath.Join(manager.workDir(), id+".adelmig.part")
}

func (manager *Manager) completePath(id string) string {
	return filepath.Join(manager.workDir(), id+".adelmig")
}

// ensureWorkDir crea la carpeta de trabajo privada si falta.
func (manager *Manager) ensureWorkDir() error {
	if manager.WorkDir == "" {
		return errors.New("sin carpeta de trabajo de la migración")
	}
	if err := os.MkdirAll(manager.WorkDir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(manager.WorkDir, 0700); err != nil {
		return err
	}
	return fileacl.Protect(manager.WorkDir)
}

func uploadError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func newUploadID() string {
	value := newUUID()
	return uuidText(value)
}

// CreateUpload empieza una subida del tamaño y la huella indicados.
func (manager *Manager) CreateUpload(actor Actor, size int64, digest string) (UploadView, error) {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if size <= 0 || !sha256Pattern.MatchString(digest) {
		return UploadView{}, uploadError(CodeUploadInvalid, "Indica el tamaño y la huella SHA-256 del paquete.")
	}
	if size > manager.maxUploadBytes() {
		return UploadView{}, uploadError(CodeUploadTooLarge,
			fmt.Sprintf("El paquete es demasiado grande: el máximo es de %d MB.", manager.maxUploadBytes()>>20))
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.ensureState()
	if len(manager.uploads) >= maxUploads {
		return UploadView{}, uploadError(CodeUploadLimit,
			"Hay demasiadas subidas abiertas. Cancela las que no uses o espera a que venzan.")
	}
	if err := manager.ensureWorkDir(); err != nil {
		return UploadView{}, failure("No se pudo preparar la carpeta de trabajo de la migración.", err)
	}
	id := newUploadID()
	file, err := fileacl.CreatePrivate(manager.partialPath(id))
	if err != nil {
		return UploadView{}, failure("No se pudo preparar la carpeta de trabajo de la migración.", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(manager.partialPath(id))
		return UploadView{}, failure("No se pudo preparar la carpeta de trabajo de la migración.", err)
	}
	now := manager.now()
	parts := int((size + PartSize - 1) / PartSize)
	entry := &upload{
		view: UploadView{
			ID: id, Status: UploadReceiving, Size: size, SHA256: digest, PartSize: PartSize, Parts: parts,
			CreatedAt: now, ExpiresAt: now.Add(manager.uploadTTL()),
		},
		userID:    actor.UserID,
		deviceID:  actor.DeviceID,
		updatedAt: now,
	}
	manager.uploads[id] = entry
	return entry.view, nil
}

// findUpload busca una subida del mismo usuario y terminal (con el mutex tomado).
func (manager *Manager) findUpload(actor Actor, id string) (*upload, error) {
	manager.ensureState()
	entry, found := manager.uploads[strings.ToLower(id)]
	if !found || entry.userID != actor.UserID || entry.deviceID != actor.DeviceID {
		return nil, uploadError(CodeUploadMissing, "La subida del paquete ya no existe. Vuelve a elegir el paquete.")
	}
	return entry, nil
}

// GetUpload consulta una subida.
func (manager *Manager) GetUpload(actor Actor, id string) (UploadView, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	entry, err := manager.findUpload(actor, id)
	if err != nil {
		return UploadView{}, err
	}
	return entry.view, nil
}

// AddPart agrega la parte index (desde 0) con su SHA-256. Las partes llegan en orden; reenviar la última ya recibida
// con la misma huella no cambia nada (un reintento después de un corte de red). Con la última parte se comprueba la
// huella del paquete completo: si no coincide, la subida se descarta.
func (manager *Manager) AddPart(actor Actor, id string, index int, data []byte, digest string) (UploadView, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	entry, err := manager.findUpload(actor, id)
	if err != nil {
		return UploadView{}, err
	}
	digest = strings.ToLower(strings.TrimSpace(digest))
	partDigest := sha256.Sum256(data)
	if !sha256Pattern.MatchString(digest) || hex.EncodeToString(partDigest[:]) != digest {
		return UploadView{}, uploadError(CodeUploadHash, "Una parte del paquete llegó dañada. Vuelve a enviarla.")
	}
	view := &entry.view
	if index == view.ReceivedParts-1 && digest == entry.lastDigest {
		return *view, nil
	}
	if view.Status != UploadReceiving || index != view.ReceivedParts {
		return UploadView{}, &Error{Code: CodeUploadOrder,
			Message: fmt.Sprintf("Se esperaba la parte %d del paquete.", view.ReceivedParts)}
	}
	expected := int64(PartSize)
	if index == view.Parts-1 {
		expected = view.Size - int64(view.Parts-1)*PartSize
	}
	if int64(len(data)) != expected {
		return UploadView{}, uploadError(CodeUploadInvalid, "Una parte del paquete no tiene el tamaño esperado.")
	}
	// La parte se escribe en su lugar (el desplazamiento de lo ya recibido), después de recortar el archivo a lo
	// recibido: si un intento anterior de esta misma parte falló a medias, sus bytes no quedan de más.
	if err := writePartAt(manager.partialPath(view.ID), view.ReceivedBytes, data); err != nil {
		return UploadView{}, failure("No se pudo guardar la parte del paquete.", err)
	}
	entry.lastDigest = digest
	view.ReceivedParts++
	view.ReceivedBytes += int64(len(data))
	now := manager.now()
	entry.updatedAt = now
	view.ExpiresAt = now.Add(manager.uploadTTL())
	if view.ReceivedParts < view.Parts {
		return *view, nil
	}
	// La huella del paquete completo se calcula del archivo tal como quedó en el disco.
	fileDigest, fileSize, err := fileSHA256(manager.partialPath(view.ID))
	if err != nil {
		return UploadView{}, failure("No se pudo comprobar el paquete.", err)
	}
	if fileSize != view.Size || fileDigest != view.SHA256 {
		manager.dropUpload(view.ID)
		return UploadView{}, uploadError(CodeUploadHash,
			"El paquete llegó dañado (su huella no coincide). Vuelve a elegirlo y a subirlo.")
	}
	if err := os.Rename(manager.partialPath(view.ID), manager.completePath(view.ID)); err != nil {
		return UploadView{}, failure("No se pudo guardar el paquete.", err)
	}
	view.Status = UploadComplete
	return *view, nil
}

// CancelUpload borra una subida (no mientras un análisis o una importación la usan).
func (manager *Manager) CancelUpload(actor Actor, id string) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	entry, err := manager.findUpload(actor, id)
	if err != nil {
		return err
	}
	if entry.busy > 0 {
		return uploadError(CodeUploadBusy, "El paquete se está usando en un análisis o una importación en curso.")
	}
	manager.dropUpload(entry.view.ID)
	return nil
}

// dropUpload borra los archivos de una subida y la olvida (con el mutex tomado).
func (manager *Manager) dropUpload(id string) {
	_ = os.Remove(manager.partialPath(id))
	_ = os.Remove(manager.completePath(id))
	delete(manager.uploads, id)
}

// claimUpload reserva una subida completa para un trabajo y devuelve su archivo.
func (manager *Manager) claimUpload(actor Actor, id string) (UploadView, string, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	entry, err := manager.findUpload(actor, id)
	if err != nil {
		return UploadView{}, "", err
	}
	if entry.view.Status != UploadComplete {
		return UploadView{}, "", uploadError(CodeUploadInvalid, "El paquete todavía no termina de subirse.")
	}
	entry.busy++
	return entry.view, manager.completePath(entry.view.ID), nil
}

// releaseUpload suelta la reserva; remove borra la subida (después de una importación correcta).
func (manager *Manager) releaseUpload(id string, remove bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.ensureState()
	entry, found := manager.uploads[id]
	if !found {
		return
	}
	entry.busy--
	now := manager.now()
	entry.updatedAt = now
	entry.view.ExpiresAt = now.Add(manager.uploadTTL())
	if remove && entry.busy <= 0 {
		manager.dropUpload(id)
	}
}

// cleanExpired borra las subidas vencidas que ningún trabajo usa.
func (manager *Manager) cleanExpired() int {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.ensureState()
	now := manager.now()
	removed := 0
	for id, entry := range manager.uploads {
		if entry.busy == 0 && now.After(entry.view.ExpiresAt) {
			manager.dropUpload(id)
			removed++
		}
	}
	return removed
}

// cleanOrphans borra lo que haya quedado en la carpeta de trabajo de un servidor anterior (las subidas viven en
// memoria). Solo borra archivos regulares con los nombres de las subidas.
func (manager *Manager) cleanOrphans() int {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.ensureState()
	entries, err := os.ReadDir(manager.workDir())
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !(strings.HasSuffix(name, ".adelmig") || strings.HasSuffix(name, ".adelmig.part")) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".part"), ".adelmig")
		if _, known := manager.uploads[id]; known {
			continue
		}
		if os.Remove(filepath.Join(manager.workDir(), name)) == nil {
			removed++
		}
	}
	return removed
}

// writePartAt recorta el archivo a offset y escribe data desde ahí.
func writePartAt(path string, offset int64, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Truncate(offset); err != nil {
		file.Close()
		return err
	}
	if _, err := file.WriteAt(data, offset); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// fileSHA256 es la huella y el tamaño de un archivo.
func fileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}
