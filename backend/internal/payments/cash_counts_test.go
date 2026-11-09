package payments

import (
	"io/fs"
	"strings"
	"testing"

	"local/ad-el-pos/server/migrations"
)

const (
	bill500ID = "00000000-0000-4000-8000-000000000500"
	bill200ID = "00000000-0000-4000-8000-000000000200"
	coin10ID  = "00000000-0000-4000-8000-000000000010"
	coin050ID = "00000000-0000-4000-8000-000000000050"
	usd20ID   = "00000000-0000-4000-8000-000000002000"
	retiredID = "00000000-0000-4000-8000-000000000999"
)

func testCatalog() map[string]Denomination {
	return map[string]Denomination{
		bill500ID: {ID: bill500ID, Currency: currencyMXN, Value: "500.00", Kind: "bill", Active: true},
		bill200ID: {ID: bill200ID, Currency: currencyMXN, Value: "200.00", Kind: "bill", Active: true},
		coin10ID:  {ID: coin10ID, Currency: currencyMXN, Value: "10.00", Kind: "coin", Active: true},
		coin050ID: {ID: coin050ID, Currency: currencyMXN, Value: "0.50", Kind: "coin", Active: true},
		usd20ID:   {ID: usd20ID, Currency: currencyUSD, Value: "20.00", Kind: "bill", Active: true},
		retiredID: {ID: retiredID, Currency: currencyMXN, Value: "1000.00", Kind: "bill", Active: false},
	}
}

// Ejemplo de la especificación: 5 × $500 + 8 × $200 + 10 × $10 = $4,200.
func TestTallyCountByDenominations(t *testing.T) {
	tally, err := tallyCount(CountInput{Lines: []CountLineInput{
		{DenominationID: bill500ID, Quantity: 5},
		{DenominationID: bill200ID, Quantity: 8},
		{DenominationID: coin10ID, Quantity: 10},
	}}, testCatalog(), false)
	if err != nil || money(tally.mxn) != "4200.00" || money(tally.usd) != "0.00" {
		t.Fatalf("total contado incorrecto: %v %s", err, money(tally.mxn))
	}
	if money(tally.lines[1].amount) != "1600.00" {
		t.Fatalf("importe por denominación incorrecto: %s", money(tally.lines[1].amount))
	}
	mixed, err := tallyCount(CountInput{
		Lines:     []CountLineInput{{DenominationID: coin050ID, Quantity: 3}, {DenominationID: usd20ID, Quantity: 2}},
		ExtraMXN:  "12.30",
		ExtraUSD:  "1.25",
		ExtraNote: "Monedas sueltas sin desglosar",
	}, testCatalog(), true)
	if err != nil || money(mixed.mxn) != "13.80" || money(mixed.usd) != "41.25" || money(mixed.extraMXN) != "12.30" {
		t.Fatalf("conteo por moneda con adicional incorrecto: %v %s %s", err, money(mixed.mxn), money(mixed.usd))
	}
}

func TestTallyCountRejectsInvalidCaptures(t *testing.T) {
	cases := []CountInput{
		{Lines: []CountLineInput{{DenominationID: bill500ID, Quantity: -1}}},
		{Lines: []CountLineInput{{DenominationID: bill500ID, Quantity: 100001}}},
		{Lines: []CountLineInput{{DenominationID: bill500ID, Quantity: 1}, {DenominationID: bill500ID, Quantity: 2}}},
		{Lines: []CountLineInput{{DenominationID: retiredID, Quantity: 1}}},
		{Lines: []CountLineInput{{DenominationID: "no-es-uuid", Quantity: 1}}},
		{Lines: []CountLineInput{{DenominationID: "00000000-0000-4000-8000-000000000123", Quantity: 1}}},
		// El adicional nunca es negativo y debe identificarse.
		{ExtraMXN: "-5", ExtraNote: "Ajuste"},
		{ExtraMXN: "5"},
		{ExtraMXN: "1.234", ExtraNote: "Monedas"},
	}
	for index, input := range cases {
		if _, err := tallyCount(input, testCatalog(), true); err == nil {
			t.Fatalf("caso %d: se aceptó un conteo inválido", index)
		}
	}
	// Sin dólares en el turno, cualquier cantidad en dólares se rechaza como función desactivada.
	for _, input := range []CountInput{
		{Lines: []CountLineInput{{DenominationID: usd20ID, Quantity: 1}}},
		{ExtraUSD: "3", ExtraNote: "Billetes sueltos"},
	} {
		_, err := tallyCount(input, testCatalog(), false)
		if problem, ok := err.(*Error); !ok || problem.Code != "FEATURE_DISABLED" {
			t.Fatalf("un conteo en dólares sin la función no se rechazó: %v", err)
		}
	}
	if _, err := tallyCount(CountInput{Lines: []CountLineInput{{DenominationID: usd20ID, Quantity: 0}}}, testCatalog(), false); err != nil {
		t.Fatal("una cantidad cero en dólares no implica dólares en el turno")
	}
}

// El fondo declarado debe coincidir con el desglose en cada moneda.
func TestOpeningCountMustMatchDeclaredFloat(t *testing.T) {
	tally, _ := tallyCount(CountInput{Lines: []CountLineInput{{DenominationID: bill500ID, Quantity: 2}}}, testCatalog(), false)
	if err := matchOpeningCount(tally, &openingPlan{float: amount("1000")}); err != nil {
		t.Fatalf("un desglose exacto se rechazó: %v", err)
	}
	problem, ok := matchOpeningCount(tally, &openingPlan{float: amount("900")}).(*Error)
	if !ok || problem.Code != "COUNT_TOTAL_MISMATCH" {
		t.Fatal("un desglose que no coincide con el fondo se aceptó")
	}
	floatUSD := amount("20")
	if err := matchOpeningCount(tally, &openingPlan{float: amount("1000"), floatUSD: &floatUSD}); err == nil {
		t.Fatal("un fondo en dólares sin desglose en dólares se aceptó")
	}
}

// Todo permiso de caja registrado en Go existe en la 0008 y se concede al administrador, para que
// los administradores existentes sigan contando como completos.
func TestCashPermissionsAreMigratedAndGrantedToAdministrator(t *testing.T) {
	content, err := fs.ReadFile(migrations.Files, "0008_cash_shifts_and_reconciliation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	grant := sql[strings.LastIndex(sql, "INSERT INTO app.role_permissions"):]
	if !strings.Contains(grant, "r.code = 'administrator'") {
		t.Fatal("la 0008 no concede sus permisos al administrador")
	}
	found := 0
	for _, permission := range Permissions {
		if !strings.HasPrefix(permission.Key, "cash.") || permission.Key == "cash.shifts.open" || permission.Key == "cash.shifts.read" {
			continue
		}
		found++
		entry := "('" + permission.Key + "', '" + permission.Description + "')"
		if !strings.Contains(sql, entry) {
			t.Fatalf("la 0008 no registra %s con la misma descripción", permission.Key)
		}
		if !strings.Contains(grant, "'"+permission.Key+"'") {
			t.Fatalf("la 0008 no concede %s al administrador", permission.Key)
		}
	}
	if found != 19 {
		t.Fatalf("se esperaban 19 permisos nuevos de caja, hay %d", found)
	}
}
