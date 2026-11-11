package documents

import (
	"strconv"
	"strings"
)

// Datos que puede imprimir el bloque «Campo de datos». Las claves dependen del tipo de documento:
// cada familia solo ofrece las que sus datos contienen. DefaultLabel es la etiqueta que propone el
// atajo del editor (se imprime seguida de un espacio y el valor).

// FieldKeyInfo describe un dato disponible para el campo de datos.
type FieldKeyInfo struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	DefaultLabel string `json:"default_label"`
}

// FieldShortcut es un atajo del editor: un campo de datos ya configurado («Fecha», «Mesa»…).
type FieldShortcut struct {
	Name    string       `json:"name"`
	Options FieldOptions `json:"options"`
}

var dateKeys = []FieldKeyInfo{
	{"date", "Fecha", "Fecha:"},
	{"time", "Hora", "Hora:"},
	{"datetime", "Fecha y hora", ""},
}

var kitchenKeys = append(append([]FieldKeyInfo{}, dateKeys...), []FieldKeyInfo{
	{"folio", "Folio de la cuenta («#21»)", "Cuenta"},
	{"account_name", "Nombre de la cuenta", ""},
	{"table", "Mesa", "Mesa:"},
	{"area", "Área", "Área:"},
	{"customer", "Cliente", "Cliente:"},
	{"origin", "Origen del pedido para llevar", "Origen:"},
	{"employee", "Quién capturó", "Capturó:"},
	{"order", "Pedido", "Pedido:"},
	{"document", "Documento", "Documento:"},
}...)

var commercialKeys = append(append([]FieldKeyInfo{}, dateKeys...), []FieldKeyInfo{
	{"folio", "Folio", "Folio:"},
	{"account_name", "Nombre de la cuenta", "Cuenta:"},
	{"context", "Mesa o modalidad («Mesa 4», «Para llevar»)", ""},
	{"cashier", "Cajero", "Atendió:"},
}...)

var voucherKeys = append(append([]FieldKeyInfo{}, dateKeys...), []FieldKeyInfo{
	{"register", "Caja", "Caja:"},
	{"business_date", "Fecha del turno", "Turno del"},
	{"recorded_by", "Quién registró", "Registró:"},
	{"device", "Terminal", "Terminal:"},
}...)

var cutKeys = append(append([]FieldKeyInfo{}, dateKeys...), []FieldKeyInfo{
	{"register", "Caja", "Caja:"},
	{"business_date", "Fecha del turno", "Turno del"},
	{"opened_by", "Responsable del turno", "Responsable:"},
	{"opened_at", "Apertura", "Apertura"},
	{"cut_at", "Fecha y hora del corte", "Corte"},
	{"created_by", "Quién realizó el corte", "Realizó:"},
	{"device", "Terminal", "Terminal:"},
}...)

// fieldKeysFor son los datos disponibles de un tipo de documento.
func fieldKeysFor(kind Kind) []FieldKeyInfo {
	switch {
	case kind.Family() == FamilyKitchen:
		return kitchenKeys
	case kind.Family() == FamilyCommercial:
		return commercialKeys
	case kind.IsVoucher():
		return voucherKeys
	case kind == KindCashPartialCut:
		return append(append([]FieldKeyInfo{}, cutKeys...), FieldKeyInfo{"previous_cut_at", "Corte anterior", "Corte anterior"})
	case kind == KindCashFinalCut:
		return append(append([]FieldKeyInfo{}, cutKeys...), FieldKeyInfo{"closed_by", "Quién cerró", "Cerró:"})
	}
	return []FieldKeyInfo{}
}

// fieldKeyAllowed indica si un dato pertenece al tipo de documento.
func fieldKeyAllowed(kind Kind, key string) bool {
	for _, info := range fieldKeysFor(kind) {
		if info.Key == key {
			return true
		}
	}
	return false
}

// shortcutsFor arma un atajo por dato disponible con su etiqueta propuesta.
func shortcutsFor(kind Kind) []FieldShortcut {
	shortcuts := []FieldShortcut{}
	for _, info := range fieldKeysFor(kind) {
		shortcuts = append(shortcuts, FieldShortcut{
			Name: info.Name,
			Options: FieldOptions{
				Items:     []FieldItem{{Key: info.Key, Label: info.DefaultLabel}},
				Layout:    FieldInline,
				OmitEmpty: true,
			},
		})
	}
	return shortcuts
}

// fieldValue devuelve el valor de un dato y si existe en el documento. Un dato opcional ausente
// (por ejemplo «Cerró» antes del cierre) no existe; un texto vacío tampoco.
func fieldValue(kind Kind, key string, data Data) (string, bool) {
	var value string
	present := true
	switch kind.Family() {
	case FamilyKitchen:
		value = kitchenFieldValue(key, data.Kitchen)
	case FamilyCommercial:
		value = commercialFieldValue(key, data.Commercial)
	case FamilyCash:
		var optional *string
		value, optional = cashFieldValue(key, data.Cash)
		if optional != nil {
			return *optional, true
		}
	}
	if value == "" {
		present = false
	}
	return value, present
}

func kitchenFieldValue(key string, data *KitchenData) string {
	if data == nil {
		return ""
	}
	switch key {
	case "date":
		return data.LocalDate
	case "time":
		return data.LocalTime
	case "datetime":
		return data.LocalDateTime
	case "folio":
		return "#" + strconv.FormatInt(data.Folio, 10)
	case "account_name":
		return data.AccountName
	case "table":
		return data.Table
	case "area":
		return data.Area
	case "customer":
		return data.Customer
	case "origin":
		return data.Origin
	case "employee":
		return data.Employee
	case "order":
		return data.OrderID
	case "document":
		return data.DocumentID
	}
	return ""
}

func commercialFieldValue(key string, data *CommercialData) string {
	if data == nil {
		return ""
	}
	date, clock, _ := strings.Cut(data.LocalTime, " ")
	switch key {
	case "date":
		return date
	case "time":
		return clock
	case "datetime":
		return data.LocalTime
	case "folio":
		return strconv.FormatInt(data.Folio, 10)
	case "account_name":
		return data.AccountName
	case "context":
		return data.Context
	case "cashier":
		return data.Cashier
	}
	return ""
}

// cashFieldValue devuelve el valor de un dato de caja; los datos opcionales (cerró, corte anterior)
// se devuelven como puntero para distinguir «ausente» de «vacío».
func cashFieldValue(key string, data *CashData) (string, *string) {
	if data == nil {
		return "", nil
	}
	switch key {
	case "register":
		return data.Register, nil
	case "business_date":
		return data.BusinessDate, nil
	}
	if data.Voucher != nil {
		voucher := data.Voucher
		switch key {
		case "date":
			return voucher.LocalDate, nil
		case "time":
			return voucher.LocalTime, nil
		case "datetime":
			return voucher.LocalDateTime, nil
		case "recorded_by":
			return voucher.ActorName, nil
		case "device":
			return voucher.DeviceName, nil
		}
		return "", nil
	}
	if data.Cut == nil {
		return "", nil
	}
	cut := data.Cut
	switch key {
	case "date":
		return cut.CutDate, nil
	case "time":
		return cut.CutTime, nil
	case "datetime", "cut_at":
		return cut.CutAt, nil
	case "opened_by":
		return cut.OpenedByName, nil
	case "opened_at":
		return cut.OpenedAt, nil
	case "created_by":
		return cut.CreatedByName, nil
	case "device":
		return cut.DeviceName, nil
	case "previous_cut_at":
		return "", cut.PreviousCutAt
	case "closed_by":
		return "", cut.ClosedByName
	}
	return "", nil
}
