package administration

import (
	"slices"
	"sort"
)

// Catálogo de permisos agrupado de la etapa A3 (§3 de docs/etapas/ETAPA_A3_AJUSTES.md; decisiones A3-D2 a A3-D6 del
// coordinador en docs/ETAPA_A3_AVANCE.md). Aquí, en un solo lugar, vive todo lo que la pantalla de permisos de un rol
// necesita saber: el grupo de cada permiso, su descripción corta, lo que necesita para servir, las capacidades de la
// vista sencilla y los conjuntos de fábrica de cada rol. La interfaz lo lee de GET permissions/catalog y nunca tiene
// listas propias. Nada de esto cambia la seguridad: cada ruta sigue validando cada permiso (Principal.require).

// Grupos de la pantalla (§3.2), en este orden.
const (
	permissionGroupTables    = "tables"
	permissionGroupOrders    = "orders"
	permissionGroupTakeaway  = "takeaway"
	permissionGroupKitchen   = "kitchen"
	permissionGroupCash      = "cash"
	permissionGroupCatalog   = "catalog"
	permissionGroupInventory = "inventory"
	permissionGroupReports   = "reports"
	permissionGroupStaff     = "staff"
	permissionGroupSystem    = "system"
)

var permissionGroups = []struct {
	code string
	name string
}{
	{code: permissionGroupTables, name: "Mesas y cuentas"},
	{code: permissionGroupOrders, name: "Captura y pedidos"},
	{code: permissionGroupTakeaway, name: "Para llevar"},
	{code: permissionGroupKitchen, name: "Cocina e impresión"},
	{code: permissionGroupCash, name: "Caja y cobros"},
	{code: permissionGroupCatalog, name: "Catálogo"},
	{code: permissionGroupInventory, name: "Inventario"},
	{code: permissionGroupReports, name: "Reportes"},
	{code: permissionGroupStaff, name: "Personal"},
	{code: permissionGroupSystem, name: "Terminales y sistema"},
}

// catalogEntry es lo que el catálogo agrega a un permiso registrado (permission_catalog_entries.go).
type catalogEntry struct {
	key         string
	group       string
	name        string
	description string
	requires    []string
}

// PermissionGroup es un área de la pantalla con sus permisos, en orden.
type PermissionGroup struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// CatalogPermission es un permiso con su nombre visible, su descripción corta, su grupo y lo que necesita.
type CatalogPermission struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	Requires    []string `json:"requires"`
}

// PermissionCapability es una opción de la vista sencilla: marca o quita su conjunto de permisos (con lo que necesitan,
// lo agrega la interfaz con requires) y se ve parcial si el rol tiene solo una parte.
type PermissionCapability struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// PermissionCatalog es la respuesta de GET permissions/catalog.
type PermissionCatalog struct {
	Groups       []PermissionGroup      `json:"groups"`
	Permissions  []CatalogPermission    `json:"permissions"`
	Capabilities []PermissionCapability `json:"capabilities"`
	// FactoryRoles son los códigos de los roles que tienen «Restablecer permisos predeterminados» (A3-D6).
	FactoryRoles []string `json:"factory_roles"`
}

// permissionCapabilities son las opciones de la vista sencilla (§3.2 y A3-D5): las nueve de la especificación más
// «Manejar inventario» y «Autorizar y corregir», sin las que Inventarios y Supervisor no se arman desde ella.
var permissionCapabilities = []PermissionCapability{
	{
		Code:        "tables",
		Name:        "Atender mesas",
		Description: "Entrar con PIN, abrir cuentas, capturar y enviar pedidos e imprimir la precuenta.",
		Permissions: []string{
			"waiter_devices.use", "restaurant.areas.read", "restaurant.tables.read", "accounts.read", "accounts.open",
			"orders.capture", "orders.confirm", "catalog.read", "catalog.prices.read", "events.read",
			"payments.tickets.print",
		},
	},
	{
		Code:        "takeaway",
		Name:        "Para llevar",
		Description: "Tomar pedidos para llevar y marcarlos como entregados.",
		Permissions: []string{"takeaway.read", "takeaway.manage", "takeaway.deliver"},
	},
	{
		Code:        "collect",
		Name:        "Cobrar",
		Description: "Cobrar cuentas con propina, descuentos dentro del límite y cargos, e imprimir tickets.",
		Permissions: []string{
			"payments.read", "payments.collect", "payments.collect_partial", "payments.collect_selection",
			"payments.tips.record", "payments.discounts.apply", "payments.charges.create", "payments.tickets.print",
			"payments.tickets.reprint",
		},
	},
	{
		Code:        "cash",
		Name:        "Manejar caja",
		Description: "Abrir y cerrar el turno, arqueos, cortes, ingresos, retiros, gastos y el cajón.",
		Permissions: []string{
			"cash.shifts.read", "cash.shifts.open", "cash.counts.perform", "cash.shifts.close", "cash.partial_cuts.create",
			"cash.income.record", "cash.withdrawals.record", "cash.expenses.record", "cash.movements.read",
			"cash.reports.print", "cash.drawer.open", "payments.usd_rate.manage", "registers.read",
		},
	},
	{
		Code:        "kitchen",
		Name:        "Cocina",
		Description: "Ver la pantalla de preparación y marcar los platillos en preparación o listos.",
		Permissions: []string{
			"kitchen.stations.read", "kitchen.tickets.read", "kitchen.states.read", "kitchen.states.manage",
			"kitchen.kds.ack", "kitchen.history.read", "kitchen.reprint", "events.read",
		},
	},
	{
		Code:        "inventory",
		Name:        "Manejar inventario",
		Description: "Insumos, recetas, compras, recepciones, mermas, conteos y transferencias.",
		Permissions: []string{
			"inventory.read", "inventory.items.manage", "inventory.stock.read", "inventory.movements.read",
			"inventory.costs.read", "inventory.recipes.manage", "inventory.subrecipes.manage",
			"inventory.production.record", "inventory.waste.record", "inventory.suppliers.manage",
			"inventory.purchases.create", "inventory.purchases.receive", "inventory.supplier_returns.record",
			"inventory.adjustments.record", "inventory.counts.perform", "inventory.transfers.record", "catalog.read",
		},
	},
	{
		Code:        "authorize",
		Name:        "Autorizar y corregir",
		Description: "Autorizar descuentos, devoluciones, retiros y correcciones, y modificar o cancelar lo enviado.",
		Permissions: []string{
			"orders.edit", "orders.cancel", "accounts.cancel", "payments.discounts.authorize", "payments.courtesy",
			"payments.charges.manage", "payments.corrections", "payments.refunds.execute", "payments.refunds.authorize",
			"cash.outflows.authorize", "cash.corrections.record", "cash.corrections.authorize", "cash.closing.cancel_any",
		},
	},
	{
		Code:        "reports",
		Name:        "Ver reportes",
		Description: "Consultar y exportar todos los reportes.",
		Permissions: []string{
			"reports.read", "reports.sales.read", "reports.payments.read", "reports.discounts.read", "reports.cash.read",
			"reports.inventory.read", "reports.purchases.read", "reports.employees.read", "reports.export.pdf",
			"reports.export.excel",
		},
	},
	{
		Code:        "catalog",
		Name:        "Administrar catálogo",
		Description: "Productos, precios, categorías, combos, promociones, imágenes y disponibilidad.",
		Permissions: []string{
			"catalog.read", "catalog.prices.read", "catalog.prices.manage", "catalog.products.manage",
			"catalog.products.state", "catalog.categories.manage", "catalog.ingredients.manage",
			"catalog.complements.manage", "catalog.groups.manage", "catalog.combos.manage", "catalog.price_lists.manage",
			"catalog.promotions.manage", "catalog.availability.manage", "catalog.availability.override",
			"catalog.images.manage", "catalog.sold_out.manage",
		},
	},
	{
		Code:        "staff",
		Name:        "Administrar personal",
		Description: "Usuarios, roles, permisos y sesiones.",
		Permissions: []string{
			"users.read", "users.manage", "roles.read", "roles.manage", "permissions.read", "permissions.manage",
			"sessions.read", "sessions.manage",
		},
	},
	{
		Code:        "system",
		Name:        "Configurar el sistema",
		Description: "Terminales, cajas, documentos, impresoras, áreas y mesas, y la configuración general.",
		Permissions: []string{
			"settings.read", "settings.manage", "settings.features.manage", "devices.read", "devices.manage",
			"registers.manage", "audit.read", "documents.templates.read", "documents.templates.manage",
			"documents.preview.real", "documents.samples.print", "documents.identity.manage",
			"documents.history.render_current", "kitchen.stations.manage", "kitchen.routing.manage",
			"kitchen.printers.manage", "kitchen.formats.manage", "kitchen.timing.manage", "restaurant.areas.manage",
			"restaurant.tables.manage", "restaurant.settings.manage", "payments.methods.manage",
			"payments.settings.manage", "payments.tax.manage", "cash.settings.manage", "inventory.settings.manage",
		},
	},
}

// factoryRoleCodes son los roles de fábrica con permisos predeterminados (A3-D6), en el orden del asistente. El
// Administrador no está: siempre recibe todos (setup.go) y nadie edita su propio rol; tampoco el soporte reservado.
var factoryRoleCodes = []string{"waiter", "cashier", "kitchen", "supervisor", "inventory", "finance"}

// factoryRolePermissions son los conjuntos de fábrica de la Fase 0 (§0.4 y su anexo, A3-D2 y A3-D3), cerrados bajo
// requires (lo comprueba permission_catalog_test.go). Se conceden solo en el asistente de una instalación nueva y con
// «Restablecer permisos predeterminados»: una versión nueva de esta lista nunca cambia un rol sin que alguien toque el
// botón (la 0027 no toca roles). El límite de descuento y las excepciones por usuario no son parte del conjunto.
var factoryRolePermissions = map[string][]string{
	// Mesero: todo lo de la tabla de docs/ANDROID_INSTALAR.md §4, con Para llevar e imprimir la precuenta, salvo
	// «Modificar» (orders.edit) y «Cancelar unidades» (orders.cancel), que quedan opcionales (§3.1). «Entregado»
	// (takeaway.deliver) va marcado (A3-D3): Go sigue sin entregar un pedido sin pagar (BALANCE_PENDING).
	"waiter": {
		"accounts.open", "accounts.read", "catalog.prices.read", "catalog.read", "events.read", "orders.capture",
		"orders.confirm", "payments.tickets.print", "restaurant.areas.read", "restaurant.tables.read",
		"takeaway.deliver", "takeaway.manage", "takeaway.read", "waiter_devices.use",
	},
	// Cajero: caja, cobro y toma de pedidos en el mostrador; sin autorizaciones (retiros, correcciones, devoluciones,
	// descuentos mayores), que son del supervisor. Los descuentos, dentro del límite del rol (0 % si no tiene).
	"cashier": {
		"accounts.open", "accounts.read", "cash.counts.perform", "cash.drawer.open", "cash.expenses.record",
		"cash.income.record", "cash.movements.read", "cash.partial_cuts.create", "cash.reports.print",
		"cash.shifts.close", "cash.shifts.open", "cash.shifts.read", "cash.withdrawals.record", "catalog.prices.read",
		"catalog.read", "events.read", "kitchen.states.read", "kitchen.stations.read", "orders.capture",
		"orders.confirm", "payments.charges.create", "payments.collect", "payments.collect_partial",
		"payments.collect_selection", "payments.discounts.apply", "payments.read", "payments.tickets.print",
		"payments.tickets.reprint", "payments.tips.record", "payments.usd_rate.manage", "registers.read",
		"restaurant.areas.read", "restaurant.tables.read", "takeaway.deliver", "takeaway.manage", "takeaway.read",
	},
	// Cocina: la pantalla de preparación de sus estaciones; sin todas las estaciones ni la configuración.
	"kitchen": {
		"events.read", "kitchen.history.read", "kitchen.kds.ack", "kitchen.reprint", "kitchen.states.manage",
		"kitchen.states.read", "kitchen.stations.read", "kitchen.tickets.read",
	},
	// Supervisor: lo del cajero y la cocina más modificar y cancelar lo enviado, las cuentas, las autorizaciones de
	// cobro y de caja, la cola de impresión, la disponibilidad del catálogo, la consulta de personal y terminales, el
	// Registro de movimientos y los reportes de la operación. Sin administrar catálogo, personal ni sistema.
	"supervisor": {
		"accounts.cancel", "accounts.manage", "accounts.merge", "accounts.move", "accounts.open", "accounts.read",
		"accounts.split", "accounts.transfer", "accounts.transfer_items", "audit.read", "cash.closing.cancel_any",
		"cash.closures.review", "cash.corrections.authorize", "cash.corrections.record", "cash.counts.perform",
		"cash.drawer.open", "cash.expected.read", "cash.expenses.record", "cash.history.read", "cash.income.record",
		"cash.movements.read", "cash.outflows.authorize", "cash.partial_cuts.create", "cash.reports.print",
		"cash.reports.reprint", "cash.shifts.close", "cash.shifts.open", "cash.shifts.read", "cash.shifts.read_all",
		"cash.withdrawals.record", "catalog.availability.manage", "catalog.availability.override",
		"catalog.prices.read", "catalog.read", "catalog.sold_out.manage", "devices.read", "events.read",
		"kitchen.history.read", "kitchen.kds.ack", "kitchen.printers.test", "kitchen.queue.read",
		"kitchen.queue.resolve", "kitchen.queue.retry", "kitchen.release", "kitchen.release_early", "kitchen.reprint",
		"kitchen.states.manage", "kitchen.states.read", "kitchen.stations.read", "kitchen.tickets.read",
		"orders.cancel", "orders.capture", "orders.confirm", "orders.edit", "orders.history.read",
		"payments.charges.create", "payments.charges.manage", "payments.collect", "payments.collect_partial",
		"payments.collect_selection", "payments.corrections", "payments.courtesy", "payments.discounts.apply",
		"payments.discounts.authorize", "payments.read", "payments.refunds.authorize", "payments.refunds.execute",
		"payments.tickets.print", "payments.tickets.reprint", "payments.tips.record", "payments.usd_rate.manage",
		"registers.read", "reports.cash.read", "reports.discounts.read", "reports.employees.read",
		"reports.export.excel", "reports.export.pdf", "reports.payments.read", "reports.read", "reports.sales.read",
		"restaurant.areas.read", "restaurant.tables.read", "sessions.manage", "sessions.read", "takeaway.deliver",
		"takeaway.manage", "takeaway.read", "users.read", "waiter_devices.use",
	},
	// Inventarios: insumos, recetas, compras, recepciones, mermas, conteos y transferencias, con sus reportes; sin
	// autorizar (ajustes, conteos, compras), costos de referencia, valuación ni parámetros.
	"inventory": {
		"catalog.read", "inventory.adjustments.record", "inventory.costs.read", "inventory.counts.perform",
		"inventory.items.manage", "inventory.movements.read", "inventory.production.record",
		"inventory.purchases.create", "inventory.purchases.receive", "inventory.read", "inventory.recipes.manage",
		"inventory.stock.read", "inventory.subrecipes.manage", "inventory.supplier_returns.record",
		"inventory.suppliers.manage", "inventory.transfers.record", "inventory.waste.record",
		"reports.inventory.read", "reports.purchases.read", "reports.read",
	},
	// Finanzas: todos los reportes, la consulta de cobros, caja e inventario con sus costos, autorizar compras y
	// vincularlas con gastos de caja, y el Registro de movimientos.
	"finance": {
		"accounts.read", "audit.read", "cash.closures.review", "cash.expected.read", "cash.history.read",
		"cash.movements.read", "cash.shifts.read", "cash.shifts.read_all", "inventory.costs.read",
		"inventory.movements.read", "inventory.purchases.authorize", "inventory.purchases.link_cash", "inventory.read",
		"inventory.stock.read", "inventory.valuation.read", "orders.history.read", "payments.read", "registers.read",
		"reports.cash.read", "reports.discounts.read", "reports.employees.read", "reports.export.excel",
		"reports.export.pdf", "reports.inventory.read", "reports.payments.read", "reports.purchases.read",
		"reports.read", "reports.sales.read",
	},
}

// factoryPermissions devuelve el conjunto de fábrica de un rol por su código, ordenado; false si el rol no tiene (un rol
// creado por el usuario, el Administrador o el soporte reservado).
func factoryPermissions(code string) ([]string, bool) {
	keys, found := factoryRolePermissions[code]
	if !found {
		return nil, false
	}
	sorted := slices.Clone(keys)
	sort.Strings(sorted)
	return sorted, true
}

// FactoryRolePermissions es el conjunto de fábrica de un rol por su código, ordenado (vacío si no tiene). Lo usan las
// pruebas de otros paquetes que comprueban lo que concede el asistente.
func FactoryRolePermissions(code string) []string {
	keys, _ := factoryPermissions(code)
	return keys
}

// permissionNames son los nombres visibles que registran los módulos (Permission.Description).
func permissionNames() map[string]string {
	names := make(map[string]string, len(Permissions))
	for _, permission := range Permissions {
		names[permission.Key] = permission.Description
	}
	return names
}

// buildPermissionCatalog arma la respuesta de GET permissions/catalog con los permisos registrados y su entrada del
// catálogo. Un permiso registrado sin entrada no aparece (la prueba unitaria lo impide).
func buildPermissionCatalog() PermissionCatalog {
	names := permissionNames()
	catalog := PermissionCatalog{
		Groups:       []PermissionGroup{},
		Permissions:  []CatalogPermission{},
		Capabilities: permissionCapabilities,
		FactoryRoles: factoryRoleCodes,
	}
	byGroup := map[string][]string{}
	for _, entry := range permissionCatalogEntries {
		name, registered := names[entry.key]
		if !registered {
			continue
		}
		if entry.name != "" {
			name = entry.name
		}
		requires := entry.requires
		if requires == nil {
			requires = []string{}
		}
		catalog.Permissions = append(catalog.Permissions, CatalogPermission{
			Key:         entry.key,
			Name:        name,
			Description: entry.description,
			Group:       entry.group,
			Requires:    requires,
		})
		byGroup[entry.group] = append(byGroup[entry.group], entry.key)
	}
	for _, group := range permissionGroups {
		keys := byGroup[group.code]
		if keys == nil {
			keys = []string{}
		}
		catalog.Groups = append(catalog.Groups, PermissionGroup{Code: group.code, Name: group.name, Permissions: keys})
	}
	return catalog
}

// permissionCatalogRoute es GET permissions/catalog: lo mismo que pide GET permissions (permissions.read).
func permissionCatalogRoute(principal Principal) (any, error) {
	if err := principal.require("permissions.read"); err != nil {
		return nil, err
	}
	return buildPermissionCatalog(), nil
}

// closePermissions agrega a un conjunto lo que necesitan sus permisos, hasta que no falte nada (sin repetir, ordenado).
// La interfaz hace lo mismo con requires al marcar un permiso; aquí lo usan las pruebas de los conjuntos de fábrica.
func closePermissions(keys []string) []string {
	requires := map[string][]string{}
	for _, entry := range permissionCatalogEntries {
		requires[entry.key] = entry.requires
	}
	closed := map[string]bool{}
	pending := slices.Clone(keys)
	for len(pending) > 0 {
		key := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if closed[key] {
			continue
		}
		closed[key] = true
		pending = append(pending, requires[key]...)
	}
	result := make([]string, 0, len(closed))
	for key := range closed {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
