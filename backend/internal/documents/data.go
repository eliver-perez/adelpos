package documents

import "time"

// Datos tipados de cada familia. Los módulos de origen (kitchen y payments) convierten sus
// estructuras a estos tipos; el renderizador no lee la base ni conoce los tipos de esos módulos.
// Las fechas llegan ya expresadas en la zona horaria del establecimiento, como las imprimían las
// plantillas anteriores. Todos los textos se sanean al renderizar.

// Identity es la copia de la identidad del establecimiento que usa un documento. Los documentos
// emitidos desde la 9B la guardan para que su reimpresión sea idéntica aunque la identidad cambie.
// En la fase A solo existen nombre, dirección, teléfono y correo (app.establishment); la fase B
// agrega la información adicional, el encabezado, el pie y el logotipo.
type Identity struct {
	Name    string       `json:"name"`
	Address string       `json:"address,omitempty"`
	Phone   string       `json:"phone,omitempty"`
	Email   string       `json:"email,omitempty"`
	Extra   string       `json:"extra,omitempty"`
	Header  string       `json:"header,omitempty"`
	Footer  string       `json:"footer,omitempty"`
	Logo    []LogoRaster `json:"logo,omitempty"`
}

// LogoRaster referencia la conversión monocromática del logotipo para un ancho de papel. Los bits
// viven en la tabla de la fase B y se cargan por RasterID al imprimir (RasterSource).
type LogoRaster struct {
	PaperWidth int    `json:"paper_width"`
	RasterID   string `json:"raster_id"`
	WidthDots  int    `json:"width_dots"`
	HeightDots int    `json:"height_dots"`
}

// logoFor devuelve la conversión del logotipo para el ancho de papel, si existe.
func (identity Identity) logoFor(paperWidth int) *LogoRaster {
	for index := range identity.Logo {
		if identity.Logo[index].PaperWidth == paperWidth && identity.Logo[index].RasterID != "" {
			return &identity.Logo[index]
		}
	}
	return nil
}

// KitchenSettings es la configuración de Cocina copiada en cada comanda: las opciones «inherit» de
// la plantilla la consultan, de modo que reimprimir no cambia si después cambia la configuración.
type KitchenSettings struct {
	ShowPrices        bool   `json:"show_prices"`
	ShowNotes         bool   `json:"show_notes"`
	ShowEstablishment bool   `json:"show_establishment"`
	TextSize          string `json:"text_size"`
	LineSpacing       int    `json:"line_spacing"`
	QuantityStyle     string `json:"quantity_style"`
}

// KitchenData son los datos de una comanda, aviso de modificación o de cancelación (o de la prueba
// técnica de impresora). TicketKind es kitchen_tickets.kind (normal, modification, cancellation o
// test): decide el título del documento.
type KitchenData struct {
	TicketKind    string           `json:"ticket_kind"`
	DocumentID    string           `json:"document_id"`
	LocalDateTime string           `json:"local_date_time"`
	LocalDate     string           `json:"local_date"`
	LocalTime     string           `json:"local_time"`
	Folio         int64            `json:"folio"`
	AccountName   string           `json:"account_name"`
	Table         string           `json:"table"`
	Area          string           `json:"area"`
	Mode          string           `json:"mode"`
	Customer      string           `json:"customer"`
	Origin        string           `json:"origin"`
	Employee      string           `json:"employee"`
	OrderID       string           `json:"order_id"`
	Settings      KitchenSettings  `json:"settings"`
	Sections      []KitchenSection `json:"sections"`
	Identity      Identity         `json:"identity"`
}

// KitchenSection es la parte de la comanda de una estación.
type KitchenSection struct {
	StationName string        `json:"station_name"`
	Items       []KitchenItem `json:"items"`
}

// KitchenItem es una partida (agrupada) con sus partes; Before es la versión anterior de una
// modificación.
type KitchenItem struct {
	ItemID   string        `json:"item_id"`
	Name     string        `json:"name"`
	Quantity int           `json:"quantity"`
	Parts    []KitchenPart `json:"parts"`
	Price    string        `json:"price,omitempty"`
	Before   *KitchenItem  `json:"before,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	Notes    string        `json:"notes,omitempty"`
}

// KitchenPart es una parte de la partida (la raíz o un componente de combo).
type KitchenPart struct {
	Path        string              `json:"path"`
	Name        string              `json:"name"`
	Variant     string              `json:"variant"`
	Quantity    int                 `json:"quantity"`
	Ingredients []KitchenIngredient `json:"ingredients"`
	Groups      []KitchenGroup      `json:"groups"`
	Notes       string              `json:"notes"`
	QuickNotes  []KitchenQuickNote  `json:"quick_notes,omitempty"`
}

// KitchenIngredient es un ingrediente con lo incluido y lo solicitado (cantidades decimales).
type KitchenIngredient struct {
	IngredientID string `json:"ingredient_id"`
	Name         string `json:"name"`
	Included     string `json:"included"`
	Requested    string `json:"requested"`
}

// KitchenGroup es un grupo de complementos con sus opciones solicitadas.
type KitchenGroup struct {
	Name    string          `json:"name"`
	Options []KitchenOption `json:"options"`
}

// KitchenOption es una opción de un grupo de complementos.
type KitchenOption struct {
	Name      string `json:"name"`
	Requested string `json:"requested"`
}

// KitchenQuickNote es una indicación rápida resuelta (etapa 8); Effect vacío o «none» no tiene
// efecto sobre un ingrediente.
type KitchenQuickNote struct {
	Text         string  `json:"text"`
	Effect       string  `json:"effect"`
	IngredientID *string `json:"ingredient_id,omitempty"`
}

// CommercialData son los datos de una precuenta o ticket de pago (no es un CFDI).
type CommercialData struct {
	Kind Kind `json:"kind"`
	// Title es el encabezado impreso de la precuenta cuando el establecimiento eligió otro nombre visible
	// (etapa 9F, §4.5: «TICKET DE VENTA» o «CUENTA»). Vacío, el de siempre («PRECUENTA»); un título propio
	// de la plantilla lo sigue sustituyendo.
	Title       string                 `json:"title,omitempty"`
	Folio       int64                  `json:"folio"`
	AccountName string                 `json:"account_name"`
	Context     string                 `json:"context"`
	LocalTime   string                 `json:"local_time"`
	Cashier     string                 `json:"cashier"`
	Lines       []CommercialLine       `json:"lines"`
	Adjustments []CommercialAdjustment `json:"adjustments"`
	Tax         []CommercialTax        `json:"tax"`
	Total       string                 `json:"total"`
	USDTotal    *string                `json:"usd_total,omitempty"`
	USDRate     *string                `json:"usd_rate,omitempty"`
	Payment     *CommercialPayment     `json:"payment,omitempty"`
	Payments    []CommercialPayment    `json:"payments"`
	Paid        string                 `json:"paid"`
	Tips        string                 `json:"tips"`
	Balance     string                 `json:"balance"`
	Identity    Identity               `json:"identity"`
}

// CommercialLine es una partida del documento comercial con su importe.
type CommercialLine struct {
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Amount   string `json:"amount"`
}

// CommercialAdjustment es un descuento, cortesía o cargo; Reason es el motivo del catálogo.
type CommercialAdjustment struct {
	Concept string `json:"concept"`
	Reason  string `json:"reason,omitempty"`
	Amount  string `json:"amount"`
}

// CommercialTax es el desglose de IVA de una tasa.
type CommercialTax struct {
	Rate int    `json:"rate"`
	Base string `json:"base"`
	Tax  string `json:"tax"`
}

// CommercialPayment es un pago registrado; los campos de dólares solo existen en el efectivo con
// dólares (etapa 6.1).
type CommercialPayment struct {
	Method         string  `json:"method"`
	Amount         string  `json:"amount"`
	Tip            string  `json:"tip"`
	Reference      string  `json:"reference"`
	Received       *string `json:"received,omitempty"`
	Change         string  `json:"change"`
	ReceivedMXN    *string `json:"received_mxn,omitempty"`
	ReceivedUSD    *string `json:"received_usd,omitempty"`
	USDRate        *string `json:"usd_rate,omitempty"`
	ChangeCurrency *string `json:"change_currency,omitempty"`
	ChangeUSD      *string `json:"change_usd,omitempty"`
}

// CashData son los datos de un documento de caja: un comprobante (Voucher) o un corte (Cut).
// Register y BusinessDate (dd/mm/aaaa) identifican la caja y el turno.
type CashData struct {
	Kind         Kind             `json:"kind"`
	Register     string           `json:"register"`
	BusinessDate string           `json:"business_date"`
	Voucher      *CashVoucherData `json:"voucher,omitempty"`
	Cut          *CashCutData     `json:"cut,omitempty"`
	Identity     Identity         `json:"identity"`
}

// CashVoucherData es una operación de efectivo (ingreso, retiro o gasto). Nunca contiene el
// efectivo esperado de la caja.
type CashVoucherData struct {
	LocalDateTime     string  `json:"local_date_time"`
	LocalDate         string  `json:"local_date"`
	LocalTime         string  `json:"local_time"`
	Category          *string `json:"category,omitempty"`
	TypeLabel         *string `json:"type_label,omitempty"`
	Concept           string  `json:"concept"`
	Currency          string  `json:"currency"`
	Amount            string  `json:"amount"`
	USDRate           *string `json:"usd_rate,omitempty"`
	EquivalentMXN     string  `json:"equivalent_mxn"`
	Reference         string  `json:"reference"`
	Notes             string  `json:"notes"`
	Exceptional       bool    `json:"exceptional"`
	Reason            string  `json:"reason"`
	ActorName         string  `json:"actor_name"`
	DeviceName        string  `json:"device_name"`
	AuthorizedByName  *string `json:"authorized_by_name,omitempty"`
	AuthorizationMode *string `json:"authorization_mode,omitempty"`
	Corrected         bool    `json:"corrected"`
}

// CashCutData es un corte parcial o definitivo tal como lo guarda su snapshot inmutable. Sensitive
// (esperado y diferencias) solo existe si el documento puede incluirlo; Dollars indica que el turno
// manejó dólares (fondo en dólares o tipo de cambio).
type CashCutData struct {
	Sequence      int            `json:"sequence"`
	OpenedByName  string         `json:"opened_by_name"`
	OpenedAt      string         `json:"opened_at"`
	CutAt         string         `json:"cut_at"`
	CutDate       string         `json:"cut_date"`
	CutTime       string         `json:"cut_time"`
	PreviousCutAt *string        `json:"previous_cut_at,omitempty"`
	ClosedByName  *string        `json:"closed_by_name,omitempty"`
	CreatedByName string         `json:"created_by_name"`
	DeviceName    string         `json:"device_name"`
	OpeningFloat  string         `json:"opening_float"`
	FloatUSD      *string        `json:"opening_float_usd,omitempty"`
	USDRate       *string        `json:"usd_rate,omitempty"`
	Totals        CashTotals     `json:"totals"`
	Declared      *CashDeclared  `json:"declared,omitempty"`
	Dollars       bool           `json:"dollars"`
	Sensitive     *CashSensitive `json:"sensitive,omitempty"`
}

// CashTotals son los totales informativos acumulados del turno (sin el esperado).
type CashTotals struct {
	Payments        int                  `json:"payments"`
	Sales           string               `json:"sales"`
	Tips            string               `json:"tips"`
	Refunded        string               `json:"refunded"`
	Discounts       string               `json:"discounts"`
	DiscountCount   int                  `json:"discount_count"`
	Courtesies      string               `json:"courtesies"`
	CourtesyCount   int                  `json:"courtesy_count"`
	Methods         []CashMethodTotals   `json:"methods"`
	Cash            []CashCurrencyTotals `json:"cash"`
	Regularizations int                  `json:"regularizations"`
}

// CashMethodTotals son las ventas, propinas y devoluciones de un método de pago.
type CashMethodTotals struct {
	Name    string `json:"name"`
	Sales   string `json:"sales"`
	Tips    string `json:"tips"`
	Refunds string `json:"refunds"`
}

// CashCurrencyTotals son los ingresos, retiros, gastos y correcciones de una moneda.
type CashCurrencyTotals struct {
	Currency       string `json:"currency"`
	Income         string `json:"income"`
	Withdrawals    string `json:"withdrawals"`
	Expenses       string `json:"expenses"`
	CorrectionsIn  string `json:"corrections_in"`
	CorrectionsOut string `json:"corrections_out"`
}

// CashDeclared es lo que el cajero declaró en el arqueo de cierre (solo el corte definitivo).
type CashDeclared struct {
	CountedMXN   string `json:"counted_mxn"`
	CountedUSD   string `json:"counted_usd"`
	ExtraMXN     string `json:"extra_mxn"`
	ExtraUSD     string `json:"extra_usd"`
	CashierNotes string `json:"cashier_notes"`
}

// CashSensitive es el bloque protegido: efectivo esperado y, en el definitivo, diferencias.
type CashSensitive struct {
	ExpectedMXN        string           `json:"expected_mxn"`
	ExpectedUSD        string           `json:"expected_usd"`
	TotalEquivalentMXN *string          `json:"total_equivalent_mxn,omitempty"`
	Differences        *CashDifferences `json:"differences,omitempty"`
}

// CashDifferences son las diferencias de arqueo por moneda (contado − esperado) y su resultado
// (even, over o short).
type CashDifferences struct {
	MXN           string  `json:"mxn"`
	USD           string  `json:"usd"`
	ResultMXN     string  `json:"result_mxn"`
	ResultUSD     string  `json:"result_usd"`
	EquivalentMXN *string `json:"equivalent_mxn,omitempty"`
}

// Data reúne los datos de un documento; solo se llena el de la familia de su tipo.
type Data struct {
	Kitchen    *KitchenData
	Commercial *CommercialData
	Cash       *CashData
}

// RecentDocument es un documento real reciente que el usuario puede consultar en su módulo de
// origen, para elegirlo en la vista previa del diseñador.
type RecentDocument struct {
	TicketID  string    `json:"ticket_id"`
	Kind      Kind      `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Label     string    `json:"label"`
}
