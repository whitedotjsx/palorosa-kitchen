package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseCatalog decodes and validates a catalog document. It returns the decoded
// catalog so the caller can index it, plus a list of every problem found; a
// catalog with problems must not be saved.
func ParseCatalog(raw []byte) (*Catalog, []string, error) {
	var catalog Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, nil, fmt.Errorf("JSON inválido: %w", err)
	}
	return &catalog, ValidateCatalog(&catalog), nil
}

// ValidateCatalog checks ids are present and unique and that every reference
// (recipe product, component unit, product container) points at something
// that exists. It returns human readable problems in Spanish.
func ValidateCatalog(catalog *Catalog) []string {
	var problems []string
	add := func(format string, args ...any) {
		if len(problems) < 50 {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}
	if catalog.SchemaVersion <= 0 {
		add("Falta schemaVersion")
	}
	units := map[string]bool{}
	for _, unit := range catalog.Units {
		switch {
		case strings.TrimSpace(unit.ID) == "":
			add("Unidad sin id (%q)", unit.Name)
		case units[unit.ID]:
			add("Unidad duplicada: %s", unit.ID)
		}
		units[unit.ID] = true
	}
	containers := map[string]bool{}
	for _, container := range catalog.Containers {
		switch {
		case strings.TrimSpace(container.ID) == "":
			add("Contenedor sin id (%q)", container.Name)
		case containers[container.ID]:
			add("Contenedor duplicado: %s", container.ID)
		}
		containers[container.ID] = true
	}
	products := map[string]bool{}
	for _, product := range catalog.Products {
		switch {
		case strings.TrimSpace(product.ID) == "":
			add("Producto sin id (%q)", product.Name)
		case products[product.ID]:
			add("Producto duplicado: %s", product.ID)
		}
		products[product.ID] = true
		if product.ContainerID != "" && !containers[product.ContainerID] {
			add("El producto %s usa un contenedor que no existe: %s", product.ID, product.ContainerID)
		}
	}
	recipes := map[string]bool{}
	for _, recipe := range catalog.Recipes {
		if !products[recipe.ProductID] {
			add("Receta de un producto que no existe: %s", recipe.ProductID)
		}
		if recipes[recipe.ProductID] {
			add("Receta duplicada: %s", recipe.ProductID)
		}
		recipes[recipe.ProductID] = true
		validateComponents(recipe.ProductID, recipe.Components, units, add)
	}
	return problems
}

func validateComponents(owner string, components []Component, units map[string]bool, add func(string, ...any)) {
	for _, component := range components {
		switch component.Kind {
		case "fixed":
			if !units[component.UnitID] {
				add("La receta %s usa una unidad que no existe: %q", owner, component.UnitID)
			}
			if component.Quantity <= 0 {
				add("La receta %s tiene una cantidad inválida para %s", owner, component.UnitID)
			}
		case "choice":
			if len(component.Options) == 0 {
				add("La receta %s tiene un grupo de opciones vacío (%q)", owner, component.Label)
			}
			for _, option := range component.Options {
				validateComponents(owner, option.Components, units, add)
			}
		default:
			add("La receta %s tiene un componente de tipo desconocido: %q", owner, component.Kind)
		}
	}
}

// SaveCatalog validates raw and writes it to path atomically, keeping the
// previous file as path.bak. The bytes are written as given (pretty printed) so
// fields the Go model does not know survive the round trip.
func SaveCatalog(path string, raw []byte) (*Catalog, error) {
	if path == "" {
		return nil, errors.New("no hay ruta de catálogo configurada")
	}
	catalog, problems, err := ParseCatalog(raw)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, &CatalogError{Problems: problems}
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if previous, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", previous, 0o644); err != nil {
			return nil, err
		}
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, pretty.Bytes(), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return nil, err
	}
	return catalog, nil
}

// CatalogError reports validation problems that blocked a save.
type CatalogError struct {
	Problems []string
}

func (e *CatalogError) Error() string {
	return "catálogo inválido: " + strings.Join(e.Problems, "; ")
}
