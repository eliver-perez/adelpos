//go:build integration

package operations_test

import (
	"fmt"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

func newAccount(t *testing.T, fixture *restaurantFixture, name string) operations.Bill {
	t.Helper()
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"name": name, "mode": "dine_in"}, 200))
}

func testCaptureRules(t *testing.T, fixture *restaurantFixture) {
	t.Run("personalizaciones variantes y complementos en pedidos reales", func(t *testing.T) {
		fixture.product.BasePrice = "150"
		fixture.product = decodeResponse[catalog.Product](t, fixture.adminCall(t, "PATCH", "catalog/products/"+fixture.product.ID, fixture.product, 200))
		groupSelection := func(options ...catalog.IncludedOption) catalog.Customization {
			return catalog.Customization{
				Groups: []catalog.GroupSelection{{
					GroupID: fixture.group.ID,
					Options: append([]catalog.IncludedOption{}, options...),
				}},
			}
		}
		tests := []struct {
			name     string
			request  catalog.LineRequest
			expected string
		}{
			{
				name: "retirar todos",
				request: catalog.LineRequest{
					ProductID:     fixture.product.ID,
					Quantity:      1,
					Customization: groupSelection(),
				},
				expected: "150.00",
			},
			{
				name: "reemplazar todos",
				request: catalog.LineRequest{
					ProductID: fixture.product.ID,
					Quantity:  1,
					Customization: groupSelection(
						catalog.IncludedOption{OptionID: fixture.group.Options[2].ID, Quantity: "1"},
						catalog.IncludedOption{OptionID: fixture.group.Options[3].ID, Quantity: "1"},
					),
				},
				expected: "150.00",
			},
			{
				name: "doble papas",
				request: catalog.LineRequest{
					ProductID: fixture.product.ID,
					Quantity:  1,
					Customization: groupSelection(
						catalog.IncludedOption{OptionID: fixture.group.Options[2].ID, Quantity: "2"},
					),
				},
				expected: "165.00",
			},
			{
				name: "ingrediente adicional",
				request: catalog.LineRequest{
					ProductID: fixture.product.ID,
					Quantity:  1,
					Customization: catalog.Customization{
						Ingredients: []catalog.IngredientSelection{{
							IngredientID: fixture.product.Ingredients[0].IngredientID,
							Quantity:     "2",
						}},
					},
				},
				expected: "152.50",
			},
			{
				name: "presentación",
				request: catalog.LineRequest{
					ProductID: fixture.product.ID,
					VariantID: &fixture.product.Variants[0].ID,
					Quantity:  1,
				},
				expected: "180.00",
			},
		}
		for _, scenario := range tests {
			t.Run(scenario.name, func(t *testing.T) {
				bill := fixture.capture(t, newAccount(t, fixture, scenario.name), []catalog.LineRequest{scenario.request})
				if bill.Account.Totals.Total != scenario.expected {
					t.Fatalf("Esperado %s recibido %s", scenario.expected, bill.Account.Totals.Total)
				}
			})
		}
	})

	t.Run("combo escalonado confirmado 90 100 140", func(t *testing.T) {
		for quantity, expected := range []string{"90.00", "100.00", "140.00"} {
			bill := newAccount(t, fixture, fmt.Sprintf("Combo con %d papas", quantity))
			bill = fixture.capture(t, bill, []catalog.LineRequest{{
				ProductID: fixture.combo.ID, Quantity: 1, Customization: catalog.Customization{Components: []catalog.ComponentSelection{{
					SlotID: fixture.combo.Slots[0].ID, ProductID: fixture.fries.ID, Quantity: quantity,
				}}},
			}})
			if bill.Account.Totals.Total != expected {
				t.Fatalf("Combo %d: %s", quantity, bill.Account.Totals.Total)
			}
		}
	})

	t.Run("modificar personalización y agregar productos después", func(t *testing.T) {
		bill := fixture.capture(t, newAccount(t, fixture, "Cambios posteriores"), []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 1}})
		line := bill.Lines[0]
		requested := line.Request
		requested.Customization.Groups = []catalog.GroupSelection{{GroupID: fixture.group.ID, Options: []catalog.IncludedOption{{OptionID: fixture.group.Options[2].ID, Quantity: "1"}, {OptionID: fixture.group.Options[1].ID, Quantity: "1"}}}}
		input := operations.EditInput{Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: line.VersionID, Line: requested, Reason: "Sustitución solicitada por cliente"}
		route := "restaurant/accounts/" + bill.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/edit-preview", input, 200))
		if preview.Quote.Total != "155.00" {
			t.Fatal("El motor no calculó la sustitución en edición confirmada")
		}
		input.ExpectedFingerprint = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/edit", input, 200))
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: 1}})
		if bill.Account.Totals.Total != "195.00" || len(bill.Lines) != 2 {
			t.Fatal("La captura posterior alteró o duplicó productos existentes")
		}
		line = bill.Lines[0]
		if line.ItemID != inputVersionItem(t, fixture, input.VersionID) {
			t.Fatal("Modificar personalización cambió la identidad del producto comercial")
		}
	})

	t.Run("promoción congelada y centavos conservados en división", func(t *testing.T) {
		promotion := catalog.Promotion{Metadata: catalog.Metadata{Name: "2x1 papas", Active: true}, Kind: "two_for_one", Percent: "0", SpecialPrice: "0", BuyQuantity: 2, PayQuantity: 1, MinimumQuantity: 2, ProductIDs: []string{fixture.fries.ID}}
		fixture.adminCall(t, "POST", "catalog/promotions", promotion, 200)
		for quantity, expected := range map[int]string{2: "40.00", 3: "80.00"} {
			bill := fixture.capture(t, newAccount(t, fixture, "Promoción por lote"), []catalog.LineRequest{{ProductID: fixture.fries.ID, Quantity: quantity}})
			if bill.Account.Totals.Total != expected {
				t.Fatalf("2x1 %d: %s", quantity, bill.Account.Totals.Total)
			}
			if quantity == 3 {
				pair := decodeResponse[struct {
					Source operations.Bill `json:"source"`
					Target operations.Bill `json:"target"`
				}](t,
					fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/split", operations.TransferInput{
						Mutation: operations.Mutation{Revision: bill.Account.Revision}, NewAccount: &operations.AccountInput{Name: "Una unidad prorrateada"},
						Selections: []operations.Selection{{VersionID: bill.Lines[0].VersionID, Quantity: 1}},
					}, 200))
				if pair.Target.Account.Totals.Total != "26.66" || pair.Source.Account.Totals.Total != "53.34" {
					t.Fatal("La división perdió centavos o volvió a evaluar la promoción")
				}
			}
		}
	})

	t.Run("lista alternativa y cambio de precio entre revisión y confirmación", func(t *testing.T) {
		list := decodeResponse[catalog.PriceList](t, fixture.adminCall(t, "POST", "catalog/price-lists", catalog.PriceList{
			Metadata: catalog.Metadata{Name: "Plataforma", Active: true}, Fallback: "base", Entries: []catalog.PriceEntry{{ProductID: fixture.product.ID, Price: "115"}},
		}, 200))
		bill := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{
			"name": "Precio de plataforma", "mode": "dine_in", "price_list_id": list.ID,
		}, 200))
		bill = fixture.capture(t, bill, []catalog.LineRequest{{ProductID: fixture.product.ID, Quantity: 1}})
		if bill.Account.Totals.Total != "115.00" {
			t.Fatal("No se respetó la lista autorizada")
		}
		route := "restaurant/accounts/" + bill.Account.ID
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{
			"revision": bill.Account.Revision, "items": []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: fixture.product.ID, Quantity: 1}}},
		}, 200))
		quote := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
		list.Entries[0].Price = "120"
		fixture.adminCall(t, "PATCH", "catalog/price-lists/"+list.ID, list, 200)
		fixture.mutate(t, "POST", route+"/confirm", map[string]any{"revision": bill.Account.Revision, "expected_fingerprint": quote.Fingerprint}, 409)
		bill = fixture.refresh(t, bill.Account.ID)
		if len(bill.Draft) != 1 || bill.Account.Totals.Total != "115.00" {
			t.Fatal("El conflicto perdió el borrador o alteró el precio confirmado")
		}
	})
}

func inputVersionItem(t *testing.T, fixture *restaurantFixture, id string) string {
	t.Helper()
	var itemID string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT item_id FROM app.restaurant_item_versions WHERE id=$1", id).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	return itemID
}
