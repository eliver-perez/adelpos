package operations

import (
	"fmt"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
	"strconv"
	"strings"
	"time"
)

const accountQuery = `SELECT to_jsonb(a) || jsonb_build_object(
    'table_name',t.name,'assigned_name',person.display_name,
    'confirmed_quantity',u.quantity,'draft_quantity',d.quantity,
    'totals',jsonb_build_object('subtotal',u.subtotal::text,'discount',u.discount::text,'total',u.total::text))
    FROM app.restaurant_accounts a
    JOIN app.users person ON person.id=a.assigned_user_id
    LEFT JOIN app.restaurant_tables t ON t.id=a.table_id
    CROSS JOIN LATERAL (SELECT count(*)::integer AS quantity,
        coalesce(sum(subtotal),0::numeric(18,2)) AS subtotal,
        coalesce(sum(discount),0::numeric(18,2)) AS discount,
        coalesce(sum(total),0::numeric(18,2)) AS total
        FROM app.restaurant_item_units WHERE account_id=a.id AND state='active') u
    CROSS JOIN LATERAL (SELECT coalesce(sum(quantity),0)::integer AS quantity
        FROM app.restaurant_draft_items WHERE account_id=a.id) d`

func (handler *service) readAccount(id string) (*Account, error) {
	account, err := readOne[Account](handler.ctx, handler.tx, accountQuery+" WHERE a.id=$1", id)
	if err != nil {
		return nil, err
	}
	if err := handler.decorateAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

func (handler *service) decorateAccount(account *Account) error {
	location, err := handler.location()
	if err != nil {
		return err
	}
	settings, err := handler.readSettings()
	if err != nil {
		return err
	}
	account.ElapsedSeconds = max(0, int64(time.Since(account.CreatedAt)/time.Second))
	account.Urgency = "normal"
	if account.ElapsedSeconds >= int64(settings.OverdueMinutes*60) {
		account.Urgency = "overdue"
	} else if account.ElapsedSeconds >= int64(settings.WarningMinutes*60) {
		account.Urgency = "warning"
	}
	// Pago y entrega se derivan de los registros financieros reales; la preparación sigue siendo independiente.
	brief, err := payments.AccountBrief(handler.ctx, handler.tx, account.ID)
	if err != nil {
		return err
	}
	account.Finance = brief
	account.PaymentState = brief.PaymentState
	account.DeliveryState = brief.DeliveryState
	account.PreparationState = "without_items"
	if account.DraftQuantity > 0 {
		account.PreparationState = "draft"
	}
	state, err := kitchen.AccountPreparation(handler.ctx, handler.tx, account.ID)
	if err != nil {
		return err
	}
	if state != "" && !(state == "cancelled" && account.DraftQuantity > 0) {
		account.PreparationState = state
	}

	if account.ScheduledAt != nil {
		account.ScheduledLocal = account.ScheduledAt.In(location).Format("2006-01-02T15:04")
	}
	// Etapa 7.1: Go decide si la cuenta se puede liberar sin motivo («Liberar mesa»); desde la 9D
	// también si se puede descartar con su borrador («Descartar»).
	account.Discardable, err = handler.discardable(account)
	if err != nil {
		return err
	}
	account.Releasable = account.Discardable && account.DraftQuantity == 0
	// Etapa 9D (§3.4): cuenta abierta por más horas que el umbral del establecimiento.
	account.LongOpen = account.State == "open" && account.ElapsedSeconds > int64(settings.LongOpenHours)*3600
	// Etapa 9D (§5.1): Go dice si «Cancelar cuenta» procede o qué hay que hacer primero, y si pedirá
	// la autorización de un supervisor. Solo para cuentas abiertas que no se descartan: una cuenta
	// descartable no tiene registros financieros (cancel_blocker vacío) y no se cancela (la ruta
	// responde ACCOUNT_DISCARDABLE sin pedir supervisor), así que tampoco pide autorización; la
	// interfaz ofrece «Descartar» por discardable.
	if account.State == "open" && !account.Discardable {
		account.CancelBlocker, err = payments.CancellationBlockerWithBrief(handler.ctx, handler.tx, account.ID, brief)
		if err != nil {
			return err
		}
		account.CancelRequiresAuthorization = !handler.request.Actor.Can(cancelPermission)
	}
	// Etapa 9F (§7.2): «Cancelar unidades» pide supervisor a quien no tiene orders.cancel.
	account.CancelUnitsRequiresAuthorization = account.State == "open" && !handler.request.Actor.Can(unitsCancelPermission)
	return nil
}

func (handler *service) listAccounts() (any, error) {
	query := handler.request.Query
	mode, tableID, search, state := query.Get("mode"), query.Get("table_id"), strings.TrimSpace(query.Get("search")), query.Get("state")
	if mode != "" && mode != "dine_in" && mode != "takeaway" {
		return nil, invalid("La modalidad de cuenta no es válida.")
	}
	if mode == "takeaway" {
		if err := handler.request.Actor.Require("takeaway.read"); err != nil {
			return nil, err
		}
	}
	if tableID != "" && tableID != "unassigned" {
		if err := validateID(tableID); err != nil {
			return nil, err
		}
	}
	if state == "" {
		state = "open"
	}
	if state != "open" && state != "closed" && state != "merged" && state != "voided" && state != "all" {
		return nil, invalid("El estado de cuenta no es válido.")
	}
	if len(search) > 100 {
		return nil, invalid("La búsqueda admite hasta 100 caracteres.")
	}
	page, size := 1, 50
	for key, destination := range map[string]*int{"page": &page, "page_size": &size} {
		if query.Get(key) != "" {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil || value < 1 {
				return nil, invalid("La página y su tamaño deben ser enteros positivos.")
			}
			*destination = value
		}
	}
	if page > 10000 || size > 100 {
		return nil, invalid("Se admiten páginas hasta 10000, con hasta 100 cuentas por página.")
	}
	filter := ` WHERE ($1='' OR a.mode=$1) AND ($2='all' OR a.state=$2)
        AND ($3='' OR a.table_id::text=$3 OR ($3='unassigned' AND a.table_id IS NULL))
        AND ($4='' OR a.folio::text ILIKE $5 OR a.customer_name ILIKE $5 OR a.phone ILIKE $5 OR a.name ILIKE $5)
        AND (a.mode<>'takeaway' OR $6)`
	arguments := []any{mode, state, tableID, search, "%" + search + "%", handler.request.Actor.Can("takeaway.read")}
	var total int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.restaurant_accounts a"+filter, arguments...).Scan(&total); err != nil {
		return nil, err
	}
	arguments = append(arguments, size, (page-1)*size)
	accounts, err := readList[Account](handler.ctx, handler.tx, accountQuery+filter+" ORDER BY a.created_at,a.id LIMIT $7 OFFSET $8", arguments...)
	if err != nil {
		return nil, err
	}
	for index := range accounts {
		if err := handler.decorateAccount(&accounts[index]); err != nil {
			return nil, err
		}
	}
	location, err := handler.location()
	if err != nil {
		return nil, err
	}
	settings, err := handler.readSettings()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"items":       accounts,
		"total":       total,
		"page":        page,
		"page_size":   size,
		"server_time": time.Now().UTC(),
		"timezone":    location.String(),
		"settings":    settings,
	}, nil
}

func (handler *service) readDraft(id string) ([]DraftItem, error) {
	return readList[DraftItem](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',id,'product_id',product_id,
        'variant_id',variant_id,'quantity',quantity,'customization',customization)
        FROM app.restaurant_draft_items WHERE account_id=$1 ORDER BY position,id`, id)
}

func (handler *service) readBill(id string) (*Bill, error) {
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	draft, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	lines, err := readList[ConfirmedLine](handler.ctx, handler.tx, `SELECT jsonb_build_object(
        'version_id',v.id,'item_id',v.item_id,'original_order_id',i.original_order_id,
        'version',v.version,'state',u.state,'quantity',count(*)::integer,'request',v.request,
        'quote_line',v.quote_line,'created_at',v.created_at,
        'totals',jsonb_build_object('subtotal',sum(u.subtotal)::text,'discount',sum(u.discount)::text,'total',sum(u.total)::text))
        FROM app.restaurant_item_units u
        JOIN app.restaurant_item_versions v ON v.id=u.version_id
        JOIN app.restaurant_items i ON i.id=u.item_id WHERE u.account_id=$1
        GROUP BY v.id,i.original_order_id,u.state ORDER BY v.created_at,v.id,u.state`, id)
	if err != nil {
		return nil, err
	}
	location, err := handler.location()
	if err != nil {
		return nil, err
	}
	for index := range lines {
		if lines[index].State == "cancelled" {
			lines[index].PreparationState = "cancelled"
			continue
		}
		state, err := kitchen.VersionPreparation(handler.ctx, handler.tx, id, lines[index].VersionID)
		if err != nil {
			return nil, err
		}
		lines[index].PreparationState = state
	}
	bill := &Bill{Account: *account, Draft: draft, Lines: lines, ServerTime: time.Now().UTC(), Timezone: location.String()}
	if account.State == "voided" {
		bill.Cancellation, err = handler.readCancellation(id)
		if err != nil {
			return nil, err
		}
	}
	return bill, nil
}

func (handler *service) history(id string) (any, error) {
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, false); err != nil {
		return nil, err
	}
	page := 1
	if value := handler.request.Query.Get("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 10000 {
			return nil, invalid("Página de historial no válida.")
		}
		page = parsed
	}
	events, err := readList[HistoryEvent](handler.ctx, handler.tx, `SELECT to_jsonb(e) ||
        jsonb_build_object('author_name',u.display_name,'device_name',d.name)
        FROM app.restaurant_events e JOIN app.users u ON u.id=e.author_id JOIN app.devices d ON d.id=e.device_id
        WHERE e.account_id=$1 OR e.related_account_id=$1 ORDER BY e.sequence DESC LIMIT 100 OFFSET $2`, id, (page-1)*100)
	if err != nil {
		return nil, err
	}
	var count int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.restaurant_events WHERE account_id=$1 OR related_account_id=$1", id).Scan(&count); err != nil {
		return nil, err
	}
	return map[string]any{"items": events, "page": page, "total": count}, nil
}

func (handler *service) accounts(parts []string) (any, error) {
	if len(parts) == 1 {
		if handler.request.Method == "GET" {
			return handler.listAccounts()
		}
		return handler.openAccount()
	}
	id := parts[1]
	if len(parts) == 2 {
		if handler.request.Method == "GET" {
			account, err := handler.readAccount(id)
			if err != nil {
				return nil, err
			}
			if err := handler.authorizeAccount(account, false); err != nil {
				return nil, err
			}
			return handler.readBill(id)
		}
		return handler.updateAccount(id)
	}
	switch parts[2] {
	case "draft":
		return handler.saveDraft(id)
	case "preview":
		return handler.previewDraft(id)
	case "draft-preview":
		return handler.previewUnsavedDraft(id)
	case "confirm":
		return handler.confirm(id)
	case "edit-preview":
		return handler.editPreview(id)
	case "edit":
		return handler.edit(id)
	case "cancel":
		return handler.cancel(id)
	case "transfer", "split":
		return handler.transfer(id, parts[2] == "split")
	case "merge":
		return handler.merge(id)
	case "move", "assign":
		return handler.accountAction(id, parts[2])
	case "release":
		return handler.releaseAccount(id)
	case "discard":
		return handler.discardAccount(id)
	case "cancel-account":
		return handler.cancelAccount(id)
	case "history":
		return handler.history(id)
	}
	return nil, invalid(fmt.Sprintf("Acción de cuenta no disponible: %s.", parts[2]))
}
