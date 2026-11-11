package inventory

import (
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Proveedores (§16 de la especificación). Reglas:
//   - nombre comercial único sin distinguir mayúsculas; razón social, teléfono, correo, domicilio,
//     contacto y observaciones opcionales;
//   - RFC opcional, en mayúsculas, con el formato del SAT (3 letras de persona moral o 4 de persona
//     física, fecha AAMMDD válida y homoclave de 3) y no único: varias sucursales pueden compartirlo;
//   - se desactivan, nunca se borran (no existe ruta de borrado y las compras los referencian); un
//     proveedor desactivado no se usa en órdenes nuevas ni en recepciones directas;
//   - insumos que surte: código del proveedor, presentación habitual, preferido (uno por insumo:
//     marcar otro lo quita del anterior) y último precio recibido, que actualizan las recepciones;
//   - revisión optimista, auditoría con antes y después y evento inventory.purchases.changed;
//   - etapa 9A (§3.3): segundo teléfono, teléfono celular y página web opcionales, validados solo en
//     estos campos nuevos para que los proveedores existentes se sigan guardando sin cambios;
//   - etapa 9A (§3.2): modalidad formal o «Compras de mostrador» (counter). El proveedor de mostrador
//     es único (lo siembra la 0016; no se crea otro ni cambia la modalidad de nadie), no lleva RFC ni
//     razón social, no surte insumos (ni preferido ni último precio) y solo se usa en recepciones
//     directas, con el comercio concreto como referencia opcional de cada recepción.

// Modalidades de proveedor (inventory_suppliers.kind).
const (
	SupplierKindFormal  = "formal"
	SupplierKindCounter = "counter"
)

var (
	rfcPattern   = regexp.MustCompile(`^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$`)
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	// phonePattern admite dígitos, espacios, +, paréntesis, punto, guion y diagonal, con una extensión
	// opcional al final («ext. 12», «ext 12», «x 12»).
	phonePattern   = regexp.MustCompile(`^[0-9+() ./-]+?( ?(?:ext\.?|x) ?[0-9]{1,6})?$`)
	phoneDigits    = regexp.MustCompile(`[0-9]`)
	phoneExtension = regexp.MustCompile(` ?(?:ext\.?|x) ?[0-9]{1,6}$`)
	websitePattern = regexp.MustCompile(`^(?i)(https?://)?([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(:[0-9]{1,5})?(/\S*)?$`)
)

// validatePhone valida un teléfono opcional nuevo (segundo teléfono o celular): una línea de hasta
// 40 caracteres con caracteres de teléfono y entre 7 y 20 dígitos sin contar la extensión. Vacío
// equivale a sin teléfono.
func validatePhone(value, subject string) (string, error) {
	trimmed, err := purchaseText(value, subject, 40, true)
	if err != nil || trimmed == "" {
		return trimmed, err
	}
	lower := strings.ToLower(trimmed)
	digits := len(phoneDigits.FindAllString(phoneExtension.ReplaceAllString(lower, ""), -1))
	if !phonePattern.MatchString(lower) || digits < 7 || digits > 20 {
		return "", invalid(subject + " no tiene un formato válido: usa de 7 a 20 dígitos con espacios, +, paréntesis o guiones " +
			"(por ejemplo 55 1234 5678 o (33) 3615-0000 ext. 12).")
	}
	return trimmed, nil
}

// validateWebsite valida la página web opcional: un dominio con una ruta opcional, con o sin
// http:// o https:// (ningún otro esquema), sin espacios y de hasta 200 caracteres. Se guarda tal
// como se escribió (recortada); la interfaz la muestra como texto, nunca como enlace.
func validateWebsite(value string) (string, error) {
	trimmed, err := purchaseText(value, "La página web", 200, true)
	if err != nil || trimmed == "" {
		return trimmed, err
	}
	// El patrón solo admite http:// o https:// como esquema: javascript:, mailto: o ftp:// no coinciden.
	if !websitePattern.MatchString(trimmed) {
		return "", invalid("La página web no tiene un formato válido: escribe un dominio como restaurante.mx o " +
			"https://www.proveedor.com.mx/catalogo, sin espacios.")
	}
	return trimmed, nil
}

// normalizeRFC recorta y pasa a mayúsculas un RFC opcional; vacío equivale a sin RFC. Valida el
// formato y que la fecha AAMMDD exista.
func normalizeRFC(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.ToUpper(strings.TrimSpace(*value))
	if trimmed == "" {
		return nil, nil
	}
	if !rfcPattern.MatchString(trimmed) {
		return nil, invalid("El RFC no tiene un formato válido: 3 o 4 letras, la fecha AAMMDD y 3 caracteres de homoclave.")
	}
	runes := []rune(trimmed)
	letters := len(runes) - 9
	if _, err := time.Parse("060102", string(runes[letters:letters+6])); err != nil {
		return nil, invalid("La fecha del RFC no es válida.")
	}
	return &trimmed, nil
}

// Supplier es un proveedor tal como lo entrega la API.
type Supplier struct {
	ID             string  `json:"id"`
	TradeName      string  `json:"trade_name"`
	LegalName      string  `json:"legal_name"`
	RFC            *string `json:"rfc"`
	Phone          string  `json:"phone"`
	PhoneSecondary string  `json:"phone_secondary"`
	MobilePhone    string  `json:"mobile_phone"`
	Website        string  `json:"website"`
	Email          string  `json:"email"`
	Address        string  `json:"address"`
	Contact        string  `json:"contact"`
	Notes          string  `json:"notes"`
	// Kind es «formal» o «counter» (Compras de mostrador).
	Kind          string     `json:"kind"`
	Active        bool       `json:"active"`
	ItemCount     int        `json:"item_count"`
	HasPurchases  bool       `json:"has_purchases"`
	LastReceiptAt *time.Time `json:"last_receipt_at"`
	Revision      int        `json:"revision"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	UpdatedByName *string    `json:"updated_by_name"`
}

// SupplierItem es un insumo que surte el proveedor. El último precio es el de la recepción más
// reciente, sin IVA y antes del descuento, por la presentación o unidad de esa recepción.
type SupplierItem struct {
	Item               ItemReference          `json:"item"`
	Presentation       *PresentationReference `json:"presentation"`
	SupplierCode       string                 `json:"supplier_code"`
	Preferred          bool                   `json:"preferred"`
	LastUnitPrice      *string                `json:"last_unit_price"`
	LastPriceUnitLabel *string                `json:"last_price_unit_label"`
	LastPriceAt        *time.Time             `json:"last_price_at"`
	LastReceiptFolio   *int64                 `json:"last_receipt_folio"`
	Revision           int                    `json:"revision"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// SupplierHistory es el historial de compras del proveedor: conteos, importes y lo más reciente.
type SupplierHistory struct {
	OrderCount    int                     `json:"order_count"`
	ReceiptCount  int                     `json:"receipt_count"`
	ReturnCount   int                     `json:"return_count"`
	ReceiptsTotal string                  `json:"receipts_total"`
	ReturnsValue  string                  `json:"returns_value"`
	Orders        []PurchaseOrderSummary  `json:"orders"`
	Receipts      []ReceiptSummary        `json:"receipts"`
	Returns       []SupplierReturnSummary `json:"returns"`
}

// SupplierDetail es el proveedor con sus insumos y su historial de compras.
type SupplierDetail struct {
	*Supplier
	Items   []SupplierItem  `json:"items"`
	History SupplierHistory `json:"history"`
}

// SupplierPage es una página del listado de proveedores.
type SupplierPage struct {
	Items    []*Supplier `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

const supplierColumns = `SELECT s.id,s.trade_name,s.legal_name,s.rfc,s.phone,s.phone_secondary,s.mobile_phone,s.website,s.email,
        s.address,s.contact,s.notes,s.kind,s.active,
        (SELECT count(*) FROM app.inventory_supplier_items si WHERE si.supplier_id=s.id),
        EXISTS(SELECT FROM app.inventory_purchase_orders o WHERE o.supplier_id=s.id)
            OR EXISTS(SELECT FROM app.inventory_receipts r WHERE r.supplier_id=s.id),
        (SELECT max(r.occurred_at) FROM app.inventory_receipts r WHERE r.supplier_id=s.id),
        s.revision,s.created_at,s.updated_at,u.display_name
        FROM app.inventory_suppliers s LEFT JOIN app.users u ON u.id=s.updated_by`

func scanSupplier(row interface{ Scan(...any) error }) (*Supplier, error) {
	var supplier Supplier
	err := row.Scan(&supplier.ID, &supplier.TradeName, &supplier.LegalName, &supplier.RFC, &supplier.Phone, &supplier.PhoneSecondary,
		&supplier.MobilePhone, &supplier.Website, &supplier.Email, &supplier.Address, &supplier.Contact, &supplier.Notes,
		&supplier.Kind, &supplier.Active, &supplier.ItemCount, &supplier.HasPurchases,
		&supplier.LastReceiptAt, &supplier.Revision, &supplier.CreatedAt, &supplier.UpdatedAt, &supplier.UpdatedByName)
	if err != nil {
		return nil, err
	}
	return &supplier, nil
}

func (handler *service) readSupplier(id string) (*Supplier, error) {
	supplier, err := scanSupplier(handler.tx.QueryRow(handler.ctx, supplierColumns+" WHERE s.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El proveedor no existe.")
	}
	return supplier, err
}

// listSuppliers lista proveedores con búsqueda (nombre comercial, razón social, RFC, contacto,
// teléfonos, página web o correo), filtros de estado, modalidad (kind formal o counter: las órdenes de
// compra solo ofrecen formales) y de insumo surtido, y paginación. El proveedor de mostrador va
// primero entre los activos, para encontrarlo sin buscar.
func (handler *service) listSuppliers(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "active", "kind", "item_id", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	if pattern != "" {
		set.add(`(s.trade_name ILIKE $%[1]d OR s.legal_name ILIKE $%[1]d OR s.rfc ILIKE $%[1]d OR s.contact ILIKE $%[1]d
            OR s.phone ILIKE $%[1]d OR s.phone_secondary ILIKE $%[1]d OR s.mobile_phone ILIKE $%[1]d OR s.website ILIKE $%[1]d
            OR s.email ILIKE $%[1]d)`, pattern)
	}
	if kind := query.Get("kind"); kind != "" {
		if kind != SupplierKindFormal && kind != SupplierKindCounter {
			return nil, invalid("El filtro de modalidad del proveedor no es válido.")
		}
		set.add("s.kind=$%d", kind)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return nil, err
	}
	if active != nil {
		set.add("s.active=$%d", *active)
	}
	itemID, err := optionalQueryID(query, "item_id", "insumo")
	if err != nil {
		return nil, err
	}
	if itemID != nil {
		set.add("EXISTS(SELECT FROM app.inventory_supplier_items si WHERE si.supplier_id=s.id AND si.item_id=$%d)", *itemID)
	}
	result := SupplierPage{Items: []*Supplier{}, Page: page, PageSize: size}
	count := "SELECT count(*) FROM app.inventory_suppliers s" + set.where()
	err = handler.tx.QueryRow(handler.ctx, count, set.arguments...).Scan(&result.Total)
	if err != nil {
		return nil, err
	}
	statement := supplierColumns + set.where() + " ORDER BY s.active DESC, s.kind='counter' DESC, lower(s.trade_name), s.id LIMIT " +
		set.next(size) +
		" OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		supplier, err := scanSupplier(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, supplier)
	}
	return result, rows.Err()
}

func (handler *service) readSupplierRoute(parts []string) (any, error) {
	return handler.supplierDetail(parts[1])
}

// supplierDetail lee el proveedor con sus insumos y su historial (las 20 órdenes, recepciones y
// devoluciones más recientes, con los conteos y los importes totales).
func (handler *service) supplierDetail(id string) (*SupplierDetail, error) {
	supplier, err := handler.readSupplier(id)
	if err != nil {
		return nil, err
	}
	detail := &SupplierDetail{Supplier: supplier, Items: []SupplierItem{}}
	if detail.Items, err = handler.supplierItems(id); err != nil {
		return nil, err
	}
	var receiptsTotal, returnsValue string
	err = handler.tx.QueryRow(handler.ctx, `SELECT
            (SELECT count(*) FROM app.inventory_purchase_orders WHERE supplier_id=$1),
            (SELECT count(*) FROM app.inventory_receipts WHERE supplier_id=$1),
            (SELECT count(*) FROM app.inventory_supplier_returns WHERE supplier_id=$1),
            (SELECT coalesce(sum(total),0)::text FROM app.inventory_receipts WHERE supplier_id=$1),
            (SELECT coalesce(sum(value),0)::text FROM app.inventory_supplier_returns WHERE supplier_id=$1)`, id).
		Scan(&detail.History.OrderCount, &detail.History.ReceiptCount, &detail.History.ReturnCount, &receiptsTotal, &returnsValue)
	if err != nil {
		return nil, err
	}
	detail.History.ReceiptsTotal = money(parseDatabaseDecimal(receiptsTotal))
	detail.History.ReturnsValue = money(roundMoney(parseDatabaseDecimal(returnsValue)))
	if detail.History.Orders, err = handler.purchaseOrderSummaries("o.supplier_id=$1", 20, id); err != nil {
		return nil, err
	}
	if detail.History.Receipts, err = handler.receiptSummaries("r.supplier_id=$1", 20, id); err != nil {
		return nil, err
	}
	if detail.History.Returns, err = handler.supplierReturnSummaries("sr.supplier_id=$1", 20, id); err != nil {
		return nil, err
	}
	return detail, nil
}

// supplierItems lee los insumos que surte el proveedor (preferidos primero).
func (handler *service) supplierItems(supplierID string) ([]SupplierItem, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT i.id,i.code,i.name,i.kind,i.active,bu.id,bu.code,bu.name,bu.dimension,
            p.id,p.name,p.base_quantity::text,p.active,si.supplier_code,si.preferred,si.last_unit_price::text,
            coalesce(lp.name,lu.name),si.last_price_at,r.folio,si.revision,si.updated_at
        FROM app.inventory_supplier_items si
        JOIN app.inventory_items i ON i.id=si.item_id
        JOIN app.inventory_units bu ON bu.id=i.base_unit_id
        LEFT JOIN app.inventory_presentations p ON p.id=si.presentation_id
        LEFT JOIN app.inventory_presentations lp ON lp.id=si.last_price_presentation_id
        LEFT JOIN app.inventory_units lu ON lu.id=si.last_price_unit_id
        LEFT JOIN app.inventory_receipts r ON r.id=si.last_receipt_id
        WHERE si.supplier_id=$1 ORDER BY si.preferred DESC, i.active DESC, i.name, i.id`, supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SupplierItem{}
	for rows.Next() {
		var item SupplierItem
		var presentationID, presentationName, presentationBase, lastPrice *string
		var presentationActive *bool
		err := rows.Scan(&item.Item.ID, &item.Item.Code, &item.Item.Name, &item.Item.Kind, &item.Item.Active, &item.Item.Unit.ID,
			&item.Item.Unit.Code, &item.Item.Unit.Name, &item.Item.Unit.Dimension, &presentationID, &presentationName,
			&presentationBase, &presentationActive, &item.SupplierCode, &item.Preferred, &lastPrice, &item.LastPriceUnitLabel,
			&item.LastPriceAt, &item.LastReceiptFolio, &item.Revision, &item.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if presentationID != nil {
			item.Presentation = &PresentationReference{
				ID: *presentationID, Name: *presentationName, BaseQuantity: quantityText(parseDatabaseDecimal(*presentationBase)),
				Active: *presentationActive,
			}
		}
		if lastPrice != nil {
			text := exactMoneyText(parseDatabaseDecimal(*lastPrice))
			item.LastUnitPrice = &text
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type supplierInput struct {
	OperationID string  `json:"operation_id"`
	Revision    int     `json:"revision"`
	TradeName   string  `json:"trade_name"`
	LegalName   string  `json:"legal_name"`
	RFC         *string `json:"rfc"`
	Phone       string  `json:"phone"`
	// Campos de la etapa 9A, opcionales. Ausentes del cuerpo (nil): al crear equivalen a vacío y
	// al editar se conservan, para que un cliente anterior a la 9A, que no los envía, no los borre.
	// Presentes se validan; la cadena vacía los borra.
	PhoneSecondary *string `json:"phone_secondary"`
	MobilePhone    *string `json:"mobile_phone"`
	Website        *string `json:"website"`
	Email          string  `json:"email"`
	Address        string  `json:"address"`
	Contact        string  `json:"contact"`
	Notes          string  `json:"notes"`
	// Active ausente: activo al crear y sin cambio al editar.
	Active *bool `json:"active"`
}

// validateSupplier recorta y valida los datos de un proveedor y devuelve el RFC normalizado.
func validateSupplier(input *supplierInput) (*string, error) {
	if err := validateName(&input.TradeName, "El nombre comercial del proveedor", 100); err != nil {
		return nil, err
	}
	var err error
	if input.LegalName, err = purchaseText(input.LegalName, "La razón social", 200, true); err != nil {
		return nil, err
	}
	rfc, err := normalizeRFC(input.RFC)
	if err != nil {
		return nil, err
	}
	if input.Phone, err = purchaseText(input.Phone, "El teléfono", 40, true); err != nil {
		return nil, err
	}
	if input.PhoneSecondary, err = validateOptionalContact(input.PhoneSecondary, func(value string) (string, error) {
		return validatePhone(value, "El segundo teléfono")
	}); err != nil {
		return nil, err
	}
	if input.MobilePhone, err = validateOptionalContact(input.MobilePhone, func(value string) (string, error) {
		return validatePhone(value, "El teléfono celular")
	}); err != nil {
		return nil, err
	}
	if input.Website, err = validateOptionalContact(input.Website, validateWebsite); err != nil {
		return nil, err
	}
	if input.Email, err = purchaseText(input.Email, "El correo electrónico", 120, true); err != nil {
		return nil, err
	}
	if input.Email != "" && !emailPattern.MatchString(input.Email) {
		return nil, invalid("El correo electrónico no tiene un formato válido.")
	}
	if input.Address, err = purchaseText(input.Address, "El domicilio", 500, false); err != nil {
		return nil, err
	}
	if input.Contact, err = purchaseText(input.Contact, "El contacto", 100, true); err != nil {
		return nil, err
	}
	if input.Notes, err = purchaseText(input.Notes, "Las observaciones", 2000, false); err != nil {
		return nil, err
	}
	return rfc, nil
}

// validateOptionalContact valida un dato de contacto de la etapa 9A que puede faltar en el cuerpo:
// ausente (nil) sigue ausente; presente se valida y se devuelve recortado.
func validateOptionalContact(value *string, validate func(string) (string, error)) (*string, error) {
	if value == nil {
		return nil, nil
	}
	validated, err := validate(*value)
	if err != nil {
		return nil, err
	}
	return &validated, nil
}

// contactOr es el dato de contacto que se guarda: el del cuerpo si llegó o, si no, current (vacío
// al crear; el guardado al editar).
func contactOr(value *string, current string) string {
	if value == nil {
		return current
	}
	return *value
}

func supplierConflict(err error) error {
	if uniqueViolation(err) {
		return conflict("DUPLICATE_NAME", "Ya existe un proveedor con ese nombre comercial.")
	}
	return err
}

// createSupplier da de alta un proveedor (activo por omisión).
func (handler *service) createSupplier(_ []string) (any, error) {
	var input supplierInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	rfc, err := validateSupplier(&input)
	if err != nil {
		return nil, err
	}
	active := input.Active == nil || *input.Active
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_suppliers(id,trade_name,legal_name,rfc,phone,phone_secondary,
            mobile_phone,website,email,address,contact,notes,active,created_by,updated_by)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)`,
		id, input.TradeName, input.LegalName, rfc, input.Phone, contactOr(input.PhoneSecondary, ""), contactOr(input.MobilePhone, ""),
		contactOr(input.Website, ""), input.Email, input.Address, input.Contact, input.Notes, active, handler.actorReference())
	if err != nil {
		return nil, supplierConflict(err)
	}
	saved, err := handler.supplierDetail(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &id); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.supplier_created", "inventory_supplier", &id, nil, saved.Supplier)
}

// updateSupplier guarda todos los datos del proveedor con revisión optimista (el segundo
// teléfono, el celular y la página web ausentes del cuerpo se conservan). Desactivarlo nunca
// borra nada: sus compras y su historial se conservan. La modalidad no cambia; el proveedor de
// mostrador se puede renombrar, describir o desactivar, pero nunca recibe RFC ni razón social.
func (handler *service) updateSupplier(parts []string) (any, error) {
	var input supplierInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	rfc, err := validateSupplier(&input)
	if err != nil {
		return nil, err
	}
	previous, err := handler.readSupplier(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("El proveedor")
	}
	if previous.Kind == SupplierKindCounter && (rfc != nil || input.LegalName != "") {
		return nil, invalid("«Compras de mostrador» no lleva RFC ni razón social: registra el comercio concreto en cada recepción.")
	}
	active := previous.Active
	if input.Active != nil {
		active = *input.Active
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_suppliers SET trade_name=$2,legal_name=$3,rfc=$4,phone=$5,
            phone_secondary=$6,mobile_phone=$7,website=$8,email=$9,address=$10,contact=$11,notes=$12,active=$13,updated_by=$14,
            revision=revision+1,updated_at=now()
        WHERE id=$1`,
		previous.ID, input.TradeName, input.LegalName, rfc, input.Phone, contactOr(input.PhoneSecondary, previous.PhoneSecondary),
		contactOr(input.MobilePhone, previous.MobilePhone), contactOr(input.Website, previous.Website), input.Email, input.Address,
		input.Contact, input.Notes, active, handler.actorReference())
	if err != nil {
		return nil, supplierConflict(err)
	}
	saved, err := handler.supplierDetail(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &previous.ID); err != nil {
		return nil, err
	}
	action := "inventory.supplier_updated"
	if previous.Active && !active {
		action = "inventory.supplier_deactivated"
	}
	return saved, handler.audit(handler.ctx, handler.tx, action, "inventory_supplier", &previous.ID, previous, saved.Supplier)
}

type supplierItemInput struct {
	OperationID string `json:"operation_id"`
	// Revision es la del insumo del proveedor (0 si todavía no lo surte).
	Revision       int     `json:"revision"`
	ItemID         string  `json:"item_id"`
	PresentationID *string `json:"presentation_id"`
	SupplierCode   string  `json:"supplier_code"`
	Preferred      bool    `json:"preferred"`
}

// supplierItemRow es la fila guardada de un insumo del proveedor (para revisión y auditoría).
type supplierItemRow struct {
	ItemID         string  `json:"item_id"`
	PresentationID *string `json:"presentation_id"`
	SupplierCode   string  `json:"supplier_code"`
	Preferred      bool    `json:"preferred"`
	Revision       int     `json:"revision"`
}

func (handler *service) readSupplierItemRow(supplierID, itemID string) (*supplierItemRow, error) {
	var row supplierItemRow
	err := handler.tx.QueryRow(handler.ctx, `SELECT item_id,presentation_id,supplier_code,preferred,revision
        FROM app.inventory_supplier_items WHERE supplier_id=$1 AND item_id=$2`, supplierID, itemID).
		Scan(&row.ItemID, &row.PresentationID, &row.SupplierCode, &row.Preferred, &row.Revision)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// saveSupplierItem agrega o edita un insumo que surte el proveedor. Marcarlo como preferido quita
// la marca del proveedor que lo era (un preferido por insumo, índice único parcial).
func (handler *service) saveSupplierItem(parts []string) (any, error) {
	var input supplierItemInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.ItemID); err != nil {
		return nil, invalid("Elige el insumo que surte el proveedor.")
	}
	code, err := purchaseText(input.SupplierCode, "El código del proveedor", 60, true)
	if err != nil {
		return nil, err
	}
	supplier, err := handler.readSupplier(parts[1])
	if err != nil {
		return nil, err
	}
	if supplier.Kind == SupplierKindCounter {
		return nil, conflict("COUNTER_SUPPLIER", "«"+supplier.TradeName+"» no registra insumos surtidos, preferidos ni último precio: "+
			"es para compras ocasionales.")
	}
	item, err := handler.readItem(strings.ToLower(input.ItemID))
	if err != nil {
		return nil, err
	}
	previous, err := handler.readSupplierItemRow(supplier.ID, item.ID)
	if err != nil {
		return nil, err
	}
	current := 0
	if previous != nil {
		current = previous.Revision
	}
	if current != input.Revision {
		return nil, revisionConflict("El insumo del proveedor")
	}
	if previous == nil && !item.Active {
		return nil, conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
	}
	presentationID, err := validateOptionalID(input.PresentationID)
	if err != nil {
		return nil, invalid("La presentación no es válida.")
	}
	if presentationID != nil {
		if err := supplierPresentation(item, *presentationID, previous); err != nil {
			return nil, err
		}
	}
	// Un preferido por insumo: marcar este quita la marca del proveedor que lo era (queda auditado).
	cleared := []string{}
	if input.Preferred {
		rows, err := handler.tx.Query(handler.ctx, `UPDATE app.inventory_supplier_items SET preferred=false,revision=revision+1,
                updated_by=$3,updated_at=now()
            WHERE item_id=$1 AND supplier_id<>$2 AND preferred RETURNING supplier_id`, item.ID, supplier.ID, handler.actorReference())
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			cleared = append(cleared, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_supplier_items(supplier_id,item_id,presentation_id,supplier_code,
            preferred,updated_by)
        VALUES ($1,$2,$3,$4,$5,$6)
        ON CONFLICT (supplier_id,item_id) DO UPDATE SET presentation_id=EXCLUDED.presentation_id,
            supplier_code=EXCLUDED.supplier_code,preferred=EXCLUDED.preferred,updated_by=EXCLUDED.updated_by,
            revision=app.inventory_supplier_items.revision+1,updated_at=now()`,
		supplier.ID, item.ID, presentationID, code, input.Preferred, handler.actorReference())
	if err != nil {
		return nil, err
	}
	saved, err := handler.readSupplierItemRow(supplier.ID, item.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &supplier.ID); err != nil {
		return nil, err
	}
	after := map[string]any{"item": saved, "preferred_removed_from_supplier_ids": cleared}
	err = handler.audit(handler.ctx, handler.tx, "inventory.supplier_item_saved", "inventory_supplier", &supplier.ID, previous, after)
	if err != nil {
		return nil, err
	}
	return handler.supplierDetail(supplier.ID)
}

// supplierPresentation exige que la presentación sea del insumo y esté activa (una inactiva solo se
// conserva si ya era la del insumo del proveedor).
func supplierPresentation(item *Item, presentationID string, previous *supplierItemRow) error {
	for _, presentation := range item.Presentations {
		if presentation.ID != presentationID {
			continue
		}
		unchanged := previous != nil && previous.PresentationID != nil && *previous.PresentationID == presentationID
		if !presentation.Active && !unchanged {
			return conflict("PRESENTATION_INACTIVE", "La presentación «"+presentation.Name+"» está desactivada.")
		}
		return nil
	}
	return invalid("La presentación no pertenece al insumo «" + item.Name + "».")
}

type supplierItemRemoval struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	ItemID      string `json:"item_id"`
}

// removeSupplierItem deja de registrar que el proveedor surte el insumo. Es configuración, no
// historia: las órdenes y recepciones del insumo con ese proveedor se conservan.
func (handler *service) removeSupplierItem(parts []string) (any, error) {
	var input supplierItemRemoval
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.ItemID); err != nil {
		return nil, invalid("Elige el insumo que se quita del proveedor.")
	}
	supplier, err := handler.readSupplier(parts[1])
	if err != nil {
		return nil, err
	}
	previous, err := handler.readSupplierItemRow(supplier.ID, strings.ToLower(input.ItemID))
	if err != nil {
		return nil, err
	}
	if previous == nil {
		return nil, notFound("El proveedor no tiene registrado ese insumo.")
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("El insumo del proveedor")
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.inventory_supplier_items WHERE supplier_id=$1 AND item_id=$2",
		supplier.ID, previous.ItemID); err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.purchases.changed", &supplier.ID); err != nil {
		return nil, err
	}
	err = handler.audit(handler.ctx, handler.tx, "inventory.supplier_item_removed", "inventory_supplier", &supplier.ID, previous, nil)
	if err != nil {
		return nil, err
	}
	return handler.supplierDetail(supplier.ID)
}

// recordSupplierPrice guarda el último precio recibido de un insumo del proveedor (sin IVA y antes
// del descuento, por la presentación o unidad capturada) y crea la relación si no existía. Solo
// una recepción igual o más reciente que la que fijó el precio anterior lo reemplaza. Las compras de
// mostrador no lo registran: sus precios son de comercios distintos y no deben alimentar el último
// precio, los preferidos ni las sugerencias.
func (handler *service) recordSupplierPrice(supplier *Supplier, receiptID string, line preparedPurchaseLine, occurredAt time.Time) error {
	if supplier.Kind == SupplierKindCounter {
		return nil
	}
	supplierID := supplier.ID
	var presentation, unit *string
	if line.capture.presentationID != nil {
		presentation = line.capture.presentationID
	} else {
		unit = line.capture.unitID
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_supplier_items(supplier_id,item_id,presentation_id,last_unit_price,
            last_price_presentation_id,last_price_unit_id,last_price_at,last_receipt_id,updated_by)
        VALUES ($1,$2,$3,$4::numeric,$3,$5,$6,$7,$8)
        ON CONFLICT (supplier_id,item_id) DO UPDATE SET last_unit_price=EXCLUDED.last_unit_price,
            last_price_presentation_id=EXCLUDED.last_price_presentation_id,last_price_unit_id=EXCLUDED.last_price_unit_id,
            last_price_at=EXCLUDED.last_price_at,last_receipt_id=EXCLUDED.last_receipt_id
        WHERE app.inventory_supplier_items.last_price_at IS NULL OR app.inventory_supplier_items.last_price_at<=EXCLUDED.last_price_at`,
		supplierID, line.capture.item.ID, presentation, line.unitPrice.String(), unit, occurredAt, receiptID, handler.actorReference())
	return err
}

// supplierForPurchase exige un proveedor existente y activo para una orden nueva o una recepción
// directa. allowCounter indica si se admite «Compras de mostrador»: solo la recepción directa lo
// admite; las órdenes de compra y los borradores desde sugerencias exigen un proveedor formal.
func (handler *service) supplierForPurchase(id string, allowCounter bool) (*Supplier, error) {
	if err := validateID(id); err != nil {
		return nil, invalid("Elige el proveedor.")
	}
	supplier, err := handler.readSupplier(strings.ToLower(id))
	if err != nil {
		return nil, err
	}
	if !supplier.Active {
		return nil, conflict("SUPPLIER_INACTIVE", "El proveedor «"+supplier.TradeName+"» está desactivado.")
	}
	if supplier.Kind == SupplierKindCounter && !allowCounter {
		return nil, conflict("COUNTER_SUPPLIER", "«"+supplier.TradeName+"» solo se usa en recepciones directas: elige un proveedor "+
			"formal para la orden de compra.")
	}
	return supplier, nil
}

// optionalMoneyText devuelve el importe exacto y su versión a centavos, o nil.
func optionalMoneyText(value *decimal.Decimal) (*string, *string) {
	if value == nil {
		return nil, nil
	}
	exact, display := exactMoneyText(*value), displayMoneyText(*value)
	return &exact, &display
}
