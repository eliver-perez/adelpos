package reports

// Permisos del módulo Reportes (etapa 9A, §27). Cada clave se inserta también en la migración 0017
// con la misma descripción y se concede inicialmente solo al rol administrador. Los permisos de los
// reportes no sustituyen a los de origen: el esperado y el efectivo físico por moneda siguen
// exigiendo cash.expected.read, los costos inventory.costs.read y la valuación
// inventory.valuation.read.
const (
	PermissionRead          = "reports.read"
	PermissionSales         = "reports.sales.read"
	PermissionPayments      = "reports.payments.read"
	PermissionDiscounts     = "reports.discounts.read"
	PermissionCash          = "reports.cash.read"
	PermissionInventory     = "reports.inventory.read"
	PermissionPurchases     = "reports.purchases.read"
	PermissionEmployees     = "reports.employees.read"
	PermissionExportPDF     = "reports.export.pdf"
	PermissionExportExcel   = "reports.export.excel"
	PermissionCashExpected  = "cash.expected.read"
	PermissionCostsRead     = "inventory.costs.read"
	PermissionValuationRead = "inventory.valuation.read"
	PermissionStockRead     = "inventory.stock.read"
)

type Permission struct {
	Key         string
	Description string
}

var Permissions = []Permission{
	{PermissionRead, "Consultar el módulo Reportes y el resumen ejecutivo"},
	{PermissionSales, "Consultar reportes de ventas, productos, categorías, cocina y operación"},
	{PermissionPayments, "Consultar reportes de cobros, devoluciones y propinas"},
	{PermissionDiscounts, "Consultar reportes de descuentos, cortesías y autorizaciones"},
	{PermissionCash, "Consultar reportes de caja"},
	{PermissionInventory, "Consultar reportes de inventarios"},
	{PermissionPurchases, "Consultar reportes de compras y proveedores"},
	{PermissionEmployees, "Consultar reportes por empleado"},
	{PermissionExportPDF, "Exportar reportes a PDF"},
	{PermissionExportExcel, "Exportar reportes a Excel"},
}
