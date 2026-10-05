package engine

// ListDeltaEntry is one changed unit between two lists.
type ListDeltaEntry struct {
	UnitID   string `json:"unitId"`
	Name     string `json:"name"`
	Previous int    `json:"previous"`
	Next     int    `json:"next"`
	Delta    int    `json:"delta"`
}

// ListDiff is the change set between two lists.
type ListDiff struct {
	Added   []ListDeltaEntry `json:"added"`
	Removed []ListDeltaEntry `json:"removed"`
	Changed []ListDeltaEntry `json:"changed"`
}

// DiffLists compares two lists (mirrors core diffLists).
func DiffLists(previous *KitchenList, next KitchenList) ListDiff {
	before := map[string]KitchenListEntry{}
	if previous != nil {
		for _, entry := range previous.Entries {
			before[entry.UnitID] = entry
		}
	}
	after := map[string]KitchenListEntry{}
	for _, entry := range next.Entries {
		after[entry.UnitID] = entry
	}

	diff := ListDiff{Added: []ListDeltaEntry{}, Removed: []ListDeltaEntry{}, Changed: []ListDeltaEntry{}}
	for _, entry := range next.Entries {
		old, ok := before[entry.UnitID]
		if !ok {
			diff.Added = append(diff.Added, deltaEntry(entry.UnitID, entry.Name, 0, entry.Quantity))
		} else if old.Quantity != entry.Quantity {
			diff.Changed = append(diff.Changed, deltaEntry(entry.UnitID, entry.Name, old.Quantity, entry.Quantity))
		}
	}
	if previous != nil {
		for _, entry := range previous.Entries {
			if _, ok := after[entry.UnitID]; !ok {
				diff.Removed = append(diff.Removed, deltaEntry(entry.UnitID, entry.Name, entry.Quantity, 0))
			}
		}
	}
	return diff
}

// IsListDiffEmpty reports whether there are no changes.
func IsListDiffEmpty(diff ListDiff) bool {
	return len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Changed) == 0
}

func deltaEntry(unitID, name string, previous, next int) ListDeltaEntry {
	return ListDeltaEntry{UnitID: unitID, Name: name, Previous: previous, Next: next, Delta: next - previous}
}
