package legacysynth

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/shopspring/decimal"
)

// Fecha y hora sin zona, como las guardaba el sistema anterior (hora local del equipo). Los milisegundos terminan en
// 0, 3 o 7 porque datetime de SQL Server los redondea a 1/300 de segundo.
const datetimeLayout = "2006-01-02T15:04:05.000"

// salesCount es el número de ventas sintéticas (de 10/2019 a 11/2026).
const salesCount = 240

// difficultTexts son textos que deben llegar intactos al paquete: acentos, comillas, saltos de línea, «|», caracteres
// de control, barra invertida y espacios al final. Solo caracteres de la página de códigos 1252 (las columnas varchar
// del sistema anterior no guardan otros).
var difficultTexts = []string{
	"Ñandú «comillas» \"dobles\" y 'simples'",
	"línea 1\nlínea 2\r\nfin",
	"barra | vertical",
	"tab\taquí",
	"control \x01 y \x1f",
	`barra \ invertida`,
	"euro € ½ ü ×",
	"<b>&amp;</b>",
	"  espacios al final  ",
	"",
}

var millis = []int{0, 3, 7, 10, 123, 250, 457, 500, 997}

type generator struct {
	tables map[string]*Table
	random *rand.Rand
}

func (generator *generator) add(name string, values ...any) {
	table := generator.tables[name]
	if len(values) != len(table.Columns) {
		panic(fmt.Sprintf("legacysynth: la tabla %s tiene %d columnas y la fila %d valores", name, len(table.Columns), len(values)))
	}
	table.Rows = append(table.Rows, values)
}

func moneyText(value decimal.Decimal) string {
	return value.StringFixed(4)
}

func quantityText(value decimal.Decimal) string {
	return value.StringFixed(3)
}

func amount(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

func stamp(year int, month time.Month, day, hour, minute, second, milli int) time.Time {
	return time.Date(year, month, day, hour, minute, second, milli*int(time.Millisecond), time.UTC)
}

func when(moment time.Time) string {
	return moment.Format(datetimeLayout)
}

func day(year int, month time.Month, dayOfMonth int) string {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// Dataset construye la base sintética completa: siempre la misma (generador con semilla fija).
func Dataset() []*Table {
	tables := schema()
	generator := &generator{tables: map[string]*Table{}, random: rand.New(rand.NewPCG(20261115, 9))}
	for _, table := range tables {
		generator.tables[table.Name] = table
	}
	generator.reference()
	catalog := generator.catalog()
	generator.people()
	generator.history(catalog)
	return tables
}

// reference son los catálogos pequeños: unidades, categorías, formas de pago, divisas, puestos, mesas, cajas,
// impresoras, datos del establecimiento, geografía, inventario y el diagrama de SSMS.
func (generator *generator) reference() {
	units := [][2]any{{"g", "Gramos"}, {"kg", "Kilogramos"}, {"ml", "Mililitros"}, {"lt", "Litros"}, {"pza", "Pieza"}, {"ord", nil}}
	for _, unit := range units {
		generator.add("unidades", unit[0], unit[1])
	}
	categories := []struct {
		name    string
		kitchen any
	}{{"Tacos", int64(1)}, {"Bebidas", int64(0)}, {"Postres", int64(1)}, {"Platillos", int64(1)}, {"Extras", nil}}
	for index, category := range categories {
		generator.add("categorias", int64(index+1), category.name, category.kitchen)
	}
	generator.add("tipos_pagos", int64(1), "Efectivo")
	generator.add("tipos_pagos", int64(2), "Tarjeta Credito/Debito")
	generator.add("divisas", int64(1), "Peso mexicano", "$", "1.0000")
	generator.add("divisas", int64(2), "Dólar", "USD", "17.2500")
	for index, kind := range []string{"Atencion", "Promocion", "Demora"} {
		generator.add("tipos_descuentos", int64(index+1), kind)
	}
	positions := [][2]any{
		{"Administrador", "Autoriza cancelaciones y cortes"}, {"Cajero", "Cobra"}, {"Mesero", "Atiende mesas"},
		{"Cocinero", nil}, {"Limpieza", "Limpieza del local"},
	}
	for index, position := range positions {
		generator.add("puestos", int64(index+1), position[0], position[1])
	}
	generator.add("mesas", int64(1), "PARA LLEVAR", nil)
	tables := []struct {
		name  any
		seats any
	}{
		{"Mesa 1", int64(4)}, {"Mesa 2", int64(2)}, {"Mesa 3", int64(6)}, {"Barra", int64(3)},
		{"Terraza «A»", int64(8)}, {nil, int64(4)}, {"Mesa 7", nil},
	}
	for index, table := range tables {
		generator.add("mesas", int64(index+2), table.name, table.seats)
	}
	generator.add("cajas", int64(1), "MAQ-SINT-01", "CAJA PRINCIPAL", when(stamp(2019, 9, 1, 9, 0, 0, 0)), int64(1), int64(2))
	generator.add("cajas", int64(2), "MAQ-SINT-02", "CAJA BARRA", when(stamp(2022, 1, 10, 9, 30, 0, 0)), int64(1), nil)
	generator.add("cajas", int64(3), "MOBILE", "MOBILE", when(stamp(2020, 3, 1, 12, 0, 0, 0)), nil, nil)
	generator.add("impresoras", int64(1), "Tickets", "Caja", "192.0.2.10", int64(1), int64(1))
	generator.add("impresoras", int64(2), "Cocina", "Cocina", "192.0.2.11", int64(0), int64(2))
	generator.add("impresoras_poscommands", int64(1), "ESC/POS")
	generator.add("impresoras_poscommands", int64(2), "STAR")
	settings := [][2]string{
		{"conf_nombre", "Restaurante Sintético «La Prueba»"}, {"conf_rfc", "XAXX010101000"},
		{"conf_calle", "Calle Inventada"}, {"conf_numero", "123"}, {"conf_colonia", "Centro"},
		{"conf_cp", "00000"}, {"conf_ciudad", "Ciudad Sintética"}, {"conf_estado", "Estado Sintético"},
		{"conf_pais", "México"}, {"conf_telefono", "000 000 0000"}, {"conf_email", "contacto@ejemplo.invalid"},
		{"conf_paginaweb", "https://ejemplo.invalid"}, {"conf_mensaje", "¡Gracias por su visita!\nVuelva pronto"},
		{"conf_iva", "16"},
	}
	for _, setting := range settings {
		generator.add("info", setting[0], setting[1])
	}
	generator.add("paises", int64(1), "México", "MEX")
	generator.add("estados", int64(1), "Estado Sintético", int64(1))
	generator.add("ciudades", int64(1), "Ciudad Sintética", int64(1))
	itemKinds := []string{"Abarrotes", "Carnes", "Verduras", "Bebidas", "Desechables", "Limpieza"}
	for index, kind := range itemKinds {
		var description any = "Tipo de artículo " + kind
		if index == 5 {
			description = nil
		}
		generator.add("articulos_tipos", int64(index+1), kind, description)
	}
	generator.add("articulos", int64(1), "Harina", int64(1), "kg", when(stamp(2020, 1, 15, 10, 0, 0, 0)))
	generator.add("inventario", int64(1), "12.500", when(stamp(2024, 5, 2, 18, 45, 10, 457)))
	generator.add("sysdiagrams", "Diagrama sintético", int64(1), int64(1), int64(1), []byte{0xd0, 0xcf, 0x11, 0xe0, 0x01})
}

// price es un precio con fecha de inicio: subtotal, IVA y total por unidad.
type price struct {
	start    time.Time
	subtotal decimal.Decimal
	tax      decimal.Decimal
	total    decimal.Decimal
}

// catalogData es lo que la historia necesita del catálogo.
type catalogData struct {
	productPrices    map[int][]price
	complementPrices map[int][]price
	relations        map[int][]relation
	sellable         []int
}

type relation struct {
	complement int
	included   decimal.Decimal
}

// priceAt es el precio más reciente con inicio anterior a la fecha o, si no hay (renglones sin precio anterior), el
// primero.
func priceAt(prices []price, moment time.Time) price {
	chosen := prices[0]
	for _, candidate := range prices {
		if !candidate.start.After(moment) {
			chosen = candidate
		}
	}
	return chosen
}

func zeroTax(total string) price {
	value := amount(total)
	return price{subtotal: value, tax: decimal.Zero, total: value}
}

func withTax(subtotal, tax string) price {
	return price{subtotal: amount(subtotal), tax: amount(tax), total: amount(subtotal).Add(amount(tax))}
}

func (generator *generator) catalog() catalogData {
	products := []struct {
		category  int
		name      string
		shortName any
		unit      string
	}{
		{1, "Taco de pastor", "Pastor", "pza"}, {1, "Taco de suadero", "Suadero", "pza"}, {1, "Gringa", nil, "pza"},
		{1, "Quesadilla", "Quesa", "pza"}, {2, "Agua fresca", "Agua", "ml"}, {2, "Refresco", nil, "pza"},
		{2, "Café de olla", "Café", "ml"}, {3, "Flan", nil, "pza"}, {3, "Pastel de elote", "Pastel", "pza"},
		{4, "Enchiladas", nil, "ord"}, {4, "Pozole", "Pozole", "ord"}, {4, "Torta «especial»", "Torta", "pza"},
		{5, "Agua fresca", "Agua extra", "ml"}, {4, "Sopa del día", nil, "ord"}, {1, "Taco campechano", nil, "pza"},
		{2, "Cerveza", nil, "pza"}, {3, "Gelatina", nil, "pza"}, {5, "Producto sin ventas", nil, "pza"},
		{4, "Tamal", nil, "pza"}, {4, "Platillo del día", "Del día", "ord"},
	}
	data := catalogData{
		productPrices:    map[int][]price{},
		complementPrices: map[int][]price{},
		relations:        map[int][]relation{},
	}
	for index, product := range products {
		id := index + 1
		var cover any
		if id <= 3 {
			cover = []byte(fmt.Sprintf("\x89PNG imagen sintética %d", id))
		}
		generator.add("productos", int64(id), int64(product.category), product.name, product.shortName, product.unit, cover)
		var prices []price
		switch id {
		case 19:
			// Precio que empieza después de su primera venta (renglón sin precio anterior).
			prices = []price{zeroTax("32.0000")}
			prices[0].start = stamp(2023, 6, 1, 0, 0, 0, 0)
		case 20:
			// El producto al 16 % (desde 11/2025), con IVA incluido en el total.
			prices = []price{withTax("103.4483", "16.5517"), withTax("112.0690", "17.9310")}
			prices[0].start = stamp(2025, 11, 1, 0, 0, 0, 0)
			prices[1].start = stamp(2026, 3, 1, 0, 0, 0, 0)
		default:
			base := decimal.NewFromInt(int64(20 + 5*id))
			prices = []price{
				zeroTax(moneyText(base)),
				zeroTax(moneyText(base.Mul(amount("1.1")).Round(1))),
				zeroTax(moneyText(base.Mul(amount("1.25")).Round(0))),
			}
			prices[0].start = stamp(2019, 9, 1, 0, 0, 0, 0)
			prices[1].start = stamp(2022, 1, 1, 0, 0, 0, 0)
			prices[2].start = stamp(2025, 1, 1, 0, 0, 0, 0)
		}
		data.productPrices[id] = prices
		for priceIndex, current := range prices {
			var validity any
			if priceIndex+1 < len(prices) && id%2 == 0 {
				validity = when(prices[priceIndex+1].start.Add(-time.Second))
			}
			generator.add("productos_precios", int64(id), when(current.start), validity,
				moneyText(current.subtotal), moneyText(current.tax), moneyText(current.total))
		}
		if id != 18 && id != 19 && id != 20 {
			data.sellable = append(data.sellable, id)
		}
	}
	complements := []struct {
		name       string
		withTax    [2]string
		withoutTax string
	}{
		{"Queso extra", [2]string{"12.9310", "2.0690"}, "18.0000"},
		{"Aguacate", [2]string{"8.6207", "1.3793"}, "12.0000"},
		{"Cebolla", [2]string{"4.3103", "0.6897"}, "5.0000"},
		{"Cilantro", [2]string{"4.3103", "0.6897"}, "5.0000"},
		{"Salsa verde", [2]string{"2.5862", "0.4138"}, "3.0000"},
		{"Tortilla de harina", [2]string{"5.1724", "0.8276"}, "7.0000"},
	}
	for index, complement := range complements {
		id := index + 1
		generator.add("complementos", int64(id), complement.name)
		taxed := withTax(complement.withTax[0], complement.withTax[1])
		taxed.start = stamp(2020, 3, 1, 0, 0, 0, 0)
		untaxed := zeroTax(complement.withoutTax)
		untaxed.start = stamp(2023, 7, 15, 0, 0, 0, 0)
		data.complementPrices[id] = []price{taxed, untaxed}
		for _, current := range data.complementPrices[id] {
			generator.add("complementos_precios", int64(id), when(current.start), nil,
				moneyText(current.subtotal), moneyText(current.tax), moneyText(current.total))
		}
	}
	// Los tacos incluyen cebolla y cilantro (cantidad 1) y admiten queso, aguacate y salsa; otros platillos, queso.
	for _, product := range []int{1, 2, 3, 4, 15} {
		for _, link := range [][2]any{{3, "1.00"}, {4, "1.00"}, {1, "0.00"}, {2, "0.00"}, {5, "0.00"}} {
			generator.add("productos_complementos", int64(product), int64(link[0].(int)), link[1])
			data.relations[product] = append(data.relations[product], relation{complement: link[0].(int), included: amount(link[1].(string))})
		}
	}
	generator.add("productos_complementos", int64(10), int64(1), "0.00")
	generator.add("productos_complementos", int64(11), int64(6), nil)
	data.relations[10] = []relation{{complement: 1, included: decimal.Zero}}
	data.relations[11] = []relation{{complement: 6, included: decimal.Zero}}
	generator.add("productos_imagenes", int64(2), []byte("\x89PNG imagen adicional"), int64(1))
	generator.add("productos_imagenes", int64(3), nil, int64(2))
	return data
}

// people son el personal, sus datos personales (que no viajan en el paquete) y los usuarios con contraseñas
// inventadas (que tampoco viajan: solo «tenía contraseña»).
func (generator *generator) people() {
	staff := []struct {
		name, last string
		second     any
		hired      any
		position   int
		active     any
	}{
		{"Administrador", "Sintético", "Uno", day(2019, 9, 1), 1, int64(1)},
		{"Gerente", "Ejemplo", nil, day(2020, 2, 1), 1, int64(1)},
		{"Cajera", "Prueba", "Dos", day(2021, 6, 15), 2, int64(1)},
		{"José", "Mesero", nil, day(2022, 8, 20), 3, int64(1)},
		{"Cocinero", "Sin", "Usuario", nil, 4, int64(1)},
		{"Limpieza", "Sin", "Usuario", day(2023, 1, 9), 5, int64(0)},
	}
	for index, person := range staff {
		generator.add("personal", int64(index+1), person.name, person.last, person.second, person.hired, int64(person.position), person.active)
	}
	generator.add("personal_detalles", int64(1), "Calle Falsa", "123", nil, "Colonia Inventada", int64(1), "00000",
		"0000000000", nil, "uno@ejemplo.invalid")
	generator.add("personal_detalles", int64(2), "Avenida Sintética", "45", "B", nil, int64(1), "00001", nil,
		"0000000001", "dos@ejemplo.invalid")
	generator.add("personal_detalles", int64(4), nil, nil, nil, nil, int64(1), nil, nil, nil, nil)
	for index, user := range SyntheticPasswords {
		active := int64(1)
		if index == 3 {
			active = 0
		}
		generator.add("usuarios", int64(index+1), user.User, user.Password, int64(index+1), active)
	}
}

// SyntheticPassword es una contraseña inventada de la base sintética. Las pruebas comprueban que ninguna aparece en el
// paquete.
type SyntheticPassword struct {
	User     string
	Password string
}

// SyntheticPasswords son las contraseñas de la base sintética (la última vacía: «sin contraseña»).
var SyntheticPasswords = []SyntheticPassword{
	{User: "admin1", Password: "SINTETICO-HASH-ADMIN-0001"},
	{User: "gerente", Password: "SINTETICO-HASH-GERENTE-0002"},
	{User: "cajera", Password: "SINTETICO-HASH-CAJERA-0003"},
	{User: "mesero1", Password: ""},
}
