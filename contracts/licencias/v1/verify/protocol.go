package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Prefijos de los mensajes que firma la instalación con su llave de licencias.
const (
	proofPrefix   = "ADELPOS-LIC-V1"
	requestPrefix = "ADELPOS-LICREQ-V1\n"
)

// ProofMessage arma el mensaje de la prueba de posesión (contrato, sección 8.2). En «activate»
// y «transfer» no hay activación todavía y el último renglón es un guion.
func ProofMessage(action, challengeID, nonce, product, serverID string, activationID *string) string {
	activation := "-"
	if activationID != nil {
		activation = *activationID
	}
	return strings.Join([]string{proofPrefix, action, challengeID, nonce, product, serverID, activation}, "\n")
}

// VerifyProof comprueba la firma de una prueba de posesión con la llave pública de la instalación.
func VerifyProof(message, proof, publicKey string) bool {
	signature, okSignature := decodeBase64URL(proof)
	key, okKey := decodeBase64URL(publicKey)
	if !okSignature || !okKey || len(signature) != ed25519.SignatureSize || len(key) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(key), []byte(message), signature)
}

// Campos del contenido de una solicitud «.licreq» (contrato, sección 9.1).
var requestFields = []string{
	"action", "request_id", "created_at", "product_id", "server_id", "installation_public_key", "app_version",
}

// VerifyOfflineRequest comprueba el sobre, los campos y la firma de una solicitud «.licreq»,
// y devuelve su contenido.
func VerifyOfflineRequest(envelope []byte) (map[string]string, error) {
	var outer map[string]string
	if err := json.Unmarshal(envelope, &outer); err != nil {
		return nil, errors.New("el sobre no es un objeto JSON de textos")
	}
	if len(outer) != 3 || outer["schema_version"] != "1.0" {
		return nil, errors.New("el sobre debe tener schema_version 1.0, payload_b64u y signature_b64u")
	}
	payloadJSON, okPayload := decodeBase64URL(outer["payload_b64u"])
	signature, okSignature := decodeBase64URL(outer["signature_b64u"])
	if !okPayload || !okSignature || len(signature) != ed25519.SignatureSize {
		return nil, errors.New("base64url inválido en el sobre")
	}
	var payload map[string]string
	decoder := json.NewDecoder(bytes.NewReader(payloadJSON))
	if err := decoder.Decode(&payload); err != nil {
		return nil, errors.New("el contenido no es un objeto JSON de textos")
	}

	// Campos exactos según la acción.
	expected := append([]string{}, requestFields...)
	switch payload["action"] {
	case "activate":
		if _, withKey := payload["license_key"]; withKey {
			expected = append(expected, "license_key")
		}
	case "renew", "deactivate":
		expected = append(expected, "license_id", "activation_id")
	default:
		return nil, errors.New("acción desconocida")
	}
	received := make([]string, 0, len(payload))
	for field := range payload {
		received = append(received, field)
	}
	sort.Strings(received)
	sort.Strings(expected)
	if strings.Join(received, ",") != strings.Join(expected, ",") {
		return nil, errors.New("campos faltantes o sobrantes")
	}
	if !uuidV4Pattern.MatchString(payload["server_id"]) || !instantPattern.MatchString(payload["created_at"]) {
		return nil, errors.New("server_id o created_at con formato inválido")
	}

	key, okKey := decodeBase64URL(payload["installation_public_key"])
	if !okKey || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("llave pública inválida")
	}
	// Se firma el segmento tal como viaja, con su prefijo.
	if !ed25519.Verify(ed25519.PublicKey(key), []byte(requestPrefix+outer["payload_b64u"]), signature) {
		return nil, errors.New("firma inválida")
	}
	return payload, nil
}
