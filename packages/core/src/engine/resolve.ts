import { lookupProducts } from '../catalog'
import type { CatalogIndex } from '../catalog'
import type { ParsedOrderLine } from '../labels/parse'
import { normalizeName } from '../normalize'
import type { ChoiceOption, IgnoredReason, Product, Recipe, UnresolvedReason } from '../schema'

export interface UnitContribution {
  unitId: string
  quantity: number
}

export interface ResolvedLine {
  line: ParsedOrderLine
  status: 'resolved' | 'unresolved' | 'ignored'
  productId?: string
  productName?: string
  contributions: UnitContribution[]
  matchedOptions: string[]
  unresolvedReason?: UnresolvedReason
  unresolvedDetail?: string
  ignoredReason?: IgnoredReason
}

export interface ResolveOptions {
  /** Normalized product text to product id, learned by the operator. */
  productMappings?: Record<string, string>
}

interface RecipeMatch {
  matched: ChoiceOption[]
  missing: string[]
  ambiguous: string[]
}

function matchAliasLength (alias: string, paddedOptions: string, tokens: string[]): number {
  const normalized = normalizeName(alias)
  if (!normalized) return 0
  if (tokens.includes(normalized)) return normalized.length
  if (paddedOptions.includes(` ${normalized} `)) return normalized.length
  return 0
}

function matchRecipeOptions (recipe: Recipe, optionTexts: string[]): RecipeMatch {
  const tokens = optionTexts.map(text => normalizeName(text)).filter(Boolean)
  const paddedOptions = ` ${tokens.join(' ')} `
  const matched: ChoiceOption[] = []
  const missing: string[] = []
  const ambiguous: string[] = []

  for (const component of recipe.components) {
    if (component.kind !== 'choice') continue

    let best: ChoiceOption | null = null
    let bestLength = 0
    let tie = false
    for (const option of component.options) {
      let length = 0
      for (const alias of [option.label, ...option.matchAliases]) {
        length = Math.max(length, matchAliasLength(alias, paddedOptions, tokens))
      }
      if (length > bestLength) {
        best = option
        bestLength = length
        tie = false
      } else if (length > 0 && length === bestLength && option.id !== best?.id) {
        tie = true
      }
    }

    if (tie) ambiguous.push(component.label)
    else if (best) matched.push(best)
    else if (component.required) {
      // The store export marks the chosen option with a column; an empty column
      // means the other option, flagged as the default.
      const fallback = component.options.find(option => option.default)
      if (fallback) matched.push(fallback)
      else missing.push(component.label)
    }
  }

  return { matched, missing, ambiguous }
}

function unresolved (
  line: ParsedOrderLine,
  reason: UnresolvedReason,
  product?: Product,
  detail?: string,
  contributions: UnitContribution[] = []
): ResolvedLine {
  return {
    line,
    status: 'unresolved',
    contributions,
    matchedOptions: [],
    unresolvedReason: reason,
    ...(detail ? { unresolvedDetail: detail } : {}),
    ...(product ? { productId: product.id, productName: product.name } : {}),
  }
}

export function resolveLine (
  line: ParsedOrderLine,
  index: CatalogIndex,
  options: ResolveOptions = {}
): ResolvedLine {
  const mappedId = options.productMappings?.[normalizeName(line.productText)]
  let product = mappedId ? index.productsById.get(mappedId) : undefined

  if (!product) {
    const candidates = lookupProducts(index, line.productText)
    // Duplicated store products: an inactive twin never wins over an active
    // one, and an inactive product alone still resolves (history).
    const activeCandidates = candidates.filter(candidate => candidate.active)
    const chosen = activeCandidates.length > 0 ? activeCandidates : candidates
    if (chosen.length > 1) {
      return unresolved(line, 'ambiguous_match', undefined, chosen.map(candidate => candidate.name).join(', '))
    }
    product = chosen[0]
  }
  if (!product) return unresolved(line, 'unknown_product')

  if (!product.isKitchen) {
    return {
      line,
      status: 'ignored',
      productId: product.id,
      productName: product.name,
      contributions: [],
      matchedOptions: [],
      ignoredReason: 'not_kitchen',
    }
  }

  const recipe = index.recipesByProductId.get(product.id)
  if (!recipe) return unresolved(line, 'missing_recipe', product)

  const matching = matchRecipeOptions(recipe, line.options)

  // Known components still count even when a required choice is missing: the
  // kitchen prepares what is known and the operator resolves the gap.
  const contributions: UnitContribution[] = []
  for (const component of recipe.components) {
    if (component.kind === 'fixed') {
      contributions.push({ unitId: component.unitId, quantity: component.quantity * line.quantity })
    }
  }
  for (const option of matching.matched) {
    for (const detail of option.components) {
      contributions.push({ unitId: detail.unitId, quantity: detail.quantity * line.quantity })
    }
  }

  if (matching.ambiguous.length > 0) {
    return unresolved(line, 'ambiguous_match', product, matching.ambiguous.join(', '), contributions)
  }
  if (matching.missing.length > 0) {
    return unresolved(line, 'missing_choice', product, matching.missing.join(', '), contributions)
  }

  const missingUnit = contributions.find(contribution => !index.unitsById.has(contribution.unitId))
  if (missingUnit) {
    return unresolved(line, 'missing_unit', product, missingUnit.unitId, contributions)
  }

  return {
    line,
    status: 'resolved',
    productId: product.id,
    productName: product.name,
    contributions,
    matchedOptions: matching.matched.map(option => option.label),
  }
}

export function resolveLines (
  lines: ParsedOrderLine[],
  index: CatalogIndex,
  options: ResolveOptions = {}
): ResolvedLine[] {
  return lines.map(line => resolveLine(line, index, options))
}
