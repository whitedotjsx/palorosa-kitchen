// Package engine is the Go port of the kitchen core engine: it indexes the
// catalog and resolves order lines into the aggregated daily kitchen list. It
// mirrors packages/core (schema, catalog, resolve, aggregate, diff, format).
package engine

import (
	"encoding/json"
	"os"
)

// KitchenUnit is a preparation unit the kitchen makes.
type KitchenUnit struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category"`
	Measure     string   `json:"measure"`
	Aliases     []string `json:"aliases"`
	Note        string   `json:"note,omitempty"`
	IsKitchen   bool     `json:"isKitchen"`
	Active      bool     `json:"active"`
}

// Product is a sellable store product.
type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Category    string   `json:"category"`
	IsKitchen   bool     `json:"isKitchen"`
	PriceCop    int      `json:"priceCop,omitempty"`
	Description string   `json:"description,omitempty"`
	ContainerID string   `json:"containerId,omitempty"`
	WpProductID int      `json:"wpProductId,omitempty"`
	Sku         string   `json:"sku,omitempty"`
	Active      bool     `json:"active"`
}

// Component is a recipe component, either fixed or a choice group.
type Component struct {
	Kind     string         `json:"kind"`
	UnitID   string         `json:"unitId,omitempty"`
	Quantity int            `json:"quantity,omitempty"`
	Label    string         `json:"label,omitempty"`
	Required bool           `json:"required,omitempty"`
	Options  []ChoiceOption `json:"options,omitempty"`
}

// ChoiceOption is one option of a choice group.
type ChoiceOption struct {
	ID           string      `json:"id"`
	Label        string      `json:"label"`
	MatchAliases []string    `json:"matchAliases"`
	Default      bool        `json:"default,omitempty"`
	Components   []Component `json:"components"`
}

// Recipe maps a product to its components.
type Recipe struct {
	ProductID  string      `json:"productId"`
	Components []Component `json:"components"`
}

// Container is a packaging reference.
type Container struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Active  bool     `json:"active"`
}

// LabelParsingRules holds the parser tuning (kept for catalog round trips).
type LabelParsingRules struct {
	QuantityPatterns  []string `json:"quantityPatterns"`
	ProductSeparators []string `json:"productSeparators"`
	OptionSeparators  []string `json:"optionSeparators"`
	NoisePatterns     []string `json:"noisePatterns"`
}

// Catalog is the whole seed.
type Catalog struct {
	SchemaVersion int               `json:"schemaVersion"`
	Units         []KitchenUnit     `json:"units"`
	Products      []Product         `json:"products"`
	Recipes       []Recipe          `json:"recipes"`
	Containers    []Container       `json:"containers"`
	LabelParsing  LabelParsingRules `json:"labelParsing"`
}

// LoadCatalog reads a catalog seed from disk.
func LoadCatalog(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

// Index is the lookup structure over a catalog.
type Index struct {
	ProductsByID       map[string]*Product
	UnitsByID          map[string]*KitchenUnit
	RecipesByProductID map[string]*Recipe
	ContainersByID     map[string]*Container
	productsByText     map[string][]*Product
	unitsByText        map[string][]*KitchenUnit
}

// IndexCatalog builds the catalog index.
func IndexCatalog(catalog *Catalog) *Index {
	index := &Index{
		ProductsByID:       make(map[string]*Product, len(catalog.Products)),
		UnitsByID:          make(map[string]*KitchenUnit, len(catalog.Units)),
		RecipesByProductID: make(map[string]*Recipe, len(catalog.Recipes)),
		ContainersByID:     make(map[string]*Container, len(catalog.Containers)),
		productsByText:     make(map[string][]*Product),
		unitsByText:        make(map[string][]*KitchenUnit),
	}
	for i := range catalog.Units {
		unit := &catalog.Units[i]
		index.UnitsByID[unit.ID] = unit
		pushUnitText(index.unitsByText, unit.Name, unit)
		for _, alias := range unit.Aliases {
			pushUnitText(index.unitsByText, alias, unit)
		}
	}
	for i := range catalog.Products {
		product := &catalog.Products[i]
		index.ProductsByID[product.ID] = product
		pushProductText(index.productsByText, product.Name, product)
		for _, alias := range product.Aliases {
			pushProductText(index.productsByText, alias, product)
		}
	}
	for i := range catalog.Recipes {
		recipe := &catalog.Recipes[i]
		index.RecipesByProductID[recipe.ProductID] = recipe
	}
	for i := range catalog.Containers {
		container := &catalog.Containers[i]
		index.ContainersByID[container.ID] = container
	}
	return index
}

// LookupProducts returns the products whose name or alias matches the text.
func (ix *Index) LookupProducts(text string) []*Product {
	return ix.productsByText[NormalizeName(text)]
}

func pushProductText(m map[string][]*Product, text string, value *Product) {
	key := NormalizeName(text)
	if key == "" {
		return
	}
	for _, existing := range m[key] {
		if existing == value {
			return
		}
	}
	m[key] = append(m[key], value)
}

func pushUnitText(m map[string][]*KitchenUnit, text string, value *KitchenUnit) {
	key := NormalizeName(text)
	if key == "" {
		return
	}
	for _, existing := range m[key] {
		if existing == value {
			return
		}
	}
	m[key] = append(m[key], value)
}
