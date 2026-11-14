package operations

import (
	"reflect"
	"strings"
	"time"
)

func (handler *service) validateAccountInput(input *AccountInput, previous *Account) (*time.Time, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.CustomerName = strings.TrimSpace(input.CustomerName)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Origin = strings.TrimSpace(input.Origin)
	if input.Name == "" || len(input.Name) > 100 || len(input.CustomerName) > 100 || len(input.Phone) > 40 || len(input.Origin) > 80 || len(input.Notes) > 2000 {
		return nil, invalid("Revisa nombre/referencia (100), cliente (100), teléfono (40), origen (80) y observaciones (2000 caracteres).")
	}
	if input.Mode != "dine_in" && input.Mode != "takeaway" {
		return nil, invalid("Selecciona cuenta de salón o pedido para llevar; ventanilla pertenece a para llevar.")
	}
	if input.Mode == "takeaway" {
		if err := handler.request.Actor.Require("takeaway.manage"); err != nil {
			return nil, err
		}
		if input.TableID != nil {
			return nil, invalid("Los pedidos para llevar no utilizan mesas.")
		}
	}
	if previous != nil && previous.Mode != input.Mode {
		return nil, invalid("La modalidad de una cuenta es estable; crea otra cuenta de la modalidad correspondiente.")
	}
	tableChanged := previous == nil || !reflect.DeepEqual(previous.TableID, input.TableID)
	if previous != nil && tableChanged {
		if err := handler.request.Actor.Require("accounts.move"); err != nil {
			return nil, err
		}
	}
	if tableChanged {
		if err := handler.validateTableTarget(input.TableID); err != nil {
			return nil, err
		}
	}
	if input.PriceListID == nil {
		var id string
		if previous != nil {
			id = previous.PriceListID
		} else if err := handler.tx.QueryRow(handler.ctx, "SELECT id FROM app.catalog_price_lists WHERE is_default AND active").Scan(&id); err != nil {
			return nil, err
		}
		input.PriceListID = &id
	} else if previous == nil || *input.PriceListID != previous.PriceListID {
		if err := handler.request.Actor.Require("orders.price_list.manage"); err != nil {
			return nil, err
		}
	}
	if err := validateID(*input.PriceListID); err != nil {
		return nil, err
	}
	var activeList bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.catalog_price_lists WHERE id=$1 AND active)", *input.PriceListID).Scan(&activeList)
	if err != nil {
		return nil, err
	}
	if !activeList && (previous == nil || previous.PriceListID != *input.PriceListID) {
		return nil, invalid("La lista de precios seleccionada está inactiva.")
	}
	if input.AssignedUserID == nil {
		assigned := handler.request.Actor.UserID
		if previous != nil {
			assigned = previous.AssignedUserID
		}
		input.AssignedUserID = &assigned
	}
	assignedChanged := *input.AssignedUserID != handler.request.Actor.UserID
	if previous != nil {
		assignedChanged = *input.AssignedUserID != previous.AssignedUserID
	}
	if assignedChanged {
		if err := handler.request.Actor.Require("accounts.transfer"); err != nil {
			return nil, err
		}
		if err := handler.validateAssignee(*input.AssignedUserID); err != nil {
			return nil, err
		}
	}
	if input.ScheduledLocal == "" {
		if input.ScheduleKind != "" {
			return nil, invalid("Indica fecha/hora para el pedido programado o retira su tipo de programación.")
		}
		return nil, nil
	}
	if input.Mode != "takeaway" || (input.ScheduleKind != "preparation" && input.ScheduleKind != "delivery") {
		return nil, invalid("La programación requiere un pedido para llevar y una hora de preparación o entrega.")
	}
	location, err := handler.location()
	if err != nil {
		return nil, err
	}
	scheduled, err := time.ParseInLocation("2006-01-02T15:04", input.ScheduledLocal, location)
	if err != nil || scheduled.Format("2006-01-02T15:04") != input.ScheduledLocal {
		return nil, invalid("La fecha/hora programada no existe o no es válida en la zona del establecimiento.")
	}
	utc := scheduled.UTC()
	return &utc, nil
}

func (handler *service) validateAssignee(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	var eligible bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.users WHERE id=$1 AND active AND NOT support)", id).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return invalid("El responsable debe ser un usuario ordinario activo.")
	}
	return nil
}

func (handler *service) insertAccount(input AccountInput, scheduled *time.Time) (*Account, error) {
	return handler.insertAccountWithID(newID(), input, scheduled)
}

// insertAccountWithID registra la cuenta con un identificador ya generado: la apertura con borrador
// inicial (etapa 7.1) valida sus partidas contra ese identificador antes de escribir.
func (handler *service) insertAccountWithID(id string, input AccountInput, scheduled *time.Time) (*Account, error) {
	var scheduleKind *string
	if scheduled != nil {
		scheduleKind = &input.ScheduleKind
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_accounts
        (id,name,mode,table_id,price_list_id,customer_name,phone,origin,notes,scheduled_at,schedule_kind,opened_by,assigned_user_id,device_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, input.Name, input.Mode, input.TableID,
		input.PriceListID, input.CustomerName, input.Phone, input.Origin, input.Notes, scheduled, scheduleKind,
		handler.request.Actor.UserID, input.AssignedUserID, handler.request.Actor.DeviceID)
	if err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	return account, handler.event(id, nil, "account.opened", nil, account)
}

// openAccount abre una cuenta sin formulario (etapa 6.2). Desde la etapa 7.1 la cuenta de una mesa se
// crea en la misma operación idempotente que guarda su primer producto: con items, la cuenta y su
// borrador se registran juntos; con seen_account_ids, se rechaza (TABLE_HAS_ACCOUNT, sin escribir
// nada) si la mesa tiene una cuenta abierta que la terminal no veía, para no abrir una segunda
// cuenta en silencio cuando otra terminal ya la abrió.
func (handler *service) openAccount() (any, error) {
	var input OpenAccountInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.Revision != 0 {
		return nil, invalid("Una cuenta nueva empieza sin revisión previa.")
	}
	// Etapa 6.2: el nombre es opcional al abrir; sin nombre, Go asigna la referencia neutra.
	if err := handler.fillAccountName(&input.AccountInput); err != nil {
		return nil, err
	}
	scheduled, err := handler.validateAccountInput(&input.AccountInput, nil)
	if err != nil {
		return nil, err
	}
	if input.SeenAccountIDs != nil {
		if err := handler.requireNoUnseenAccounts(input.TableID, *input.SeenAccountIDs); err != nil {
			return nil, err
		}
	}
	id := newID()
	if err := handler.validateDraftItems(id, input.Items); err != nil {
		return nil, err
	}
	// Revisión de la fase D (hallazgo 3): tampoco se abre una cuenta con un producto agotado.
	if err := handler.rejectSoldOutCapture(*input.PriceListID, input.Items, nil); err != nil {
		return nil, err
	}
	if _, err := handler.insertAccountWithID(id, input.AccountInput, scheduled); err != nil {
		return nil, err
	}
	if len(input.Items) > 0 {
		if err := handler.insertDraftItems(id, input.Items); err != nil {
			return nil, err
		}
		if err := handler.event(id, nil, "draft.saved", []DraftItem{}, input.Items); err != nil {
			return nil, err
		}
	}
	return handler.readBill(id)
}

// requireNoUnseenAccounts compara las cuentas abiertas de la mesa con las que la terminal conocía.
// Una cuenta nueva de otra terminal se informa en el detalle del rechazo para que el operador elija
// entre agregar sus productos a esa cuenta o abrir otra a sabiendas.
func (handler *service) requireNoUnseenAccounts(tableID *string, seen []string) error {
	if len(seen) > 100 {
		return invalid("Se admiten hasta 100 cuentas vistas en la mesa.")
	}
	for _, id := range seen {
		if err := validateID(id); err != nil {
			return err
		}
	}
	if tableID == nil {
		return nil
	}
	unseen, err := readList[TableAccountRef](handler.ctx, handler.tx, `SELECT jsonb_build_object(
        'id',a.id,'folio',a.folio,'name',a.name) FROM app.restaurant_accounts a
        WHERE a.table_id=$1 AND a.state='open' AND NOT (a.id::text = ANY($2::text[]))
        ORDER BY a.created_at,a.id`, *tableID, seen)
	if err != nil {
		return err
	}
	if len(unseen) == 0 {
		return nil
	}
	return &Error{
		Status:  409,
		Code:    "TABLE_HAS_ACCOUNT",
		Message: "Esta mesa ya tiene una cuenta abierta en otra terminal. Elige si agregas los productos a esa cuenta o abres una cuenta nueva.",
		Details: map[string]any{"accounts": unseen},
	}
}

func (handler *service) updateAccount(id string) (any, error) {
	var input AccountInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	// Una cuenta identificada por su referencia neutra que cambia de mesa toma la de la mesa
	// nueva; vaciar el nombre restaura la referencia neutra en lugar de fallar.
	if strings.TrimSpace(input.Name) == account.Name {
		moved, err := handler.movedAccountName(account, input.TableID)
		if err != nil {
			return nil, err
		}
		input.Name = moved
	}
	if err := handler.fillAccountName(&input); err != nil {
		return nil, err
	}
	scheduled, err := handler.validateAccountInput(&input, account)
	if err != nil {
		return nil, err
	}
	var scheduleKind *string
	if scheduled != nil {
		scheduleKind = &input.ScheduleKind
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET name=$1,table_id=$2,price_list_id=$3,
        customer_name=$4,phone=$5,origin=$6,notes=$7,scheduled_at=$8,schedule_kind=$9,assigned_user_id=$10 WHERE id=$11`,
		input.Name, input.TableID, input.PriceListID, input.CustomerName, input.Phone, input.Origin, input.Notes,
		scheduled, scheduleKind, input.AssignedUserID, id)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readBill(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.event(id, nil, "account.updated", account, saved.Account)
}

// accountAction mueve una cuenta a otra mesa o cambia su responsable. La anulación anterior con
// motivo libre (void) se retiró en la etapa 9F: Handle la responde con 410 ROUTE_RETIRED antes de
// llegar aquí. El campo reason se sigue aceptando (y guardando en el evento) como antes, aunque
// mover y asignar no lo exigen.
func (handler *service) accountAction(id, action string) (any, error) {
	var input struct {
		Mutation
		TableID        *string `json:"table_id"`
		AssignedUserID string  `json:"assigned_user_id"`
		Reason         string  `json:"reason"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, true); err != nil {
		return nil, err
	}
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	switch action {
	case "move":
		if account.Mode != "dine_in" {
			return nil, invalid("Para llevar no utiliza mesas.")
		}
		if err := handler.validateTableTarget(input.TableID); err != nil {
			return nil, err
		}
	case "assign":
		if err := handler.validateAssignee(input.AssignedUserID); err != nil {
			return nil, err
		}
	}
	// Cambiar de mesa conserva un nombre capturado; la referencia neutra sigue a la mesa nueva.
	name := account.Name
	if action == "move" {
		name, err = handler.movedAccountName(account, input.TableID)
		if err != nil {
			return nil, err
		}
	}
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	switch action {
	case "move":
		_, err = handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET table_id=$1,name=$2 WHERE id=$3", input.TableID, name, id)
	case "assign":
		_, err = handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET assigned_user_id=$1 WHERE id=$2", input.AssignedUserID, id)
	}
	if err != nil {
		return nil, err
	}
	saved, err := handler.readBill(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.event(id, nil, "account."+action, map[string]any{"account": account}, map[string]any{"account": saved.Account, "reason": input.Reason})
}
