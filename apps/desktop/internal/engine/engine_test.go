package engine

import "testing"

func testCatalog() *Catalog {
	return &Catalog{
		SchemaVersion: 1,
		Units: []KitchenUnit{
			{ID: "u-sandwich", Name: "Sandwich", Category: "main", Measure: "unit", IsKitchen: true, Active: true},
			{ID: "u-juice", Name: "Jugo de naranja", Category: "drink", Measure: "glass", Aliases: []string{"Naranja"}, IsKitchen: true, Active: true},
			{ID: "u-hatsu", Name: "Hatsu", Category: "drink", Measure: "bottle", IsKitchen: true, Active: true},
			{ID: "u-old", Name: "Unidad vieja", Category: "other", Measure: "unit", IsKitchen: true, Active: false},
		},
		Products: []Product{
			{ID: "p-breakfast", Name: "Desayuno 1", Category: "breakfast", IsKitchen: true, Active: true},
			{ID: "p-drink", Name: "Bebida suelta", Category: "add_on", IsKitchen: true, Active: true},
			{ID: "p-twin-active", Name: "Duplicado", Category: "breakfast", IsKitchen: true, Active: true},
			{ID: "p-twin-inactive", Name: "Duplicado", Category: "breakfast", IsKitchen: true, Active: false},
			{ID: "p-nonkitchen", Name: "Globo", Category: "accessory", IsKitchen: false, Active: true},
		},
		Recipes: []Recipe{
			{
				ProductID: "p-breakfast",
				Components: []Component{
					{Kind: "fixed", UnitID: "u-sandwich", Quantity: 1},
					{
						Kind: "choice", Label: "Bebida", Required: true,
						Options: []ChoiceOption{
							{ID: "o-naranja", Label: "Jugo de naranja", MatchAliases: []string{"naranja"}, Components: []Component{{Kind: "fixed", UnitID: "u-juice", Quantity: 1}}},
							{ID: "o-hatsu", Label: "Hatsu", Default: true, Components: []Component{{Kind: "fixed", UnitID: "u-hatsu", Quantity: 1}}},
						},
					},
				},
			},
			{
				ProductID: "p-drink",
				Components: []Component{
					{Kind: "fixed", UnitID: "u-old", Quantity: 1},
				},
			},
		},
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Café  MOCA":                   "cafe moca",
		"Jugo de naranja 100% natural": "jugo de naranja 100 natural",
		"  Üva--Verde ":                "uva verde",
	}
	for input, want := range cases {
		if got := NormalizeName(input); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveDefaultChoice(t *testing.T) {
	index := IndexCatalog(testCatalog())
	line := ParsedOrderLine{Raw: "2 desayuno 1", ProductText: "Desayuno 1", Quantity: 2, Options: []string{}, Source: "breakfast", OrderNumber: "10"}
	resolved := ResolveLine(line, index)
	if resolved.Status != "resolved" {
		t.Fatalf("status = %q, want resolved", resolved.Status)
	}
	if len(resolved.MatchedOptions) != 1 || resolved.MatchedOptions[0] != "Hatsu" {
		t.Fatalf("matched = %v, want [Hatsu]", resolved.MatchedOptions)
	}
	want := map[string]int{"u-sandwich": 2, "u-hatsu": 2}
	if len(resolved.Contributions) != 2 {
		t.Fatalf("contributions = %v", resolved.Contributions)
	}
	for _, contribution := range resolved.Contributions {
		if want[contribution.UnitID] != contribution.Quantity {
			t.Errorf("contribution %s = %d, want %d", contribution.UnitID, contribution.Quantity, want[contribution.UnitID])
		}
	}
}

func TestResolveOptionByAlias(t *testing.T) {
	index := IndexCatalog(testCatalog())
	line := ParsedOrderLine{ProductText: "Desayuno 1", Quantity: 1, Options: []string{"Jugo de naranja"}, Source: "breakfast"}
	resolved := ResolveLine(line, index)
	found := false
	for _, contribution := range resolved.Contributions {
		if contribution.UnitID == "u-juice" && contribution.Quantity == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a u-juice contribution, got %v", resolved.Contributions)
	}
}

func TestResolveUnknownAndIgnored(t *testing.T) {
	index := IndexCatalog(testCatalog())
	if got := ResolveLine(ParsedOrderLine{ProductText: "Nada"}, index); got.Status != "unresolved" || got.UnresolvedReason != "unknown_product" {
		t.Fatalf("unknown: %+v", got)
	}
	if got := ResolveLine(ParsedOrderLine{ProductText: "Globo"}, index); got.Status != "ignored" || got.IgnoredReason != "not_kitchen" {
		t.Fatalf("ignored: %+v", got)
	}
}

func TestActiveTwinWins(t *testing.T) {
	index := IndexCatalog(testCatalog())
	if got := ResolveLine(ParsedOrderLine{ProductText: "Duplicado"}, index); got.Status == "unresolved" && got.UnresolvedReason == "ambiguous_match" {
		t.Fatalf("an inactive twin should not make the match ambiguous: %+v", got)
	}
}

func TestAggregateAndDiff(t *testing.T) {
	index := IndexCatalog(testCatalog())
	lines := []ParsedOrderLine{
		{ProductText: "Desayuno 1", Quantity: 1, Options: []string{"Jugo de naranja"}, OrderNumber: "1"},
		{ProductText: "Desayuno 1", Quantity: 2, Options: []string{"Jugo de naranja"}, OrderNumber: "2"},
	}
	next := AggregateUnits(ResolveLines(lines, index), index)

	var sandwich *KitchenListEntry
	for i := range next.Entries {
		if next.Entries[i].UnitID == "u-sandwich" {
			sandwich = &next.Entries[i]
		}
	}
	if sandwich == nil || sandwich.Quantity != 3 {
		t.Fatalf("sandwich entry = %+v, want quantity 3", sandwich)
	}
	if len(sandwich.References) != 2 {
		t.Fatalf("references = %v, want 2", sandwich.References)
	}

	diff := DiffLists(nil, next)
	if len(diff.Added) != 2 || len(diff.Changed) != 0 || len(diff.Removed) != 0 {
		t.Fatalf("diff from nil = %+v", diff)
	}
	if IsListDiffEmpty(diff) {
		t.Fatalf("diff should not be empty")
	}

	emptyDiff := DiffLists(&next, next)
	if !IsListDiffEmpty(emptyDiff) {
		t.Fatalf("diff of equal lists should be empty: %+v", emptyDiff)
	}
}
