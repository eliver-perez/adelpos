// Package maintenance reúne las tareas de mantenimiento del administrador (etapa 9F, §3): «Quitar datos de
// prueba», que identifica lo que crearon las pruebas automáticas por la firma exacta de cada archivo de
// prueba (nunca por un nombre genérico suelto ni por suposición), lo elimina o lo desactiva en una sola
// transacción y antes crea un respaldo de la base.
//
// Nunca toca ventas, pagos, cortes, movimientos de inventario, cuentas, documentos ni auditoría: un dato de
// prueba con operaciones registradas se desactiva en lugar de borrarse, y la configuración que apunta a él
// (impresora de tickets de Cobros, cajón, impresora de respaldo, estaciones por terminal, rutas) se
// desvincula.
package maintenance

// La lista cerrada de firmas. Cada firma lleva el archivo de prueba que la crea (Source, también en el
// comentario); signatures_test.go comprueba que ese archivo siga creando exactamente esos datos, de modo
// que cambiar una prueba sin actualizar la firma falla.

// PrinterSignature identifica una impresora de prueba por nombre, transporte y destino exactos.
type PrinterSignature struct {
	Name        string
	Transport   string
	Destination string
	Source      string
}

// StationSignature identifica una estación de prueba por su nombre exacto, y solo si su impresora es una
// impresora de prueba (TestPrinters). Una estación con el mismo nombre y otra impresora es del restaurante.
type StationSignature struct {
	Name   string
	Source string
}

// ProductSignature identifica un producto de prueba por nombre, código y tipo exactos.
type ProductSignature struct {
	Name   string
	Code   string
	Kind   string
	Source string
}

// CategorySignature identifica una categoría de prueba por nombre y color exactos, y solo si contiene un
// producto de prueba (TestProducts): «Platillos» en rojo es también una elección habitual del restaurante.
type CategorySignature struct {
	Name   string
	Color  string
	Source string
}

// acceptanceDestination es el destino de las impresoras de aceptación: un agente que no existe, de modo
// que todo lo que se les envía queda en «Requiere intervención» y nunca sale en papel.
const acceptanceDestination = "Agente de aceptación no disponible"

var TestPrinters = []PrinterSignature{
	// desktop/e2e/kitchen-flow.ts (configureKitchen): la usan restaurant.spec.ts y la prueba de humo de la
	// aplicación instalada en Windows (desktop/e2e-installed/installed-smoke.spec.ts), que además la eligió
	// como impresora de tickets de Cobros (desktop/e2e/payments-flow.ts, configureReceiptPrinter).
	{
		Name:        "Destino controlado E2E",
		Transport:   "agent",
		Destination: acceptanceDestination,
		Source:      "desktop/e2e/kitchen-flow.ts",
	},
	// desktop/e2e/workflow-stage9b-designer.spec.ts (paidTicket): el mismo destino de aceptación, creado por
	// la API y elegido como impresora de tickets.
	{
		Name:        "Destino controlado 9B",
		Transport:   "agent",
		Destination: acceptanceDestination,
		Source:      "desktop/e2e/workflow-stage9b-designer.spec.ts",
	},
}

var TestStations = []StationSignature{
	// desktop/e2e/kitchen-flow.ts (configureKitchen): recepción «printer» con «Destino controlado E2E».
	{Name: "Cocina caliente", Source: "desktop/e2e/kitchen-flow.ts"},
	// desktop/e2e/kitchen-flow.ts (configureKitchen): recepción «both» con «Destino controlado E2E».
	{Name: "Freidoras", Source: "desktop/e2e/kitchen-flow.ts"},
}

var TestProducts = []ProductSignature{
	// desktop/e2e-installed/installed-smoke.spec.ts («catálogo, cocina, impresora de tickets y pago en
	// dólares») y el catálogo mínimo de desktop/e2e/native-driver.ts (prepareCatalog).
	{
		Name:   "Filete de prueba",
		Code:   "FILETE",
		Kind:   "product",
		Source: "desktop/e2e-installed/installed-smoke.spec.ts",
	},
}

var TestCategories = []CategorySignature{
	// desktop/e2e-installed/installed-smoke.spec.ts y desktop/e2e/native-driver.ts (prepareCatalog):
	// «Platillos» con el rojo de la paleta.
	{
		Name:   "Platillos",
		Color:  "#d64545",
		Source: "desktop/e2e-installed/installed-smoke.spec.ts",
	},
}
