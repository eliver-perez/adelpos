package legacyimport

import "time"

// Model son las filas del paquete que la importación usa, ya tipadas y tal como las guardó el sistema anterior
// (nombres de tabla y columna en docs/MIGRACION_ANALISIS.md §1.3). Las fechas ya están interpretadas en la zona del
// establecimiento; los importes, en diezmilésimos exactos. Las filas conservan el orden del paquete (el lector ordena
// por la llave primaria o, sin ella, por todas las columnas), que es también el «position» de las tablas sin llave.
type Model struct {
	Info     PackageInfo
	Location *time.Location

	Categories         []Category
	Products           []Product
	ProductPrices      []Price
	Complements        []Complement
	ComplementPrices   []Price
	ProductComplements []ProductComplement
	Tables             []DiningTable
	Positions          []NamedRow
	People             []Person
	Users              []User
	Registers          []NamedRow
	PaymentTypes       []NamedRow
	DiscountTypes      []NamedRow

	Sales           []Sale
	Lines           []Line
	LineComplements []LineComplement
	QuantityChanges []QuantityChange
	Discounts       []Discount
	Cancellations   []Cancellation
	Payments        []Payment
	Shifts          []Shift
	Withdrawals     []Withdrawal

	// Expenses y Purchases son las salidas de efectivo (salidas) y las compras recibidas (entradas, lo que la interfaz
	// anterior llamaba «Egresos»): no se importan (en el respaldo real están vacías), pero su número y su total se
	// informan en la validación.
	Expenses        int64
	ExpensesTotal   Money
	Purchases       int64
	PurchasesTotal  Money
	CurrencyRates   int64
	InventoryItems  int64
	TableRows       map[string]int64
	UnreadTables    []string
	PasswordPresent int
}

// Category es categorias.
type Category struct {
	ID      int32
	Name    string
	Kitchen NullInt
}

// Product es productos.
type Product struct {
	ID        int32
	Category  NullInt
	Name      string
	ShortName string
	Unit      string
}

// Price es una fila de productos_precios o complementos_precios: el precio de un producto o complemento desde una
// fecha, con su desglose guardado (la tasa no se guardaba: se deduce de iva/subtotal).
type Price struct {
	Item       int32
	StartsAt   time.Time
	ValidUntil time.Time
	Subtotal   Money
	Tax        NullMoney
	Total      Money
}

// Complement es complementos.
type Complement struct {
	ID   int32
	Name string
}

// ProductComplement es productos_complementos: qué complementos admite un producto y cuántos incluye.
type ProductComplement struct {
	Product    int32
	Complement int32
	Included   NullQuantity
}

// DiningTable es mesas (la id 1 es «para llevar» en el anterior).
type DiningTable struct {
	ID       int32
	Name     string
	Capacity NullInt
}

// NamedRow es una tabla de catálogo simple del anterior (puestos, cajas, tipos_pagos, tipos_descuentos).
type NamedRow struct {
	ID   int32
	Name string
}

// Person es personal (sin domicilio ni contacto: personal_detalles no viaja en el paquete).
type Person struct {
	ID       int32
	Name     string
	Paternal string
	Maternal string
	Position NullInt
	Active   NullInt
}

// User es usuarios. La contraseña nunca viaja: el paquete solo dice si existía (password_present).
type User struct {
	ID       int32
	Username string
	Person   NullInt
	Active   NullInt
}

// Sale es ventas.
type Sale struct {
	ID          int32
	Register    NullInt
	Table       NullInt
	AccountName string
	Folio       string
	OpenedAt    time.Time
	Cashier     NullInt
	Invoiced    NullInt
	Subtotal    Money
	Tax         NullMoney
	Total       Money
	AmountPaid  NullMoney
	Paid        NullInt
	Charge      string
	ChargeSet   bool
	ChargeTotal NullMoney
	InTable     NullInt
	Shift       NullInt
	Cancelled   NullInt
}

// Line es ventas_productos.
type Line struct {
	ID               int32
	Sale             int32
	Product          int32
	Quantity         Quantity
	OriginalQuantity NullQuantity
	Subtotal         Money
	Tax              NullMoney
	Total            Money
	Notes            string
}

// LineComplement es ventas_complementos (sin llave: Position es su orden en el paquete, desde 1).
type LineComplement struct {
	Position   int32
	Line       int32
	Complement int32
	Included   NullQuantity
	Requested  NullQuantity
	Subtotal   Money
	Tax        NullMoney
	Total      Money
}

// QuantityChange es ventas_modificaciones.
type QuantityChange struct {
	Position     int32
	Line         int32
	Original     NullQuantity
	New          NullQuantity
	AuthorizedBy NullInt
	ChangedAt    time.Time
}

// Discount es descuentos.
type Discount struct {
	Position    int32
	Type        NullInt
	Sale        int32
	Description string
	Amount      Money
}

// Cancellation es cancelaciones.
type Cancellation struct {
	ID           int32
	Sale         int32
	AuthorizedBy NullInt
	CancelledAt  time.Time
}

// Payment es pagos.
type Payment struct {
	ID         int32
	Sale       int32
	Reference  string
	Type       NullInt
	Amount     NullMoney
	ReceivedBy NullInt
	PaidAt     time.Time
	Shift      NullInt
}

// Shift es cortes (el turno de un cajero en una caja).
type Shift struct {
	ID           int32
	Cashier      NullInt
	OpeningCash  NullMoney
	ClosingCash  NullMoney
	Sales        NullMoney
	Register     NullInt
	Status       string
	StatusSet    bool
	AuthorizedBy NullInt
	OpenedAt     time.Time
	ClosedAt     time.Time
	Withdrawals  NullMoney
	Difference   NullMoney
}

// Withdrawal es cortes_parciales (retiro de efectivo del turno).
type Withdrawal struct {
	ID           int32
	Shift        NullInt
	Amount       Money
	Reason       string
	WithdrawnAt  time.Time
	AuthorizedBy NullInt
}
