import {
  slugify,
  type ChoiceGroup,
  type ChoiceOption,
  type FixedComponent,
  type KitchenUnit,
  type Product,
  type Recipe,
  type RecipeComponent,
} from '@palorosa-kitchen/core'
import type { ResolvedUnit } from './build-units'
import { findExactName, findByLooseName } from './match'
import { makeReviewId, type ReviewQueue } from './review'
import type { SourceBreakfast, SourceSelection } from './source-types'
import type { WpSnapshot } from './wp-types'

export type ResolveFn = (rawName: string) => ResolvedUnit | null

export interface UnmappedDecoration {
  name: string
  breakfasts: string[]
}

export interface DuplicateChoice {
  breakfastId: string
  breakfastName: string
  groups: string[]
}

export interface BuildRecipesInput {
  breakfasts: Record<string, SourceBreakfast>
  products: Product[]
  units: KitchenUnit[]
  resolve: ResolveFn
  queue: ReviewQueue
  wpSnapshot: WpSnapshot | null
}

export interface BuildRecipesResult {
  recipes: Recipe[]
  unmappedDecorations: UnmappedDecoration[]
  duplicateChoices: DuplicateChoice[]
  addonsWithRecipe: number
  productsWithoutRecipe: { productId: string; name: string; category: string }[]
}

const PERSONALIZATION_PATTERN =
  /rosado|dorado|negro|azul|blanco|rojo|plateado|verde|palo rosa|oro rosa|beige|morado|transparente/i

function toFixed (unit: ResolvedUnit): FixedComponent {
  return { kind: 'fixed', unitId: unit.unit.id, quantity: unit.quantity }
}

function toOption (unit: ResolvedUnit, rawName: string): ChoiceOption {
  return {
    id: slugify(rawName),
    label: rawName,
    matchAliases: [...new Set([rawName, ...unit.unit.aliases])],
    components: [toFixed(unit)],
  }
}

interface Converted {
  components: RecipeComponent[]
  choiceGroups: ChoiceGroup[]
}

function convertSelection (
  selection: SourceSelection<string>,
  resolve: ResolveFn,
  onUnmapped: (rawName: string) => void
): Converted {
  switch (selection.type) {
    case 'none':
      return { components: [], choiceGroups: [] }
    case 'single': {
      const unit = resolve(selection.value)
      if (!unit) {
        onUnmapped(selection.value)
        return { components: [], choiceGroups: [] }
      }
      return { components: [toFixed(unit)], choiceGroups: [] }
    }
    case 'multiple': {
      const components: RecipeComponent[] = []
      for (const value of selection.values) {
        const unit = resolve(value)
        if (unit) components.push(toFixed(unit))
        else onUnmapped(value)
      }
      return { components, choiceGroups: [] }
    }
    case 'optional':
    case 'choose_between': {
      const options: ChoiceOption[] = []
      for (const value of selection.values) {
        const unit = resolve(value)
        if (unit) options.push(toOption(unit, value))
        else onUnmapped(value)
      }
      if (options.length === 0) return { components: [], choiceGroups: [] }
      const group: ChoiceGroup = {
        kind: 'choice',
        label: options.map((option) => option.label).join(' o '),
        required: selection.type === 'choose_between',
        options,
      }
      return { components: [], choiceGroups: [group] }
    }
    case 'choose_between_multiple': {
      const components: RecipeComponent[] = []
      const choiceGroups: ChoiceGroup[] = []
      for (const inner of selection.values) {
        const converted = convertSelection(inner, resolve, onUnmapped)
        components.push(...converted.components)
        choiceGroups.push(...converted.choiceGroups)
      }
      return { components, choiceGroups }
    }
  }
}

function unitIdsOf (group: ChoiceGroup): Set<string> {
  const ids = new Set<string>()
  for (const option of group.options) {
    for (const component of option.components) ids.add(component.unitId)
  }
  return ids
}

function findOptionForVariation (name: string, groups: ChoiceGroup[]): ChoiceOption | null {
  const entries = groups.flatMap((group) =>
    group.options.flatMap((option) =>
      (option.matchAliases.length > 0 ? option.matchAliases : [option.label]).map((alias) => ({
        alias,
        option,
      }))
    )
  )
  const match =
    findExactName(name, entries, (entry) => entry.alias) ??
    findByLooseName(name, entries, (entry) => entry.alias)
  return match?.option ?? null
}

export function buildRecipes (input: BuildRecipesInput): BuildRecipesResult {
  const recipes: Recipe[] = []
  const unmapped = new Map<string, Set<string>>()
  const duplicateChoices: DuplicateChoice[] = []

  for (const breakfast of Object.values(input.breakfasts)) {
    const product =
      findExactName(breakfast.name, input.products, (item) => item.name) ??
      findByLooseName(breakfast.name, input.products, (item) => item.name)
    const productId = product?.id ?? slugify(breakfast.name)
    const components: RecipeComponent[] = []
    const choiceGroups: ChoiceGroup[] = []
    const onUnmapped = (rawName: string): void => {
      const entry = unmapped.get(rawName) ?? new Set<string>()
      entry.add(breakfast.name)
      unmapped.set(rawName, entry)
    }

    for (const selection of [...breakfast.foodItems, ...breakfast.decorations]) {
      const converted = convertSelection(selection, input.resolve, onUnmapped)
      components.push(...converted.components)
      choiceGroups.push(...converted.choiceGroups)
    }

    if (product && input.wpSnapshot && product.wpProductId) {
      const variationNames = [
        ...new Set(
          input.wpSnapshot.variations
            .filter(
              (variation) =>
                variation.parent_id === product.wpProductId &&
                variation.status === 'publish' &&
                variation.name.trim()
            )
            .map((variation) => variation.name.trim())
        ),
      ].filter((name) => !PERSONALIZATION_PATTERN.test(name))

      for (const name of variationNames) {
        const option = findOptionForVariation(name, choiceGroups)
        if (option) {
          if (!option.matchAliases.includes(name)) option.matchAliases.push(name)
          continue
        }
        input.queue.add({
          type: 'variation_unmapped',
          severity: 'info',
          title: `${product.name}: ${name}`,
          detail: 'WooCommerce variation without a matching choice option.',
          refs: [productId],
        })
      }

      if (choiceGroups.length === 0 && variationNames.length >= 2) {
        input.queue.add({
          type: 'variation_suggests_choice',
          severity: 'decision',
          title: product.name,
          detail: `WooCommerce variations suggest a choice group: ${variationNames.join(', ')}.`,
          proposal: 'create-choice-group',
          refs: [productId],
        })
      }
    }

    for (let i = 0; i < choiceGroups.length; i += 1) {
      for (let j = i + 1; j < choiceGroups.length; j += 1) {
        const first = choiceGroups[i]
        const second = choiceGroups[j]
        if (!first || !second) continue
        const firstIds = unitIdsOf(first)
        const intersects = [...unitIdsOf(second)].some((id) => firstIds.has(id))
        if (intersects) {
          duplicateChoices.push({
            breakfastId: productId,
            breakfastName: breakfast.name,
            groups: [first.label, second.label],
          })
          input.queue.add({
            id: makeReviewId('duplicate_choice', `${breakfast.name} ${first.label}`),
            type: 'duplicate_choice',
            severity: 'decision',
            title: `${breakfast.name}: ${first.label}`,
            detail: `Two choice groups share units: "${first.label}" and "${second.label}". Merge them into one group if the customer picks once per product.`,
            refs: [productId],
          })
        }
      }
    }

    recipes.push({ productId, components: [...components, ...choiceGroups] })
  }

  let addonsWithRecipe = 0
  for (const product of input.products.filter((candidate) => candidate.category === 'add_on')) {
    const direct = input.resolve(product.name)
    const unit = direct?.unit ?? findByLooseName(product.name, input.units, (item) => item.name)
    if (!unit) continue
    recipes.push({
      productId: product.id,
      components: [{ kind: 'fixed', unitId: unit.id, quantity: 1 }],
    })
    addonsWithRecipe += 1
  }

  const productsWithoutRecipe: { productId: string; name: string; category: string }[] = []
  const recipeIds = new Set(recipes.map((recipe) => recipe.productId))
  for (const product of input.products) {
    if (!product.isKitchen || !product.active || recipeIds.has(product.id)) continue
    productsWithoutRecipe.push({
      productId: product.id,
      name: product.name,
      category: product.category,
    })
    input.queue.add({
      type: 'product_without_recipe',
      severity: 'decision',
      title: product.name,
      detail:
        product.category === 'breakfast'
          ? 'Breakfast without a recipe. Define its components.'
          : 'Kitchen product without a matching unit. Map it to a unit or mark it as not kitchen.',
      proposal: product.category === 'breakfast' ? 'define-recipe' : 'map-to-unit',
      refs: [product.id],
    })
  }

  const unmappedDecorations: UnmappedDecoration[] = [...unmapped.entries()]
    .map(([name, breakfasts]) => ({ name, breakfasts: [...breakfasts].sort() }))
    .sort((a, b) => a.name.localeCompare(b.name))

  for (const decoration of unmappedDecorations) {
    input.queue.add({
      type: 'unmapped_decoration',
      severity: 'info',
      title: decoration.name,
      detail: `Presentation decoration, not part of the kitchen list. Referenced by: ${decoration.breakfasts.join(', ')}.`,
      refs: decoration.breakfasts,
    })
  }

  return {
    recipes,
    unmappedDecorations,
    duplicateChoices,
    addonsWithRecipe,
    productsWithoutRecipe,
  }
}
