package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/businessday"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/idempotency"
)

// «Agotado» (etapa 9G, §5.4, decisión D8). Quien tiene catalog.sold_out.manage marca un producto como agotado desde
// la captura del punto de venta con un toque, y con otro lo vuelve a «Disponible otra vez». Reglas:
//   - Es distinto de «Pausar» (temporary_unavailable, catalog.availability.manage), que no cambia.
//   - Un producto agotado no se puede capturar ni confirmar (409 PRODUCT_SOLD_OUT), tampoco como complemento ni como
//     componente de un combo, y ninguna excepción de disponibilidad lo pasa por alto.
//   - Vuelve solo a «disponible» cuando empieza el día comercial siguiente («Hora de inicio del día», 05:00 por
//     omisión): el fin se calcula al marcarlo con internal/businessday y se guarda en sold_out_until. «Hasta nuevo
//     aviso» (indefinite) no vence. Mientras el trabajador no limpia la marca vencida, la lectura ya la ignora.
//   - Aplica a productos (también a los que se usan como complemento o componente de combo). Los complementos que
//     son ingredientes y las porciones no se agotan desde la captura: se desactivan en Catálogo.
//   - Lleva clave de operación (idempotente), se audita y se avisa en tiempo real a todas las terminales (ámbito
//     «catalog»). No cambia la revisión del producto, para no interrumpir a quien lo edita en Catálogo.

const (
	soldOutReasonText           = "Agotado."
	soldOutIndefiniteReasonText = "Agotado hasta nuevo aviso."
)

// soldOutNow dice si el producto está agotado en este instante.
func soldOutNow(product Product, now time.Time) bool {
	if product.SoldOutAt == nil {
		return false
	}
	return product.SoldOutUntil == nil || now.Before(*product.SoldOutUntil)
}

// soldOutReason es el motivo de no disponibilidad de un producto agotado, en palabras del restaurante.
func soldOutReason(product Product) string {
	if product.SoldOutUntil == nil {
		return soldOutIndefiniteReasonText
	}
	return soldOutReasonText
}

func isSoldOutReason(reason string) bool {
	return reason == soldOutReasonText || reason == soldOutIndefiniteReasonText
}

// SoldOutState es la respuesta de «Agotado» y el estado que acompaña a cada producto en la captura y en la descarga
// del catálogo.
type SoldOutState struct {
	ProductID      string     `json:"product_id"`
	Name           string     `json:"name"`
	SoldOut        bool       `json:"sold_out"`
	Indefinite     bool       `json:"indefinite"`
	SoldOutAt      *time.Time `json:"sold_out_at"`
	SoldOutUntil   *time.Time `json:"sold_out_until"`
	CatalogVersion int64      `json:"catalog_version"`
}

// soldOutState resume el estado vigente de un producto.
func soldOutState(product Product, now time.Time) SoldOutState {
	state := SoldOutState{ProductID: product.ID, Name: product.Name}
	if soldOutNow(product, now) {
		state.SoldOut = true
		state.Indefinite = product.SoldOutUntil == nil
		state.SoldOutAt = product.SoldOutAt
		state.SoldOutUntil = product.SoldOutUntil
	}
	return state
}

// productSoldOut atiende POST catalog/products/{id}/sold-out con {operation_id, sold_out, indefinite}. Catalog.Handle
// ya tomó el lock del catálogo exclusivo, así que dos reintentos simultáneos de la misma clave se forman uno detrás
// del otro.
func productSoldOut(ctx context.Context, tx pgx.Tx, request Request, id string, audit Audit) (any, error) {
	if err := request.Actor.Require("catalog.sold_out.manage"); err != nil {
		return nil, err
	}
	if request.Method != "POST" {
		return nil, invalid("Marcar un producto como agotado requiere POST.")
	}
	var input struct {
		OperationID string `json:"operation_id"`
		SoldOut     bool   `json:"sold_out"`
		Indefinite  bool   `json:"indefinite"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.OperationID); err != nil {
		return nil, invalid("La operación requiere una clave de operación válida.")
	}
	if input.Indefinite && !input.SoldOut {
		return nil, invalid("«Hasta nuevo aviso» solo aplica al marcar un producto como agotado.")
	}
	fingerprint, err := idempotency.Fingerprint(request.Method, request.Route, request.Body)
	if err != nil {
		return nil, invalid("La operación no contiene JSON válido.")
	}
	key := idempotency.Key{
		OperationID: input.OperationID,
		ActorID:     request.Actor.UserID,
		DeviceID:    request.Actor.DeviceID,
		Route:       request.Method + " " + request.Route,
		Fingerprint: fingerprint,
	}
	stored, found, err := idempotency.Replay(ctx, tx, key)
	if errors.Is(err, idempotency.ErrConflict) {
		return nil, &Error{Status: 409, Code: "IDEMPOTENCY_CONFLICT", Message: idempotency.ConflictMessage}
	}
	if err != nil {
		return nil, err
	}
	if found {
		return stored, nil
	}
	document, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	product := document.(*Product)
	now := time.Now()
	before := soldOutState(*product, now)
	if input.SoldOut && (!product.Active || product.Archived) {
		return nil, invalid("Solo un producto activo se puede marcar como agotado.")
	}
	changed := input.SoldOut || before.SoldOut || product.SoldOutAt != nil
	if changed {
		if err := writeSoldOut(ctx, tx, request.Actor.UserID, id, input.SoldOut, input.Indefinite, now); err != nil {
			return nil, err
		}
	}
	saved, err := readDocument(ctx, tx, "products", id)
	if err != nil {
		return nil, err
	}
	after := soldOutState(*saved.(*Product), now)
	if after.CatalogVersion, err = currentVersion(ctx, tx); err != nil {
		return nil, err
	}
	// «Disponible otra vez» de un producto que no estaba agotado no cambia nada: no se audita ni se avisa.
	if before.SoldOut || after.SoldOut {
		action := "catalog.product.sold_out"
		kind := "catalog.product.sold_out"
		if !after.SoldOut {
			action = "catalog.product.available_again"
			kind = "catalog.product.available"
		}
		if err := audit(ctx, tx, action, "products", &id, before, after); err != nil {
			return nil, err
		}
		if err := events.Append(ctx, tx, kind, "catalog", &id, nil); err != nil {
			return nil, err
		}
	}
	// La primera respuesta es la misma que se guarda, para que un reintento reciba exactamente los mismos bytes.
	return idempotency.Remember(ctx, tx, key, after)
}

// writeSoldOut marca o limpia «Agotado». Al marcarlo, el fin es el inicio del día comercial siguiente con la zona y la
// «Hora de inicio del día» vigentes, salvo «hasta nuevo aviso».
func writeSoldOut(ctx context.Context, tx pgx.Tx, userID, productID string, soldOut, indefinite bool, now time.Time) error {
	if !soldOut {
		_, err := tx.Exec(ctx, `UPDATE app.catalog_products SET sold_out_at=NULL,sold_out_until=NULL,sold_out_by=NULL
            WHERE id=$1`, productID)
		return err
	}
	var until *time.Time
	if !indefinite {
		configuration, err := businessday.Load(ctx, tx)
		if err != nil {
			return err
		}
		deadline := configuration.DayOf(now).Deadline
		until = &deadline
	}
	var by *string
	if userID != "" {
		by = &userID
	}
	_, err := tx.Exec(ctx, `UPDATE app.catalog_products SET sold_out_at=$2,sold_out_until=$3,sold_out_by=$4 WHERE id=$1`,
		productID, now, until, by)
	return err
}

// Database es lo que necesitan el trabajador de «Agotado» y ResetExpiredSoldOut: el pool de pgx.
type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// SoldOutWorker devuelve a «disponible» los productos agotados cuando empieza el día comercial siguiente. Arranca con
// el servidor, como los trabajadores de cocina e inventario, y revisa cada SoldOutInterval; al iniciar limpia lo que
// venció mientras el servidor estaba apagado.
type SoldOutWorker struct {
	Pool   Database
	Logger *slog.Logger
}

// SoldOutInterval es cada cuánto revisa el trabajador. La lectura ya ignora una marca vencida; el trabajador solo la
// limpia, sube la versión del catálogo y avisa a las terminales.
const SoldOutInterval = 30 * time.Second

// Run revisa hasta que se cancela el contexto.
func (worker *SoldOutWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(SoldOutInterval)
	defer ticker.Stop()
	for {
		if _, err := ResetExpiredSoldOut(ctx, worker.Pool, time.Now()); err != nil && ctx.Err() == nil && worker.Logger != nil {
			worker.Logger.Warn("catalog_sold_out_reset_unavailable")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// soldOutWorkerProcess identifica al trabajador en el Registro de movimientos.
const soldOutWorkerProcess = "catalog_sold_out_worker"

// ResetExpiredSoldOut limpia las marcas de «Agotado» que ya vencieron (empezó el día comercial siguiente), las anota
// en el Registro de movimientos y avisa a las terminales. Devuelve cuántos productos volvieron a estar disponibles.
// Sin marcas vencidas no toma el lock del catálogo.
func ResetExpiredSoldOut(ctx context.Context, pool Database, now time.Time) (int, error) {
	var pending bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.catalog_products
        WHERE sold_out_at IS NOT NULL AND sold_out_until <= $1)`, now).Scan(&pending); err != nil {
		return 0, err
	}
	if !pending {
		return 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148304)"); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `UPDATE app.catalog_products SET sold_out_at=NULL,sold_out_until=NULL,sold_out_by=NULL
        WHERE sold_out_at IS NOT NULL AND sold_out_until <= $1
        RETURNING id::text,name`, now)
	if err != nil {
		return 0, err
	}
	type reset struct {
		id, name string
	}
	resets := []reset{}
	for rows.Next() {
		var item reset
		if err := rows.Scan(&item.id, &item.name); err != nil {
			rows.Close()
			return 0, err
		}
		resets = append(resets, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range resets {
		after, err := json.Marshal(map[string]any{"product_id": item.id, "name": item.name, "sold_out": false, "reset_at": now})
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_id,actor_username,actor_kind,device_id,action,entity_type,
                entity_id,success,before_state,after_state)
            VALUES (NULL,$1,'process',NULL,'catalog.product.sold_out_reset','products',$2,true,NULL,$3)`,
			soldOutWorkerProcess, item.id, after)
		if err != nil {
			return 0, err
		}
		id := item.id
		if err := events.Append(ctx, tx, "catalog.product.available", "catalog", &id, nil); err != nil {
			return 0, err
		}
	}
	return len(resets), tx.Commit(ctx)
}
