package legacyimport

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// Tablas que una importación necesita: sin ellas el paquete no es del sistema anterior (o de una versión con otra
// estructura). Las demás son opcionales (vacías si faltan).
var requiredTables = []string{"categorias", "productos", "mesas", "usuarios", "ventas", "ventas_productos", "pagos"}

// LoadProgress informa el avance de la lectura: filas leídas de todas las tablas y el total del manifiesto.
type LoadProgress func(table string, rowsRead, totalRows int64)

// progressEvery es cada cuántas filas se informa el avance de la lectura.
const progressEvery = 20000

// Load lee el paquete completo y devuelve sus filas tipadas. Las fechas se interpretan en location (la zona del
// establecimiento). Las tablas que la importación no usa se recorren igual (el lector comprueba sus huellas) y solo
// se cuentan. Devuelve un *Error con CodeIncompatible si falta una tabla o columna necesaria o un valor no tiene la
// forma esperada, o el error del lector (frase incorrecta, paquete dañado) tal cual. ctx permite interrumpir la
// lectura al apagar el servidor.
func Load(ctx context.Context, source Source, location *time.Location, progress LoadProgress) (*Model, error) {
	model := &Model{Info: source.Info(), Location: location, TableRows: map[string]int64{}}
	total := model.Info.TotalRows()
	var read int64
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		table, err := source.NextTable()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.ToLower(table.Name())
		seen[name] = true
		rows := &rowReader{model: model, table: name, columns: columnIndex(table.Columns())}
		decode, known := decoders[name]
		if !known {
			model.UnreadTables = append(model.UnreadTables, name)
		} else if err := rows.require(decode.columns); err != nil {
			return nil, err
		}
		var count int64
		for {
			values, err := table.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			count++
			read++
			if known {
				rows.values = values
				rows.position = int32(count)
				decode.read(rows)
				if rows.err != nil {
					return nil, rows.err
				}
			}
			if progress != nil && read%progressEvery == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				progress(name, read, total)
			}
		}
		model.TableRows[name] = count
	}
	for _, name := range requiredTables {
		if !seen[name] {
			return nil, incompatible(name, "", "tabla requerida ausente")
		}
	}
	if progress != nil {
		progress("", read, total)
	}
	return model, nil
}

// columnIndex ubica cada columna por su nombre sin distinguir mayúsculas (como SQL Server con la intercalación del
// anterior).
func columnIndex(columns []string) map[string]int {
	index := make(map[string]int, len(columns))
	for position, name := range columns {
		index[strings.ToLower(name)] = position
	}
	return index
}

// rowReader lee los valores de una fila por nombre de columna y guarda el primer error (sin el valor, que es dato del
// restaurante).
type rowReader struct {
	model    *Model
	table    string
	columns  map[string]int
	values   []any
	position int32
	err      error
}

func (rows *rowReader) require(columns []string) error {
	for _, column := range columns {
		if _, ok := rows.columns[strings.ToLower(column)]; !ok {
			return incompatible(rows.table, column, "columna requerida ausente")
		}
	}
	return nil
}

func (rows *rowReader) raw(column string) (any, bool) {
	position, ok := rows.columns[strings.ToLower(column)]
	if !ok || position >= len(rows.values) {
		return nil, false
	}
	return rows.values[position], true
}

func (rows *rowReader) fail(column string) {
	if rows.err == nil {
		rows.err = incompatible(rows.table, column, "valor con formato inesperado")
	}
}

func (rows *rowReader) nullInt(column string) NullInt {
	value, _ := rows.raw(column)
	parsed, err := ParseInt(value)
	if err != nil {
		rows.fail(column)
	}
	return parsed
}

// integer lee una llave obligatoria (una llave nula es un paquete con otra estructura).
func (rows *rowReader) integer(column string) int32 {
	parsed := rows.nullInt(column)
	if !parsed.Valid {
		rows.fail(column)
	}
	return parsed.Value
}

func (rows *rowReader) nullMoney(column string) NullMoney {
	value, _ := rows.raw(column)
	parsed, err := ParseMoney(value)
	if err != nil {
		rows.fail(column)
	}
	return parsed
}

// money lee un importe; uno nulo cuenta como 0 (el llamador decide si eso merece una marca).
func (rows *rowReader) money(column string) Money {
	return rows.nullMoney(column).Value
}

func (rows *rowReader) nullQuantity(column string) NullQuantity {
	value, _ := rows.raw(column)
	parsed, err := ParseQuantity(value)
	if err != nil {
		rows.fail(column)
	}
	return parsed
}

func (rows *rowReader) text(column string) string {
	value, _ := rows.raw(column)
	parsed, _, err := ParseText(value)
	if err != nil {
		rows.fail(column)
	}
	return parsed
}

func (rows *rowReader) textPresent(column string) (string, bool) {
	value, _ := rows.raw(column)
	parsed, present, err := ParseText(value)
	if err != nil {
		rows.fail(column)
	}
	return parsed, present
}

func (rows *rowReader) time(column string) time.Time {
	value, _ := rows.raw(column)
	parsed, err := ParseLocalTime(value, rows.model.Location)
	if err != nil {
		rows.fail(column)
	}
	return parsed
}

// requiredTime lee una fecha obligatoria (DATETIME NOT NULL en el anterior).
func (rows *rowReader) requiredTime(column string) time.Time {
	parsed := rows.time(column)
	if parsed.IsZero() {
		rows.fail(column)
	}
	return parsed
}

func (rows *rowReader) truthy(column string) bool {
	value, ok := rows.raw(column)
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case nil:
		return false
	}
	parsed, err := ParseInt(value)
	return err == nil && parsed.Valid && parsed.Value != 0
}

// tableDecoder son las columnas obligatorias de una tabla y cómo se agrega cada fila al modelo.
type tableDecoder struct {
	columns []string
	read    func(rows *rowReader)
}

var decoders = map[string]tableDecoder{
	"categorias": {[]string{"id", "categoria"}, func(rows *rowReader) {
		rows.model.Categories = append(rows.model.Categories, Category{
			ID: rows.integer("id"), Name: rows.text("categoria"), Kitchen: rows.nullInt("cocina"),
		})
	}},
	"productos": {[]string{"id", "categoria", "producto"}, func(rows *rowReader) {
		rows.model.Products = append(rows.model.Products, Product{
			ID: rows.integer("id"), Category: rows.nullInt("categoria"), Name: rows.text("producto"),
			ShortName: rows.text("nombre_corto"), Unit: rows.text("unidad"),
		})
	}},
	"productos_precios": {[]string{"producto", "fecha_inicio", "subtotal", "iva", "total"}, func(rows *rowReader) {
		rows.model.ProductPrices = append(rows.model.ProductPrices, readPrice(rows, "producto"))
	}},
	"complementos": {[]string{"id", "complemento"}, func(rows *rowReader) {
		rows.model.Complements = append(rows.model.Complements, Complement{ID: rows.integer("id"), Name: rows.text("complemento")})
	}},
	"complementos_precios": {[]string{"id", "fecha_inicio", "subtotal", "iva", "total"}, func(rows *rowReader) {
		rows.model.ComplementPrices = append(rows.model.ComplementPrices, readPrice(rows, "id"))
	}},
	"productos_complementos": {[]string{"producto", "complemento"}, func(rows *rowReader) {
		rows.model.ProductComplements = append(rows.model.ProductComplements, ProductComplement{
			Product: rows.integer("producto"), Complement: rows.integer("complemento"), Included: rows.nullQuantity("cantidad"),
		})
	}},
	"mesas": {[]string{"id"}, func(rows *rowReader) {
		rows.model.Tables = append(rows.model.Tables, DiningTable{
			ID: rows.integer("id"), Name: rows.text("mesa"), Capacity: rows.nullInt("capacidad"),
		})
	}},
	"puestos": {[]string{"id", "puesto"}, func(rows *rowReader) {
		rows.model.Positions = append(rows.model.Positions, NamedRow{ID: rows.integer("id"), Name: rows.text("puesto")})
	}},
	"personal": {[]string{"id", "nombre"}, func(rows *rowReader) {
		rows.model.People = append(rows.model.People, Person{
			ID: rows.integer("id"), Name: rows.text("nombre"), Paternal: rows.text("paterno"), Maternal: rows.text("materno"),
			Position: rows.nullInt("puesto"), Active: rows.nullInt("activo"),
		})
	}},
	"usuarios": {[]string{"id", "usuario"}, func(rows *rowReader) {
		rows.model.Users = append(rows.model.Users, User{
			ID: rows.integer("id"), Username: rows.text("usuario"), Person: rows.nullInt("personal"), Active: rows.nullInt("activo"),
		})
		// El paquete nunca trae la contraseña, solo si existía (password_present); aquí solo se cuenta.
		if rows.truthy("password_present") {
			rows.model.PasswordPresent++
		}
	}},
	"cajas": {[]string{"id", "caja"}, func(rows *rowReader) {
		rows.model.Registers = append(rows.model.Registers, NamedRow{ID: rows.integer("id"), Name: rows.text("caja")})
	}},
	"tipos_pagos": {[]string{"id", "tipo"}, func(rows *rowReader) {
		rows.model.PaymentTypes = append(rows.model.PaymentTypes, NamedRow{ID: rows.integer("id"), Name: rows.text("tipo")})
	}},
	"tipos_descuentos": {[]string{"id", "tipo"}, func(rows *rowReader) {
		rows.model.DiscountTypes = append(rows.model.DiscountTypes, NamedRow{ID: rows.integer("id"), Name: rows.text("tipo")})
	}},
	"ventas": {[]string{"id", "folio", "fecha", "subtotal", "iva", "total"}, func(rows *rowReader) {
		charge, chargeSet := rows.textPresent("extra")
		rows.model.Sales = append(rows.model.Sales, Sale{
			ID: rows.integer("id"), Register: rows.nullInt("caja"), Table: rows.nullInt("mesa"), AccountName: rows.text("nombre"),
			Folio: rows.text("folio"), OpenedAt: rows.requiredTime("fecha"), Cashier: rows.nullInt("cajero"),
			Invoiced: rows.nullInt("facturado"), Subtotal: rows.money("subtotal"), Tax: rows.nullMoney("iva"),
			Total: rows.money("total"), AmountPaid: rows.nullMoney("montoPagado"), Paid: rows.nullInt("pagado"),
			Charge: charge, ChargeSet: chargeSet, ChargeTotal: rows.nullMoney("montoExtra"), InTable: rows.nullInt("enmesa"),
			Shift: rows.nullInt("corte"), Cancelled: rows.nullInt("cancelado"),
		})
	}},
	"ventas_productos": {[]string{"id", "venta", "producto", "cantidad", "subtotal", "iva", "total"}, func(rows *rowReader) {
		quantity := rows.nullQuantity("cantidad")
		if !quantity.Valid {
			rows.fail("cantidad")
		}
		rows.model.Lines = append(rows.model.Lines, Line{
			ID: rows.integer("id"), Sale: rows.integer("venta"), Product: rows.integer("producto"), Quantity: quantity.Value,
			OriginalQuantity: rows.nullQuantity("cantidadOriginal"), Subtotal: rows.money("subtotal"), Tax: rows.nullMoney("iva"),
			Total: rows.money("total"), Notes: rows.text("observaciones"),
		})
	}},
	"ventas_complementos": {[]string{"producto", "complemento", "subtotal", "iva", "total"}, func(rows *rowReader) {
		rows.model.LineComplements = append(rows.model.LineComplements, LineComplement{
			Position: rows.position, Line: rows.integer("producto"), Complement: rows.integer("complemento"),
			Included: rows.nullQuantity("incluido"), Requested: rows.nullQuantity("pedido"), Subtotal: rows.money("subtotal"),
			Tax: rows.nullMoney("iva"), Total: rows.money("total"),
		})
	}},
	"ventas_modificaciones": {[]string{"producto"}, func(rows *rowReader) {
		rows.model.QuantityChanges = append(rows.model.QuantityChanges, QuantityChange{
			Position: rows.position, Line: rows.integer("producto"), Original: rows.nullQuantity("cantAnt"),
			New: rows.nullQuantity("cantNueva"), AuthorizedBy: rows.nullInt("autorizacion"), ChangedAt: rows.time("fecha"),
		})
	}},
	"descuentos": {[]string{"venta", "descuento"}, func(rows *rowReader) {
		rows.model.Discounts = append(rows.model.Discounts, Discount{
			Position: rows.position, Type: rows.nullInt("tipo"), Sale: rows.integer("venta"),
			Description: rows.text("descripcion"), Amount: rows.money("descuento"),
		})
	}},
	"cancelaciones": {[]string{"id", "venta", "fecha"}, func(rows *rowReader) {
		rows.model.Cancellations = append(rows.model.Cancellations, Cancellation{
			ID: rows.integer("id"), Sale: rows.integer("venta"), AuthorizedBy: rows.nullInt("autorizo"), CancelledAt: rows.requiredTime("fecha"),
		})
	}},
	"pagos": {[]string{"id", "venta", "monto", "fecha"}, func(rows *rowReader) {
		rows.model.Payments = append(rows.model.Payments, Payment{
			ID: rows.integer("id"), Sale: rows.integer("venta"), Reference: rows.text("folio"), Type: rows.nullInt("tipo"),
			Amount: rows.nullMoney("monto"), ReceivedBy: rows.nullInt("recibio"), PaidAt: rows.requiredTime("fecha"),
			Shift: rows.nullInt("corte"),
		})
	}},
	"cortes": {[]string{"id"}, func(rows *rowReader) {
		status, statusSet := rows.textPresent("status")
		rows.model.Shifts = append(rows.model.Shifts, Shift{
			ID: rows.integer("id"), Cashier: rows.nullInt("cajero"), OpeningCash: rows.nullMoney("inicio_caja"),
			ClosingCash: rows.nullMoney("fin_caja"), Sales: rows.nullMoney("venta"), Register: rows.nullInt("caja"),
			Status: status, StatusSet: statusSet, AuthorizedBy: rows.nullInt("autorizacion"),
			OpenedAt: rows.time("fecha_inicio"), ClosedAt: rows.time("fecha_fin"), Withdrawals: rows.nullMoney("retiros"),
			Difference: rows.nullMoney("diferencia"),
		})
	}},
	"cortes_parciales": {[]string{"id", "monto"}, func(rows *rowReader) {
		rows.model.Withdrawals = append(rows.model.Withdrawals, Withdrawal{
			ID: rows.integer("id"), Shift: rows.nullInt("corte"), Amount: rows.money("monto"), Reason: rows.text("razon"),
			WithdrawnAt: rows.time("fecha"), AuthorizedBy: rows.nullInt("autorizacion"),
		})
	}},
	"salidas": {[]string{"monto"}, func(rows *rowReader) {
		rows.model.Expenses++
		rows.model.ExpensesTotal += rows.money("monto")
	}},
	"entradas": {[]string{"total"}, func(rows *rowReader) {
		rows.model.Purchases++
		rows.model.PurchasesTotal += rows.money("total")
	}},
	"divisas": {nil, func(rows *rowReader) {
		rows.model.CurrencyRates++
	}},
	"articulos": {nil, func(rows *rowReader) {
		rows.model.InventoryItems++
	}},
}

func readPrice(rows *rowReader, itemColumn string) Price {
	return Price{
		Item: rows.integer(itemColumn), StartsAt: rows.requiredTime("fecha_inicio"), ValidUntil: rows.time("vigencia"),
		Subtotal: rows.money("subtotal"), Tax: rows.nullMoney("iva"), Total: rows.money("total"),
	}
}
