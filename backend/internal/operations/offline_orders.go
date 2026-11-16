package operations

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"local/ad-el-pos/server/internal/catalog"
)

// Pedido capturado sin conexión (etapa 9G, §5.5 y §0-B de la especificación). La futura aplicación Android de
// meseros podrá capturar sin señal con el catálogo guardado en el teléfono y enviar después cada pedido de su cola
// con POST restaurant/offline-orders. Reglas:
//
//   - El cuerpo lleva la clave de operación (una por pedido de la cola, la misma en todos sus reenvíos), la versión
//     del catálogo con que se capturó, la hora de captura, el destino (una cuenta abierta o una mesa donde abrir una
//     cuenta nueva) y, por partida, lo que pidió y el precio por unidad que vio el mesero (el de la descarga del
//     catálogo, §5.3).
//   - Go vuelve a calcular todo con el catálogo vigente (catalog.QuoteForOrder, la única autoridad de precios) y
//     revisa el destino. Si nada cambió, el pedido se acepta como «Enviar» en la captura: se confirma en una sola
//     operación idempotente (pedido, versiones y unidades con los precios del servidor), sin tocar el borrador de la
//     cuenta, y cocina lo recibe como cualquier confirmación.
//   - Si cambió algo, no se acepta en silencio: 409 ORDER_REQUIRES_CONFIRMATION con la lista de cambios (producto,
//     antes, ahora y motivo) y la versión vigente del catálogo. Hay dos clases de cambio:
//       · aceptables con la confirmación del mesero: el precio cambió (price_changed) o la mesa ya tiene una cuenta
//         que el mesero no veía (table_has_account);
//       · que exigen corregir el pedido: producto agotado, eliminado o no disponible, complemento o parte de la
//         partida no disponible, partida que ya no cumple las reglas del catálogo, cuenta cobrada o cerrada, mesa
//         fuera de servicio. El cliente quita o cambia la partida (o el destino) y reenvía.
//   - Se acepta un pedido con cambios aceptables solo si el cliente lo reenvía con confirmation: la versión vigente
//     del catálogo (la que devolvió el rechazo) y los cambios que vio el mesero, que deben cubrir todos los cambios
//     de ese momento. Una confirmación con una versión que ya no es la vigente se rechaza de nuevo con los cambios
//     actuales. Siempre entra con los precios del servidor.
//   - El rechazo no guarda la clave de operación (se revierte), así que el reenvío confirmado usa la misma clave; un
//     envío repetido de un pedido ya aceptado devuelve la misma respuesta sin repetirlo, y un envío con la misma
//     clave y otro contenido después de aceptado es IDEMPOTENCY_CONFLICT (nunca un pedido doble).
//   - Registro de movimientos: restaurant.offline_order.accepted (hora de captura, hora de llegada, versiones del
//     catálogo y cambios aceptados) y, en cada rechazo, restaurant.offline_order.requires_confirmation con los
//     cambios. Ambos con la clave de operación como identificador.
//
// Borrador del servidor (etapa A1, decisión A1-D6). En línea, el teléfono captura como el escritorio: la cuenta nace con
// el primer producto y el borrador se guarda en Go (uno por cuenta, que cada guardado reemplaza completo). Si la red se
// corta después de guardar, el pedido que el mesero termina sin conexión contiene esos renglones; enviarlo tal cual los
// duplicaría (el borrador seguiría en la cuenta). Por eso, con account_id, el pedido puede decir en
// replaces_draft_items qué renglones del borrador sustituye, tal como el dispositivo los vio (id, producto,
// presentación, cantidad y personalización):
//
//   - al aceptarlo, en la misma transacción, Go quita del borrador los renglones que siguen exactamente así y publica
//     lo mismo que un guardado del borrador (restaurant.draft.saved, con su historia y su Registro de movimientos)
//     para que las demás terminales se enteren. Lo demás del borrador (lo de otras terminales) no se toca;
//   - un renglón que ya no está como el dispositivo lo vio no se quita ni se acepta en silencio: es un cambio más de
//     la lista (draft_item_changed si otra terminal lo cambió, draft_item_removed si lo quitaron, draft_item_sent si ya
//     se envió a cocina: la partida confirmada conserva el id del renglón en restaurant_items.draft_item_id). Los tres
//     se aceptan con la confirmación del mesero, como un cambio de precio: el pedido entra como lo capturó y el
//     renglón de la otra terminal se queda como está; si no, el mesero corrige su pedido y lo reenvía.
//   - Idempotencia, límites y credencial de envío: los mismos del pedido (la clave guarda el resultado completo).

// Motivos de cambio de un pedido capturado sin conexión.
const (
	OfflinePriceChanged          = "price_changed"
	OfflineProductSoldOut        = "product_sold_out"
	OfflineProductUnavailable    = "product_unavailable"
	OfflineProductRemoved        = "product_removed"
	OfflineComplementUnavailable = "complement_unavailable"
	OfflineItemUnavailable       = "item_unavailable"
	OfflineItemInvalid           = "item_invalid"
	OfflineAccountClosed         = "account_closed"
	OfflineTableUnavailable      = "table_unavailable"
	OfflineTableHasAccount       = "table_has_account"
	// Etapa A1 (A1-D6): un renglón del borrador que el pedido sustituye ya no está como lo vio el dispositivo.
	OfflineDraftItemChanged = "draft_item_changed"
	OfflineDraftItemRemoved = "draft_item_removed"
	OfflineDraftItemSent    = "draft_item_sent"
)

// offlineClockSkew es cuánto puede adelantarse el reloj del teléfono a la hora del servidor.
const offlineClockSkew = 10 * time.Minute

// OfflineOrderInput es el cuerpo de POST restaurant/offline-orders.
type OfflineOrderInput struct {
	OperationID    string               `json:"operation_id"`
	CatalogVersion int64                `json:"catalog_version"`
	CapturedAt     time.Time            `json:"captured_at"`
	Destination    OfflineDestination   `json:"destination"`
	Items          []OfflineItem        `json:"items"`
	Confirmation   *OfflineConfirmation `json:"confirmation"`
	// ReplacesDraftItems (etapa A1, A1-D6; opcional y solo con account_id) son los renglones del borrador de la cuenta
	// que el dispositivo había guardado y que este pedido sustituye, tal como los vio.
	ReplacesDraftItems []DraftItem `json:"replaces_draft_items"`
}

// OfflineDestination es una cuenta abierta (account_id) o una mesa donde abrir una cuenta nueva (table_id, con las
// cuentas que el mesero veía abiertas en ella y un nombre opcional).
type OfflineDestination struct {
	AccountID      *string  `json:"account_id"`
	TableID        *string  `json:"table_id"`
	SeenAccountIDs []string `json:"seen_account_ids"`
	Name           string   `json:"name"`
}

// OfflineItem es una partida del pedido con el precio por unidad que vio el mesero.
type OfflineItem struct {
	DraftItem
	SeenUnitPrice string `json:"seen_unit_price"`
}

// OfflineConfirmation es la confirmación del mesero: la versión del catálogo con que se revisaron los cambios y los
// cambios que aceptó, tal como los devolvió el rechazo.
type OfflineConfirmation struct {
	CatalogVersion int64           `json:"catalog_version"`
	Changes        []OfflineChange `json:"changes"`
}

// OfflineChange es un cambio entre lo que vio el mesero y el catálogo o el destino vigentes.
type OfflineChange struct {
	Kind        string  `json:"kind"`
	ItemID      *string `json:"item_id"`
	ProductID   *string `json:"product_id"`
	ProductName string  `json:"product_name"`
	Before      *string `json:"before"`
	Now         *string `json:"now"`
	Message     string  `json:"message"`
	// Acceptable: el mesero puede aceptarlo con la confirmación; si no, hay que corregir el pedido.
	Acceptable bool `json:"acceptable"`
}

// OfflineOrderResult es la respuesta de un pedido aceptado.
type OfflineOrderResult struct {
	Status                 string          `json:"status"`
	OperationID            string          `json:"operation_id"`
	OrderID                string          `json:"order_id"`
	AccountID              string          `json:"account_id"`
	CatalogVersion         int64           `json:"catalog_version"`
	CapturedCatalogVersion int64           `json:"captured_catalog_version"`
	CapturedAt             time.Time       `json:"captured_at"`
	ReceivedAt             time.Time       `json:"received_at"`
	AcceptedChanges        []OfflineChange `json:"accepted_changes"`
	// RemovedDraftItemIDs (etapa A1) son los renglones de replaces_draft_items que se quitaron del borrador.
	RemovedDraftItemIDs []string `json:"removed_draft_item_ids"`
	Bill                *Bill    `json:"bill"`
}

// offlineDiagnosis es lo que encontró la revisión del pedido.
type offlineDiagnosis struct {
	changes     []OfflineChange
	quote       *catalog.Quote
	snapshot    catalog.Snapshot
	account     *Account
	priceListID string
	// replacedDraft son los renglones del borrador que siguen como los vio el dispositivo y se quitan al aceptarlo.
	replacedDraft []string
}

func (handler *service) offlineOrder() (any, error) {
	var input OfflineOrderInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	received := time.Now().UTC()
	if err := validateOfflineInput(&input, received); err != nil {
		return nil, err
	}
	version, err := catalog.Version(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	diagnosis, err := handler.diagnoseOffline(input)
	if err != nil {
		return nil, err
	}
	if len(diagnosis.changes) > 0 && !offlineConfirmed(input.Confirmation, version, diagnosis.changes) {
		return nil, offlineRejection(input, version, received, diagnosis.changes)
	}
	return handler.acceptOffline(input, diagnosis, version, received)
}

// validateOfflineInput revisa la forma del pedido (un error aquí es una solicitud mal formada, no un cambio).
func validateOfflineInput(input *OfflineOrderInput, received time.Time) error {
	if input.CatalogVersion < 0 {
		return invalid("La versión del catálogo con que se capturó el pedido no es válida.")
	}
	if input.CapturedAt.IsZero() || input.CapturedAt.After(received.Add(offlineClockSkew)) {
		return invalid("La hora de captura del pedido falta o es posterior a la hora del servidor.")
	}
	destination := input.Destination
	if (destination.AccountID == nil) == (destination.TableID == nil) {
		return invalid("Indica el destino del pedido: una cuenta abierta o una mesa.")
	}
	if destination.AccountID != nil && (len(destination.SeenAccountIDs) > 0 || destination.Name != "") {
		return invalid("Las cuentas vistas y el nombre solo aplican a una cuenta nueva en una mesa.")
	}
	for _, id := range destination.SeenAccountIDs {
		if err := validateID(id); err != nil {
			return err
		}
	}
	if len(destination.SeenAccountIDs) > 100 || len(strings.TrimSpace(destination.Name)) > 100 {
		return invalid("Se admiten hasta 100 cuentas vistas y un nombre de hasta 100 caracteres.")
	}
	for _, id := range []*string{destination.AccountID, destination.TableID} {
		if id != nil {
			if err := validateID(*id); err != nil {
				return err
			}
		}
	}
	if len(input.Items) < 1 || len(input.Items) > 20 {
		return invalid("Un pedido capturado sin conexión admite entre 1 y 20 partidas.")
	}
	seen := map[string]bool{}
	units := 0
	for _, item := range input.Items {
		if err := validateID(item.ID); err != nil {
			return err
		}
		if err := validateID(item.ProductID); err != nil {
			return err
		}
		if item.VariantID != nil {
			if err := validateID(*item.VariantID); err != nil {
				return err
			}
		}
		if seen[item.ID] || item.Quantity < 1 {
			return invalid("Las partidas del pedido deben ser únicas y tener cantidades positivas.")
		}
		seen[item.ID] = true
		units += item.Quantity
		if units > 1000 {
			return invalid("El pedido admite hasta 1000 unidades.")
		}
		if err := validateCustomizationShape(item.Customization, 0); err != nil {
			return err
		}
		price, err := decimal.NewFromString(item.SeenUnitPrice)
		if err != nil || price.IsNegative() || price.Exponent() < -2 {
			return invalid("Cada partida requiere el precio por unidad que vio el mesero, como importe con hasta dos decimales.")
		}
	}
	if input.Confirmation != nil && len(input.Confirmation.Changes) > 100 {
		return invalid("La confirmación admite hasta 100 cambios.")
	}
	return validateReplacedDraft(input)
}

// validateReplacedDraft revisa la forma de replaces_draft_items (etapa A1): solo con una cuenta abierta como destino,
// hasta 20 renglones (lo que admite un borrador) únicos y con la misma forma que los del borrador.
func validateReplacedDraft(input *OfflineOrderInput) error {
	if len(input.ReplacesDraftItems) == 0 {
		return nil
	}
	if input.Destination.AccountID == nil {
		return invalid("Los renglones del borrador que sustituye el pedido solo aplican a una cuenta abierta.")
	}
	if len(input.ReplacesDraftItems) > 20 {
		return invalid("Un pedido sustituye hasta 20 renglones del borrador.")
	}
	seen := map[string]bool{}
	for _, item := range input.ReplacesDraftItems {
		if err := validateID(item.ID); err != nil {
			return err
		}
		if err := validateID(item.ProductID); err != nil {
			return err
		}
		if item.VariantID != nil {
			if err := validateID(*item.VariantID); err != nil {
				return err
			}
		}
		if seen[item.ID] || item.Quantity < 1 || item.Quantity > 1000 {
			return invalid("Los renglones del borrador que sustituye el pedido deben ser únicos y tener cantidades de 1 a 1000.")
		}
		seen[item.ID] = true
		if err := validateCustomizationShape(item.Customization, 0); err != nil {
			return err
		}
	}
	return nil
}

// diagnoseOffline revisa el destino y cada partida contra el estado vigente y devuelve los cambios. Con el pedido
// sin cambios de producto, también la cotización completa (con las promociones vigentes) para confirmarlo.
func (handler *service) diagnoseOffline(input OfflineOrderInput) (*offlineDiagnosis, error) {
	diagnosis := &offlineDiagnosis{changes: []OfflineChange{}}
	destination := input.Destination
	if destination.AccountID != nil {
		account, err := handler.readAccount(*destination.AccountID)
		if errors.Is(err, pgx.ErrNoRows) {
			diagnosis.changes = append(diagnosis.changes, OfflineChange{
				Kind:    OfflineAccountClosed,
				Message: "La cuenta del pedido ya no existe. Elige otra cuenta o una mesa.",
			})
		} else if err != nil {
			return nil, err
		} else {
			if err := handler.authorizeAccount(account, true); err != nil {
				return nil, err
			}
			diagnosis.account = account
			diagnosis.priceListID = account.PriceListID
			if account.State != "open" {
				// Etapa A1 (§6.4): now lleva el estado de la cuenta (closed, voided o merged) y el mensaje dice qué
				// pasó, como en el rechazo en línea (accountNotOpen, queries.go).
				state := account.State
				diagnosis.changes = append(diagnosis.changes, OfflineChange{
					Kind:    OfflineAccountClosed,
					Now:     &state,
					Message: accountNotOpenMessage(account) + " Elige otra cuenta o abre una nueva en la mesa.",
				})
			} else {
				replaced, changes, err := handler.diagnoseReplacedDraft(account.ID, input.ReplacesDraftItems)
				if err != nil {
					return nil, err
				}
				diagnosis.replacedDraft = replaced
				diagnosis.changes = append(diagnosis.changes, changes...)
			}
		}
	} else {
		changes, err := handler.diagnoseOfflineTable(destination)
		if err != nil {
			return nil, err
		}
		diagnosis.changes = append(diagnosis.changes, changes...)
	}
	if diagnosis.priceListID == "" {
		if err := handler.tx.QueryRow(handler.ctx, "SELECT id::text FROM app.catalog_price_lists WHERE is_default AND active").
			Scan(&diagnosis.priceListID); err != nil {
			return nil, err
		}
	}
	lines := make([]catalog.LineRequest, 0, len(input.Items))
	for _, item := range input.Items {
		lines = append(lines, item.LineRequest)
	}
	quote, snapshot, err := catalog.QuoteForOrder(handler.ctx, handler.tx, catalog.PreviewRequest{
		PriceListID: &diagnosis.priceListID,
		Lines:       lines,
	})
	if err == nil {
		diagnosis.quote, diagnosis.snapshot = quote, snapshot
		for index, item := range input.Items {
			if change := offlinePriceChange(item, quote.Lines[index]); change != nil {
				diagnosis.changes = append(diagnosis.changes, *change)
			}
		}
		return diagnosis, nil
	}
	// Un producto borrado no llega al motor (no se encuentra su documento); los demás rechazos son del catálogo.
	var problem *catalog.Error
	if !errors.As(err, &problem) && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// Algún producto o parte del pedido ya no se puede cotizar: se revisa cada partida por separado para decir cuál
	// y por qué (y los precios de las que sí se pueden cotizar).
	for _, item := range input.Items {
		change, err := handler.diagnoseOfflineItem(item, diagnosis.priceListID)
		if err != nil {
			return nil, err
		}
		if change != nil {
			diagnosis.changes = append(diagnosis.changes, *change)
		}
	}
	blocking := slices.ContainsFunc(diagnosis.changes, func(change OfflineChange) bool {
		return !change.Acceptable
	})
	if !blocking {
		// Cada partida se cotiza sola pero juntas no (por ejemplo, el límite de unidades): no es un cambio del
		// catálogo sino un pedido que el servidor no acepta, ni con confirmación.
		return nil, err
	}
	return diagnosis, nil
}

// diagnoseOfflineTable revisa una mesa de destino: que siga en servicio y qué cuentas abiertas no veía el mesero.
func (handler *service) diagnoseOfflineTable(destination OfflineDestination) ([]OfflineChange, error) {
	if err := handler.request.Actor.Require("accounts.open"); err != nil {
		return nil, err
	}
	var name string
	var available bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT t.name,t.active AND NOT t.out_of_service AND a.active
        FROM app.restaurant_tables t JOIN app.restaurant_areas a ON a.id=t.area_id WHERE t.id=$1`, *destination.TableID).
		Scan(&name, &available)
	if errors.Is(err, pgx.ErrNoRows) {
		return []OfflineChange{{Kind: OfflineTableUnavailable, Message: "La mesa del pedido ya no existe. Elige otra mesa."}}, nil
	}
	if err != nil {
		return nil, err
	}
	if !available {
		return []OfflineChange{{
			Kind:    OfflineTableUnavailable,
			Message: "La mesa «" + name + "» está fuera de servicio o inactiva. Elige otra mesa.",
		}}, nil
	}
	unseen, err := readList[TableAccountRef](handler.ctx, handler.tx, `SELECT jsonb_build_object(
        'id',a.id,'folio',a.folio,'name',a.name) FROM app.restaurant_accounts a
        WHERE a.table_id=$1 AND a.state='open' AND NOT (a.id::text = ANY($2::text[]))
        ORDER BY a.created_at,a.id`, *destination.TableID, destination.SeenAccountIDs)
	if err != nil {
		return nil, err
	}
	if len(unseen) == 0 {
		return nil, nil
	}
	labels := []string{}
	for _, account := range unseen {
		labels = append(labels, fmt.Sprintf("folio %d", account.Folio))
	}
	now := strings.Join(labels, ", ")
	return []OfflineChange{{
		Kind:       OfflineTableHasAccount,
		Now:        &now,
		Message:    "La mesa «" + name + "» ya tiene una cuenta abierta en otra terminal (" + now + "). Confirma para abrir otra cuenta o envía el pedido a esa cuenta.",
		Acceptable: true,
	}}, nil
}

// diagnoseReplacedDraft compara los renglones que el pedido sustituye con el borrador vigente de la cuenta. Devuelve
// los que siguen exactamente como los vio el dispositivo (se quitarán al aceptar el pedido) y un cambio por cada uno
// que no: cambiado en otra terminal, quitado o ya enviado a cocina.
func (handler *service) diagnoseReplacedDraft(accountID string, replaced []DraftItem) ([]string, []OfflineChange, error) {
	matched := []string{}
	changes := []OfflineChange{}
	if len(replaced) == 0 {
		return matched, changes, nil
	}
	current, err := handler.readDraft(accountID)
	if err != nil {
		return nil, nil, err
	}
	saved := map[string]DraftItem{}
	for _, item := range current {
		saved[item.ID] = item
	}
	for _, seen := range replaced {
		now, exists := saved[seen.ID]
		if exists && now.Quantity == seen.Quantity && sameDraftLine(now, seen) {
			matched = append(matched, seen.ID)
			continue
		}
		change, err := handler.replacedDraftChange(accountID, seen, exists)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, *change)
	}
	return matched, changes, nil
}

// replacedDraftChange explica qué pasó con un renglón del borrador que ya no está como lo vio el dispositivo. Los tres
// casos se aceptan con la confirmación del mesero: su pedido entra como lo capturó y lo de la otra terminal no se toca.
func (handler *service) replacedDraftChange(accountID string, seen DraftItem, exists bool) (*OfflineChange, error) {
	itemID, productID := seen.ID, seen.ProductID
	name := "Un producto del borrador"
	err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_products WHERE id=$1", seen.ProductID).Scan(&name)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	change := &OfflineChange{ItemID: &itemID, ProductID: &productID, ProductName: name, Acceptable: true}
	if exists {
		change.Kind = OfflineDraftItemChanged
		change.Message = name + ": lo cambiaron en otra terminal. Si confirmas, tu pedido entra como lo capturaste y el " +
			"cambio de la otra terminal se queda sin enviar en la cuenta."
		return change, nil
	}
	// La partida confirmada conserva el id del renglón con que se capturó (restaurant_items.draft_item_id): si existe en
	// esta cuenta, el renglón ya se envió a cocina; si no, lo quitaron del borrador.
	var deviceID string
	err = handler.tx.QueryRow(handler.ctx, `SELECT o.device_id::text FROM app.restaurant_items i
        JOIN app.restaurant_orders o ON o.id=i.original_order_id
        WHERE i.draft_item_id=$1 AND o.original_account_id=$2 ORDER BY o.created_at DESC,o.id LIMIT 1`, seen.ID, accountID).
		Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		change.Kind = OfflineDraftItemRemoved
		change.Message = name + ": lo quitaron de la cuenta en otra terminal. Si confirmas, tu pedido entra como lo capturaste."
		return change, nil
	}
	if err != nil {
		return nil, err
	}
	change.Kind = OfflineDraftItemSent
	change.Message = name + ": ya se envió desde otra terminal. Si confirmas, tu pedido entra completo y puede repetirlo."
	if deviceID == handler.request.Actor.DeviceID {
		change.Message = name + ": ya se había enviado desde este dispositivo. Si confirmas, tu pedido entra completo y puede repetirlo."
	}
	return change, nil
}

// diagnoseOfflineItem cotiza una partida sola y traduce un rechazo del catálogo al cambio que lo explica.
func (handler *service) diagnoseOfflineItem(item OfflineItem, priceListID string) (*OfflineChange, error) {
	itemID, productID := item.ID, item.ProductID
	var name string
	err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_products WHERE id=$1", item.ProductID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return &OfflineChange{
			Kind:      OfflineProductRemoved,
			ItemID:    &itemID,
			ProductID: &productID,
			Before:    &item.SeenUnitPrice,
			Message:   "Un producto del pedido ya no existe en el catálogo. Quítalo del pedido.",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	quote, _, err := catalog.QuoteForOrder(handler.ctx, handler.tx, catalog.PreviewRequest{
		PriceListID: &priceListID,
		Lines:       []catalog.LineRequest{item.LineRequest},
	})
	if err == nil {
		return offlinePriceChange(item, quote.Lines[0]), nil
	}
	var problem *catalog.Error
	if !errors.As(err, &problem) {
		return nil, err
	}
	change := &OfflineChange{ItemID: &itemID, ProductID: &productID, ProductName: name, Before: &item.SeenUnitPrice}
	switch {
	case problem.Code == "PRODUCT_SOLD_OUT" && problem.ProductID == item.ProductID:
		change.Kind = OfflineProductSoldOut
		change.Message = name + ": agotado. Quítalo del pedido."
	case problem.Code == "PRODUCT_UNAVAILABLE" && problem.ProductID == item.ProductID:
		change.Kind = OfflineProductUnavailable
		change.Message = name + ": ya no está disponible. Quítalo del pedido."
	case problem.ProductID != "":
		change.Kind = OfflineComplementUnavailable
		change.Message = name + ": " + problem.Message + " Cambia la partida o quítala del pedido."
	case problem.Code == "PRODUCT_UNAVAILABLE":
		change.Kind = OfflineItemUnavailable
		change.Message = name + ": " + problem.Message + " Cambia la partida o quítala del pedido."
	default:
		change.Kind = OfflineItemInvalid
		change.Message = name + ": " + problem.Message + " Cambia la partida o quítala del pedido."
	}
	return change, nil
}

// offlinePriceChange compara el precio por unidad que vio el mesero con el del servidor (antes de promociones, que
// el servidor aplica al confirmar como en cualquier captura).
func offlinePriceChange(item OfflineItem, line catalog.QuoteLine) *OfflineChange {
	seen := decimal.RequireFromString(item.SeenUnitPrice)
	current := decimal.RequireFromString(line.UnitPrice)
	if seen.Equal(current) {
		return nil
	}
	itemID, productID, now := item.ID, item.ProductID, line.UnitPrice
	before := money(seen)
	return &OfflineChange{
		Kind:        OfflinePriceChanged,
		ItemID:      &itemID,
		ProductID:   &productID,
		ProductName: line.Name,
		Before:      &before,
		Now:         &now,
		Message:     line.Name + ": $" + before + " → $" + now,
		Acceptable:  true,
	}
}

// offlineConfirmed dice si la confirmación del mesero cubre todos los cambios vigentes con la versión vigente del
// catálogo. Un cambio que exige corregir el pedido nunca se confirma.
func offlineConfirmed(confirmation *OfflineConfirmation, version int64, changes []OfflineChange) bool {
	if confirmation == nil || confirmation.CatalogVersion != version {
		return false
	}
	for _, change := range changes {
		if !change.Acceptable {
			return false
		}
		covered := slices.ContainsFunc(confirmation.Changes, func(seen OfflineChange) bool {
			return seen.Kind == change.Kind && sameText(seen.ItemID, change.ItemID) &&
				sameText(seen.ProductID, change.ProductID) && sameText(seen.Now, change.Now)
		})
		if !covered {
			return false
		}
	}
	return true
}

func sameText(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// offlineRejection es el 409 con los cambios, que conserva su propia auditoría aunque la operación se revierta.
func offlineRejection(input OfflineOrderInput, version int64, received time.Time, changes []OfflineChange) error {
	blocking := slices.ContainsFunc(changes, func(change OfflineChange) bool {
		return !change.Acceptable
	})
	message := "El pedido capturado sin conexión tiene cambios de precio, de la mesa o de lo que se capturó en otra terminal. " +
		"Muéstralos al mesero y reenvíalo con su confirmación."
	if blocking {
		message = "El pedido capturado sin conexión tiene productos o un destino que ya no están disponibles. Corrige el pedido y vuelve a enviarlo."
	}
	details := map[string]any{
		"status":                   "requires_confirmation",
		"catalog_version":          version,
		"captured_catalog_version": input.CatalogVersion,
		"requires_changes":         blocking,
		"changes":                  changes,
	}
	operationID := input.OperationID
	return &Error{
		Status:  409,
		Code:    "ORDER_REQUIRES_CONFIRMATION",
		Message: message,
		Details: details,
		Rejection: &Rejection{
			Action:   "restaurant.offline_order.requires_confirmation",
			Entity:   "offline_order",
			EntityID: &operationID,
			Details: map[string]any{
				"captured_at":              input.CapturedAt,
				"received_at":              received,
				"captured_catalog_version": input.CatalogVersion,
				"catalog_version":          version,
				"destination":              input.Destination,
				"changes":                  changes,
			},
		},
	}
}

// acceptOffline confirma el pedido en su cuenta (abriéndola en la mesa si hace falta) con los precios del servidor.
func (handler *service) acceptOffline(input OfflineOrderInput, diagnosis *offlineDiagnosis, version int64, received time.Time) (any, error) {
	items := make([]DraftItem, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, item.DraftItem)
	}
	account := diagnosis.account
	if account == nil {
		// Mesa: se abre una cuenta nueva, como el primer producto de una mesa en la captura.
		accountInput := AccountInput{Mode: "dine_in", TableID: input.Destination.TableID, Name: input.Destination.Name}
		if err := handler.fillAccountName(&accountInput); err != nil {
			return nil, err
		}
		if _, err := handler.validateAccountInput(&accountInput, nil); err != nil {
			return nil, err
		}
		id := newID()
		if err := handler.validateDraftItems(id, items); err != nil {
			return nil, err
		}
		opened, err := handler.insertAccountWithID(id, accountInput, nil)
		if err != nil {
			return nil, err
		}
		account = opened
	} else if err := handler.validateDraftItems(account.ID, items); err != nil {
		return nil, err
	}
	if err := validateStorageAmount(diagnosis.quote); err != nil {
		return nil, err
	}
	if err := handler.touch(account.ID, account.Revision); err != nil {
		return nil, err
	}
	removed, err := handler.removeReplacedDraft(account.ID, diagnosis.replacedDraft)
	if err != nil {
		return nil, err
	}
	orderID, confirmed, err := handler.confirmLines(account.ID, items, diagnosis.quote, diagnosis.snapshot)
	if err != nil {
		return nil, err
	}
	accepted := diagnosis.changes
	confirmedVersion := any(nil)
	if input.Confirmation != nil {
		confirmedVersion = input.Confirmation.CatalogVersion
	}
	offline := map[string]any{
		"captured_at":                  input.CapturedAt,
		"received_at":                  received,
		"captured_catalog_version":     input.CatalogVersion,
		"catalog_version":              version,
		"confirmation_catalog_version": confirmedVersion,
		"accepted_changes":             accepted,
		"removed_draft_item_ids":       removed,
	}
	if err := handler.event(account.ID, nil, "order.confirmed", nil, map[string]any{
		"order_id": orderID, "items": confirmed, "quote": diagnosis.quote, "preparation_state": "confirmed_pending_dispatch",
		"offline": offline,
	}); err != nil {
		return nil, err
	}
	operationID := input.OperationID
	audited := map[string]any{"account_id": account.ID, "order_id": orderID}
	for key, value := range offline {
		audited[key] = value
	}
	if err := handler.audit(handler.ctx, handler.tx, "restaurant.offline_order.accepted", "offline_order", &operationID, nil, audited); err != nil {
		return nil, err
	}
	bill, err := handler.readBill(account.ID)
	if err != nil {
		return nil, err
	}
	return &OfflineOrderResult{
		Status:                 "accepted",
		OperationID:            input.OperationID,
		OrderID:                orderID,
		AccountID:              account.ID,
		CatalogVersion:         version,
		CapturedCatalogVersion: input.CatalogVersion,
		CapturedAt:             input.CapturedAt.UTC(),
		ReceivedAt:             received,
		AcceptedChanges:        accepted,
		RemovedDraftItemIDs:    removed,
		Bill:                   bill,
	}, nil
}

// removeReplacedDraft quita del borrador los renglones que el pedido sustituye y que siguen como los vio el
// dispositivo, y lo publica como un guardado del borrador (historia de la cuenta, evento restaurant.draft.saved y
// Registro de movimientos), en la misma transacción del pedido. Los demás renglones conservan su lugar.
func (handler *service) removeReplacedDraft(accountID string, replaced []string) ([]string, error) {
	removed := []string{}
	if len(replaced) == 0 {
		return removed, nil
	}
	before, err := handler.readDraft(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `DELETE FROM app.restaurant_draft_items WHERE account_id=$1 AND id=ANY($2::uuid[])
        RETURNING id::text`, accountID, replaced)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		removed = append(removed, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Sort(removed)
	after, err := handler.readDraft(accountID)
	if err != nil {
		return nil, err
	}
	return removed, handler.event(accountID, nil, "draft.saved", before, after)
}
