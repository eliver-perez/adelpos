package documents

import "strings"

// Datos de muestra: claramente ficticios («de muestra» en cada nombre), nunca se insertan como
// ventas, pagos ni movimientos. La identidad sí es la real del establecimiento, para que la vista
// previa muestre el encabezado que imprimirá el restaurante.

// SampleMarker es la línea obligatoria al inicio y al final de una muestra impresa.
const SampleMarker = "*** MUESTRA · NO ES UN DOCUMENTO VÁLIDO ***"

func sampleText(value string) *string {
	return &value
}

// SampleData devuelve los datos de muestra de un tipo con la identidad indicada.
func SampleData(kind Kind, identity Identity) Data {
	switch {
	case kind.Family() == FamilyKitchen:
		return Data{Kitchen: sampleKitchen(kind, identity)}
	case kind.Family() == FamilyCommercial:
		return Data{Commercial: sampleCommercial(kind, identity)}
	case kind.IsVoucher():
		return Data{Cash: sampleVoucher(kind, identity)}
	}
	return Data{Cash: sampleCut(kind, identity)}
}

func sampleKitchen(kind Kind, identity Identity) *KitchenData {
	ticketKind := "normal"
	reason := ""
	var before *KitchenItem
	switch kind {
	case KindKitchenModification:
		ticketKind = "modification"
		before = &KitchenItem{Name: "Platillo de muestra", Quantity: 1, Parts: []KitchenPart{
			{Path: "root", Name: "Platillo de muestra", Quantity: 1},
		}}
	case KindKitchenCancellation:
		ticketKind = "cancellation"
		reason = "Motivo de muestra"
	}
	return &KitchenData{
		TicketKind: ticketKind, DocumentID: "documento-de-muestra", LocalDateTime: "01/01/2026 13:00:00",
		LocalDate: "01/01/2026", LocalTime: "13:00", Folio: 123, AccountName: "Cuenta de muestra", Table: "Mesa de muestra",
		Area: "Área de muestra", Mode: "dine_in", Employee: "Mesero de muestra", OrderID: "pedido-de-muestra",
		Settings: KitchenSettings{ShowNotes: true, ShowEstablishment: true, TextSize: TextSizeNormal, LineSpacing: 1, QuantityStyle: QuantityPrefix},
		Sections: []KitchenSection{{
			StationName: "Estación de muestra",
			Items: []KitchenItem{
				{
					ItemID: "partida-de-muestra-1", Name: "Platillo de muestra", Quantity: 2, Price: "250.00", Reason: reason,
					Notes: "Observación de muestra", Before: before,
					Parts: []KitchenPart{{
						Path: "root", Name: "Platillo de muestra", Quantity: 1,
						Ingredients: []KitchenIngredient{{IngredientID: "ingrediente-de-muestra", Name: "Ingrediente de muestra", Included: "1", Requested: "0"}},
						Groups: []KitchenGroup{{Name: "Complementos de muestra", Options: []KitchenOption{
							{Name: "Complemento de muestra", Requested: "1"},
						}}},
					}},
				},
				{ItemID: "partida-de-muestra-2", Name: "Bebida de muestra", Quantity: 1, Price: "45.00", Reason: reason, Parts: []KitchenPart{}},
			},
		}},
		Identity: identity,
	}
}

// UseSettings aplica a los datos de muestra de cocina la configuración vigente de Cocina con la
// misma regla que una comanda real: las opciones «Según la configuración de Cocina» de la plantilla
// la consultan y, si Cocina no muestra precios, la comanda no guarda importes (kitchen/documents.go),
// así que la muestra tampoco los lleva. Así la vista previa y la muestra impresa anticipan el papel.
func (data *KitchenData) UseSettings(settings KitchenSettings) {
	data.Settings = settings
	if settings.ShowPrices {
		return
	}
	for sectionIndex := range data.Sections {
		items := data.Sections[sectionIndex].Items
		for itemIndex := range items {
			items[itemIndex].Price = ""
			if items[itemIndex].Before != nil {
				items[itemIndex].Before.Price = ""
			}
		}
	}
}

func sampleCommercial(kind Kind, identity Identity) *CommercialData {
	data := &CommercialData{
		Kind: kind, Folio: 123, AccountName: "Cuenta de muestra", Context: "Mesa de muestra", LocalTime: "01/01/2026 13:00",
		Cashier: "Cajero de muestra",
		Lines: []CommercialLine{
			{Quantity: 2, Name: "Platillo de muestra", Notes: "Observación de muestra", Amount: "500.00"},
			{Quantity: 1, Name: "Bebida de muestra", Amount: "45.00"},
		},
		Adjustments: []CommercialAdjustment{{Concept: "Descuento", Reason: "Motivo de muestra", Amount: "-45.00"}},
		Tax:         []CommercialTax{{Rate: 16, Base: "431.03", Tax: "68.97"}},
		Total:       "500.00",
		USDTotal:    sampleText("27.03"),
		USDRate:     sampleText("18.50"),
		Payments:    []CommercialPayment{},
		Paid:        "0.00",
		Tips:        "0.00",
		Balance:     "500.00",
		Identity:    identity,
	}
	if kind == KindReceipt {
		payment := CommercialPayment{
			Method: "Efectivo de muestra", Amount: "500.00", Tip: "50.00", Reference: "", Received: sampleText("600.00"), Change: "50.00",
		}
		data.Payment = &payment
		data.Payments = []CommercialPayment{payment}
		data.Paid = "500.00"
		data.Tips = "50.00"
		data.Balance = "0.00"
	}
	return data
}

func sampleVoucher(kind Kind, identity Identity) *CashData {
	voucher := &CashVoucherData{
		LocalDateTime: "01/01/2026 13:00", LocalDate: "01/01/2026", LocalTime: "13:00", Concept: "Concepto de muestra",
		Currency: "MXN", Amount: "350.00", EquivalentMXN: "350.00", Reference: "Referencia de muestra", Notes: "Observación de muestra",
		ActorName: "Cajero de muestra", DeviceName: "Terminal de muestra",
		AuthorizedByName: sampleText("Encargado de muestra"), AuthorizationMode: sampleText("supervisor"),
	}
	switch kind {
	case KindCashIncome:
		voucher.TypeLabel = sampleText("Tipo de muestra")
		voucher.AuthorizedByName = nil
		voucher.AuthorizationMode = nil
	case KindCashWithdrawal:
		voucher.TypeLabel = sampleText("Motivo de muestra")
	case KindCashExpense:
		voucher.Category = sampleText("Categoría de muestra")
	}
	return &CashData{Kind: kind, Register: "Caja de muestra", BusinessDate: "01/01/2026", Voucher: voucher, Identity: identity}
}

func sampleCut(kind Kind, identity Identity) *CashData {
	cut := &CashCutData{
		Sequence: 1, OpenedByName: "Cajero de muestra", OpenedAt: "01/01/2026 09:00", CutAt: "01/01/2026 17:00",
		CutDate: "01/01/2026", CutTime: "17:00", CreatedByName: "Cajero de muestra", DeviceName: "Terminal de muestra",
		OpeningFloat: "500.00",
		Totals: CashTotals{
			Payments: 12, Sales: "4500.00", Tips: "300.00", Refunded: "0.00", Discounts: "120.00", DiscountCount: 2,
			Courtesies: "0.00", CourtesyCount: 0,
			Methods: []CashMethodTotals{
				{Name: "Efectivo de muestra", Sales: "2500.00", Tips: "100.00", Refunds: "0.00"},
				{Name: "Tarjeta de muestra", Sales: "2000.00", Tips: "200.00", Refunds: "0.00"},
			},
			Cash: []CashCurrencyTotals{{Currency: "MXN", Income: "200.00", Withdrawals: "1000.00", Expenses: "150.00", CorrectionsIn: "0.00", CorrectionsOut: "0.00"}},
		},
		Sensitive: &CashSensitive{ExpectedMXN: "2150.00", ExpectedUSD: "0.00"},
	}
	if kind == KindCashFinalCut {
		cut.ClosedByName = sampleText("Cajero de muestra")
		cut.Declared = &CashDeclared{CountedMXN: "2100.00", CountedUSD: "0.00", ExtraMXN: "0.00", ExtraUSD: "0.00", CashierNotes: "Observación de muestra"}
		cut.Sensitive.Differences = &CashDifferences{MXN: "-50.00", USD: "0.00", ResultMXN: "short", ResultUSD: "even"}
	} else {
		cut.PreviousCutAt = nil
	}
	return &CashData{Kind: kind, Register: "Caja de muestra", BusinessDate: "01/01/2026", Cut: cut, Identity: identity}
}

// AddSampleMarkers agrega al inicio y al final de una muestra impresa la línea que la identifica
// como muestra, centrada y cortada al ancho del papel. Estas líneas no pertenecen a ningún bloque.
func AddSampleMarkers(layout *Layout) {
	marker := []Line{}
	for _, line := range wrapWords(SampleMarker, layout.Columns) {
		marker = append(marker, Line{Text: alignText(strings.TrimSpace(line), AlignCenter, layout.Columns), Size: SizeNormal})
	}
	lines := make([]Line, 0, len(layout.Lines)+2*len(marker))
	lines = append(lines, marker...)
	lines = append(lines, layout.Lines...)
	lines = append(lines, marker...)
	layout.Lines = lines
}
