package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealSeed indexes the repository seed when it is present (it is private, so
// a public clone skips this test).
func TestRealSeed(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "data", "seed.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("data/seed.json not present")
	}
	catalog, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	index := IndexCatalog(catalog)
	if len(index.ProductsByID) == 0 || len(index.UnitsByID) == 0 {
		t.Fatalf("empty index: %d products, %d units", len(index.ProductsByID), len(index.UnitsByID))
	}
	activeKitchen := 0
	for id, product := range index.ProductsByID {
		if !product.IsKitchen || !product.Active {
			continue
		}
		activeKitchen++
		if index.RecipesByProductID[id] == nil {
			t.Errorf("active kitchen product %q has no recipe", id)
		}
	}
	t.Logf("seed: %d products, %d units, %d recipes, %d active kitchen products",
		len(index.ProductsByID), len(index.UnitsByID), len(index.RecipesByProductID), activeKitchen)
}
