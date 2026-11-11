package documents

import "encoding/json"

// Plantillas predeterminadas: la versión 1 de cada plantilla del sistema. Reproducen byte a byte
// los documentos de las etapas 5 a 7.1 para los diez tipos y ambos anchos (lo comprueban las
// pruebas de equivalencia de kitchen y payments contra copias literales de las plantillas
// anteriores). La migración 0018 siembra exactamente este JSON; una prueba de integración lo
// compara con lo sembrado.

// newBlock arma un bloque visible, alineado a la izquierda y de tamaño normal con sus opciones.
func newBlock(id, blockType string, options any) Block {
	encoded, _ := json.Marshal(options)
	return Block{
		ID: id, Type: blockType, Visible: true, Align: AlignLeft, Size: SizeNormal, Options: encoded, parsed: options,
	}
}

// fieldBlock es un campo de datos en línea que se imprime aunque sus valores estén vacíos, como
// las líneas fijas de las plantillas anteriores («Capturó: », «Pedido: »).
func fieldBlock(id string, items ...FieldItem) Block {
	return newBlock(id, BlockField, &FieldOptions{Items: items, Layout: FieldInline, OmitEmpty: false})
}

// pairBlock es un campo de datos etiqueta-valor; optional lo omite si el dato no existe.
func pairBlock(id, key, label string, optional bool) Block {
	return newBlock(id, BlockField, &FieldOptions{Items: []FieldItem{{Key: key, Label: label}}, Layout: FieldPair, OmitEmpty: optional})
}

func dividerBlock(id string) Block {
	return newBlock(id, BlockDivider, &DividerOptions{Character: "-"})
}

func titleBlock() Block {
	return newBlock("title", BlockDocumentTitle, &TitleOptions{})
}

// Default devuelve una copia nueva de la plantilla predeterminada del tipo.
func Default(kind Kind) *Template {
	switch {
	case kind.Family() == FamilyKitchen:
		return defaultKitchen()
	case kind.Family() == FamilyCommercial:
		return defaultCommercial(kind)
	case kind.IsVoucher():
		return defaultVoucher()
	case kind == KindCashPartialCut:
		return defaultCut(false)
	case kind == KindCashFinalCut:
		return defaultCut(true)
	}
	return &Template{Options: TemplateOptions{TextSize: TextSizeNormal}, Blocks: []Block{}}
}

// defaultKitchen es la comanda de kitchen.RenderText: nombre según Cocina, título, fecha con
// segundos, cuenta, ubicación, quién capturó, pedido, estaciones con sus partidas y documento.
func defaultKitchen() *Template {
	return &Template{
		Options: TemplateOptions{TextSize: TextSizeInherit},
		Blocks: []Block{
			newBlock("name", BlockEstablishmentName, &EstablishmentNameOptions{When: WhenKitchenSetting}),
			titleBlock(),
			fieldBlock("date-time", FieldItem{Key: "datetime"}),
			fieldBlock("account", FieldItem{Key: "folio", Label: "Cuenta"}, FieldItem{Key: "account_name"}),
			newBlock("location", BlockKitchenLocation, &KitchenLocationOptions{ShowCustomer: true}),
			fieldBlock("employee", FieldItem{Key: "employee", Label: "Capturó:"}),
			fieldBlock("order", FieldItem{Key: "order", Label: "Pedido:"}),
			newBlock("items", BlockKitchenItems, &KitchenItemsOptions{
				ShowNotes: Inherit, ShowPrices: Inherit, QuantityStyle: Inherit, LineSpacing: SpacingInherit,
				StationDivider: true, StationHeadings: true, ItemReference: true,
			}),
			dividerBlock("divider-end"),
			fieldBlock("document", FieldItem{Key: "document", Label: "Documento:"}),
		},
	}
}

// defaultCommercial es la precuenta o el ticket de payments.renderDocument.
func defaultCommercial(kind Kind) *Template {
	blocks := []Block{
		newBlock("name", BlockEstablishmentName, &EstablishmentNameOptions{}),
		titleBlock(),
		fieldBlock("account", FieldItem{Key: "folio", Label: "Folio"}, FieldItem{Key: "account_name"}),
		fieldBlock("context", FieldItem{Key: "context"}, FieldItem{Key: "datetime"}),
		fieldBlock("cashier", FieldItem{Key: "cashier", Label: "Atendió:"}),
		dividerBlock("divider-1"),
		newBlock("products", BlockProducts, &NotesOptions{ShowNotes: true}),
		newBlock("adjustments", BlockAdjustments, &ReasonOptions{ShowReason: true}),
		dividerBlock("divider-2"),
		newBlock("taxes", BlockTaxes, &TaxesOptions{ShowBase: true}),
		newBlock("total", BlockTotal, &EmptyOptions{}),
		newBlock("usd-total", BlockUSDTotal, &EmptyOptions{}),
	}
	if kind == KindReceipt {
		blocks = append(blocks, newBlock("this-payment", BlockThisPayment, &SectionOptions{Divider: true, Heading: true}))
	}
	blocks = append(blocks,
		newBlock("payments", BlockPayments, &SectionOptions{Divider: true, Heading: true}),
		newBlock("balance", BlockBalance, &EmptyOptions{}),
		dividerBlock("divider-3"),
		newBlock("fiscal", BlockFiscalNotice, &EmptyOptions{}),
	)
	return &Template{Options: TemplateOptions{TextSize: TextSizeNormal}, Blocks: blocks}
}

// defaultVoucher es el comprobante de ingreso, retiro o gasto de payments.renderVoucher.
func defaultVoucher() *Template {
	return &Template{
		Options: TemplateOptions{TextSize: TextSizeNormal},
		Blocks: []Block{
			newBlock("name", BlockEstablishmentName, &EstablishmentNameOptions{}),
			titleBlock(),
			fieldBlock("register", FieldItem{Key: "register", Label: "Caja:"}),
			fieldBlock("shift", FieldItem{Key: "business_date", Label: "Turno del"}),
			pairBlock("date-time", "datetime", "Fecha", false),
			dividerBlock("divider-1"),
			newBlock("detail", BlockVoucherDetail, &VoucherDetailOptions{ShowReference: true, ShowNotes: true}),
			dividerBlock("divider-2"),
			newBlock("signatures", BlockSignatures, &SignaturesOptions{ShowStatus: true, SignatureLine: true}),
			dividerBlock("divider-3"),
			newBlock("fiscal", BlockFiscalNotice, &EmptyOptions{}),
		},
	}
}

// defaultCut es el corte parcial o definitivo de payments.renderReport.
func defaultCut(final bool) *Template {
	blocks := []Block{
		newBlock("name", BlockEstablishmentName, &EstablishmentNameOptions{}),
		titleBlock(),
		fieldBlock("register", FieldItem{Key: "register", Label: "Caja:"}),
		fieldBlock("shift", FieldItem{Key: "business_date", Label: "Turno del"}, FieldItem{Key: "opened_by", Label: "responsable"}),
		pairBlock("opened-at", "opened_at", "Apertura", false),
	}
	if final {
		blocks = append(blocks,
			pairBlock("cut-at", "cut_at", "Cierre", false),
			newBlock("closed-by", BlockField, &FieldOptions{
				Items: []FieldItem{{Key: "closed_by", Label: "Cerró:"}}, Layout: FieldInline, OmitEmpty: true,
			}),
		)
	} else {
		blocks = append(blocks,
			pairBlock("cut-at", "cut_at", "Corte", false),
			pairBlock("previous-cut", "previous_cut_at", "Corte anterior", true),
		)
	}
	blocks = append(blocks,
		fieldBlock("made-by", FieldItem{Key: "created_by", Label: "Realizó:"}, FieldItem{Key: "device"}),
		dividerBlock("divider-1"),
		newBlock("float", BlockOpeningFloat, &EmptyOptions{}),
		dividerBlock("divider-2"),
		newBlock("sales", BlockSalesByMethod, &HeadingOptions{Heading: true}),
		newBlock("totals", BlockShiftTotals, &EmptyOptions{}),
		dividerBlock("divider-3"),
		newBlock("cash", BlockCashMovements, &HeadingOptions{Heading: true}),
	)
	if final {
		blocks = append(blocks, newBlock("declared", BlockDeclaredCash, &SectionOptions{Divider: true, Heading: true}))
	}
	blocks = append(blocks,
		dividerBlock("divider-4"),
		newBlock("expected", BlockExpectedDifferences, &EmptyOptions{}),
	)
	if final {
		blocks = append(blocks, newBlock("observations", BlockObservations, &SectionOptions{Divider: true, Heading: true}))
	}
	blocks = append(blocks,
		dividerBlock("divider-5"),
		newBlock("fiscal", BlockFiscalNotice, &EmptyOptions{}),
	)
	return &Template{Options: TemplateOptions{TextSize: TextSizeNormal}, Blocks: blocks}
}
