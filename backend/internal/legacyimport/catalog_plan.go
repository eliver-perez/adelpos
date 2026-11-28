package legacyimport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Correspondencia del catálogo, las mesas y los usuarios del anterior con AD-EL POS (§4.1 y decisiones del
// propietario; tabla completa en docs/MIGRACION_ANALISIS.md §6). Cada cosa importada se identifica por su clave
// anterior (legacy_id_map): al repetir la migración, lo que ya existía conserva su identificador en AD-EL POS y se
// actualiza en su lugar.

// takeawayTableID es la mesa «para llevar» del anterior: la id 1 nunca aparece en su mapa (PuntoDeVenta.cs:182) y es
// la modalidad para llevar de AD-EL POS, no una mesa.
const takeawayTableID = 1

// Grupos de complementos: el anterior no tenía grupos, así que se crea uno por cada conjunto distinto de complementos
// que admite un producto (en el respaldo real, uno o pocos).
const complementsGroupName = "Complementos"

// Tamaño de la cuadrícula de mesas por área (el lienzo de AD-EL POS es de 1000 × 560).
const (
	tablesPerRow   = 8
	tablesPerArea  = 48
	tableWidth     = 100
	tableHeight    = 72
	tableCellX     = 125
	tableCellY     = 93
	tableMarginX   = 12
	tableMarginY   = 10
	defaultSeats   = 4
	maxSeats       = 40
	areaNamePrefix = "Salón"
)

// CatalogPlan es lo que se importa al catálogo, a las mesas y a los usuarios.
type CatalogPlan struct {
	Categories    []PlannedCategory
	Products      []PlannedProduct
	Complements   []PlannedComplement
	Groups        []PlannedGroup
	ProductGroups []PlannedProductGroup
	Defaults      []PlannedDefault
	Areas         []PlannedArea
	Tables        []PlannedTable
	// TableGroups (etapa M1, M1-D5) son las mesas divisibles reconocidas en los nombres; AbsorbedTables, las mesas
	// «Mesa N» que se volvieron un grupo: no se crean como mesas vivas, pero sus ventas conservan su nombre.
	TableGroups    []PlannedTableGroup
	AbsorbedTables []PlannedTable
	Users          []PlannedUser
	Notes          []PlanNote
}

// PlanNote es una advertencia de la correspondencia para el informe (sin datos sensibles: nombres del catálogo y
// usuarios renombrados, que el propietario necesita ver).
type PlanNote struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PlannedCategory struct {
	Key       string
	LegacyID  int32
	Name      string
	SortOrder int
}

type PlannedProduct struct {
	Key              string
	LegacyID         int32
	CategoryKey      string
	CategoryName     string
	Code             string
	Name             string
	Description      string
	BasePrice        Money
	TaxRate          int
	PriceCanDecrease bool
	Active           bool
	SortOrder        int
	HasPrice         bool
	RateAdjusted     bool
	PriceRounded     bool
}

type PlannedComplement struct {
	Key      string
	LegacyID int32
	Name     string
	Price    Money
	TaxRate  int
	HasPrice bool
}

type PlannedGroup struct {
	Key     string
	Name    string
	Options []PlannedOption
}

type PlannedOption struct {
	Key            string
	ComplementKey  string
	ReferenceValue Money
}

type PlannedProductGroup struct {
	ProductKey string
	GroupKey   string
}

type PlannedDefault struct {
	ProductKey string
	GroupKey   string
	OptionKey  string
	Quantity   Quantity
}

type PlannedArea struct {
	Key       string
	Name      string
	SortOrder int
}

type PlannedTable struct {
	Key      string
	LegacyID int32
	AreaKey  string
	AreaName string
	Name     string
	Seats    int
	X        int
	Y        int
}

type PlannedUser struct {
	Key         string
	LegacyID    int32
	Username    string
	Original    string
	DisplayName string
	RoleCode    string
	Active      bool
	Renamed     bool
}

func legacyKey(id int32) string {
	return strconv.Itoa(int(id))
}

// latestPrices es el precio más reciente de cada producto o complemento (TOP 1 … ORDER BY fecha_inicio DESC del
// anterior, Seleccionar_Producto.cs:116); con la misma fecha, el último del paquete.
func latestPrices(prices []Price) map[int32]Price {
	latest := map[int32]Price{}
	for _, price := range prices {
		current, found := latest[price.Item]
		if !found || !price.StartsAt.Before(current.StartsAt) {
			latest[price.Item] = price
		}
	}
	return latest
}

// deduceRate deduce la tasa de IVA de un desglose guardado (la tasa no se guardaba: iva / subtotal × 100,
// ModificarProducto.cs:43) y elige la más cercana de 0, 8 y 16. exact dice si coincide con ella salvo el redondeo de
// MONEY (medio diezmilésimo).
func deduceRate(subtotal Money, tax Money) (rate int, exact bool) {
	if subtotal == 0 {
		return 0, tax == 0
	}
	magnitude := int64(subtotal)
	taxed := int64(tax)
	if magnitude < 0 {
		magnitude = -magnitude
		taxed = -taxed
	}
	best := 0
	var bestDistance int64 = -1
	for _, candidate := range []int{0, 8, 16} {
		distance := taxed*100 - magnitude*int64(candidate)
		if distance < 0 {
			distance = -distance
		}
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best, bestDistance <= 50
}

// buildCatalog arma la correspondencia del catálogo, las mesas y los usuarios. reservedUsernames son los nombres de
// usuario que ya existen en AD-EL POS fuera del lote (admin, sa): un usuario importado nunca los toma.
func buildCatalog(model *Model, reservedUsernames map[string]bool) CatalogPlan {
	plan := CatalogPlan{}
	categoryNames := map[string]bool{}
	categoryByID := map[int32]PlannedCategory{}
	for index, category := range model.Categories {
		planned := PlannedCategory{
			Key:       legacyKey(category.ID),
			LegacyID:  category.ID,
			Name:      uniqueName(cleanName(category.Name, 100, fmt.Sprintf("Categoría %d", category.ID)), categoryNames, 100),
			SortOrder: index,
		}
		plan.Categories = append(plan.Categories, planned)
		categoryByID[category.ID] = planned
	}
	// Un producto cuya categoría no existe (imposible en el anterior por su llave foránea) va a una categoría propia.
	var orphanCategory *PlannedCategory
	productPrices := latestPrices(model.ProductPrices)
	included := map[int32]bool{}
	for _, relation := range model.ProductComplements {
		if relation.Included.Valid && relation.Included.Value > 0 {
			included[relation.Product] = true
		}
	}
	for index, product := range model.Products {
		category, found := PlannedCategory{}, false
		if product.Category.Valid {
			category, found = categoryByID[product.Category.Value]
		}
		if !found {
			if orphanCategory == nil {
				orphan := PlannedCategory{
					Key:       "sin-categoria",
					Name:      uniqueName("Sin categoría (sistema anterior)", categoryNames, 100),
					SortOrder: len(plan.Categories),
				}
				plan.Categories = append(plan.Categories, orphan)
				orphanCategory = &orphan
			}
			category = *orphanCategory
		}
		name := cleanName(product.Name, 100, fmt.Sprintf("Producto %d", product.ID))
		planned := PlannedProduct{
			Key:              legacyKey(product.ID),
			LegacyID:         product.ID,
			CategoryKey:      category.Key,
			CategoryName:     category.Name,
			Code:             fmt.Sprintf("SA-%d", product.ID),
			Name:             name,
			PriceCanDecrease: included[product.ID],
			Active:           true,
			SortOrder:        index,
		}
		if short := cleanName(product.ShortName, 100, ""); short != "" && short != name {
			planned.Description = "Nombre corto en el sistema anterior: " + short
		}
		price, hasPrice := productPrices[product.ID]
		planned.HasPrice = hasPrice
		if hasPrice {
			planned.BasePrice = roundMoney(price.Total, 2)
			planned.PriceRounded = planned.BasePrice != price.Total
			rate, exact := deduceRate(price.Subtotal, price.Tax.Or(0))
			planned.TaxRate = rate
			planned.RateAdjusted = !exact
			if !price.Tax.Valid {
				planned.RateAdjusted = true
			}
			if planned.BasePrice < 0 {
				planned.BasePrice = 0
				planned.Active = false
				plan.Notes = append(plan.Notes, PlanNote{Code: "negative_price",
					Message: fmt.Sprintf("«%s» tenía un precio negativo: se importó en $0.00 y desactivado.", name)})
			}
		} else {
			planned.Active = false
			plan.Notes = append(plan.Notes, PlanNote{Code: "product_without_price",
				Message: fmt.Sprintf("«%s» no tenía precio: se importó desactivado, en $0.00.", name)})
		}
		if planned.RateAdjusted {
			plan.Notes = append(plan.Notes, PlanNote{Code: "rate_adjusted",
				Message: fmt.Sprintf("La tasa de IVA de «%s» no era exactamente 0, 8 o 16 %%: se importó con la más cercana, %d %%.", name, planned.TaxRate)})
		}
		if planned.PriceRounded {
			plan.Notes = append(plan.Notes, PlanNote{Code: "price_rounded",
				Message: fmt.Sprintf("El precio de «%s» tenía más de dos decimales: se importó redondeado a %s.", name, planned.BasePrice.Display())})
		}
		plan.Products = append(plan.Products, planned)
	}
	plan.buildComplements(model)
	plan.buildTables(model)
	plan.buildUsers(model, reservedUsernames)
	return plan
}

// buildComplements arma los complementos, un grupo por cada conjunto distinto de complementos que admite un producto
// (modo valor, con el precio del complemento como valor de referencia y sin extra explícito, para cobrar
// «(pedido − incluido) × precio» como el anterior) y la cantidad incluida de cada producto.
func (plan *CatalogPlan) buildComplements(model *Model) {
	names := map[string]bool{}
	complementPrices := latestPrices(model.ComplementPrices)
	complementByID := map[int32]PlannedComplement{}
	for _, complement := range model.Complements {
		planned := PlannedComplement{
			Key:      legacyKey(complement.ID),
			LegacyID: complement.ID,
			Name:     uniqueName(cleanName(complement.Name, 100, fmt.Sprintf("Complemento %d", complement.ID)), names, 100),
		}
		if price, found := complementPrices[complement.ID]; found {
			planned.HasPrice = true
			planned.Price = roundMoney(price.Total, 2)
			planned.TaxRate, _ = deduceRate(price.Subtotal, price.Tax.Or(0))
			if planned.Price < 0 {
				planned.Price = 0
			}
		} else {
			plan.Notes = append(plan.Notes, PlanNote{Code: "complement_without_price",
				Message: fmt.Sprintf("El complemento «%s» no tenía precio: se importó con valor $0.00.", planned.Name)})
		}
		plan.Complements = append(plan.Complements, planned)
		complementByID[complement.ID] = planned
	}
	// Conjuntos de complementos por producto, solo con productos y complementos que existen.
	products := map[int32]bool{}
	for _, product := range plan.Products {
		products[product.LegacyID] = true
	}
	sets := map[int32][]int32{}
	includedQuantity := map[[2]int32]Quantity{}
	for _, relation := range model.ProductComplements {
		if !products[relation.Product] {
			continue
		}
		if _, found := complementByID[relation.Complement]; !found {
			continue
		}
		key := [2]int32{relation.Product, relation.Complement}
		if _, repeated := includedQuantity[key]; repeated {
			continue
		}
		sets[relation.Product] = append(sets[relation.Product], relation.Complement)
		quantity := Quantity(0)
		if relation.Included.Valid && relation.Included.Value > 0 {
			quantity = relation.Included.Value
		}
		includedQuantity[key] = quantity
	}
	usage := map[string]int{}
	setMembers := map[string][]int32{}
	productSet := map[int32]string{}
	for product, members := range sets {
		sort.Slice(members, func(left, right int) bool {
			return members[left] < members[right]
		})
		parts := make([]string, len(members))
		for index, member := range members {
			parts[index] = legacyKey(member)
		}
		key := strings.Join(parts, ",")
		usage[key]++
		setMembers[key] = members
		productSet[product] = key
	}
	keys := make([]string, 0, len(usage))
	for key := range usage {
		keys = append(keys, key)
	}
	// El conjunto más usado se llama «Complementos»; los demás, «Complementos: A, B, C».
	sort.Slice(keys, func(left, right int) bool {
		if usage[keys[left]] != usage[keys[right]] {
			return usage[keys[left]] > usage[keys[right]]
		}
		return keys[left] < keys[right]
	})
	groupNames := map[string]bool{}
	groupKeys := map[string]bool{}
	for index, key := range keys {
		name := complementsGroupName
		if index > 0 {
			labels := []string{}
			for _, member := range setMembers[key] {
				labels = append(labels, complementByID[member].Name)
			}
			name = truncateRunes(complementsGroupName+": "+strings.Join(labels, ", "), 100)
		}
		group := PlannedGroup{Key: key, Name: uniqueName(name, groupNames, 100)}
		for _, member := range setMembers[key] {
			complement := complementByID[member]
			group.Options = append(group.Options, PlannedOption{
				Key:            key + ":" + complement.Key,
				ComplementKey:  complement.Key,
				ReferenceValue: complement.Price,
			})
		}
		plan.Groups = append(plan.Groups, group)
		groupKeys[key] = true
	}
	for _, product := range plan.Products {
		key, found := productSet[product.LegacyID]
		if !found {
			continue
		}
		plan.ProductGroups = append(plan.ProductGroups, PlannedProductGroup{ProductKey: product.Key, GroupKey: key})
		for _, member := range setMembers[key] {
			quantity := includedQuantity[[2]int32{product.LegacyID, member}]
			if quantity <= 0 {
				continue
			}
			plan.Defaults = append(plan.Defaults, PlannedDefault{
				ProductKey: product.Key,
				GroupKey:   key,
				OptionKey:  key + ":" + legacyKey(member),
				Quantity:   quantity,
			})
		}
	}
	// El IVA de un complemento lo desglosa AD-EL POS con la tasa del producto: se avisa si un complemento tenía otra.
	for _, complement := range plan.Complements {
		if complement.TaxRate != 0 {
			plan.Notes = append(plan.Notes, PlanNote{Code: "complement_rate",
				Message: fmt.Sprintf("El complemento «%s» tenía IVA del %d %%; en AD-EL POS su precio se desglosa con la tasa del producto.", complement.Name, complement.TaxRate)})
		}
	}
}

// buildTables acomoda las mesas (sin «para llevar») en áreas «Salón» de hasta 48 mesas, en una cuadrícula de 8 por
// renglón dentro del lienzo de 1000 × 560, con su capacidad (4 si no la tenía; entre 1 y 40).
//
// Etapa M1 (M1-D5): las secciones de una mesa divisible van juntas, en el orden de sus letras, donde aparece la
// primera; si no caben en lo que queda del renglón empiezan uno nuevo (y si no caben en el área, la siguiente). La mesa
// «Mesa N» que se volvió el grupo no ocupa lugar. Sin grupos, el acomodo es el de siempre.
func (plan *CatalogPlan) buildTables(model *Model) {
	tables := []DiningTable{}
	for _, table := range model.Tables {
		if table.ID != takeawayTableID {
			tables = append(tables, table)
		}
	}
	grouping := detectTableGroups(tables)
	plan.Notes = append(plan.Notes, grouping.notes...)
	names := map[string]map[string]bool{}
	slot := 0
	place := func(table DiningTable) PlannedTable {
		areaNumber := slot / tablesPerArea
		for areaNumber >= len(plan.Areas) {
			number := len(plan.Areas)
			key := "salon"
			name := areaNamePrefix
			if number > 0 {
				key = fmt.Sprintf("salon-%d", number+1)
				name = fmt.Sprintf("%s %d", areaNamePrefix, number+1)
			}
			plan.Areas = append(plan.Areas, PlannedArea{Key: key, Name: name, SortOrder: number})
			names[key] = map[string]bool{}
		}
		area := plan.Areas[areaNumber]
		cell := slot % tablesPerArea
		slot++
		planned := PlannedTable{
			Key:      legacyKey(table.ID),
			LegacyID: table.ID,
			AreaKey:  area.Key,
			AreaName: area.Name,
			Name:     uniqueName(legacyTableName(table), names[area.Key], 60),
			Seats:    legacySeats(table),
			X:        (cell%tablesPerRow)*tableCellX + tableMarginX,
			Y:        (cell/tablesPerRow)*tableCellY + tableMarginY,
		}
		plan.Tables = append(plan.Tables, planned)
		return planned
	}
	placedGroups := map[int]bool{}
	for _, table := range tables {
		if grouping.absorbed[table.ID] {
			continue
		}
		index, section := grouping.sectionOf[table.ID]
		if !section {
			place(table)
			continue
		}
		if placedGroups[index] {
			continue
		}
		placedGroups[index] = true
		group := grouping.groups[index]
		size := len(group.sections)
		if column := (slot % tablesPerArea) % tablesPerRow; size <= tablesPerRow && column+size > tablesPerRow {
			slot += tablesPerRow - column
		}
		if cell := slot % tablesPerArea; size <= tablesPerArea && cell+size > tablesPerArea {
			slot += tablesPerArea - cell
		}
		planned := PlannedTableGroup{Key: legacyGroupKey(group.number), Name: group.name, Base: group.base}
		for _, sectionTable := range group.sections {
			placed := place(sectionTable)
			planned.AreaKey = placed.AreaKey
			planned.TableKeys = append(planned.TableKeys, placed.Key)
			planned.SectionNames = append(planned.SectionNames, placed.Name)
		}
		plan.TableGroups = append(plan.TableGroups, planned)
		if group.base != nil {
			// La mesa que se volvió el grupo: sus ventas conservan su nombre, en el área del grupo, sin mesa viva.
			area := plan.Areas[len(plan.Areas)-1]
			for _, candidate := range plan.Areas {
				if candidate.Key == planned.AreaKey {
					area = candidate
				}
			}
			plan.AbsorbedTables = append(plan.AbsorbedTables, PlannedTable{Key: legacyKey(group.base.ID),
				LegacyID: group.base.ID, AreaKey: area.Key, AreaName: area.Name, Name: group.name,
				Seats: legacySeats(*group.base)})
		}
	}
}

// legacySeats es la capacidad de una mesa del anterior: 4 si no la tenía; entre 1 y 40.
func legacySeats(table DiningTable) int {
	seats := defaultSeats
	if table.Capacity.Valid {
		seats = int(table.Capacity.Value)
	}
	if seats < 1 {
		seats = 1
	}
	if seats > maxSeats {
		seats = maxSeats
	}
	return seats
}

// roleForPosition es la correspondencia de puestos del anterior con los roles predeterminados de AD-EL POS
// (MIGRACION_ANALISIS.md §6.4): Administrador → administrator; Cajero → cashier; Mesero → waiter; Cocinero →
// kitchen; Limpieza y cualquier otro → waiter (no hay rol sin permisos; quedan sin acceso igual).
func roleForPosition(position string) (string, bool) {
	switch foldText(position) {
	case "administrador", "administradora", "admin":
		return "administrator", true
	case "cajero", "cajera":
		return "cashier", true
	case "mesero", "mesera":
		return "waiter", true
	case "cocinero", "cocinera", "cocina":
		return "kitchen", true
	case "limpieza":
		return "waiter", true
	}
	return "waiter", false
}

// buildUsers arma los usuarios: sin contraseña (sin acceso hasta «Asignar contraseña»), nombre de usuario válido en
// minúsculas, nombre visible desde personal, activo según el anterior y rol según su puesto.
func (plan *CatalogPlan) buildUsers(model *Model, reserved map[string]bool) {
	people := map[int32]Person{}
	for _, person := range model.People {
		people[person.ID] = person
	}
	positions := map[int32]string{}
	for _, position := range model.Positions {
		positions[position.ID] = position.Name
	}
	used := map[string]bool{}
	for name := range reserved {
		used[name] = true
	}
	for _, user := range model.Users {
		username, renamed := deriveUsername(user.Username, user.ID, used)
		person, hasPerson := Person{}, false
		if user.Person.Valid {
			person, hasPerson = people[user.Person.Value]
		}
		display := cleanName(user.Username, 100, username)
		if hasPerson {
			display = cleanName(strings.Join([]string{person.Name, person.Paternal, person.Maternal}, " "), 100, display)
		}
		role, known := "waiter", false
		if hasPerson && person.Position.Valid {
			role, known = roleForPosition(positions[person.Position.Value])
		}
		active := !user.Active.Is(0) && !(hasPerson && person.Active.Is(0))
		planned := PlannedUser{
			Key:         legacyKey(user.ID),
			LegacyID:    user.ID,
			Username:    username,
			Original:    user.Username,
			DisplayName: display,
			RoleCode:    role,
			Active:      active,
			Renamed:     renamed,
		}
		plan.Users = append(plan.Users, planned)
		if renamed {
			plan.Notes = append(plan.Notes, PlanNote{Code: "username_changed",
				Message: fmt.Sprintf("El usuario «%s» se importó como «%s».", cleanName(user.Username, 40, "(sin nombre)"), username)})
		}
		if !known {
			plan.Notes = append(plan.Notes, PlanNote{Code: "role_default",
				Message: fmt.Sprintf("«%s» no tenía un puesto conocido: se importó como Mesero (sin acceso hasta que le asignes contraseña).", display)})
		}
	}
}

// priceAt es el precio de un producto o complemento vigente en un instante: el más reciente con fecha de inicio en o
// antes de él (así lo elegía el anterior al vender) o, si no hay, el más antiguo (found = false).
func priceAt(prices []Price, instant time.Time) (Price, bool) {
	var chosen Price
	have := false
	for _, price := range prices {
		if price.StartsAt.After(instant) {
			continue
		}
		if !have || !price.StartsAt.Before(chosen.StartsAt) {
			chosen, have = price, true
		}
	}
	if have {
		return chosen, true
	}
	for _, price := range prices {
		if !have || price.StartsAt.Before(chosen.StartsAt) {
			chosen, have = price, true
		}
	}
	return chosen, false
}
