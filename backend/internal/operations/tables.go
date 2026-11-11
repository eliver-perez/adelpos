package operations

import (
	"strings"

	"local/ad-el-pos/server/internal/payments"
)

// tableQuery entrega cada mesa con su ocupación calculada por Go en una sola consulta para todo el
// mapa (etapa 7.1, sin consultas por mesa ni por cuenta desde la aplicación):
//
//   - out_of_service: mesa desactivada o fuera de servicio;
//   - free: sin cuentas abiertas;
//   - awaiting_payment: alguna cuenta tiene una precuenta vigente y le queda saldo. La vigencia es
//     el predicado único de payments (precheck_current de AccountStandingSQL, etapa 9A): la
//     precuenta se imprimió con la revisión de la cuenta y el número de ajustes actuales, así que
//     cualquier cambio de contenido, ajuste o cancelación de ajuste posterior la invalida aunque
//     haya esperado el lock mientras se imprimía; los pagos posteriores no la invalidan;
//   - partial_payment: alguna cuenta tiene pagado neto mayor que cero y saldo pendiente;
//   - occupied: cualquier otra cuenta abierta, también la que aún no tiene productos.
//
// Con varias cuentas gana el estado más avanzado (esperando pago > pago parcial > ocupada). El total,
// lo pagado y el saldo salen de la fórmula única de payments (AccountStandingSQL).
var tableQuery = `SELECT to_jsonb(t) || jsonb_build_object(
    'account_count',occupancy.account_count,
    'occupied_since',occupancy.occupied_since,
    'occupied_seconds',occupancy.occupied_seconds,
    'balance',occupancy.balance::text,
    'state',CASE WHEN NOT t.active OR t.out_of_service THEN 'out_of_service'
        WHEN occupancy.account_count=0 THEN 'free'
        WHEN occupancy.awaiting_payment THEN 'awaiting_payment'
        WHEN occupancy.partial_payment THEN 'partial_payment'
        ELSE 'occupied' END)
    FROM app.restaurant_tables t
    CROSS JOIN LATERAL (SELECT count(*)::integer AS account_count,
        min(a.created_at) AS occupied_since,
        greatest(0,coalesce(floor(extract(epoch FROM now()-min(a.created_at))),0))::bigint AS occupied_seconds,
        coalesce(sum(f.balance),0)::numeric(18,2) AS balance,
        coalesce(bool_or(f.balance>0 AND f.precheck_current),false) AS awaiting_payment,
        coalesce(bool_or(f.paid>0 AND f.balance>0),false) AS partial_payment
        FROM app.restaurant_accounts a
        CROSS JOIN LATERAL (` + payments.AccountStandingSQL("a.id") + `) f
        WHERE a.table_id=t.id AND a.state='open') occupancy`

// tableHasOpenAccounts rechaza poner fuera de servicio una mesa con cuentas abiertas (etapa 7.1).
func tableHasOpenAccounts() error {
	return &Error{
		Status:  409,
		Code:    "TABLE_HAS_OPEN_ACCOUNTS",
		Message: "La mesa tiene cuentas abiertas. Cóbralas, muévelas o libéralas antes de ponerla fuera de servicio.",
	}
}

func validateGeometry(table Table) error {
	validPosition := table.X >= 0 && table.Y >= 0 && table.X+table.Width <= 1000 && table.Y+table.Height <= 560
	validSize := table.Width >= 64 && table.Width <= 240 && table.Height >= 56 && table.Height <= 180
	validShape := table.Shape == "rectangle" || table.Shape == "round"
	if !validPosition || !validSize || !validShape {
		return invalid("La mesa debe caber en el mapa de 1000 × 560, medir entre 64–240 × 56–180 y usar forma rectangular o redonda.")
	}
	return nil
}

func (handler *service) validateTableTarget(id *string) error {
	if id == nil {
		return nil
	}
	if err := validateID(*id); err != nil {
		return err
	}
	var available bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.restaurant_tables t
        JOIN app.restaurant_areas a ON a.id=t.area_id WHERE t.id=$1 AND t.active AND NOT t.out_of_service AND a.active)`, *id).Scan(&available)
	if err != nil {
		return err
	}
	if !available {
		return invalid("La mesa de destino o su área está inactiva o fuera de servicio.")
	}
	return nil
}

func (handler *service) tables(parts []string) (any, error) {
	if len(parts) == 3 && parts[2] == "service" {
		return handler.tableService(parts[1])
	}
	if handler.request.Method == "GET" {
		return readList[Table](handler.ctx, handler.tx, tableQuery+" ORDER BY t.area_id,t.name,t.id")
	}
	var input struct {
		OperationID string `json:"operation_id"`
		Table
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	table := input.Table
	table.Name = strings.TrimSpace(table.Name)
	if table.Name == "" || len(table.Name) > 60 || table.Seats < 1 || table.Seats > 40 {
		return nil, invalid("Indica nombre de mesa de hasta 60 caracteres y capacidad de 1 a 40 personas.")
	}
	if err := validateGeometry(table); err != nil {
		return nil, err
	}
	if err := validateID(table.AreaID); err != nil {
		return nil, err
	}
	area, err := readOne[Area](handler.ctx, handler.tx, areaQuery+" WHERE a.id=$1", table.AreaID)
	if err != nil {
		return nil, err
	}
	if !area.Active {
		return nil, invalid("Activa el área antes de configurar sus mesas.")
	}
	var before any
	if len(parts) == 1 {
		if table.ID != "" || table.Revision != 0 {
			return nil, invalid("La mesa nueva no debe reutilizar identidad ni revisión.")
		}
		table.ID = newID()
		_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.restaurant_tables
            (id,area_id,name,shape,x,y,width,height,seats,active,out_of_service)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, table.ID, table.AreaID, table.Name,
			table.Shape, table.X, table.Y, table.Width, table.Height, table.Seats, table.Active, table.OutOfService)
	} else {
		old, problem := readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", parts[1])
		if problem != nil {
			return nil, problem
		}
		if table.Revision != old.Revision || (table.ID != "" && table.ID != old.ID) {
			return nil, conflict()
		}
		table.ID = old.ID
		before = old
		if !table.Active && old.AccountCount > 0 {
			return nil, invalid("Mueve o resuelve las cuentas abiertas antes de desactivar la mesa.")
		}
		// Etapa 7.1: tampoco se pone fuera de servicio una mesa con cuentas abiertas. Una mesa que ya
		// estaba fuera de servicio con cuentas (registros anteriores) puede seguir editándose.
		if table.OutOfService && !old.OutOfService && old.AccountCount > 0 {
			return nil, tableHasOpenAccounts()
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_tables SET area_id=$1,name=$2,shape=$3,x=$4,y=$5,
            width=$6,height=$7,seats=$8,active=$9,out_of_service=$10,revision=revision+1,updated_at=now()
            WHERE id=$11 AND revision=$12`, table.AreaID, table.Name, table.Shape, table.X, table.Y,
			table.Width, table.Height, table.Seats, table.Active, table.OutOfService, table.ID, table.Revision)
	}
	if err != nil {
		return nil, err
	}
	saved, err := readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", table.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "restaurant.table.saved", "table", &table.ID, before, saved)
}

func (handler *service) layout() (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Tables      []struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
			X        int    `json:"x"`
			Y        int    `json:"y"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
			Shape    string `json:"shape"`
		} `json:"tables"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if len(input.Tables) == 0 || len(input.Tables) > 100 {
		return nil, invalid("Guarda de 1 a 100 posiciones de mesa a la vez.")
	}
	before, after := []Table{}, []Table{}
	seen := map[string]bool{}
	for _, position := range input.Tables {
		if err := validateID(position.ID); err != nil {
			return nil, err
		}
		if seen[position.ID] {
			return nil, invalid("Una mesa está repetida en la distribución.")
		}
		seen[position.ID] = true
		old, err := readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", position.ID)
		if err != nil {
			return nil, err
		}
		if old.Revision != position.Revision {
			return nil, conflict()
		}
		before = append(before, *old)
		updated := *old
		updated.X, updated.Y, updated.Width, updated.Height, updated.Shape = position.X, position.Y, position.Width, position.Height, position.Shape
		if err := validateGeometry(updated); err != nil {
			return nil, err
		}
		after = append(after, updated)
	}
	for _, table := range after {
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_tables SET x=$1,y=$2,width=$3,height=$4,
            shape=$5,revision=revision+1,updated_at=now() WHERE id=$6 AND revision=$7`,
			table.X, table.Y, table.Width, table.Height, table.Shape, table.ID, table.Revision)
		if err != nil {
			return nil, err
		}
	}
	return map[string]bool{"ok": true}, handler.audit(handler.ctx, handler.tx, "restaurant.layout.saved", "layout", nil, before, after)
}
