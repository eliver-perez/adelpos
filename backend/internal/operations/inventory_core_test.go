//go:build integration

package operations_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa 8, fase A: núcleo de inventario (0010 y paquete inventory). Se encadena al final de la
// aceptación con sus propios usuarios, insumos y almacenes para no depender del estado de pruebas
// anteriores (solo reutiliza las estaciones de cocina que ya existen).

type inventoryFixture struct {
	*restaurantFixture
	// stock consulta existencias y movimientos sin costos; costs además ve costos y valuación;
	// clerk administra insumos sin poder fijar costos; outsider solo lee eventos.
	stock    testTerminal
	costs    testTerminal
	clerk    testTerminal
	outsider testTerminal
	units    map[string]inventory.Unit
	general  inventory.Warehouse
	adminID  string
}

// inventoryUser crea un rol a medida, un usuario y una terminal enrolada con sesión iniciada.
func inventoryUser(t *testing.T, fixture *restaurantFixture, username string, permissions []string) testTerminal {
	t.Helper()
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.adminCall(t, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role.ID, "active": true, "password": password,
	}, 200)
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	terminal := testTerminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200))
	terminal.sessionToken = login.Token
	return terminal
}

// write envía una escritura de inventario con un operation_id nuevo.
func (fixture *inventoryFixture) write(t *testing.T, terminal testTerminal, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	copied := map[string]any{"operation_id": randomID()}
	for key, value := range body {
		copied[key] = value
	}
	return fixture.call(t, terminal, method, route, copied, status)
}

func (fixture *inventoryFixture) adminWrite(t *testing.T, method, route string, body map[string]any, status int) []byte {
	t.Helper()
	return fixture.write(t, fixture.admin, method, route, body, status)
}

func testInventoryCore(t *testing.T, base *restaurantFixture) {
	fixture := &inventoryFixture{restaurantFixture: base, units: map[string]inventory.Unit{}}
	if err := base.pool.QueryRow(base.ctx, "SELECT id FROM app.users WHERE username='admin'").Scan(&fixture.adminID); err != nil {
		t.Fatal(err)
	}
	t.Run("inventory_seed_and_permissions", func(t *testing.T) {
		testInventorySeed(t, fixture)
	})
	fixture.stock = inventoryUser(t, base, "inventario_existencias", []string{
		"events.read", "inventory.read", "inventory.stock.read", "inventory.movements.read",
	})
	fixture.costs = inventoryUser(t, base, "inventario_costos", []string{
		"events.read", "inventory.read", "inventory.stock.read", "inventory.movements.read", "inventory.costs.read",
		"inventory.valuation.read",
	})
	fixture.clerk = inventoryUser(t, base, "inventario_capturista", []string{"inventory.read", "inventory.items.manage"})
	fixture.outsider = inventoryUser(t, base, "inventario_ajeno", []string{"events.read", "kitchen.states.read"})
	t.Run("inventory_units_and_categories", func(t *testing.T) {
		testInventoryUnitsAndCategories(t, fixture)
	})
	t.Run("inventory_items_and_presentations", func(t *testing.T) {
		testInventoryItems(t, fixture)
	})
	t.Run("inventory_warehouses_and_stations", func(t *testing.T) {
		testInventoryWarehouses(t, fixture)
	})
	t.Run("inventory_initial_balances", func(t *testing.T) {
		testInventoryInitialBalances(t, fixture)
	})
	t.Run("inventory_ledger_consistency", func(t *testing.T) {
		testInventoryLedger(t, fixture)
	})
	t.Run("inventory_cost_visibility", func(t *testing.T) {
		testInventoryCostVisibility(t, fixture)
	})
	t.Run("inventory_settings_feature_and_events", func(t *testing.T) {
		testInventorySettingsAndFeature(t, fixture)
	})
	// Fase B: recetas, subrecetas, porciones, elaboraciones y consumo por liberación.
	testInventoryRecipesAndConsumption(t, fixture)
}

// testInventorySeed comprueba las siembras de referencia de la 0010 y que los permisos de
// inventario de Go y de PostgreSQL coinciden y solo los tiene el administrador.
func testInventorySeed(t *testing.T, fixture *inventoryFixture) {
	var units string
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT string_agg(code||':'||dimension||':'||factor::text||':'||system,'|' ORDER BY code)
        FROM app.inventory_units`).Scan(&units)
	if err != nil {
		t.Fatal(err)
	}
	expected := "g:mass:1.0000000000:true|kg:mass:1000.0000000000:true|l:volume:1000.0000000000:true|" +
		"ml:volume:1.0000000000:true|porcion:portion:1.0000000000:true|pza:count:1.0000000000:true"
	if units != expected {
		t.Fatalf("Unidades sembradas: %s", units)
	}
	warehouses := decodeResponse[[]inventory.Warehouse](t, fixture.adminCall(t, "GET", "inventory/warehouses", nil, 200))
	if len(warehouses) != 1 || warehouses[0].Name != "Almacén general" || !warehouses[0].IsDefault || !warehouses[0].Active {
		t.Fatalf("Almacén predeterminado: %+v", warehouses)
	}
	fixture.general = warehouses[0]
	var enabled bool
	var enabledAt *string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT enabled,enabled_at::text FROM app.establishment_features
        WHERE code='inventory_control'`).Scan(&enabled, &enabledAt); err != nil {
		t.Fatal(err)
	}
	if enabled || enabledAt != nil {
		t.Fatalf("La función de inventario debe iniciar desactivada: %v %v", enabled, enabledAt)
	}
	settings := decodeResponse[inventory.Settings](t, fixture.adminCall(t, "GET", "inventory/settings", nil, 200))
	if !settings.PurchaseTaxInCost || settings.OperatingDayCutoff != "05:00" || !settings.WarnProductsWithoutRecipe ||
		settings.CountFrequency != "none" || settings.CountIntervalDays != nil || settings.Revision != 1 {
		t.Fatalf("Configuración sembrada: %+v", settings)
	}
	// Go ↔ SQL: mismas claves y descripciones.
	rows, err := fixture.pool.Query(fixture.ctx, "SELECT key,description FROM app.permissions WHERE key LIKE 'inventory.%' ORDER BY key")
	if err != nil {
		t.Fatal(err)
	}
	database := map[string]string{}
	for rows.Next() {
		var key, description string
		if err := rows.Scan(&key, &description); err != nil {
			t.Fatal(err)
		}
		database[key] = description
	}
	rows.Close()
	if len(database) != len(inventory.Permissions) {
		t.Fatalf("Permisos de inventario: %d en PostgreSQL y %d en Go", len(database), len(inventory.Permissions))
	}
	for _, permission := range inventory.Permissions {
		if database[permission.Key] != permission.Description {
			t.Fatalf("El permiso %s no coincide entre Go y PostgreSQL", permission.Key)
		}
	}
	// Solo el administrador tiene permisos de inventario (el rol «Inventarios» sigue vacío).
	var administrator, others int
	err = fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FILTER (WHERE r.code='administrator'),
            count(*) FILTER (WHERE r.code IS DISTINCT FROM 'administrator')
        FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE p.permission_key LIKE 'inventory.%'`).
		Scan(&administrator, &others)
	if err != nil {
		t.Fatal(err)
	}
	if administrator != len(inventory.Permissions) || others != 0 {
		t.Fatalf("Concesiones de inventario: administrador %d, otros roles %d", administrator, others)
	}
	if rowCount(t, fixture.restaurantFixture, `SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
        WHERE r.code='inventory'`) != 0 {
		t.Fatal("El rol «Inventarios» recibió permisos por inferencia")
	}
}

func testInventoryUnitsAndCategories(t *testing.T, fixture *inventoryFixture) {
	for _, unit := range decodeResponse[[]inventory.Unit](t, fixture.adminCall(t, "GET", "inventory/units", nil, 200)) {
		fixture.units[unit.Code] = unit
	}
	if len(fixture.units) != 6 || fixture.units["kg"].Factor != "1000" || fixture.units["kg"].DimensionLabel != "Masa" {
		t.Fatalf("Unidades: %+v", fixture.units)
	}
	ounce := decodeResponse[inventory.Unit](t, fixture.adminWrite(t, "POST", "inventory/units", map[string]any{
		"code": "oz", "name": "Onza", "dimension": "mass", "factor": "28.349523125", "active": true,
	}, 200))
	if ounce.System || ounce.Factor != "28.349523125" || ounce.InUse {
		t.Fatalf("Unidad propia: %+v", ounce)
	}
	fixture.units["oz"] = ounce
	if code := errorCode(t, fixture.adminWrite(t, "POST", "inventory/units", map[string]any{
		"code": "oz", "name": "Onza líquida", "dimension": "volume", "factor": "29.5735", "active": true,
	}, 409)); code != "DUPLICATE_CODE" {
		t.Fatalf("Abreviatura repetida: %s", code)
	}
	kilogram := fixture.units["kg"]
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/units/"+kilogram.ID, map[string]any{
		"revision": kilogram.Revision, "name": "Kilo", "factor": "1000", "active": true,
	}, 409)); code != "UNIT_SYSTEM" {
		t.Fatalf("Unidad del sistema modificada: %s", code)
	}
	fixture.adminWrite(t, "PATCH", "inventory/units/"+ounce.ID, map[string]any{
		"revision": ounce.Revision, "name": "Onza", "factor": "28.35", "active": true,
	}, 200)
	fixture.adminWrite(t, "PATCH", "inventory/units/"+ounce.ID, map[string]any{
		"revision": ounce.Revision, "name": "Onza", "factor": "28.35", "active": true,
	}, 409)
	// Categorías con orden por posiciones.
	grains := decodeResponse[inventory.Category](t, fixture.adminWrite(t, "POST", "inventory/categories", map[string]any{
		"name": "Abarrotes", "active": true,
	}, 200))
	drinks := decodeResponse[inventory.Category](t, fixture.adminWrite(t, "POST", "inventory/categories", map[string]any{
		"name": "Bebidas", "active": true, "position": map[string]any{"after": ""},
	}, 200))
	listed := decodeResponse[[]inventory.Category](t, fixture.adminCall(t, "GET", "inventory/categories", nil, 200))
	if len(listed) != 2 || listed[0].ID != drinks.ID || listed[1].ID != grains.ID {
		t.Fatalf("Orden de categorías: %+v", listed)
	}
	if code := errorCode(t, fixture.adminWrite(t, "POST", "inventory/categories", map[string]any{
		"name": "abarrotes", "active": true,
	}, 409)); code != "DUPLICATE_NAME" {
		t.Fatalf("Categoría repetida: %s", code)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action IN ('inventory.unit_created','inventory.unit_updated','inventory.category_created')") < 4 {
		t.Fatal("Faltan auditorías de unidades y categorías")
	}
}

// inventoryItem crea un insumo como administrador.
func (fixture *inventoryFixture) createItem(t *testing.T, body map[string]any) inventory.Item {
	t.Helper()
	return decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items", body, 200))
}

func (fixture *inventoryFixture) itemByCode(t *testing.T, terminal testTerminal, code string) inventory.Item {
	t.Helper()
	page := decodeResponse[inventory.ItemPage](t, fixture.call(t, terminal, "GET", "inventory/items?search="+code, nil, 200))
	for _, item := range page.Items {
		if item.Code == code {
			return *item
		}
	}
	t.Fatalf("No se encontró el insumo %s", code)
	return inventory.Item{}
}

func testInventoryItems(t *testing.T, fixture *inventoryFixture) {
	categories := decodeResponse[[]inventory.Category](t, fixture.adminCall(t, "GET", "inventory/categories", nil, 200))
	rice := fixture.createItem(t, map[string]any{
		"code": "ARZ-01", "name": "Arroz", "base_unit_id": fixture.units["kg"].ID, "category_id": categories[1].ID,
		"active": true, "reference_cost": "18", "min_stock": "5", "reorder_point": "8", "barcode": "7501000000011",
	})
	if !rice.CostsVisible || rice.ReferenceCost == nil || *rice.ReferenceCost != "18.00" || *rice.MinStock != "5" ||
		rice.BaseUnit.Code != "kg" || rice.Kind != "purchased" || rice.CategoryName == nil || *rice.CategoryName != "Abarrotes" {
		t.Fatalf("Insumo creado: %+v", rice)
	}
	soda := fixture.createItem(t, map[string]any{
		"code": "REF-355", "name": "Refresco de lata", "base_unit_id": fixture.units["pza"].ID, "category_id": categories[0].ID,
		"active": true,
	})
	if soda.ReferenceCost != nil {
		t.Fatal("Un insumo sin costo de referencia no debe tener costo")
	}
	fixture.createItem(t, map[string]any{
		"code": "ACE-01", "name": "Aceite", "base_unit_id": fixture.units["ml"].ID, "active": true, "reference_cost": "0.045",
	})
	onion := fixture.createItem(t, map[string]any{
		"code": "CEB-01", "name": "Cebolla", "base_unit_id": fixture.units["g"].ID, "active": true,
	})
	// Duplicados: código y nombre sin distinguir mayúsculas, código de barras único.
	for body, code := range map[string]string{
		`{"code":"arz-01","name":"Arroz 2"}`:                           "DUPLICATE_CODE",
		`{"code":"ARZ-02","name":"ARROZ"}`:                             "DUPLICATE_NAME",
		`{"code":"ARZ-03","name":"Arroz 3","barcode":"7501000000011"}`: "DUPLICATE_BARCODE",
	} {
		fields := map[string]any{"base_unit_id": fixture.units["kg"].ID, "active": true}
		if err := json.Unmarshal([]byte(body), &fields); err != nil {
			t.Fatal(err)
		}
		if actual := errorCode(t, fixture.adminWrite(t, "POST", "inventory/items", fields, 409)); actual != code {
			t.Fatalf("%s: esperado %s, obtenido %s", body, code, actual)
		}
	}
	// Presentaciones: caja de 24 piezas y costal de 25 kg en un insumo en gramos (25,000 g).
	withBox := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items/"+soda.ID+"/presentations", map[string]any{
		"name": "Caja con 24", "quantity": "24", "active": true, "purchase_default": true, "barcode": "17501000000018",
	}, 200))
	if len(withBox.Presentations) != 1 || withBox.Presentations[0].BaseQuantity != "24" || !withBox.Presentations[0].PurchaseDefault {
		t.Fatalf("Presentación por caja: %+v", withBox.Presentations)
	}
	withPack := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items/"+soda.ID+"/presentations", map[string]any{
		"name": "Paquete con 12", "quantity": "12", "active": true, "purchase_default": true,
	}, 200))
	defaults := 0
	for _, presentation := range withPack.Presentations {
		if presentation.PurchaseDefault {
			defaults++
			if presentation.Name != "Paquete con 12" {
				t.Fatalf("La presentación predeterminada debió cambiar: %+v", presentation)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("Debe haber una sola presentación predeterminada: %+v", withPack.Presentations)
	}
	// Edición de una presentación con revisión optimista y auditoría.
	var pack inventory.Presentation
	for _, presentation := range withPack.Presentations {
		if presentation.Name == "Paquete con 12" {
			pack = presentation
		}
	}
	renamed := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "PATCH", "inventory/presentations/"+pack.ID, map[string]any{
		"revision": pack.Revision, "name": "Paquete de 12", "quantity": "12", "active": true, "purchase_default": false,
	}, 200))
	for _, presentation := range renamed.Presentations {
		if presentation.ID == pack.ID && (presentation.Name != "Paquete de 12" || presentation.PurchaseDefault ||
			presentation.Revision != pack.Revision+1) {
			t.Fatalf("Presentación editada: %+v", presentation)
		}
	}
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/presentations/"+pack.ID, map[string]any{
		"revision": pack.Revision, "name": "Paquete viejo", "quantity": "12", "active": true, "purchase_default": false,
	}, 409)); code != "REVISION_CONFLICT" {
		t.Fatalf("Presentación con revisión obsoleta: %s", code)
	}
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/presentations/"+pack.ID, map[string]any{
		"revision": pack.Revision + 1, "name": "Paquete de 12", "quantity": "12", "active": false, "purchase_default": true,
	}, 400)); code != "INVENTORY_VALIDATION" {
		t.Fatalf("Presentación predeterminada inactiva: %s", code)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='inventory.presentation_updated' AND entity_id=$1",
		pack.ID) != 1 {
		t.Fatal("La edición de la presentación debe auditarse una vez")
	}
	sack := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items/"+onion.ID+"/presentations", map[string]any{
		"name": "Costal 25 kg", "quantity": "25", "unit_id": fixture.units["kg"].ID, "active": true, "purchase_default": false,
	}, 200))
	if sack.Presentations[0].BaseQuantity != "25000" {
		t.Fatalf("Costal convertido: %+v", sack.Presentations)
	}
	if code := errorCode(t, fixture.adminWrite(t, "POST", "inventory/items/"+onion.ID+"/presentations", map[string]any{
		"name": "Caja", "quantity": "2", "unit_id": fixture.units["l"].ID, "active": true, "purchase_default": false,
	}, 400)); code != "INVENTORY_VALIDATION" {
		t.Fatalf("Conversión entre dimensiones aceptada: %s", code)
	}
	if code := errorCode(t, fixture.adminWrite(t, "POST", "inventory/items/"+onion.ID+"/presentations", map[string]any{
		"name": "Repetida", "quantity": "2", "barcode": "17501000000018", "active": true, "purchase_default": false,
	}, 409)); code != "DUPLICATE_BARCODE" {
		t.Fatalf("Código de barras repetido entre presentaciones: %s", code)
	}
	// Edición con revisión optimista y auditoría con antes y después.
	updated := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "PATCH", "inventory/items/"+rice.ID, map[string]any{
		"revision": rice.Revision, "code": "ARZ-01", "name": "Arroz blanco", "base_unit_id": fixture.units["kg"].ID,
		"category_id": categories[1].ID, "active": true, "min_stock": "5", "reorder_point": "8", "barcode": "7501000000011",
	}, 200))
	if updated.Name != "Arroz blanco" || updated.Revision != rice.Revision+1 || *updated.ReferenceCost != "18.00" {
		t.Fatalf("Sin reference_cost el costo vigente se conserva: %+v", updated)
	}
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/items/"+rice.ID, map[string]any{
		"revision": rice.Revision, "code": "ARZ-01", "name": "Arroz viejo", "base_unit_id": fixture.units["kg"].ID, "active": true,
	}, 409)); code != "REVISION_CONFLICT" {
		t.Fatalf("Revisión obsoleta: %s", code)
	}
	var before, after string
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT before_state->>'name',after_state->>'name' FROM app.audit_events
        WHERE action='inventory.item_updated' AND entity_id=$1 ORDER BY occurred_at DESC LIMIT 1`, rice.ID).Scan(&before, &after)
	if err != nil || before != "Arroz" || after != "Arroz blanco" {
		t.Fatalf("Auditoría del insumo: %s → %s (%v)", before, after, err)
	}
	// Cambiar la unidad base de un insumo con presentaciones se rechaza; con costo, exige capturarlo.
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/items/"+onion.ID, map[string]any{
		"revision": sack.Revision, "code": "CEB-01", "name": "Cebolla", "base_unit_id": fixture.units["kg"].ID, "active": true,
	}, 409)); code != "BASE_UNIT_HAS_PRESENTATIONS" {
		t.Fatalf("Unidad base con presentaciones: %s", code)
	}
	oil := fixture.itemByCode(t, fixture.admin, "ACE-01")
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/items/"+oil.ID, map[string]any{
		"revision": oil.Revision, "code": "ACE-01", "name": "Aceite", "base_unit_id": fixture.units["l"].ID, "active": true,
	}, 409)); code != "REFERENCE_COST_REQUIRED" {
		t.Fatalf("Cambio de unidad sin recapturar el costo: %s", code)
	}
	oil = decodeResponse[inventory.Item](t, fixture.adminWrite(t, "PATCH", "inventory/items/"+oil.ID, map[string]any{
		"revision": oil.Revision, "code": "ACE-01", "name": "Aceite", "base_unit_id": fixture.units["l"].ID, "active": true,
		"reference_cost": "45",
	}, 200))
	if oil.BaseUnit.Code != "l" || *oil.ReferenceCost != "45.00" {
		t.Fatalf("Cambio de unidad base sin movimientos: %+v", oil)
	}
	// Permisos: el capturista administra insumos pero no fija costos; sin items.manage no escribe.
	items := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_items")
	fixture.write(t, fixture.clerk, "POST", "inventory/items", map[string]any{
		"code": "SAL-01", "name": "Sal", "base_unit_id": fixture.units["g"].ID, "active": true, "reference_cost": "0.02",
	}, 403)
	fixture.write(t, fixture.stock, "POST", "inventory/items", map[string]any{
		"code": "SAL-01", "name": "Sal", "base_unit_id": fixture.units["g"].ID, "active": true,
	}, 403)
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_items") != items {
		t.Fatal("Una escritura denegada dejó registros")
	}
	salt := decodeResponse[inventory.Item](t, fixture.write(t, fixture.clerk, "POST", "inventory/items", map[string]any{
		"code": "SAL-01", "name": "Sal", "base_unit_id": fixture.units["g"].ID, "active": true,
	}, 200))
	if salt.CostsVisible || salt.ReferenceCost != nil {
		t.Fatalf("El capturista no consulta costos: %+v", salt)
	}
	fixture.call(t, fixture.outsider, "GET", "inventory/summary", nil, 403)
	fixture.call(t, fixture.outsider, "GET", "inventory/items", nil, 403)
	// Listado con filtros y paginación.
	page := decodeResponse[inventory.ItemPage](t, fixture.adminCall(t, "GET", "inventory/items?page_size=2&active=true", nil, 200))
	if page.Total != 5 || len(page.Items) != 2 || page.PageSize != 2 {
		t.Fatalf("Paginación de insumos: %d %d", page.Total, len(page.Items))
	}
	byCategory := decodeResponse[inventory.ItemPage](t, fixture.adminCall(t, "GET", "inventory/items?category_id="+categories[0].ID, nil, 200))
	if byCategory.Total != 1 || byCategory.Items[0].Code != "REF-355" {
		t.Fatalf("Filtro por categoría: %+v", byCategory.Items)
	}
	byBarcode := decodeResponse[inventory.ItemPage](t, fixture.adminCall(t, "GET", "inventory/items?search=17501000000018", nil, 200))
	if byBarcode.Total != 1 || byBarcode.Items[0].Code != "REF-355" {
		t.Fatalf("Búsqueda por código de barras de la presentación: %+v", byBarcode.Items)
	}
	fixture.adminCall(t, "GET", "inventory/items?order_by=name", nil, 400)
}

func testInventoryWarehouses(t *testing.T, fixture *inventoryFixture) {
	bar := decodeResponse[inventory.Warehouse](t, fixture.adminWrite(t, "POST", "inventory/warehouses", map[string]any{
		"name": "Barra", "description": "Bebidas y licores", "active": true,
	}, 200))
	kitchenWarehouse := decodeResponse[inventory.Warehouse](t, fixture.adminWrite(t, "POST", "inventory/warehouses", map[string]any{
		"name": "Cocina", "active": true,
	}, 200))
	if bar.IsDefault || kitchenWarehouse.IsDefault {
		t.Fatal("Solo el almacén sembrado es predeterminado")
	}
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/warehouses/"+fixture.general.ID, map[string]any{
		"revision": fixture.general.Revision, "name": "Almacén general", "active": false,
	}, 409)); code != "DEFAULT_WAREHOUSE" {
		t.Fatalf("El predeterminado se desactivó: %s", code)
	}
	fixture.write(t, fixture.clerk, "POST", "inventory/warehouses", map[string]any{"name": "Bodega", "active": true}, 403)
	stations := decodeResponse[[]kitchen.Station](t, fixture.adminCall(t, "GET", "kitchen/stations", nil, 200))
	if len(stations) == 0 {
		t.Fatal("La aceptación debió crear estaciones de cocina")
	}
	station := stations[0]
	assignments := decodeResponse[[]inventory.StationWarehouse](t, fixture.adminCall(t, "GET", "inventory/station-warehouses", nil, 200))
	var current inventory.StationWarehouse
	for _, assignment := range assignments {
		if assignment.StationID == station.ID {
			current = assignment
		}
	}
	if !current.UsesDefault || current.WarehouseName != "Almacén general" || current.Revision != 0 {
		t.Fatalf("Sin asignación, la estación usa el predeterminado: %+v", current)
	}
	route := "inventory/station-warehouses/" + station.ID
	first := decodeResponse[inventory.StationWarehouse](t, fixture.adminWrite(t, "PUT", route, map[string]any{
		"revision": 0, "warehouse_id": bar.ID,
	}, 200))
	second := decodeResponse[inventory.StationWarehouse](t, fixture.adminWrite(t, "PUT", route, map[string]any{
		"revision": first.Revision, "warehouse_id": kitchenWarehouse.ID,
	}, 200))
	if first.WarehouseName != "Barra" || second.WarehouseName != "Cocina" || second.Revision != 2 || len(second.History) != 2 ||
		second.History[0].WarehouseName != "Cocina" || second.History[1].WarehouseName != "Barra" {
		t.Fatalf("Historial de asignaciones: %+v", second)
	}
	if code := errorCode(t, fixture.adminWrite(t, "PUT", route, map[string]any{"revision": first.Revision, "warehouse_id": nil}, 409)); code != "REVISION_CONFLICT" {
		t.Fatalf("Asignación con revisión obsoleta: %s", code)
	}
	// Repetir el almacén vigente no agrega historial.
	same := decodeResponse[inventory.StationWarehouse](t, fixture.adminWrite(t, "PUT", route, map[string]any{
		"revision": second.Revision, "warehouse_id": kitchenWarehouse.ID,
	}, 200))
	if same.Revision != 2 {
		t.Fatalf("La misma asignación agregó historial: %+v", same)
	}
	// Un almacén asignado no se desactiva.
	listed := decodeResponse[[]inventory.Warehouse](t, fixture.adminCall(t, "GET", "inventory/warehouses", nil, 200))
	for _, warehouse := range listed {
		if warehouse.ID == kitchenWarehouse.ID && (len(warehouse.Stations) != 1 || warehouse.Stations[0].ID != station.ID) {
			t.Fatalf("Estaciones del almacén Cocina: %+v", warehouse.Stations)
		}
	}
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/warehouses/"+kitchenWarehouse.ID, map[string]any{
		"revision": kitchenWarehouse.Revision, "name": "Cocina", "active": false,
	}, 409)); code != "WAREHOUSE_ASSIGNED" {
		t.Fatalf("Almacén asignado desactivado: %s", code)
	}
	back := decodeResponse[inventory.StationWarehouse](t, fixture.adminWrite(t, "PUT", route, map[string]any{
		"revision": second.Revision, "warehouse_id": nil,
	}, 200))
	if !back.UsesDefault || back.WarehouseName != "Almacén general" || back.Revision != 3 {
		t.Fatalf("Volver al predeterminado: %+v", back)
	}
	// El historial es de solo inserción también en PostgreSQL.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_station_warehouses SET warehouse_id=NULL"); err == nil {
		t.Fatal("Se modificó el historial de asignaciones")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "DELETE FROM app.inventory_station_warehouses"); err == nil {
		t.Fatal("Se borró el historial de asignaciones")
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='inventory.station_warehouse_assigned'") != 3 {
		t.Fatal("Cada asignación debe auditarse")
	}
}

// inventoryEvents devuelve los eventos del ámbito de inventario que una terminal recibe después del cursor.
func (fixture *inventoryFixture) inventoryEvents(t *testing.T, terminal testTerminal, cursor int64) []events.Change {
	t.Helper()
	batch := decodeResponse[events.Batch](t, fixture.call(t, terminal, "GET", fmt.Sprintf("events/snapshot?after=%d", cursor), nil, 200))
	result := []events.Change{}
	for _, change := range batch.Events {
		if change.Scope == "inventory" {
			result = append(result, change)
		}
	}
	return result
}

func eventCursor(t *testing.T, fixture *restaurantFixture) int64 {
	t.Helper()
	var head int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(sequence),0) FROM app.realtime_events").Scan(&head); err != nil {
		t.Fatal(err)
	}
	return head
}

func eventKinds(changes []events.Change) []string {
	kinds := []string{}
	for _, change := range changes {
		if !slices.Contains(kinds, change.Kind) {
			kinds = append(kinds, change.Kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
