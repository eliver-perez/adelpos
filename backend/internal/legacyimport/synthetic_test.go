package legacyimport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Paquete sintético en memoria con la estructura del sistema anterior (docs/MIGRACION_ANALISIS.md §1.3) y sus casos:
// IVA por producto (0 % y 16 %), historial de precios, complementos incluidos y quitados (bonificación), descuentos,
// cargos extra, cancelaciones de venta y de unidades, ventas abiertas, pagos en efectivo y con tarjeta, pagos repetidos,
// en cero y negativos, cortes abiertos y cerrados, retiros dudosos, ventas sin corte, ventas que no cuadran y un IVA
// nulo. Todo es inventado: nunca el respaldo real.

func money(text string) json.Number {
	return json.Number(text)
}

type syntheticTable struct {
	name    string
	columns []string
	rows    [][]any
}

func (table *syntheticTable) add(values map[string]any) {
	row := make([]any, len(table.columns))
	for index, column := range table.columns {
		value, found := values[column]
		if !found {
			continue
		}
		switch typed := value.(type) {
		case int:
			row[index] = json.Number(fmt.Sprint(typed))
		default:
			row[index] = typed
		}
	}
	table.rows = append(table.rows, row)
}

// syntheticPackage arma las tablas del paquete sintético. extraSales agrega ventas pagadas sencillas (para medir con
// volumen o probar avance) con fechas repartidas desde 2019.
type syntheticPackage struct {
	tables map[string]*syntheticTable
	order  []string
}

func newSyntheticPackage() *syntheticPackage {
	pkg := &syntheticPackage{tables: map[string]*syntheticTable{}}
	define := func(name string, columns ...string) {
		pkg.tables[name] = &syntheticTable{name: name, columns: columns}
		pkg.order = append(pkg.order, name)
	}
	// Orden alfabético, como un lector que enumera las tablas por nombre: el importador no depende del orden.
	define("cajas", "id", "machine_id", "caja", "fecha_alta")
	define("cancelaciones", "id", "venta", "autorizo", "fecha")
	define("categorias", "id", "categoria", "cocina")
	define("complementos", "id", "complemento")
	define("complementos_precios", "id", "fecha_inicio", "vigencia", "subtotal", "iva", "total")
	define("cortes", "id", "cajero", "inicio_caja", "fin_caja", "venta", "caja", "status", "autorizacion", "fecha_inicio",
		"fecha_fin", "retiros", "diferencia")
	define("cortes_parciales", "id", "corte", "monto", "razon", "fecha", "autorizacion")
	define("descuentos", "tipo", "venta", "descripcion", "descuento")
	define("divisas", "id", "divisa", "simbolo", "cambio")
	define("entradas", "id", "recibio", "proveedor", "subtotal", "iva", "total", "fecha")
	define("mesas", "id", "mesa", "capacidad")
	define("pagos", "id", "venta", "folio", "tipo", "monto", "recibio", "fecha", "corte")
	define("personal", "id", "nombre", "paterno", "materno", "fechaAlta", "puesto", "activo")
	define("productos", "id", "categoria", "producto", "nombre_corto", "unidad")
	define("productos_complementos", "producto", "complemento", "cantidad")
	define("productos_precios", "producto", "fecha_inicio", "vigencia", "subtotal", "iva", "total")
	define("puestos", "id", "puesto", "funciones")
	define("salidas", "corte", "monto", "fecha", "autorizacion")
	define("tipos_descuentos", "id", "tipo")
	define("tipos_pagos", "id", "tipo")
	define("usuarios", "id", "usuario", "password_present", "personal", "activo")
	define("ventas", "id", "caja", "mesa", "nombre", "folio", "fecha", "cajero", "facturado", "subtotal", "iva", "total",
		"montoPagado", "pagado", "extra", "montoExtra", "enmesa", "corte", "cancelado")
	define("ventas_complementos", "producto", "complemento", "incluido", "pedido", "subtotal", "iva", "total")
	define("ventas_modificaciones", "producto", "cantAnt", "cantNueva", "autorizacion", "fecha")
	define("ventas_productos", "id", "venta", "producto", "cantidad", "cantidadOriginal", "subtotal", "iva", "total",
		"observaciones")
	return pkg
}

func (pkg *syntheticPackage) table(name string) *syntheticTable {
	return pkg.tables[name]
}

func (pkg *syntheticPackage) source() *MemorySource {
	source := &MemorySource{Package: PackageInfo{
		FormatVersion:  1,
		Tool:           "legacyimport-test",
		ProductVersion: "0.10.0",
		Fingerprint:    strings.Repeat("ab", 32),
		BackupTakenAt:  "2026-11-14T22:10:05.123",
		Source:         json.RawMessage(`{"kind":"synthetic","database_name":"possys"}`),
	}}
	for _, name := range pkg.order {
		table := pkg.tables[name]
		source.Tables = append(source.Tables, MemoryTable{TableName: table.name, Names: table.columns, Data: table.rows})
	}
	return source
}

// Venta sintética: sus renglones (producto, cantidad, importes) y sus pagos.
type syntheticLine struct {
	id       int
	product  int
	quantity string
	subtotal string
	tax      any
	total    string
	notes    string
}

// standardSynthetic es el paquete de las pruebas con todos los casos. Las cifras esperadas de cada venta están
// comentadas junto a ella.
func standardSynthetic() *syntheticPackage {
	pkg := newSyntheticPackage()
	pkg.table("cajas").add(map[string]any{"id": 1, "machine_id": "CAJA-PRINCIPAL", "caja": "Caja principal", "fecha_alta": "2018-12-01T09:00:00"})
	pkg.table("cajas").add(map[string]any{"id": 2, "machine_id": "MOBILE", "caja": "Aplicación móvil", "fecha_alta": "2019-01-01T09:00:00"})
	pkg.table("categorias").add(map[string]any{"id": 1, "categoria": "Bebidas", "cocina": 0})
	pkg.table("categorias").add(map[string]any{"id": 2, "categoria": "Platillos", "cocina": 1})
	pkg.table("categorias").add(map[string]any{"id": 3, "categoria": "Postres", "cocina": 1})
	products := pkg.table("productos")
	products.add(map[string]any{"id": 1, "categoria": 1, "producto": "Agua fresca", "nombre_corto": "Agua", "unidad": "pza"})
	products.add(map[string]any{"id": 2, "categoria": 1, "producto": "Refresco", "nombre_corto": "Refresco", "unidad": "pza"})
	products.add(map[string]any{"id": 3, "categoria": 2, "producto": "Enchiladas \"suizas\" | rojas", "nombre_corto": "", "unidad": "pza"})
	products.add(map[string]any{"id": 4, "categoria": 2, "producto": "Filete", "nombre_corto": "Filete", "unidad": "pza"})
	products.add(map[string]any{"id": 5, "categoria": 2, "producto": "Hamburguesa", "nombre_corto": "Hambur", "unidad": "pza"})
	products.add(map[string]any{"id": 6, "categoria": 3, "producto": "Flan sin precio", "nombre_corto": "", "unidad": "pza"})
	prices := pkg.table("productos_precios")
	prices.add(map[string]any{"producto": 1, "fecha_inicio": "2019-01-01T08:00:00", "subtotal": money("25.0000"), "iva": money("0.0000"), "total": money("25.0000")})
	prices.add(map[string]any{"producto": 1, "fecha_inicio": "2023-06-01T08:00:00", "subtotal": money("30.0000"), "iva": money("0.0000"), "total": money("30.0000")})
	// El refresco pasó al 16 % en 2025: su precio más reciente lo tiene (decisión 1 del propietario).
	prices.add(map[string]any{"producto": 2, "fecha_inicio": "2019-01-01T08:00:00", "subtotal": money("20.0000"), "iva": money("0.0000"), "total": money("20.0000")})
	prices.add(map[string]any{"producto": 2, "fecha_inicio": "2025-11-01T08:00:00", "subtotal": money("25.8621"), "iva": money("4.1379"), "total": money("30.0000")})
	prices.add(map[string]any{"producto": 3, "fecha_inicio": "2019-01-01T08:00:00", "subtotal": money("95.5000"), "iva": money("0.0000"), "total": money("95.5000")})
	prices.add(map[string]any{"producto": 4, "fecha_inicio": "2019-01-01T08:00:00", "subtotal": money("150.0000"), "iva": money("0.0000"), "total": money("150.0000")})
	// Precio con tres decimales y una tasa que no es 0, 8 ni 16 (≈ 10 %): se redondea y se marca.
	prices.add(map[string]any{"producto": 5, "fecha_inicio": "2020-01-01T08:00:00", "subtotal": money("100.0000"), "iva": money("10.0000"), "total": money("110.0050")})
	pkg.table("complementos").add(map[string]any{"id": 1, "complemento": "Arroz"})
	pkg.table("complementos").add(map[string]any{"id": 2, "complemento": "Ensalada"})
	pkg.table("complementos").add(map[string]any{"id": 3, "complemento": "Papas"})
	complementPrices := pkg.table("complementos_precios")
	complementPrices.add(map[string]any{"id": 1, "fecha_inicio": "2020-03-01T08:00:00", "subtotal": money("17.2414"), "iva": money("2.7586"), "total": money("20.0000")})
	complementPrices.add(map[string]any{"id": 1, "fecha_inicio": "2023-08-01T08:00:00", "subtotal": money("20.0000"), "iva": money("0.0000"), "total": money("20.0000")})
	complementPrices.add(map[string]any{"id": 2, "fecha_inicio": "2020-03-01T08:00:00", "subtotal": money("15.0000"), "iva": money("0.0000"), "total": money("15.0000")})
	complementPrices.add(map[string]any{"id": 3, "fecha_inicio": "2020-03-01T08:00:00", "subtotal": money("25.0000"), "iva": money("0.0000"), "total": money("25.0000")})
	relations := pkg.table("productos_complementos")
	// El filete incluye arroz y ensalada ($35) y admite papas; la hamburguesa solo admite papas (otro conjunto).
	relations.add(map[string]any{"producto": 4, "complemento": 1, "cantidad": money("1.00")})
	relations.add(map[string]any{"producto": 4, "complemento": 2, "cantidad": money("1.00")})
	relations.add(map[string]any{"producto": 4, "complemento": 3, "cantidad": money("0.00")})
	relations.add(map[string]any{"producto": 5, "complemento": 3, "cantidad": money("0.00")})
	tables := pkg.table("mesas")
	tables.add(map[string]any{"id": 1, "mesa": "PARA LLEVAR", "capacidad": 0})
	tables.add(map[string]any{"id": 2, "mesa": "Mesa 1", "capacidad": 4})
	tables.add(map[string]any{"id": 3, "mesa": "Mesa 2", "capacidad": nil})
	tables.add(map[string]any{"id": 4, "mesa": "Terraza grande", "capacidad": 50})
	pkg.table("puestos").add(map[string]any{"id": 1, "puesto": "Administrador", "funciones": ""})
	pkg.table("puestos").add(map[string]any{"id": 2, "puesto": "Mesero", "funciones": ""})
	pkg.table("puestos").add(map[string]any{"id": 3, "puesto": "Limpieza", "funciones": ""})
	people := pkg.table("personal")
	people.add(map[string]any{"id": 1, "nombre": "Ana", "paterno": "Pérez", "materno": "Ruiz", "puesto": 1, "activo": 1})
	people.add(map[string]any{"id": 2, "nombre": "Beto", "paterno": "López", "materno": nil, "puesto": 2, "activo": 1})
	people.add(map[string]any{"id": 3, "nombre": "Carla", "paterno": "Díaz", "materno": "", "puesto": 3, "activo": 0})
	people.add(map[string]any{"id": 4, "nombre": "Dora", "paterno": "Sin usuario", "puesto": 2, "activo": 1})
	users := pkg.table("usuarios")
	// «Admin» choca con el administrador del asistente inicial; «1beto» empieza con número; «José» tiene acento.
	users.add(map[string]any{"id": 1, "usuario": "Admin", "password_present": true, "personal": 1, "activo": 1})
	users.add(map[string]any{"id": 2, "usuario": "1beto", "password_present": true, "personal": 2, "activo": 1})
	users.add(map[string]any{"id": 3, "usuario": "José", "password_present": true, "personal": 3, "activo": 1})
	pkg.table("tipos_pagos").add(map[string]any{"id": 1, "tipo": "Efectivo"})
	pkg.table("tipos_pagos").add(map[string]any{"id": 2, "tipo": "Tarjeta Credito/Debito"})
	pkg.table("tipos_descuentos").add(map[string]any{"id": 1, "tipo": "Atencion"})
	pkg.table("tipos_descuentos").add(map[string]any{"id": 2, "tipo": "Promocion"})
	pkg.table("divisas").add(map[string]any{"id": 1, "divisa": "Dólar", "simbolo": "USD", "cambio": money("17.5000")})
	shifts := pkg.table("cortes")
	// Corte 1 cerrado y cuadrado; corte 2 cerrado con «venta» y «retiros» que no cuadran; corte 3 abierto.
	shifts.add(map[string]any{"id": 1, "cajero": 1, "inicio_caja": money("500.0000"), "fin_caja": money("1000.0000"),
		"venta": money("328.0000"), "caja": 1, "status": "C", "autorizacion": 1, "fecha_inicio": "2024-03-15T08:00:00",
		"fecha_fin": "2024-03-15T22:00:00", "retiros": money("200.0000"), "diferencia": money("409.0000")})
	shifts.add(map[string]any{"id": 2, "cajero": 2, "inicio_caja": money("300.0000"), "fin_caja": money("0.0000"),
		"venta": money("999.0000"), "caja": 1, "status": "C", "autorizacion": 2, "fecha_inicio": "2025-01-10T08:00:00",
		"fecha_fin": "2025-01-10T22:00:00", "retiros": money("1.0000"), "diferencia": money("-300.0000")})
	shifts.add(map[string]any{"id": 3, "cajero": 1, "inicio_caja": money("100.0000"), "caja": 2, "status": "A",
		"fecha_inicio": "2026-11-14T08:00:00"})
	withdrawals := pkg.table("cortes_parciales")
	withdrawals.add(map[string]any{"id": 1, "corte": 1, "monto": money("200.0000"), "razon": "Pago a proveedor", "fecha": "2024-03-15T15:00:00", "autorizacion": 1})
	withdrawals.add(map[string]any{"id": 2, "corte": 2, "monto": money("0.0000"), "razon": "Prueba", "fecha": "2025-01-10T12:00:00", "autorizacion": 2})
	withdrawals.add(map[string]any{"id": 3, "corte": 2, "monto": money("150000.0000"), "razon": "¿Error?\nde captura", "fecha": "2025-01-10T13:00:00", "autorizacion": 2})

	sale := func(values map[string]any, lines []syntheticLine) {
		pkg.table("ventas").add(values)
		for _, line := range lines {
			pkg.table("ventas_productos").add(map[string]any{
				"id": line.id, "venta": values["id"], "producto": line.product, "cantidad": money(line.quantity),
				"cantidadOriginal": money(line.quantity), "subtotal": money(line.subtotal), "iva": line.tax,
				"total": money(line.total), "observaciones": line.notes,
			})
		}
	}
	payment := func(id, saleID, kind int, amount, date string, shift any) {
		pkg.table("pagos").add(map[string]any{"id": id, "venta": saleID, "folio": "", "tipo": kind, "monto": money(amount),
			"recibio": 1, "fecha": date, "corte": shift})
	}
	// Venta 1 (mesa, 2019): 2 aguas y unas enchiladas = 145.5; un pago en efectivo; sin corte (el anterior no tenía
	// cortes en 2019). Debe 145.5.
	sale(map[string]any{"id": 1, "caja": 1, "mesa": 2, "nombre": "Mesa de Ana", "folio": "1", "fecha": "2019-10-05T13:00:00",
		"cajero": 1, "facturado": 0, "subtotal": money("145.5000"), "iva": money("0.0000"), "total": money("145.5000"),
		"montoPagado": money("145.5000"), "pagado": 1, "extra": "", "montoExtra": money("0.0000"), "enmesa": 0,
		"corte": nil, "cancelado": 0}, []syntheticLine{
		{id: 1, product: 1, quantity: "2.000", subtotal: "50.0000", tax: money("0.0000"), total: "50.0000"},
		{id: 2, product: 3, quantity: "1.000", subtotal: "95.5000", tax: money("0.0000"), total: "95.5000", notes: "Sin \"crema\"\tbien | caliente"},
	})
	payment(1, 1, 1, "145.5000", "2019-10-05T13:50:00", nil)
	// Venta 2 (para llevar, 2024, corte 1): un filete con arroz quitado (−20, bonificación) y papas agregadas (+25) =
	// 155; con tarjeta. Debe 155.
	sale(map[string]any{"id": 2, "caja": 1, "mesa": 1, "nombre": "", "folio": "2", "fecha": "2024-03-15T14:00:00",
		"cajero": 2, "facturado": 0, "subtotal": money("155.0000"), "iva": money("0.0000"), "total": money("155.0000"),
		"montoPagado": money("155.0000"), "pagado": 1, "montoExtra": money("0.0000"), "enmesa": 0, "corte": 1,
		"cancelado": 0}, []syntheticLine{
		{id: 3, product: 4, quantity: "1.000", subtotal: "150.0000", tax: money("0.0000"), total: "150.0000"},
	})
	pkg.table("ventas_complementos").add(map[string]any{"producto": 3, "complemento": 1, "incluido": money("1.000"),
		"pedido": money("0.000"), "subtotal": money("-20.0000"), "iva": money("0.0000"), "total": money("-20.0000")})
	pkg.table("ventas_complementos").add(map[string]any{"producto": 3, "complemento": 2, "incluido": money("1.000"),
		"pedido": money("1.000"), "subtotal": money("0.0000"), "iva": money("0.0000"), "total": money("0.0000")})
	pkg.table("ventas_complementos").add(map[string]any{"producto": 3, "complemento": 3, "incluido": money("0.000"),
		"pedido": money("1.000"), "subtotal": money("25.0000"), "iva": money("0.0000"), "total": money("25.0000")})
	payment(2, 2, 2, "155.0000", "2024-03-15T14:40:00", 1)
	// Venta 3 (cancelada, 2024, corte 1, con cargo extra): no es venta. Al estilo del anterior: 95.5 + 10.
	sale(map[string]any{"id": 3, "caja": 1, "mesa": 3, "folio": "3", "fecha": "2024-03-15T15:00:00", "cajero": 1,
		"subtotal": money("95.5000"), "iva": money("0.0000"), "total": money("95.5000"), "montoPagado": money("0.0000"),
		"pagado": 1, "extra": "Servicio a domicilio", "montoExtra": money("10.0000"), "enmesa": 1, "corte": 1,
		"cancelado": 0}, []syntheticLine{
		{id: 4, product: 3, quantity: "1.000", subtotal: "95.5000", tax: money("0.0000"), total: "95.5000"},
	})
	pkg.table("cancelaciones").add(map[string]any{"id": 1, "venta": 3, "autorizo": 1, "fecha": "2024-03-15T15:20:00"})
	// Venta 4 (abierta al hacer el respaldo): historial, no venta.
	sale(map[string]any{"id": 4, "caja": 2, "mesa": 2, "folio": "4", "fecha": "2026-11-14T20:00:00", "cajero": 2,
		"subtotal": money("30.0000"), "iva": money("0.0000"), "total": money("30.0000"), "pagado": 0, "corte": nil,
		"cancelado": 0}, []syntheticLine{
		{id: 5, product: 1, quantity: "1.000", subtotal: "30.0000", tax: money("0.0000"), total: "30.0000"},
	})
	// Venta 5 (2025, corte 2): el total guardado (100) no cuadra con sus renglones (95.5): la diferencia de 4.5 es
	// «Diferencia del sistema anterior». Debe 100.
	sale(map[string]any{"id": 5, "caja": 1, "mesa": 2, "folio": "5", "fecha": "2025-01-10T13:00:00", "cajero": 2,
		"subtotal": money("100.0000"), "iva": money("0.0000"), "total": money("100.0000"), "montoPagado": money("100.0000"),
		"pagado": 1, "corte": 2, "cancelado": 0}, []syntheticLine{
		{id: 6, product: 3, quantity: "1.000", subtotal: "95.5000", tax: money("0.0000"), total: "95.5000"},
	})
	payment(3, 5, 1, "100.0000", "2025-01-10T13:30:00", 2)
	// Venta 6 (2024, corte 1): 3 refrescos de 20 = 60, descuento de 5 «Atencion» y cargo extra de 12.5 = debe 67.5.
	sale(map[string]any{"id": 6, "caja": 1, "mesa": 2, "folio": "6", "fecha": "2024-03-15T16:00:00", "cajero": 1,
		"subtotal": money("60.0000"), "iva": money("0.0000"), "total": money("60.0000"), "montoPagado": money("67.5000"),
		"pagado": 1, "extra": "Descorche", "montoExtra": money("12.5000"), "corte": 1, "cancelado": 0}, []syntheticLine{
		{id: 7, product: 2, quantity: "3.000", subtotal: "60.0000", tax: money("0.0000"), total: "60.0000"},
	})
	pkg.table("descuentos").add(map[string]any{"tipo": 1, "venta": 6, "descripcion": "Cliente frecuente", "descuento": money("5.0000")})
	payment(4, 6, 1, "67.5000", "2024-03-15T16:30:00", 1)
	// Venta 7 (2025, corte 2): un refresco al 16 % (30 = 25.8621 + 4.1379); el pago se repitió (60 de 30): cobrada de
	// más, el segundo pago es posiblemente repetido. Debe 30.
	sale(map[string]any{"id": 7, "caja": 1, "mesa": 1, "folio": "7", "fecha": "2025-11-20T12:00:00", "cajero": 1,
		"subtotal": money("25.8621"), "iva": money("4.1379"), "total": money("30.0000"), "montoPagado": money("60.0000"),
		"pagado": 1, "corte": 2, "cancelado": 0}, []syntheticLine{
		{id: 8, product: 2, quantity: "1.000", subtotal: "25.8621", tax: money("4.1379"), total: "30.0000"},
	})
	payment(5, 7, 1, "30.0000", "2025-11-20T12:10:00", 2)
	payment(6, 7, 1, "30.0000", "2025-11-20T12:11:00", 2)
	// Venta 8 (2026, corte 3): un pago en cero y uno negativo además del bueno; folio repetido con la venta 9.
	sale(map[string]any{"id": 8, "caja": 2, "mesa": 1, "folio": "8", "fecha": "2026-02-01T12:00:00", "cajero": 1,
		"subtotal": money("30.0000"), "iva": money("0.0000"), "total": money("30.0000"), "montoPagado": money("30.0000"),
		"pagado": 1, "corte": 3, "cancelado": 0}, []syntheticLine{
		{id: 9, product: 1, quantity: "1.000", subtotal: "30.0000", tax: money("0.0000"), total: "30.0000"},
	})
	payment(7, 8, 1, "0.0000", "2026-02-01T12:05:00", 3)
	payment(8, 8, 1, "-1.0000", "2026-02-01T12:06:00", 3)
	payment(9, 8, 1, "31.0000", "2026-02-01T12:07:00", 3)
	// Venta 9 (2025 → pagada en 2026, sin corte): el último pago llega días después (cuenta en 2026). Un renglón sin IVA
	// guardado (IVA estimado con la tasa vigente del refresco: 16 % desde 11/2025) y la venta sin IVA guardado.
	sale(map[string]any{"id": 9, "caja": 1, "mesa": 3, "folio": "8", "fecha": "2025-12-30T21:00:00", "cajero": 2,
		"subtotal": money("25.8621"), "iva": nil, "total": money("30.0000"), "montoPagado": money("30.0000"),
		"pagado": 1, "corte": nil, "cancelado": 0}, []syntheticLine{
		{id: 10, product: 2, quantity: "1.000", subtotal: "25.8621", tax: nil, total: "30.0000"},
	})
	payment(10, 9, 1, "30.0000", "2026-01-04T10:00:00", nil)
	// Venta 10 (2024, sin renglones, total 0, pagada sin pagos): cuenta en su fecha de apertura con 0.
	sale(map[string]any{"id": 10, "caja": 1, "mesa": 2, "folio": "10", "fecha": "2024-07-01T12:00:00", "cajero": 1,
		"subtotal": money("0.0000"), "iva": money("0.0000"), "total": money("0.0000"), "montoPagado": money("0.0000"),
		"pagado": 1, "corte": 1, "cancelado": 0}, nil)
	// Venta 11 (2026, corte 3): cobrada de menos (paga 20 de 25.5) y con una cantidad reducida y una cancelación de unidad
	// registrada en ventas_modificaciones.
	sale(map[string]any{"id": 11, "caja": 2, "mesa": 4, "folio": "11", "fecha": "2026-03-03T14:00:00", "cajero": 3,
		"subtotal": money("25.5000"), "iva": money("0.0000"), "total": money("25.5000"), "montoPagado": money("20.0000"),
		"pagado": 1, "corte": 3, "cancelado": 0}, []syntheticLine{
		{id: 11, product: 1, quantity: "0.850", subtotal: "25.5000", tax: money("0.0000"), total: "25.5000"},
		{id: 12, product: 3, quantity: "0.000", subtotal: "0.0000", tax: money("0.0000"), total: "0.0000"},
	})
	payment(11, 11, 2, "20.0000", "2026-03-03T14:30:00", 3)
	pkg.table("ventas_modificaciones").add(map[string]any{"producto": 11, "cantAnt": money("1.000"), "cantNueva": money("0.850"),
		"autorizacion": 1, "fecha": "2026-03-03T14:10:00"})
	pkg.table("ventas_modificaciones").add(map[string]any{"producto": 12, "cantAnt": money("1.000"), "cantNueva": money("0.000"),
		"autorizacion": 1, "fecha": "2026-03-03T14:11:00"})
	return pkg
}

// addVolumeSales agrega count ventas pagadas sencillas (una por cada minuto desde 2019) con dos renglones, un
// complemento cada tercera venta y un pago, para medir con volumen.
func (pkg *syntheticPackage) addVolumeSales(count int) {
	sales := pkg.table("ventas")
	lines := pkg.table("ventas_productos")
	complements := pkg.table("ventas_complementos")
	payments := pkg.table("pagos")
	firstSale, firstLine, firstPayment := 1000, 1000, 1000
	for index := 0; index < count; index++ {
		id := firstSale + index
		minute := index * 17
		date := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:00", 2019+index%8, 1+index%12, 1+index%28, 8+minute/60%12, minute%60)
		sales.add(map[string]any{"id": id, "caja": 1, "mesa": 1 + index%4, "folio": fmt.Sprint(id), "fecha": date, "cajero": 1,
			"subtotal": money("175.0000"), "iva": money("0.0000"), "total": money("175.0000"), "montoPagado": money("175.0000"),
			"pagado": 1, "corte": 1, "cancelado": 0})
		lineID := firstLine + index*3
		lines.add(map[string]any{"id": lineID, "venta": id, "producto": 4, "cantidad": money("1.000"),
			"cantidadOriginal": money("1.000"), "subtotal": money("150.0000"), "iva": money("0.0000"), "total": money("150.0000"),
			"observaciones": ""})
		lines.add(map[string]any{"id": lineID + 1, "venta": id, "producto": 1, "cantidad": money("1.000"),
			"cantidadOriginal": money("1.000"), "subtotal": money("25.0000"), "iva": money("0.0000"), "total": money("25.0000"),
			"observaciones": ""})
		if index%3 == 0 {
			complements.add(map[string]any{"producto": lineID, "complemento": 2, "incluido": money("1.000"), "pedido": money("1.000"),
				"subtotal": money("0.0000"), "iva": money("0.0000"), "total": money("0.0000")})
		}
		payments.add(map[string]any{"id": firstPayment + index, "venta": id, "tipo": 1 + index%2, "monto": money("175.0000"),
			"recibio": 1, "fecha": date, "corte": 1})
	}
}
