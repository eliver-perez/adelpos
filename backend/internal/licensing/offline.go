package licensing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
)

// Activación y renovación sin internet (contrato §9; §3 de la especificación; Fase B).
//
//   - La solicitud `.licreq` se firma con la llave de licencias: `activate` (con el código comercial, opcional),
//     `renew` y `deactivate`. Recibirla no concede nada: el propietario la aprueba en el panel.
//   - La respuesta `.lic` es el mismo JWS de la API en línea y se importa con Manager.Accept, igual que una respuesta
//     en línea (§10). Un archivo no trae server_time: solo su issued_at alimenta la última hora confiable.
//
// El contenido se arma con los campos en el orden de los archivos de prueba del contrato y JSON compacto, así que la
// solicitud de AD-EL POS con los mismos datos es idéntica byte por byte a la publicada (offline_test.go).

// Acciones de una solicitud sin internet (§9.1).
const (
	OfflineActivate   = "activate"
	OfflineRenew      = "renew"
	OfflineDeactivate = "deactivate"
)

// maxLicenseFile limita un .lic importado (una licencia mide alrededor de 1,5 KiB).
const maxLicenseFile = 16 << 10

// OfflineRequest es una solicitud .licreq lista para guardarse como archivo.
type OfflineRequest struct {
	Action    string
	RequestID string
	// Filename es el nombre sugerido del archivo (solicitud-<acción>-AAAAMMDD-HHMMSS.licreq).
	Filename string
	// Content es el archivo, tal como viaja (JSON en UTF-8, sin salto de línea final).
	Content string
}

// offlinePayload es el contenido firmado (§9.1), con los campos en el orden de los archivos de prueba.
type offlinePayload struct {
	Action                string `json:"action"`
	RequestID             string `json:"request_id"`
	CreatedAt             string `json:"created_at"`
	ProductID             string `json:"product_id"`
	ServerID              string `json:"server_id"`
	InstallationPublicKey string `json:"installation_public_key"`
	AppVersion            string `json:"app_version"`
	LicenseKey            string `json:"license_key,omitempty"`
	LicenseID             string `json:"license_id,omitempty"`
	ActivationID          string `json:"activation_id,omitempty"`
}

// offlineEnvelope es el sobre del .licreq.
type offlineEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	PayloadB64U   string `json:"payload_b64u"`
	SignatureB64U string `json:"signature_b64u"`
}

// actionFileNames es la acción en el nombre del archivo, en palabras del restaurante.
var actionFileNames = map[string]string{
	OfflineActivate:   "activar",
	OfflineRenew:      "renovar",
	OfflineDeactivate: "liberar",
}

// OfflineRequest arma y firma una solicitud sin internet. En activate, licenseKey es opcional (se valida y se
// normaliza si viene); en renew y deactivate no se admite y hace falta una activación instalada.
func (manager *Manager) OfflineRequest(action, licenseKey, appVersion string) (OfflineRequest, error) {
	installation, private, err := manager.signingIdentity()
	if err != nil {
		return OfflineRequest{}, err
	}
	now := manager.now()
	payload := offlinePayload{
		Action:                action,
		RequestID:             newRequestID(),
		CreatedAt:             formatInstant(now),
		ProductID:             ProductID,
		ServerID:              installation.ServerID,
		InstallationPublicKey: installation.PublicKey,
		AppVersion:            appVersion,
	}
	switch action {
	case OfflineActivate:
		if strings.TrimSpace(licenseKey) != "" {
			normalized, err := NormalizeLicenseKey(licenseKey)
			if err != nil {
				return OfflineRequest{}, err
			}
			payload.LicenseKey = normalized
		}
	case OfflineRenew, OfflineDeactivate:
		if strings.TrimSpace(licenseKey) != "" {
			return OfflineRequest{}, errors.New("el código de la licencia solo va en la solicitud de activación")
		}
		licenseID, activationID, ok := manager.currentLicenseIDs()
		if !ok {
			return OfflineRequest{}, ErrNotActivated
		}
		payload.LicenseID = licenseID
		payload.ActivationID = activationID
	default:
		return OfflineRequest{}, errors.New("acción desconocida para una solicitud sin internet")
	}
	if payload.AppVersion == "" || len(payload.AppVersion) > 100 || controlPattern.MatchString(payload.AppVersion) {
		payload.AppVersion = "0.0.0"
	}
	content, err := buildOfflineRequest(payload, private)
	if err != nil {
		return OfflineRequest{}, err
	}
	manager.logger.Info("license_offline_request_created", "action", action, "request_id", payload.RequestID)
	return OfflineRequest{
		Action:    action,
		RequestID: payload.RequestID,
		Filename:  "solicitud-" + actionFileNames[action] + "-" + now.UTC().Format("20060102-150405") + ".licreq",
		Content:   content,
	}, nil
}

// buildOfflineRequest firma el contenido: Ed25519(llave, "ADELPOS-LICREQ-V1\n" + payload_b64u) (§9.1), y comprueba la
// solicitud con la misma verificación de referencia que usa el servidor antes de entregarla.
func buildOfflineRequest(payload offlinePayload, private ed25519.PrivateKey) (string, error) {
	payloadJSON, err := compactJSON(payload)
	if err != nil {
		return "", err
	}
	payloadB64U := encodeBase64URL(payloadJSON)
	signature := ed25519.Sign(private, []byte(requestPrefix+payloadB64U))
	envelope, err := compactJSON(offlineEnvelope{SchemaVersion: "1.0", PayloadB64U: payloadB64U, SignatureB64U: encodeBase64URL(signature)})
	if err != nil {
		return "", err
	}
	if _, err := VerifyOfflineRequest(envelope); err != nil {
		return "", errors.New("la solicitud sin internet no pasó su propia verificación")
	}
	return string(envelope), nil
}

// compactJSON es JSON compacto sin escapar <, > ni & (como los archivos de prueba) y sin salto de línea final.
func compactJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// ImportLicense importa un archivo .lic (la respuesta de una solicitud sin internet): el mismo JWS, verificado con
// la §10 como una respuesta en línea. Se quitan los espacios y saltos de línea de alrededor que agregan los editores
// y el correo. Un archivo rechazado (*VerifyError) no cambia nada.
func (manager *Manager) ImportLicense(content string) (Status, error) {
	if len(content) > maxLicenseFile {
		return manager.Status(), &VerifyError{Code: ErrMalformed, Detail: "archivo demasiado grande"}
	}
	jws := strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
	status, err := manager.Accept(jws)
	if err == nil {
		manager.logger.Info("license_imported", "license_id", status.LicenseID, "revision", status.Revision)
	}
	return status, err
}
