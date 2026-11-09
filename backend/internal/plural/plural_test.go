package plural

import (
	"encoding/json"
	"os"
	"testing"
)

// countCase es una fila de la tabla compartida con Vitest (testdata/cases.json).
type countCase struct {
	Count    int    `json:"count"`
	Singular string `json:"singular"`
	Plural   string `json:"plural"`
	Text     string `json:"text"`
}

func TestSharedCaseTable(t *testing.T) {
	content, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Count []countCase `json:"count"`
	}
	if err := json.Unmarshal(content, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Count) == 0 {
		t.Fatal("La tabla compartida de casos está vacía")
	}
	for _, item := range table.Count {
		if got := Count(item.Count, item.Singular, item.Plural); got != item.Text {
			t.Errorf("Count(%d, %q, %q) = %q; se esperaba %q", item.Count, item.Singular, item.Plural, got, item.Text)
		}
	}
}

func TestWordAgreesWithTheCount(t *testing.T) {
	if Word(1, "cambia", "cambian") != "cambia" {
		t.Fatal("El singular no corresponde al uno")
	}
	for _, count := range []int{0, 2, 15} {
		if Word(count, "cambia", "cambian") != "cambian" {
			t.Fatalf("El plural no corresponde a %d", count)
		}
	}
}
