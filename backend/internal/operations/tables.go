package operations

import "strings"

const tableQuery = `SELECT to_jsonb(t) || jsonb_build_object(
    'account_count',(SELECT count(*) FROM app.restaurant_accounts a WHERE a.table_id=t.id AND a.state='open'),
    'state',CASE WHEN NOT t.active OR t.out_of_service THEN 'out_of_service'
        WHEN EXISTS(SELECT FROM app.restaurant_accounts a WHERE a.table_id=t.id AND a.state='open') THEN 'occupied'
        ELSE 'free' END) FROM app.restaurant_tables t`

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
