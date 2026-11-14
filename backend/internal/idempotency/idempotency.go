// Package idempotency guarda y repite el resultado de una escritura con clave de operación (etapa 9G, §5.2) para los
// módulos que no tenían la suya: hoy, «Agotado» del catálogo. Usa la misma tabla y las mismas reglas que pedidos,
// cuentas, cocina y cobros (app.restaurant_operations): la clave es un UUID que genera el cliente y conserva al
// reintentar; si llega otra vez con el mismo usuario, la misma terminal y el mismo contenido, se devuelve la respuesta
// guardada sin repetir la operación; con otro contenido, otro usuario u otra terminal se rechaza
// (IDEMPOTENCY_CONFLICT). Las claves se conservan para siempre.
//
// Concurrencia: el llamador busca y guarda la clave con el lock exclusivo de su módulo ya tomado, de modo que dos
// reintentos simultáneos se forman uno detrás del otro y el segundo encuentra la respuesta del primero. Un error
// revierte la transacción y no guarda la clave: el reintento vuelve a ejecutar.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrConflict indica que la clave ya se usó con otro contenido, otro usuario u otra terminal.
var ErrConflict = errors.New("IDEMPOTENCY_CONFLICT")

// ConflictMessage es el texto para el usuario del conflicto, el mismo de pedidos y cobros.
const ConflictMessage = "Esta operación ya se había enviado con otros datos y no se repitió. Consulta el resultado " +
	"antes de intentarlo de nuevo."

// Key identifica una escritura: la clave del cliente, quién y desde dónde la envía, la ruta y la huella del cuerpo.
type Key struct {
	OperationID string
	ActorID     string
	DeviceID    string
	Route       string
	Fingerprint string
}

// Fingerprint es la huella del contenido: SHA-256 del método, la ruta y el cuerpo en JSON canónico (claves
// ordenadas, números sin reinterpretar), como en pedidos.
func Fingerprint(method, route string, body []byte) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(method + " " + route + "\n" + string(canonical)))
	return fmt.Sprintf("%x", digest), nil
}

// Replay busca la clave. found es verdadero si ya existía con el mismo usuario, terminal y huella: response es la
// respuesta guardada. Con datos distintos devuelve ErrConflict.
func Replay(ctx context.Context, tx pgx.Tx, key Key) (json.RawMessage, bool, error) {
	var actorID, deviceID, fingerprint string
	var response json.RawMessage
	err := tx.QueryRow(ctx, `SELECT actor_id::text,device_id::text,fingerprint,response
        FROM app.restaurant_operations WHERE id=$1`, key.OperationID).Scan(&actorID, &deviceID, &fingerprint, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != key.ActorID || deviceID != key.DeviceID || fingerprint != key.Fingerprint {
		return nil, false, ErrConflict
	}
	return response, true, nil
}

// Remember guarda la respuesta de la escritura en la misma transacción que la escritura y la devuelve tal como quedó
// guardada (jsonb ordena las claves): así la primera respuesta y la de un reintento son idénticas.
func Remember(ctx context.Context, tx pgx.Tx, key Key, response any) (json.RawMessage, error) {
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	var stored json.RawMessage
	err = tx.QueryRow(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6) RETURNING response`, key.OperationID, key.ActorID, key.DeviceID, key.Route, key.Fingerprint,
		encoded).Scan(&stored)
	return stored, err
}
