package engine

import "strings"

// ParsedOrderLine is an order line as delivered by the WordPress hook or the
// rótulos parser.
type ParsedOrderLine struct {
	Raw         string   `json:"raw"`
	ProductText string   `json:"productText"`
	Quantity    int      `json:"quantity"`
	Options     []string `json:"options"`
	Source      string   `json:"source"`
	OrderNumber string   `json:"orderNumber,omitempty"`
	Warning     string   `json:"warning,omitempty"`
}

// UnitContribution is one unit a line contributes.
type UnitContribution struct {
	UnitID   string
	Quantity int
}

// ResolvedLine is the resolver output for one line.
type ResolvedLine struct {
	Line             ParsedOrderLine
	Status           string // resolved | unresolved | ignored
	ProductID        string
	ProductName      string
	Contributions    []UnitContribution
	MatchedOptions   []string
	UnresolvedReason string
	UnresolvedDetail string
	IgnoredReason    string
}

type recipeMatch struct {
	matched   []ChoiceOption
	missing   []string
	ambiguous []string
}

// ResolveLine resolves one line against the catalog (mirrors core resolveLine).
func ResolveLine(line ParsedOrderLine, index *Index) ResolvedLine {
	candidates := index.LookupProducts(line.ProductText)
	active := make([]*Product, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Active {
			active = append(active, candidate)
		}
	}
	chosen := candidates
	if len(active) > 0 {
		chosen = active
	}
	if len(chosen) > 1 {
		names := make([]string, 0, len(chosen))
		for _, candidate := range chosen {
			names = append(names, candidate.Name)
		}
		return unresolved(line, "ambiguous_match", nil, strings.Join(names, ", "), nil)
	}
	if len(chosen) == 0 {
		return unresolved(line, "unknown_product", nil, "", nil)
	}
	product := chosen[0]

	if !product.IsKitchen {
		return ResolvedLine{
			Line:          line,
			Status:        "ignored",
			ProductID:     product.ID,
			ProductName:   product.Name,
			IgnoredReason: "not_kitchen",
		}
	}

	recipe := index.RecipesByProductID[product.ID]
	if recipe == nil {
		return unresolved(line, "missing_recipe", product, "", nil)
	}

	matching := matchRecipeOptions(recipe, line.Options)

	contributions := make([]UnitContribution, 0)
	for _, component := range recipe.Components {
		if component.Kind == "fixed" {
			contributions = append(contributions, UnitContribution{
				UnitID:   component.UnitID,
				Quantity: component.Quantity * line.Quantity,
			})
		}
	}
	for _, option := range matching.matched {
		for _, detail := range option.Components {
			contributions = append(contributions, UnitContribution{
				UnitID:   detail.UnitID,
				Quantity: detail.Quantity * line.Quantity,
			})
		}
	}

	if len(matching.ambiguous) > 0 {
		return unresolved(line, "ambiguous_match", product, strings.Join(matching.ambiguous, ", "), contributions)
	}
	if len(matching.missing) > 0 {
		return unresolved(line, "missing_choice", product, strings.Join(matching.missing, ", "), contributions)
	}
	for _, contribution := range contributions {
		if _, ok := index.UnitsByID[contribution.UnitID]; !ok {
			return unresolved(line, "missing_unit", product, contribution.UnitID, contributions)
		}
	}

	matched := make([]string, 0, len(matching.matched))
	for _, option := range matching.matched {
		matched = append(matched, option.Label)
	}
	return ResolvedLine{
		Line:           line,
		Status:         "resolved",
		ProductID:      product.ID,
		ProductName:    product.Name,
		Contributions:  contributions,
		MatchedOptions: matched,
	}
}

// ResolveLines resolves many lines.
func ResolveLines(lines []ParsedOrderLine, index *Index) []ResolvedLine {
	out := make([]ResolvedLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, ResolveLine(line, index))
	}
	return out
}

func unresolved(line ParsedOrderLine, reason string, product *Product, detail string, contributions []UnitContribution) ResolvedLine {
	if contributions == nil {
		contributions = []UnitContribution{}
	}
	resolved := ResolvedLine{
		Line:             line,
		Status:           "unresolved",
		Contributions:    contributions,
		MatchedOptions:   []string{},
		UnresolvedReason: reason,
		UnresolvedDetail: detail,
	}
	if product != nil {
		resolved.ProductID = product.ID
		resolved.ProductName = product.Name
	}
	return resolved
}

func matchRecipeOptions(recipe *Recipe, optionTexts []string) recipeMatch {
	tokens := make([]string, 0, len(optionTexts))
	for _, text := range optionTexts {
		if normalized := NormalizeName(text); normalized != "" {
			tokens = append(tokens, normalized)
		}
	}
	paddedOptions := " " + strings.Join(tokens, " ") + " "

	result := recipeMatch{matched: []ChoiceOption{}, missing: []string{}, ambiguous: []string{}}
	for _, component := range recipe.Components {
		if component.Kind != "choice" {
			continue
		}
		var best *ChoiceOption
		bestLength := 0
		tie := false
		for i := range component.Options {
			option := component.Options[i]
			length := 0
			aliases := append([]string{option.Label}, option.MatchAliases...)
			for _, alias := range aliases {
				if candidate := matchAliasLength(alias, paddedOptions, tokens); candidate > length {
					length = candidate
				}
			}
			if length > bestLength {
				best = &component.Options[i]
				bestLength = length
				tie = false
			} else if length > 0 && length == bestLength && (best == nil || option.ID != best.ID) {
				tie = true
			}
		}

		switch {
		case tie:
			result.ambiguous = append(result.ambiguous, component.Label)
		case best != nil:
			result.matched = append(result.matched, *best)
		case component.Required:
			for i := range component.Options {
				if component.Options[i].Default {
					result.matched = append(result.matched, component.Options[i])
					break
				}
			}
			if !hasDefault(component.Options) {
				result.missing = append(result.missing, component.Label)
			}
		}
	}
	return result
}

func hasDefault(options []ChoiceOption) bool {
	for _, option := range options {
		if option.Default {
			return true
		}
	}
	return false
}

func matchAliasLength(alias, paddedOptions string, tokens []string) int {
	normalized := NormalizeName(alias)
	if normalized == "" {
		return 0
	}
	for _, token := range tokens {
		if token == normalized {
			return len(normalized)
		}
	}
	if strings.Contains(paddedOptions, " "+normalized+" ") {
		return len(normalized)
	}
	return 0
}
