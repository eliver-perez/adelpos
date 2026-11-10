package inventory

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/features"
)

// Registro de solicitudes de consumo desde cocina (§0-B «Consumo asíncrono»). Estas funciones se
// ejecutan dentro de la transacción de restaurante y cocina (lock 7820148305) y deben ser mínimas:
//   - comprueban la función «Control de inventario» (una lectura por clave primaria);
//   - insertan solicitudes con una sola sentencia INSERT … SELECT … ON CONFLICT DO NOTHING;
//   - no toman el lock de inventario 7820148308, no leen recetas ni calculan nada.
// El trabajador de inventario (worker.go) las procesa después en sus propias transacciones. Así la
// captura y la liberación a cocina nunca esperan al cálculo de consumo.

// ReleasedUnit es una unidad comercial liberada en una liberación canónica.
type ReleasedUnit struct {
	UnitID    string
	VersionID string
}

// RecordReleases registra una solicitud «release» por cada unidad de la liberación con fila
// canónica en kitchen_unit_releases (released_at y versión se toman de esa fila). mode es la
// modalidad congelada en la liberación (dine_in o takeaway). Con la función desactivada no
// registra nada; una liberación repetida no duplica (restricción única por unidad y liberación).
func RecordReleases(ctx context.Context, tx pgx.Tx, releaseID string, units []ReleasedUnit, mode string) error {
	if len(units) == 0 {
		return nil
	}
	enabled, err := features.Enabled(ctx, tx, features.InventoryControl)
	if err != nil || !enabled {
		return err
	}
	identifiers := make([]string, 0, len(units))
	for _, unit := range units {
		identifiers = append(identifiers, unit.UnitID)
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.inventory_consumption_requests(kind,restaurant_unit_id,source_id,version_id,released_at,
            occurred_at,service_mode)
        SELECT 'release',r.unit_id,r.release_id,r.version_id,r.released_at,r.released_at,$3
        FROM app.kitchen_unit_releases r WHERE r.release_id=$1 AND r.unit_id=ANY($2::uuid[])
        ON CONFLICT ON CONSTRAINT inventory_consumption_requests_source_key DO NOTHING`, releaseID, identifiers, mode)
	return err
}

// changePayload es lo que el inventario lee del evento comercial item.modified o item.cancelled.
type changePayload struct {
	After struct {
		Modified  []string `json:"modified_unit_ids"`
		Cancelled []string `json:"cancelled_unit_ids"`
		UnitIDs   []string `json:"unit_ids"`
	} `json:"after"`
}

// RecordChange registra las solicitudes de ajuste de un cambio comercial a unidades ya liberadas:
// «amend» por cada unidad modificada (con la versión vigente de la unidad tras el cambio) y
// «cancel» por cada unidad cancelada (incluidas las reducciones de cantidad de item.modified).
// Solo para unidades con fila canónica y con solicitud de liberación (es decir, cuya liberación
// consumió con la función activada): una unidad liberada antes de activar nunca se ajusta. Se
// llama antes del procesamiento de cocina, de modo que se registra también si el cambio de cocina
// queda pendiente por advertencias. Con la función desactivada no registra nada.
func RecordChange(ctx context.Context, tx pgx.Tx, eventID, kind string, payload []byte) error {
	if kind != "item.modified" && kind != "item.cancelled" {
		return nil
	}
	enabled, err := features.Enabled(ctx, tx, features.InventoryControl)
	if err != nil || !enabled {
		return err
	}
	var change changePayload
	if err := json.Unmarshal(payload, &change); err != nil {
		return err
	}
	modified := change.After.Modified
	cancelled := change.After.Cancelled
	if kind == "item.cancelled" {
		modified = nil
		cancelled = change.After.UnitIDs
	}
	if len(modified) == 0 && len(cancelled) == 0 {
		return nil
	}
	if modified == nil {
		modified = []string{}
	}
	if cancelled == nil {
		cancelled = []string{}
	}
	// Una sola sentencia para las dos clases de unidades. occurred_at es la hora del cambio (la de
	// la cancelación en las canceladas); la modalidad es la congelada en su liberación.
	_, err = tx.Exec(ctx, `INSERT INTO app.inventory_consumption_requests(kind,restaurant_unit_id,source_id,version_id,released_at,
            occurred_at,service_mode)
        SELECT changed.kind,u.id,$1,u.version_id,k.released_at,coalesce(u.cancelled_at,now()),c.service_mode
        FROM (SELECT unnest($2::uuid[]) AS unit_id,'amend'::text AS kind
              UNION ALL SELECT unnest($3::uuid[]),'cancel') changed
        JOIN app.restaurant_item_units u ON u.id=changed.unit_id
        JOIN app.kitchen_unit_releases k ON k.unit_id=u.id
        JOIN app.inventory_consumption_requests c ON c.kind='release' AND c.restaurant_unit_id=u.id
        ON CONFLICT ON CONSTRAINT inventory_consumption_requests_source_key DO NOTHING`, eventID, modified, cancelled)
	return err
}
