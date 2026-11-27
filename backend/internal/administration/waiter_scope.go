package administration

import "slices"

// Permisos de mesero (etapa A1, decisión A1-D17 del propietario). Una sesión abierta con PIN en un teléfono o tableta
// de meseros (auth/pin-login) y la credencial de envío de pedidos sin conexión solo tienen los permisos del usuario
// que también están en esta lista cerrada: aunque la persona sea administrador o supervisor, lo administrativo se
// sigue haciendo con su contraseña en la computadora. Es la única lista del servidor; se documenta en el contrato
// (openapi.json, pinLogin), en docs/API_V1.md y en API_CONTRACTS.md, y waiter_scope_test.go comprueba que los tres la
// repiten completa. Solo crece con las funciones de la aplicación de meseros (en la A2: Para llevar e impresión de la
// precuenta).
//
// Sale de lo que la aplicación usa en la A1 (Fase 0, §4 del avance, y las rutas del contrato que llama):
var WaiterPermissions = []string{
	// Entrar con PIN y «Mi PIN».
	"waiter_devices.use",
	// Mesas y áreas: restaurant/state, restaurant/overview.
	"restaurant.tables.read",
	"restaurant.areas.read",
	// Cuentas de la mesa: listAccounts, getAccount, previewDraft.
	"accounts.read",
	// Abrir una cuenta con su primer producto (también desde un pedido sin conexión a una mesa).
	"accounts.open",
	// Unir mesas (etapa M1, M1-D8): «Unir con…», agregar y quitar mesas de una unión, el atajo de una mesa divisible y
	// la unión que nace con el primer producto (previewTableUnion, createTableUnion, addTableToUnion,
	// removeTableFromUnion y openAccount con union_table_ids o union_group_id).
	"restaurant.tables.join",
	// Capturar y modificar lo no enviado: saveDraft.
	"orders.capture",
	// «Enviar a cocina» y el pedido sin conexión: confirmDraft, submitOfflineOrder.
	"orders.confirm",
	// Modificar y cancelar unidades enviadas: previewEdit, editUnits, cancelUnits.
	"orders.edit",
	"orders.cancel",
	// Catálogo y precios de referencia: getCatalogSync, listCatalogProducts, imágenes.
	"catalog.read",
	"catalog.prices.read",
	// Tiempo real: streamEvents.
	"events.read",
	// Para llevar (etapa A2, §3 de la especificación y decisiones A2-D1 y A2-D2): ver los pedidos para llevar abiertos
	// con su preparación, pago y entrega (listAccounts, getAccount, restaurant/state) y sus eventos del ámbito takeaway;
	// abrirlos y capturarlos, también sin conexión (destination.takeaway), y cambiar su nombre y teléfono
	// (POST restaurant/accounts/{id}/customer); y marcar «Entregado» un pedido ya pagado (deliverTakeaway). Este último es
	// opcional en el rol: Go sigue rechazando la entrega con saldo pendiente (BALANCE_PENDING), así que el mesero
	// nunca entrega un pedido sin pagar ni cobra.
	"takeaway.read",
	"takeaway.manage",
	"takeaway.deliver",
	// Imprimir la precuenta desde el dispositivo (etapa A2, §5 y decisión A2-D3): printPrecheck, retryDocument y
	// getDocumentStatus, siempre en la impresora de precuentas del dispositivo. No incluye payments.read (el cobro) ni
	// payments.tickets.reprint: volver a tocar «Imprimir precuenta» arma una precuenta nueva, como en la caja.
	"payments.tickets.print",
}

// waiterScope deja de los permisos efectivos del usuario solo los de mesero, en el mismo orden.
func waiterScope(permissions []string) []string {
	scoped := []string{}
	for _, key := range permissions {
		if slices.Contains(WaiterPermissions, key) {
			scoped = append(scoped, key)
		}
	}
	return scoped
}
