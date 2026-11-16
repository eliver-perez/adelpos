// Package legacysynth construye una base sintética con la misma estructura que el respaldo del sistema anterior
// (docs/MIGRACION_ANALISIS.md: sus 52 tablas, columnas y llaves) y datos inventados que imitan sus casos: IVA por
// producto, complementos (también con importe negativo), descuentos, cargos extra, cancelaciones, dólares, cortes,
// retiros y egresos, contraseñas, imágenes y casos incoherentes (totales que no cuadran, pagos repetidos, en cero o
// negativos, ventas sin renglones, folios repetidos, renglones sin precio anterior…).
//
// Con el mismo conjunto de datos se obtiene:
//
//   - TSQL: el script que crea esa base en un SQL Server (la prueba con Docker la respalda a un .bak sintético y lo
//     pasa por legacy-prepare);
//   - WritePackage: el paquete de migración que legacy-prepare produciría de ese respaldo, sin Docker (lo usan las
//     pruebas del importador y el subcomando legacy-synthetic de Playwright).
//
// Ningún dato de aquí sale del respaldo real: todo es inventado.
package legacysynth

// Column es una columna de la base sintética con su tipo de SQL Server tal como lo informa el catálogo («int»,
// «money», «numeric(18,3)», «varchar(50)», «varbinary(max)»…).
type Column struct {
	Name     string
	Type     string
	Nullable bool
	Identity bool
}

// Table es una tabla con sus filas. Cada valor es nil, int64, string (texto, fecha o importe ya con su número de
// decimales) o []byte (binario), según el tipo de la columna.
type Table struct {
	Name       string
	Columns    []Column
	PrimaryKey []string
	Rows       [][]any
}

type columnFlag int

const (
	nullable columnFlag = iota + 1
	identity
)

func column(name, sqlType string, flags ...columnFlag) Column {
	result := Column{Name: name, Type: sqlType}
	for _, flag := range flags {
		switch flag {
		case nullable:
			result.Nullable = true
		case identity:
			result.Identity = true
		}
	}
	return result
}

func table(name string, primaryKey []string, columns ...Column) *Table {
	if primaryKey == nil {
		primaryKey = []string{}
	}
	return &Table{Name: name, Columns: columns, PrimaryKey: primaryKey}
}

var idKey = []string{"id"}

// schema son las 52 tablas del respaldo del sistema anterior, con sus columnas en el mismo orden y sus llaves
// primarias (MIGRACION_ANALISIS.md §1.3). Las llaves foráneas y los valores predeterminados no hacen falta para leerlas.
func schema() []*Table {
	return []*Table{
		// Ventas e historial.
		table("ventas", idKey,
			column("id", "int", identity), column("caja", "smallint"), column("mesa", "smallint"),
			column("nombre", "varchar(50)", nullable), column("folio", "varchar(12)"), column("fecha", "datetime"),
			column("cajero", "smallint"), column("facturado", "tinyint", nullable), column("subtotal", "money"),
			column("iva", "money"), column("total", "money"), column("montoPagado", "money", nullable),
			column("pagado", "tinyint", nullable), column("extra", "varchar(250)", nullable),
			column("montoExtra", "money", nullable), column("enmesa", "tinyint", nullable),
			column("corte", "int", nullable), column("cancelado", "tinyint")),
		table("ventas_productos", idKey,
			column("id", "int", identity), column("venta", "int"), column("producto", "int"),
			column("cantidad", "numeric(18,3)"), column("cantidadOriginal", "numeric(18,3)"), column("subtotal", "money"),
			column("iva", "money"), column("total", "money"), column("observaciones", "varchar(80)", nullable)),
		table("ventas_complementos", nil,
			column("producto", "int"), column("complemento", "int"), column("incluido", "numeric(18,3)"),
			column("pedido", "numeric(18,3)"), column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("ventas_modificaciones", nil,
			column("producto", "int"), column("cantAnt", "numeric(18,3)"), column("cantNueva", "numeric(18,3)"),
			column("autorizacion", "smallint"), column("fecha", "datetime", nullable)),
		table("ventas_especial", nil,
			column("venta", "int"), column("producto", "int"), column("ingrediente", "int")),
		table("ventas_servicios", nil,
			column("venta", "int"), column("servicio", "int"), column("cantidad", "numeric(18,3)", nullable),
			column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("descuentos", nil,
			column("tipo", "smallint"), column("venta", "int"), column("descripcion", "varchar(255)", nullable),
			column("descuento", "money")),
		table("tipos_descuentos", idKey,
			column("id", "smallint", identity), column("tipo", "varchar(30)")),
		table("cancelaciones", idKey,
			column("id", "int", identity), column("venta", "int"), column("autorizo", "smallint"),
			column("fecha", "datetime")),
		// Pagos.
		table("pagos", idKey,
			column("id", "int", identity), column("venta", "int"), column("folio", "varchar(20)", nullable),
			column("tipo", "tinyint", nullable), column("monto", "money", nullable), column("recibio", "smallint"),
			column("fecha", "datetime"), column("corte", "int", nullable)),
		table("pagos_productos", nil,
			column("pago", "int"), column("producto", "int"), column("cantidad", "numeric(18,3)"),
			column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("tipos_pagos", idKey,
			column("id", "tinyint", identity), column("tipo", "varchar(25)")),
		table("divisas", idKey,
			column("id", "smallint", identity), column("divisa", "varchar(30)"), column("simbolo", "varchar(5)"),
			column("cambio", "money")),
		// Caja y cortes.
		table("cortes", idKey,
			column("id", "int", identity), column("cajero", "smallint"), column("inicio_caja", "money"),
			column("fin_caja", "money", nullable), column("venta", "money", nullable), column("caja", "smallint"),
			column("status", "varchar(1)", nullable), column("autorizacion", "smallint", nullable),
			column("fecha_inicio", "datetime", nullable), column("fecha_fin", "datetime", nullable),
			column("retiros", "money", nullable), column("diferencia", "money", nullable)),
		table("cortes_parciales", idKey,
			column("id", "int", identity), column("corte", "int"), column("monto", "money"),
			column("razon", "varchar(255)"), column("fecha", "datetime"), column("autorizacion", "smallint")),
		table("salidas", nil,
			column("corte", "int"), column("monto", "money"), column("fecha", "datetime"),
			column("autorizacion", "smallint")),
		table("topes_caja", idKey,
			column("id", "smallint", identity), column("tope", "money"), column("fecha", "datetime")),
		table("cajas", idKey,
			column("id", "smallint", identity), column("machine_id", "varchar(30)"), column("caja", "varchar(30)"),
			column("fecha_alta", "datetime"), column("impresora_caja", "tinyint", nullable),
			column("impresora_cocina", "tinyint", nullable)),
		// Catálogo.
		table("categorias", idKey,
			column("id", "smallint", identity), column("categoria", "varchar(50)"), column("cocina", "tinyint", nullable)),
		table("productos", idKey,
			column("id", "int", identity), column("categoria", "smallint"), column("producto", "varchar(50)"),
			column("nombre_corto", "varchar(25)", nullable), column("unidad", "varchar(4)"),
			column("portada", "varbinary(max)", nullable)),
		table("productos_precios", nil,
			column("producto", "int"), column("fecha_inicio", "datetime"), column("vigencia", "datetime", nullable),
			column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("complementos", idKey,
			column("id", "int", identity), column("complemento", "varchar(30)")),
		table("complementos_precios", nil,
			column("id", "int"), column("fecha_inicio", "datetime"), column("vigencia", "datetime", nullable),
			column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("productos_complementos", nil,
			column("producto", "int"), column("complemento", "int"), column("cantidad", "numeric(18,2)", nullable)),
		table("productos_imagenes", nil,
			column("producto", "int"), column("imagen", "varbinary(max)", nullable), column("posicion", "tinyint", nullable)),
		table("promociones", idKey,
			column("id", "int", identity), column("producto", "int"), column("promocion", "money"),
			column("inicio", "datetime", nullable), column("vigencia", "datetime", nullable)),
		table("servicios", idKey,
			column("id", "int", identity), column("servicio", "varchar(50)"), column("descripcion", "varchar(255)", nullable),
			column("fechaInicio", "datetime"), column("vigencia", "datetime"), column("precio", "money", nullable)),
		table("unidades", idKey,
			column("id", "varchar(4)"), column("unidad", "varchar(20)", nullable)),
		// Mesas.
		table("mesas", idKey,
			column("id", "smallint", identity), column("mesa", "varchar(30)", nullable), column("capacidad", "tinyint", nullable)),
		// Personal y usuarios.
		table("puestos", idKey,
			column("id", "tinyint", identity), column("puesto", "varchar(50)"), column("funciones", "varchar(255)", nullable)),
		table("personal", idKey,
			column("id", "int", identity), column("nombre", "varchar(50)"), column("paterno", "varchar(30)", nullable),
			column("materno", "varchar(30)", nullable), column("fechaAlta", "date", nullable), column("puesto", "tinyint"),
			column("activo", "tinyint", nullable)),
		table("personal_detalles", nil,
			column("id", "int"), column("calle", "varchar(40)", nullable), column("num_ext", "varchar(6)", nullable),
			column("num_int", "varchar(6)", nullable), column("colonia", "varchar(50)", nullable), column("ciudad", "int"),
			column("cp", "char(5)", nullable), column("telefono", "varchar(13)", nullable),
			column("celular", "varchar(13)", nullable), column("email", "varchar(100)", nullable)),
		table("usuarios", idKey,
			column("id", "smallint", identity), column("usuario", "varchar(30)"), column("password", "varchar(128)"),
			column("personal", "int"), column("activo", "tinyint", nullable)),
		// Inventario y compras.
		table("articulos", idKey,
			column("id", "int", identity), column("articulo", "varchar(50)"), column("tipo", "smallint"),
			column("unidad", "varchar(4)"), column("registrado", "datetime", nullable)),
		table("articulos_tipos", idKey,
			column("id", "smallint", identity), column("tipo", "varchar(30)"), column("descripcion", "varchar(255)", nullable)),
		table("articulos_imagenes", nil,
			column("articulo", "int"), column("imagen", "varbinary(max)", nullable)),
		table("inventario", nil,
			column("articulo", "int"), column("stock", "numeric(18,3)", nullable),
			column("ultimaModificacion", "datetime", nullable)),
		table("ajustes_inventario", idKey,
			column("id", "int", identity), column("articulo", "int"), column("cantidad", "numeric(18,3)"),
			column("fecha", "datetime", nullable), column("descripcion", "varchar(255)", nullable)),
		table("recetas", nil,
			column("producto", "int"), column("articulo", "int"), column("cantidad", "numeric(18,3)"),
			column("ultimaModificacion", "datetime", nullable)),
		table("entradas", idKey,
			column("id", "int", identity), column("recibio", "smallint"), column("proveedor", "int"),
			column("subtotal", "money"), column("iva", "money"), column("total", "money"), column("fecha", "datetime")),
		table("entradas_detalles", nil,
			column("id", "int"), column("articulo", "int"), column("cantidad", "numeric(18,3)"),
			column("precioUnitario", "money"), column("subtotal", "money"), column("iva", "money"), column("total", "money")),
		table("proveedores", idKey,
			column("id", "int", identity), column("rfc", "varchar(14)"), column("proveedor", "varchar(50)"),
			column("fechaRegistrado", "date", nullable)),
		table("proveedores_detalles", nil,
			column("id", "int"), column("representante", "varchar(30)", nullable), column("calle", "varchar(40)", nullable),
			column("num_ext", "varchar(6)", nullable), column("num_int", "varchar(6)", nullable),
			column("colonia", "varchar(50)", nullable), column("ciudad", "int"), column("cp", "char(5)", nullable),
			column("telefono1", "varchar(13)", nullable), column("telefono2", "varchar(13)", nullable),
			column("celular", "varchar(13)", nullable), column("email", "varchar(100)", nullable),
			column("paginaweb", "varchar(80)", nullable)),
		// Impresión y equipos.
		table("impresoras", idKey,
			column("id", "tinyint", identity), column("impresora", "varchar(30)"), column("ubicacion", "varchar(30)"),
			column("ip", "varchar(15)"), column("cajaRegistradora", "tinyint", nullable),
			column("poscommand", "tinyint", nullable)),
		table("impresoras_poscommands", idKey,
			column("id", "tinyint"), column("com", "varchar(20)")),
		table("categorias_impresora", nil,
			column("categoria", "smallint"), column("impresora", "tinyint")),
		// Configuración y catálogos geográficos.
		table("info", nil,
			column("info", "varchar(30)"), column("valor", "varchar(255)")),
		table("paises", idKey,
			column("id", "smallint", identity), column("pais", "varchar(30)"), column("abbr", "varchar(3)", nullable)),
		table("estados", idKey,
			column("id", "int", identity), column("estado", "varchar(30)"), column("pais", "smallint")),
		table("ciudades", idKey,
			column("id", "int", identity), column("ciudad", "varchar(30)"), column("estado", "int")),
		table("colonias", idKey,
			column("id", "int", identity), column("colonias", "varchar(30)"), column("ciudad", "int")),
		// Herramienta (SQL Server Management Studio).
		table("sysdiagrams", []string{"diagram_id"},
			column("name", "nvarchar(128)"), column("principal_id", "int"), column("diagram_id", "int", identity),
			column("version", "int", nullable), column("definition", "varbinary(max)", nullable)),
	}
}
