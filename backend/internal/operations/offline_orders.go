package operations

import (
	"encoding/json"
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
//     se aceptan con la confirmación del mesero, como un cambio de precio: el pedido entra como lo capturó (menos lo
//     ya enviado, A1-D18) y el renglón de la otra terminal se queda como está; si no, el mesero corrige su pedido y lo
//     reenvía.
//   - Idempotencia, límites y credencial de envío: los mismos del pedido (la clave guarda el resultado completo).
//
// El servidor nunca repite lo que ya entró (etapa A1, decisión A1-D18, después de la revisión independiente R6):
//
//   - Los renglones del borrador con el mismo id que una partida del pedido son el mismo renglón (el id lo genera el
//     dispositivo al capturar): se tratan como sustituidos aunque no vengan en replaces_draft_items. Si siguen como
//     los vio el dispositivo (o como los trae el pedido) o los guardó por última vez el mismo mesero desde el mismo
//     dispositivo, se quitan del borrador al aceptar; si otra terminal los cambió, draft_item_changed.
//   - Una partida del pedido que ya se envió a cocina (alguna partida confirmada conserva su id en
//     restaurant_items.draft_item_id) es draft_item_sent: confirmarla significa «enviar lo demás». Lo ya enviado nunca
//     se vuelve a enviar; si no queda nada, el pedido se acepta sin partidas nuevas (already_sent_item_ids, y order_id
//     es el pedido en que ya habían entrado).
//   - Un pedido con la misma clave de operación que un «Enviar a cocina» (confirmDraft) ya aplicado en esa cuenta por
//     el mismo mesero desde el mismo dispositivo es el mismo pedido: OFFLINE_ORDER_ALREADY_ACCEPTED con el resultado
//     (handler.go, replay). Con otra cuenta, otro usuario, otro dispositivo u otra operación, IDEMPOTENCY_CONFLICT.
//   - Un pedido para una mesa cuyas partidas ya están en una cuenta abierta de esa mesa (en su borrador o enviadas)
//     responde items_in_account con esa cuenta (account_id) para que el dispositivo lo dirija ahí.
//
// Pedido para llevar nuevo capturado sin conexión (etapa A2, §3.2 de la especificación): el destino puede ser, en
// lugar de una cuenta o una mesa, destination.takeaway (con el nombre del cliente y el teléfono, opcionales). Al
// aceptarlo se abre un pedido para llevar como «Nuevo pedido» de la caja (sin mesa ficticia, origen «Para llevar»,
// responsable = quien envía: el mesero de la sesión o de la credencial de envío) y exige además accounts.open y
// takeaway.manage. Las reglas de la clave son las mismas de A1-D6, A1-D18 y A1-D19: la misma clave nunca abre dos
// pedidos (el resultado guardado se devuelve en cada reintento) y, si sus partidas ya están en un pedido para llevar
// abierto (la apertura en línea entró y su respuesta se perdió), responde items_in_account con ese pedido, igual que en
// una mesa, para que el dispositivo lo reenvíe con la misma clave a destination.account_id.

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
	// Etapa A1 (A1-D18): las partidas del pedido a una mesa ya están en una cuenta abierta de esa mesa (account_id).
	OfflineItemsInAccount = "items_in_account"
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

// OfflineDestination es una cuenta abierta (account_id), una mesa donde abrir una cuenta nueva (table_id, con las
// cuentas que el mesero veía abiertas en ella y un nombre opcional) o, desde la etapa A2, un pedido para llevar nuevo
// (takeaway). Solo uno de los tres.
type OfflineDestination struct {
	AccountID      *string          `json:"account_id"`
	TableID        *string          `json:"table_id"`
	SeenAccountIDs []string         `json:"seen_account_ids"`
	Name           string           `json:"name"`
	Takeaway       *OfflineTakeaway `json:"takeaway"`
}

// OfflineTakeaway (etapa A2) son los datos opcionales del cliente de un pedido para llevar nuevo, con los mismos límites
// que al abrir la cuenta (validCustomerFields).
type OfflineTakeaway struct {
	CustomerName string `json:"customer_name"`
	Phone        string `json:"phone"`
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
	// AccountID (etapa A1, solo en items_in_account) es la cuenta abierta de la mesa donde ya están las partidas.
	AccountID *string `json:"account_id,omitempty"`
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
	// RemovedDraftItemIDs (etapa A1) son los renglones del borrador que el pedido sustituyó y se quitaron.
	RemovedDraftItemIDs []string `json:"removed_draft_item_ids"`
	// AlreadySentItemIDs (etapa A1, A1-D18) son las partidas del pedido que ya se habían enviado a cocina y no se
	// volvieron a enviar. Si son todas, no se creó un pedido nuevo y OrderID es el pedido en que ya habían entrado.
	AlreadySentItemIDs []string `json:"already_sent_item_ids"`
	Bill               *Bill    `json:"bill"`
}

// offlineDiagnosis es lo que encontró la revisión del pedido.
type offlineDiagnosis struct {
	changes     []OfflineChange
	quote       *catalog.Quote
	snapshot    catalog.Snapshot
	account     *Account
	priceListID string
	// replacedDraft son los renglones del borrador que el pedido sustituye y se quitan al aceptarlo.
	replacedDraft []string
	// sent son las partidas del pedido que ya se enviaron a cocina (no se vuelven a enviar) y sentOrderID el último
	// pedido en que entraron.
	sent        map[string]bool
	sentOrderID string
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
	chosen := 0
	for _, given := range []bool{destination.AccountID != nil, destination.TableID != nil, destination.Takeaway != nil} {
		if given {
			chosen++
		}
	}
	if chosen != 1 {
		return invalid("Indica el destino del pedido: una cuenta abierta, una mesa o un pedido para llevar nuevo.")
	}
	if destination.TableID == nil && (len(destination.SeenAccountIDs) > 0 || destination.Name != "") {
		return invalid("Las cuentas vistas y el nombre solo aplican a una cuenta nueva en una mesa.")
	}
	if takeaway := destination.Takeaway; takeaway != nil {
		takeaway.CustomerName = strings.TrimSpace(takeaway.CustomerName)
		takeaway.Phone = strings.TrimSpace(takeaway.Phone)
		if !validCustomerFields(takeaway.CustomerName, takeaway.Phone) {
			return invalid(customerFieldsMessage)
		}
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
				overlap, err := handler.diagnoseDraftOverlap(account.ID, input)
				if err != nil {
					return nil, err
				}
				diagnosis.replacedDraft = overlap.remove
				diagnosis.sent, diagnosis.sentOrderID = overlap.sent, overlap.sentOrderID
				diagnosis.changes = append(diagnosis.changes, overlap.changes...)
			}
		}
	} else if destination.TableID != nil {
		changes, err := handler.diagnoseOfflineTable(input)
		if err != nil {
			return nil, err
		}
		diagnosis.changes = append(diagnosis.changes, changes...)
	} else {
		changes, err := handler.diagnoseOfflineTakeaway(input)
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
	// Lo que ya se envió a cocina no se vuelve a cotizar ni a enviar (A1-D18): solo se revisa y se cotiza lo demás.
	pending := pendingOfflineItems(input.Items, diagnosis.sent)
	if len(pending) == 0 {
		return diagnosis, nil
	}
	lines := make([]catalog.LineRequest, 0, len(pending))
	for _, item := range pending {
		lines = append(lines, item.LineRequest)
	}
	quote, snapshot, err := catalog.QuoteForOrder(handler.ctx, handler.tx, catalog.PreviewRequest{
		PriceListID: &diagnosis.priceListID,
		Lines:       lines,
	})
	if err == nil {
		diagnosis.quote, diagnosis.snapshot = quote, snapshot
		for index, item := range pending {
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
	for _, item := range pending {
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

// pendingOfflineItems son las partidas del pedido que todavía no se enviaron a cocina, en su orden.
func pendingOfflineItems(items []OfflineItem, sent map[string]bool) []OfflineItem {
	pending := make([]OfflineItem, 0, len(items))
	for _, item := range items {
		if !sent[item.ID] {
			pending = append(pending, item)
		}
	}
	return pending
}

// diagnoseOfflineTable revisa una mesa de destino: que siga en servicio, que sus partidas no estén ya en una cuenta
// abierta de la mesa y qué cuentas abiertas no veía el mesero.
func (handler *service) diagnoseOfflineTable(input OfflineOrderInput) ([]OfflineChange, error) {
	destination := input.Destination
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
	holding, err := handler.openAccountsWithItems(destination.TableID, input.Items)
	if err != nil {
		return nil, err
	}
	if len(holding) > 0 {
		// Etapa A1 (A1-D18): por ejemplo, la cuenta nació en Go con estas partidas (la apertura entró y su respuesta se
		// perdió). Abrir otra cuenta las duplicaría; el dispositivo reenvía el pedido a esa cuenta (account_id).
		changes := []OfflineChange{}
		for _, account := range holding {
			accountID := account.ID
			folio := fmt.Sprintf("folio %d", account.Folio)
			changes = append(changes, OfflineChange{
				Kind:      OfflineItemsInAccount,
				Now:       &folio,
				AccountID: &accountID,
				Message: "Lo que capturaste ya está en la cuenta «" + account.Name + "» (" + folio + ") de la mesa «" + name +
					"». Envía el pedido a esa cuenta.",
			})
		}
		return changes, nil
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

// diagnoseOfflineTakeaway revisa un pedido para llevar nuevo (etapa A2): quien lo envía debe poder abrir cuentas y
// administrar pedidos para llevar, y sus partidas no deben estar ya en un pedido para llevar abierto (A1-D18, como en
// una mesa: por ejemplo, la apertura en línea con el primer producto entró y su respuesta se perdió; abrir otro pedido
// las duplicaría). El dispositivo reenvía el pedido con la misma clave a esa cuenta (account_id).
func (handler *service) diagnoseOfflineTakeaway(input OfflineOrderInput) ([]OfflineChange, error) {
	if err := handler.request.Actor.Require("accounts.open", "takeaway.manage"); err != nil {
		return nil, err
	}
	holding, err := handler.openAccountsWithItems(nil, input.Items)
	if err != nil {
		return nil, err
	}
	changes := []OfflineChange{}
	for _, account := range holding {
		accountID := account.ID
		folio := fmt.Sprintf("folio %d", account.Folio)
		order := "el pedido para llevar"
		if account.CustomerName != "" {
			order += " de " + account.CustomerName
		}
		changes = append(changes, OfflineChange{
			Kind:      OfflineItemsInAccount,
			Now:       &folio,
			AccountID: &accountID,
			Message:   "Lo que capturaste ya está en " + order + " (" + folio + "). Envía el pedido a esa cuenta.",
		})
	}
	return changes, nil
}

// heldAccount es una cuenta abierta que ya tiene partidas de un pedido sin conexión (items_in_account).
type heldAccount struct {
	ID           string `json:"id"`
	Folio        int64  `json:"folio"`
	Name         string `json:"name"`
	CustomerName string `json:"customer_name"`
}

// openAccountsWithItems son las cuentas abiertas de la mesa (con tableID) o los pedidos para llevar abiertos (sin
// tableID, etapa A2) que ya tienen alguna partida del pedido: en su borrador o ya enviada (una partida confirmada con
// ese id cuyas unidades siguen en la cuenta o que se confirmó en ella).
func (handler *service) openAccountsWithItems(tableID *string, items []OfflineItem) ([]heldAccount, error) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return readList[heldAccount](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',a.id,'folio',a.folio,'name',a.name,
        'customer_name',a.customer_name)
        FROM app.restaurant_accounts a WHERE a.state='open'
            AND (($1::uuid IS NOT NULL AND a.table_id=$1) OR ($1::uuid IS NULL AND a.mode='takeaway')) AND (
            EXISTS(SELECT FROM app.restaurant_draft_items d WHERE d.account_id=a.id AND d.id=ANY($2::uuid[]))
            OR EXISTS(SELECT FROM app.restaurant_items i JOIN app.restaurant_orders o ON o.id=i.original_order_id
                WHERE i.draft_item_id=ANY($2::uuid[]) AND (o.original_account_id=a.id
                    OR EXISTS(SELECT FROM app.restaurant_item_units u WHERE u.item_id=i.id AND u.account_id=a.id))))
        ORDER BY a.created_at,a.id`, tableID, ids)
}

// draftRow es un renglón del borrador vigente con el usuario y el dispositivo que guardaron el borrador por última vez
// (cada guardado reescribe todas las filas con los suyos, operations/drafts.go).
type draftRow struct {
	item     DraftItem
	authorID string
	deviceID string
}

func (handler *service) readDraftRows(accountID string) (map[string]draftRow, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT jsonb_build_object('id',id,'product_id',product_id,
        'variant_id',variant_id,'quantity',quantity,'customization',customization),author_id::text,device_id::text
        FROM app.restaurant_draft_items WHERE account_id=$1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]draftRow{}
	for rows.Next() {
		var encoded []byte
		var row draftRow
		if err := rows.Scan(&encoded, &row.authorID, &row.deviceID); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(encoded, &row.item); err != nil {
			return nil, err
		}
		result[row.item.ID] = row
	}
	return result, rows.Err()
}

// draftOverlap es lo que el pedido tiene en común con el borrador y con lo ya enviado de su cuenta.
type draftOverlap struct {
	// remove son los renglones del borrador que el pedido sustituye y se quitan al aceptarlo.
	remove []string
	// sent son las partidas del pedido (o renglones que sustituye) que ya se enviaron a cocina.
	sent        map[string]bool
	sentOrderID string
	changes     []OfflineChange
}

// diagnoseDraftOverlap compara el pedido con el borrador vigente de la cuenta y con lo ya enviado (A1-D6 y A1-D18).
// Revisa primero los renglones que el pedido nombra en replaces_draft_items (tal como los vio el dispositivo) y
// después cada partida del pedido que no nombra (el mismo id es el mismo renglón):
//
//   - sigue en el borrador igual a como lo vio el dispositivo, o lo guardó por última vez el mismo mesero desde el
//     mismo dispositivo (entonces lo que hay es suyo, aunque lo cambiara después sin conexión): se quita al aceptar;
//   - sigue en el borrador con otro contenido que guardó otra terminal: draft_item_changed (se queda);
//   - ya se envió a cocina: draft_item_sent, y esa partida no se vuelve a enviar;
//   - un renglón nombrado que ya no está y nunca se envió: draft_item_removed. Una partida nueva no es un cambio.
func (handler *service) diagnoseDraftOverlap(accountID string, input OfflineOrderInput) (*draftOverlap, error) {
	overlap := &draftOverlap{remove: []string{}, sent: map[string]bool{}, changes: []OfflineChange{}}
	current, err := handler.readDraftRows(accountID)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		seen  DraftItem
		named bool
	}
	candidates := []candidate{}
	named := map[string]bool{}
	for _, seen := range input.ReplacesDraftItems {
		candidates = append(candidates, candidate{seen: seen, named: true})
		named[seen.ID] = true
	}
	for _, item := range input.Items {
		if !named[item.ID] {
			candidates = append(candidates, candidate{seen: item.DraftItem})
		}
	}
	for _, candidate := range candidates {
		seen := candidate.seen
		row, exists := current[seen.ID]
		if exists {
			unchanged := row.item.Quantity == seen.Quantity && sameDraftLine(row.item, seen)
			own := row.authorID == handler.request.Actor.UserID && row.deviceID == handler.request.Actor.DeviceID
			if unchanged || own {
				overlap.remove = append(overlap.remove, seen.ID)
				continue
			}
			change, err := handler.draftChange(OfflineDraftItemChanged, seen, "")
			if err != nil {
				return nil, err
			}
			overlap.changes = append(overlap.changes, *change)
			continue
		}
		orderID, deviceID, found, err := handler.sentDraftItem(seen.ID)
		if err != nil {
			return nil, err
		}
		if found {
			overlap.sent[seen.ID] = true
			if overlap.sentOrderID == "" {
				overlap.sentOrderID = orderID
			}
			change, err := handler.draftChange(OfflineDraftItemSent, seen, deviceID)
			if err != nil {
				return nil, err
			}
			overlap.changes = append(overlap.changes, *change)
			continue
		}
		if candidate.named {
			change, err := handler.draftChange(OfflineDraftItemRemoved, seen, "")
			if err != nil {
				return nil, err
			}
			overlap.changes = append(overlap.changes, *change)
		}
	}
	slices.Sort(overlap.remove)
	return overlap, nil
}

// sentDraftItem busca la partida confirmada más reciente que conserva el id del renglón (restaurant_items.draft_item_id,
// en cualquier cuenta: el id lo generó el dispositivo para ese renglón) y devuelve su pedido y el dispositivo que la
// envió.
func (handler *service) sentDraftItem(id string) (string, string, bool, error) {
	var orderID, deviceID string
	err := handler.tx.QueryRow(handler.ctx, `SELECT o.id::text,o.device_id::text FROM app.restaurant_items i
        JOIN app.restaurant_orders o ON o.id=i.original_order_id WHERE i.draft_item_id=$1
        ORDER BY o.created_at DESC,o.id LIMIT 1`, id).Scan(&orderID, &deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return orderID, deviceID, true, nil
}

// draftChange explica qué pasó con un renglón del borrador o una partida del pedido. Los tres casos se aceptan con la
// confirmación del mesero: su pedido entra como lo capturó, lo de la otra terminal no se toca y lo ya enviado no se
// vuelve a enviar. sentFrom es el dispositivo que envió el renglón (solo en draft_item_sent).
func (handler *service) draftChange(kind string, seen DraftItem, sentFrom string) (*OfflineChange, error) {
	itemID, productID := seen.ID, seen.ProductID
	name := "Un producto del borrador"
	err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.catalog_products WHERE id=$1", seen.ProductID).Scan(&name)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	change := &OfflineChange{Kind: kind, ItemID: &itemID, ProductID: &productID, ProductName: name, Acceptable: true}
	switch kind {
	case OfflineDraftItemChanged:
		change.Message = name + ": lo cambiaron en otra terminal. Si confirmas, tu pedido entra como lo capturaste y el " +
			"cambio de la otra terminal se queda sin enviar en la cuenta."
	case OfflineDraftItemRemoved:
		change.Message = name + ": lo quitaron de la cuenta en otra terminal. Si confirmas, tu pedido entra como lo capturaste."
	case OfflineDraftItemSent:
		change.Message = name + ": ya se envió a cocina desde otra terminal y no se vuelve a enviar. Si confirmas, se " +
			"envía lo demás del pedido."
		if sentFrom == handler.request.Actor.DeviceID {
			change.Message = name + ": ya se había enviado a cocina desde este dispositivo y no se vuelve a enviar. Si " +
				"confirmas, se envía lo demás del pedido."
		}
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
// Desde la etapa A1 (A1-D18) solo confirma las partidas que todavía no se enviaron a cocina; si no queda ninguna, el
// pedido se acepta sin pedido nuevo (order_id es el pedido en que ya habían entrado).
func (handler *service) acceptOffline(input OfflineOrderInput, diagnosis *offlineDiagnosis, version int64, received time.Time) (any, error) {
	items := []DraftItem{}
	alreadySent := []string{}
	for _, item := range input.Items {
		if diagnosis.sent[item.ID] {
			alreadySent = append(alreadySent, item.ID)
			continue
		}
		items = append(items, item.DraftItem)
	}
	account := diagnosis.account
	if account == nil {
		accountInput := offlineAccountInput(input.Destination)
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
	if diagnosis.quote != nil {
		if err := validateStorageAmount(diagnosis.quote); err != nil {
			return nil, err
		}
	}
	// La revisión de la cuenta sube solo si el pedido cambia algo (partidas nuevas o renglones quitados del borrador).
	if len(items) > 0 || len(diagnosis.replacedDraft) > 0 {
		if err := handler.touch(account.ID, account.Revision); err != nil {
			return nil, err
		}
	}
	removed, err := handler.removeReplacedDraft(account.ID, diagnosis.replacedDraft)
	if err != nil {
		return nil, err
	}
	orderID := diagnosis.sentOrderID
	confirmed := []map[string]any{}
	if len(items) > 0 {
		orderID, confirmed, err = handler.confirmLines(account.ID, items, diagnosis.quote, diagnosis.snapshot)
		if err != nil {
			return nil, err
		}
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
		"already_sent_item_ids":        alreadySent,
	}
	if len(items) > 0 {
		if err := handler.event(account.ID, nil, "order.confirmed", nil, map[string]any{
			"order_id": orderID, "items": confirmed, "quote": diagnosis.quote, "preparation_state": "confirmed_pending_dispatch",
			"offline": offline,
		}); err != nil {
			return nil, err
		}
	}
	operationID := input.OperationID
	audited := map[string]any{"account_id": account.ID, "order_id": orderID, "new_order": len(items) > 0}
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
		AlreadySentItemIDs:     alreadySent,
		Bill:                   bill,
	}, nil
}

// offlineAccountInput es la cuenta nueva que abre un pedido sin cuenta de destino. En una mesa, como el primer producto
// de una mesa en la captura. Para llevar (etapa A2), como «Nuevo pedido» de la caja: sin mesa, con el origen
// «Para llevar» y el nombre y el teléfono del cliente que capturó el mesero. En las dos el nombre es la referencia
// neutra (fillAccountName) y el responsable, quien envía el pedido (validateAccountInput).
func offlineAccountInput(destination OfflineDestination) AccountInput {
	if takeaway := destination.Takeaway; takeaway != nil {
		return AccountInput{Mode: "takeaway", Origin: TakeawayOrigin, CustomerName: takeaway.CustomerName, Phone: takeaway.Phone}
	}
	return AccountInput{Mode: "dine_in", TableID: destination.TableID, Name: destination.Name}
}

// confirmedAccountOf reconoce la ruta guardada de un «Enviar a cocina» (POST restaurant/accounts/{id}/confirm) y
// devuelve su cuenta.
func confirmedAccountOf(route string) (string, bool) {
	rest, found := strings.CutPrefix(route, "POST restaurant/accounts/")
	if !found {
		return "", false
	}
	accountID, found := strings.CutSuffix(rest, "/confirm")
	if !found || validateID(accountID) != nil {
		return "", false
	}
	return accountID, true
}

// offlineAlreadyConfirmed responde a un pedido de la cola cuya clave ya entró como «Enviar a cocina» (confirmDraft) en
// la misma cuenta (A1-D18): OFFLINE_ORDER_ALREADY_ACCEPTED con el resultado en la forma de un pedido aceptado
// (el pedido que entró, la cuenta vigente y las partidas del pedido que ya estaban en él), sin escribir nada. Devuelve
// nil si el pedido va a otra cuenta o a una mesa (entonces la clave es de otra operación: IDEMPOTENCY_CONFLICT).
func (handler *service) offlineAlreadyConfirmed(accountID string) (*Error, error) {
	var input OfflineOrderInput
	if err := json.Unmarshal(handler.request.Body, &input); err != nil {
		return nil, nil
	}
	if input.Destination.AccountID == nil || *input.Destination.AccountID != accountID {
		return nil, nil
	}
	var orderID string
	var received time.Time
	err := handler.tx.QueryRow(handler.ctx, `SELECT id::text,created_at FROM app.restaurant_orders
        WHERE operation_id=$1 AND original_account_id=$2`, handler.operationID, accountID).Scan(&orderID, &received)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	account, err := handler.readAccount(accountID)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, false); err != nil {
		return nil, err
	}
	version, err := catalog.Version(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(input.Items))
	for _, item := range input.Items {
		ids = append(ids, item.ID)
	}
	alreadySent := []string{}
	rows, err := handler.tx.Query(handler.ctx, `SELECT DISTINCT draft_item_id::text FROM app.restaurant_items
        WHERE original_order_id=$1 AND draft_item_id=ANY($2::uuid[]) ORDER BY 1`, orderID, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		alreadySent = append(alreadySent, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	bill, err := handler.readBill(accountID)
	if err != nil {
		return nil, err
	}
	result := &OfflineOrderResult{
		Status:                 "accepted",
		OperationID:            handler.operationID,
		OrderID:                orderID,
		AccountID:              accountID,
		CatalogVersion:         version,
		CapturedCatalogVersion: input.CatalogVersion,
		CapturedAt:             input.CapturedAt.UTC(),
		ReceivedAt:             received.UTC(),
		AcceptedChanges:        []OfflineChange{},
		RemovedDraftItemIDs:    []string{},
		AlreadySentItemIDs:     alreadySent,
		Bill:                   bill,
	}
	return &Error{
		Status:  409,
		Code:    "OFFLINE_ORDER_ALREADY_ACCEPTED",
		Message: "Este pedido ya había entrado al enviarlo a cocina y no se volvió a registrar.",
		Details: map[string]any{"status": "already_accepted", "origin": "confirm_draft", "result": result},
	}, nil
}

// removeReplacedDraft quita del borrador los renglones que el pedido sustituye (diagnoseDraftOverlap), y lo publica como un guardado del borrador (historia de la cuenta, evento restaurant.draft.saved y
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
