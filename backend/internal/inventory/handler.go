package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// route describe una ruta de inventario: permisos exigidos (además de inventory.read), si solo
// lee y qué acción la atiende.
type route struct {
	permissions []string
	// anyPermission exige al menos uno de estos permisos (por ejemplo, consultar compras con
	// cualquiera de los permisos de compras).
	anyPermission []string
	// cashRead indica que la ruta lee gastos de caja: toma 7820148305 compartido antes que
	// 7820148308 para leerlos de forma consistente con payments (orden de locks de CLAUDE.md §3).
	cashRead bool
	readOnly bool
	action   func(handler *service, parts []string) (any, error)
}

func read(action func(handler *service, parts []string) (any, error), permissions ...string) *route {
	return &route{permissions: permissions, readOnly: true, action: action}
}

func write(action func(handler *service, parts []string) (any, error), permissions ...string) *route {
	return &route{permissions: permissions, readOnly: false, action: action}
}

// purchasingRead es una lectura de compras: la ve quien tiene cualquiera de los permisos de
// compras (proveedores, órdenes, recepciones, devoluciones o vínculos con caja).
func purchasingRead(action func(handler *service, parts []string) (any, error)) *route {
	return &route{anyPermission: purchasingPermissions, readOnly: true, action: action}
}

// withCash marca una ruta que lee gastos de caja.
func (selected *route) withCash() *route {
	selected.cashRead = true
	return selected
}

// resolve enumera las rutas de inventario (fases A, B y C). El puente nativo admite un solo identificador por ruta,
// siempre en la segunda posición; los registros relacionados viajan en el cuerpo.
func resolve(method string, parts []string) *route {
	if selected := resolvePurchasing(method, parts); selected != nil {
		return selected
	}
	length := len(parts)
	resource := parts[0]
	if length == 1 {
		switch resource + " " + method {
		case "summary GET":
			return read((*service).summary)
		case "settings GET":
			return read((*service).readSettings)
		case "settings PUT":
			return write((*service).saveSettings, permissionSettingsManage)
		case "units GET":
			return read((*service).listUnits)
		case "units POST":
			return write((*service).createUnit, permissionItemsManage)
		case "categories GET":
			return read((*service).listCategories)
		case "categories POST":
			return write((*service).createCategory, permissionItemsManage)
		case "items GET":
			return read((*service).listItems)
		case "items POST":
			return write((*service).createItem, permissionItemsManage)
		case "warehouses GET":
			return read((*service).listWarehouses)
		case "warehouses POST":
			return write((*service).createWarehouse, permissionWarehousesManage)
		case "station-warehouses GET":
			return read((*service).listStationWarehouses)
		case "stock GET":
			return read((*service).listStock, permissionStockRead)
		case "movements GET":
			return read((*service).listMovements, permissionMovementsRead)
		case "initial-balances POST":
			// La existencia inicial es una entrada autorizada con costo: exige autorizar ajustes y
			// administrar costos (§0-B, regla 7 del motor).
			return write((*service).recordInitialBalances, permissionAdjustmentsAuthorize, permissionCostsManage)
		// Fase B: recetas, porciones, cancelaciones, elaboraciones y diagnóstico del consumo. Alta de
		// receta: el permiso depende del tipo (recetas o subrecetas) y se comprueba al decodificar.
		case "recipes GET":
			return read((*service).listRecipes)
		case "recipes POST":
			return write((*service).createRecipe)
		case "recipe-targets GET":
			return read((*service).listRecipeTargets)
		case "ingredient-portions GET":
			return read((*service).listIngredientPortions)
		case "cancellations GET":
			return read((*service).listCancellations)
		case "productions GET":
			// Elaborar o consultar movimientos (se comprueba en la acción).
			return read((*service).listProductions)
		case "productions POST":
			return write((*service).recordProduction, permissionProductionRecord)
		case "consumption-requests GET":
			return read((*service).listConsumptionRequests, permissionMovementsRead)
		}
		return nil
	}
	if length == 2 {
		switch resource + " " + method {
		case "units PATCH":
			return write((*service).updateUnit, permissionItemsManage)
		case "categories PATCH":
			return write((*service).updateCategory, permissionItemsManage)
		case "items GET":
			return read((*service).readItemRoute)
		case "items PATCH":
			return write((*service).updateItem, permissionItemsManage)
		case "presentations PATCH":
			return write((*service).updatePresentation, permissionItemsManage)
		case "warehouses PATCH":
			return write((*service).updateWarehouse, permissionWarehousesManage)
		case "station-warehouses PUT":
			return write((*service).assignStationWarehouse, permissionWarehousesManage)
		case "recipes GET":
			return read((*service).readRecipeRoute)
		case "recipes PUT":
			// Nueva versión: el permiso depende del tipo de la receta (se comprueba antes de escribir).
			return write((*service).updateRecipe)
		case "recipe-versions GET":
			return read((*service).readRecipeVersionRoute)
		case "ingredient-portions PUT":
			return write((*service).saveIngredientPortion, permissionRecipesManage)
		case "consumption GET":
			return read((*service).readUnitConsumption, permissionMovementsRead)
		}
		return nil
	}
	if length == 3 && resource == "items" && parts[2] == "presentations" && method == "POST" {
		return write((*service).createPresentation, permissionItemsManage)
	}
	if length == 3 && resource == "cancellations" && parts[2] == "classify" && method == "POST" {
		// inventory.waste.record hasta el corte o inventory.adjustments.authorize después (se
		// comprueba en la acción, antes de escribir).
		return write((*service).classifyCancellation)
	}
	return nil
}

// Handle atiende las rutas inventory/* dentro de la transacción autenticada de administración.
//   - Todos los permisos de la ruta (inventory.read más el propio) se comprueban antes del lock y
//     de cualquier escritura; los permisos que dependen del contenido (costos) se comprueban
//     después de decodificar el cuerpo, también antes de escribir.
//   - Las escrituras toman el lock 7820148308 exclusivo y las lecturas compartido, para que una
//     respuesta con varias consultas sea consistente con el libro.
//   - Toda escritura exige operation_id y se registra en restaurant_operations con su huella: un
//     reintento con el mismo cuerpo devuelve la respuesta guardada sin repetir efectos y uno con
//     otro cuerpo responde 409 IDEMPOTENCY_CONFLICT. Las entidades administrables usan además
//     revisión optimista (REVISION_CONFLICT).
func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "inventory/"), "/"), "/")
	selected := resolve(request.Method, parts)
	if selected == nil {
		return nil, notFound("La ruta de inventario no existe.")
	}
	if err := request.Actor.Require(permissionRead); err != nil {
		return nil, err
	}
	if err := request.Actor.Require(selected.permissions...); err != nil {
		return nil, err
	}
	if err := request.Actor.RequireAny(selected.anyPermission...); err != nil {
		return nil, err
	}
	if len(parts) > 1 {
		if err := validateID(parts[1]); err != nil {
			return nil, err
		}
		parts[1] = strings.ToLower(parts[1])
	}
	if selected.cashRead {
		if _, err := tx.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_xact_lock_shared(%d)", cashLockKey)); err != nil {
			return nil, err
		}
	}
	lock := fmt.Sprintf("SELECT pg_advisory_xact_lock(%d)", lockKey)
	if selected.readOnly {
		lock = fmt.Sprintf("SELECT pg_advisory_xact_lock_shared(%d)", lockKey)
	}
	if _, err := tx.Exec(ctx, lock); err != nil {
		return nil, err
	}
	handler := &service{ctx: ctx, tx: tx, request: request, audit: audit}
	if selected.readOnly {
		return selected.action(handler, parts)
	}
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(request.Body, &mutation); err != nil {
		return nil, invalid("La operación requiere un cuerpo JSON válido.")
	}
	if err := validateID(mutation.OperationID); err != nil {
		return nil, invalid("La operación requiere un operation_id válido.")
	}
	handler.operationID = strings.ToLower(mutation.OperationID)
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return nil, err
	}
	cached, found, err := handler.replay(fingerprint)
	if err != nil || found {
		return cached, err
	}
	result, err := selected.action(handler, parts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6)`, handler.operationID, request.Actor.UserID, request.Actor.DeviceID,
		request.Method+" "+request.Route, fingerprint, jsonValue(result))
	if err != nil {
		return nil, err
	}
	return result, nil
}

// requestFingerprint identifica el contenido de la operación: método, ruta y JSON canónico del
// cuerpo (los números se conservan como texto).
func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(jsonValue(value))))
	return fmt.Sprintf("%x", digest), nil
}

// replay devuelve la respuesta guardada de una operación ya confirmada por el mismo actor, desde la
// misma terminal y con la misma huella.
func (handler *service) replay(fingerprint string) (any, bool, error) {
	var actorID, deviceID, previousFingerprint string
	var result json.RawMessage
	err := handler.tx.QueryRow(handler.ctx, `SELECT actor_id,device_id,fingerprint,response
        FROM app.restaurant_operations WHERE id=$1`, handler.operationID).Scan(&actorID, &deviceID, &previousFingerprint, &result)
	if isNoRows(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != handler.request.Actor.UserID || deviceID != handler.request.Actor.DeviceID || previousFingerprint != fingerprint {
		return nil, false, conflict(
			"IDEMPOTENCY_CONFLICT",
			"El identificador de operación ya corresponde a otra solicitud. No se repitieron sus cambios.",
		)
	}
	return result, true, nil
}

// operationReference devuelve el operation_id de la escritura en curso para las columnas que lo
// referencian (la fila de restaurant_operations se inserta al final, por eso la llave es diferida).
func (handler *service) operationReference() *string {
	if handler.operationID == "" {
		return nil
	}
	operation := handler.operationID
	return &operation
}

func (handler *service) actorReference() *string {
	if handler.request.Actor.UserID == "" {
		return nil
	}
	actor := handler.request.Actor.UserID
	return &actor
}
