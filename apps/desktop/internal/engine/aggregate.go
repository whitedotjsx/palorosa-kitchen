package engine

import (
	"sort"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// UnitCategoryOrder is the display order of kitchen unit categories.
var UnitCategoryOrder = []string{"drink", "main", "side", "dessert", "fruit", "condiment", "other"}

var spanishCollator = collate.New(language.Spanish)

// KitchenUnitSource is one order line that contributes to a unit, so the panel
// can show where each counted unit comes from (its breakfast or add-on).
type KitchenUnitSource struct {
	OrderNumber string `json:"orderNumber,omitempty"`
	ProductText string `json:"productText"`
	// Source is "breakfast" or "add_on".
	Source   string `json:"source,omitempty"`
	Quantity int    `json:"quantity"`
}

// KitchenListEntry is one aggregated unit row.
type KitchenListEntry struct {
	UnitID     string              `json:"unitId"`
	Name       string              `json:"name"`
	Measure    string              `json:"measure"`
	Category   string              `json:"category"`
	Note       string              `json:"note,omitempty"`
	Quantity   int                 `json:"quantity"`
	References []string            `json:"references"`
	Sources    []KitchenUnitSource `json:"sources,omitempty"`
}

// UnresolvedEntry is an aggregated unresolved row.
type UnresolvedEntry struct {
	ProductText string   `json:"productText"`
	Quantity    int      `json:"quantity"`
	Count       int      `json:"count"`
	Reason      string   `json:"reason"`
	Detail      string   `json:"detail,omitempty"`
	References  []string `json:"references"`
}

// IgnoredEntry is an aggregated ignored row (not kitchen or inactive unit).
type IgnoredEntry struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Quantity   int      `json:"quantity"`
	Count      int      `json:"count"`
	Reason     string   `json:"reason"`
	References []string `json:"references"`
}

// WarningEntry is an aggregated warning row.
type WarningEntry struct {
	Raw         string   `json:"raw"`
	ProductText string   `json:"productText"`
	Quantity    int      `json:"quantity"`
	Count       int      `json:"count"`
	References  []string `json:"references"`
}

// KitchenList is the aggregated daily list.
type KitchenList struct {
	Entries         []KitchenListEntry `json:"entries"`
	Unresolved      []UnresolvedEntry  `json:"unresolved"`
	IgnoredProducts []IgnoredEntry     `json:"ignoredProducts"`
	IgnoredUnits    []IgnoredEntry     `json:"ignoredUnits"`
	Warnings        []WarningEntry     `json:"warnings"`
}

func referenceOf(line ResolvedLine) string {
	if line.Line.OrderNumber != "" {
		return line.Line.OrderNumber
	}
	return line.Line.Raw
}

func pushReference(references []string, reference string) []string {
	if reference == "" {
		return references
	}
	for _, existing := range references {
		if existing == reference {
			return references
		}
	}
	return append(references, reference)
}

// AggregateUnits turns resolved lines into the daily list (mirrors core
// aggregateUnits).
func AggregateUnits(resolved []ResolvedLine, index *Index) KitchenList {
	entries := map[string]*KitchenListEntry{}
	unresolvedEntries := map[string]*UnresolvedEntry{}
	ignoredProducts := map[string]*IgnoredEntry{}
	ignoredUnits := map[string]*IgnoredEntry{}
	warnings := map[string]*WarningEntry{}
	var warningOrder []string

	for _, line := range resolved {
		if line.Line.Warning != "" {
			if _, ok := warnings[line.Line.Raw]; !ok {
				warningOrder = append(warningOrder, line.Line.Raw)
			}
			addWarning(warnings, line)
		}

		if line.Status == "ignored" {
			id := line.ProductID
			if id == "" {
				id = line.Line.ProductText
			}
			name := line.ProductName
			if name == "" {
				name = line.Line.ProductText
			}
			reason := line.IgnoredReason
			if reason == "" {
				reason = "not_kitchen"
			}
			addIgnored(ignoredProducts, id, name, reason, line.Line.Quantity, referenceOf(line))
			continue
		}
		if line.Status == "unresolved" {
			addUnresolved(unresolvedEntries, line)
		}

		for _, contribution := range line.Contributions {
			unit := index.UnitsByID[contribution.UnitID]
			reference := referenceOf(line)
			if unit == nil {
				continue
			}
			if !unit.IsKitchen || !unit.Active {
				reason := "not_kitchen"
				if unit.IsKitchen {
					reason = "inactive_unit"
				}
				addIgnored(ignoredUnits, unit.ID, unit.Name, reason, contribution.Quantity, reference)
				continue
			}
			addEntry(entries, unit, contribution.Quantity, reference, KitchenUnitSource{
				OrderNumber: line.Line.OrderNumber,
				ProductText: line.Line.ProductText,
				Source:      line.Line.Source,
				Quantity:    contribution.Quantity,
			})
		}
	}

	list := KitchenList{
		Entries:         make([]KitchenListEntry, 0, len(entries)),
		Unresolved:      make([]UnresolvedEntry, 0, len(unresolvedEntries)),
		IgnoredProducts: make([]IgnoredEntry, 0, len(ignoredProducts)),
		IgnoredUnits:    make([]IgnoredEntry, 0, len(ignoredUnits)),
		Warnings:        make([]WarningEntry, 0, len(warnings)),
	}
	for _, entry := range entries {
		list.Entries = append(list.Entries, *entry)
	}
	for _, entry := range unresolvedEntries {
		list.Unresolved = append(list.Unresolved, *entry)
	}
	for _, entry := range ignoredProducts {
		list.IgnoredProducts = append(list.IgnoredProducts, *entry)
	}
	for _, entry := range ignoredUnits {
		list.IgnoredUnits = append(list.IgnoredUnits, *entry)
	}
	for _, raw := range warningOrder {
		list.Warnings = append(list.Warnings, *warnings[raw])
	}

	sort.SliceStable(list.Entries, func(i, j int) bool { return byCategoryThenName(list.Entries[i], list.Entries[j]) })
	for index := range list.Entries {
		sources := list.Entries[index].Sources
		sort.SliceStable(sources, func(i, j int) bool {
			if sources[i].Quantity != sources[j].Quantity {
				return sources[i].Quantity > sources[j].Quantity
			}
			if sources[i].ProductText != sources[j].ProductText {
				return spanishCollator.CompareString(sources[i].ProductText, sources[j].ProductText) < 0
			}
			return sources[i].OrderNumber < sources[j].OrderNumber
		})
	}
	sort.SliceStable(list.Unresolved, func(i, j int) bool {
		a, b := list.Unresolved[i], list.Unresolved[j]
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		return spanishCollator.CompareString(a.ProductText, b.ProductText) < 0
	})
	sort.SliceStable(list.IgnoredProducts, func(i, j int) bool {
		return spanishCollator.CompareString(list.IgnoredProducts[i].Name, list.IgnoredProducts[j].Name) < 0
	})
	sort.SliceStable(list.IgnoredUnits, func(i, j int) bool {
		return spanishCollator.CompareString(list.IgnoredUnits[i].Name, list.IgnoredUnits[j].Name) < 0
	})
	return list
}

func byCategoryThenName(a, b KitchenListEntry) bool {
	if rank(a.Category) != rank(b.Category) {
		return rank(a.Category) < rank(b.Category)
	}
	return spanishCollator.CompareString(a.Name, b.Name) < 0
}

func rank(category string) int {
	for i, value := range UnitCategoryOrder {
		if value == category {
			return i
		}
	}
	return len(UnitCategoryOrder)
}

func addEntry(entries map[string]*KitchenListEntry, unit *KitchenUnit, quantity int, reference string, source KitchenUnitSource) {
	if existing, ok := entries[unit.ID]; ok {
		existing.Quantity += quantity
		existing.References = pushReference(existing.References, reference)
		addSource(existing, source)
		return
	}
	entry := &KitchenListEntry{
		UnitID:   unit.ID,
		Name:     unit.Name,
		Measure:  unit.Measure,
		Category: unit.Category,
		Note:     unit.Note,
		Quantity: quantity,
	}
	entry.References = pushReference(nil, reference)
	addSource(entry, source)
	entries[unit.ID] = entry
}

// addSource merges a line contribution into the entry's provenance, so the
// same breakfast of the same order is one row and different orders stay apart.
func addSource(entry *KitchenListEntry, source KitchenUnitSource) {
	if source.ProductText == "" {
		return
	}
	for index := range entry.Sources {
		existing := &entry.Sources[index]
		if existing.OrderNumber == source.OrderNumber && existing.ProductText == source.ProductText && existing.Source == source.Source {
			existing.Quantity += source.Quantity
			return
		}
	}
	entry.Sources = append(entry.Sources, source)
}

func addIgnored(entries map[string]*IgnoredEntry, id, name, reason string, quantity int, reference string) {
	key := reason + "|" + id
	if existing, ok := entries[key]; ok {
		existing.Quantity += quantity
		existing.Count++
		existing.References = pushReference(existing.References, reference)
		return
	}
	entry := &IgnoredEntry{ID: id, Name: name, Quantity: quantity, Count: 1, Reason: reason}
	entry.References = pushReference(nil, reference)
	entries[key] = entry
}

func addUnresolved(entries map[string]*UnresolvedEntry, line ResolvedLine) {
	reason := line.UnresolvedReason
	if reason == "" {
		reason = "unknown_product"
	}
	key := reason + "|" + line.UnresolvedDetail + "|" + NormalizeName(line.Line.ProductText)
	reference := referenceOf(line)
	if existing, ok := entries[key]; ok {
		existing.Quantity += line.Line.Quantity
		existing.Count++
		existing.References = pushReference(existing.References, reference)
		return
	}
	entry := &UnresolvedEntry{
		ProductText: line.Line.ProductText,
		Quantity:    line.Line.Quantity,
		Count:       1,
		Reason:      reason,
		Detail:      line.UnresolvedDetail,
	}
	entry.References = pushReference(nil, reference)
	entries[key] = entry
}

func addWarning(entries map[string]*WarningEntry, line ResolvedLine) {
	reference := referenceOf(line)
	if existing, ok := entries[line.Line.Raw]; ok {
		existing.Quantity += line.Line.Quantity
		existing.Count++
		existing.References = pushReference(existing.References, reference)
		return
	}
	entry := &WarningEntry{
		Raw:         line.Line.Raw,
		ProductText: line.Line.ProductText,
		Quantity:    line.Line.Quantity,
		Count:       1,
	}
	entry.References = pushReference(nil, reference)
	entries[line.Line.Raw] = entry
}

// CategoryGroup is a run of entries sharing a category.
type CategoryGroup struct {
	Category string
	Entries  []KitchenListEntry
}

// GroupEntriesByCategory groups consecutive entries by category.
func GroupEntriesByCategory(list KitchenList) []CategoryGroup {
	var groups []CategoryGroup
	for _, entry := range list.Entries {
		last := len(groups) - 1
		if last >= 0 && groups[last].Category == entry.Category {
			groups[last].Entries = append(groups[last].Entries, entry)
		} else {
			groups = append(groups, CategoryGroup{Category: entry.Category, Entries: []KitchenListEntry{entry}})
		}
	}
	return groups
}
