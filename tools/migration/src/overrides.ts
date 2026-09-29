import type {
  Container,
  FixedComponent,
  KitchenUnit,
  MeasureUnit,
  Product,
  Recipe,
  RecipeComponent,
  UnitCategory,
} from '@palorosa-kitchen/core'
import type { ReviewQueue } from './review'

/**
 * Owner decisions applied on top of the generated catalog. Versioned data.
 * See docs/migration.md.
 */
export interface CatalogOverrides {
  units?: Record<string, UnitOverride>
  unitMerges?: Record<string, string>
  unitRewrites?: Record<string, Record<string, UnitRewrite>>
  products?: Record<string, ProductOverride>
  containers?: Record<string, ContainerOverride>
  recipes?: Record<string, RecipeOverride>
  optionOverrides?: Record<string, OptionOverride>
  queue?: Record<string, QueueOverride>
}

export interface ContainerOverride {
  name?: string
  aliases?: string[]
  active?: boolean
}

export interface UnitOverride {
  name?: string
  description?: string
  note?: string
  aliases?: string[]
  active?: boolean
  isKitchen?: boolean
  category?: UnitCategory
  measure?: MeasureUnit
}

/** Per product reference rewrite: source unit id to target unit id and quantity. */
export interface UnitRewrite {
  unitId: string
  quantity?: number
}

export interface ProductOverride {
  name?: string
  description?: string
  aliases?: string[]
  active?: boolean
  isKitchen?: boolean
}

export interface RecipeOverride {
  components: RecipeComponent[]
}

export interface OptionOverride {
  quantity?: number
}

export interface QueueOverride {
  status?: 'resolved'
  note?: string
}

function mergeAliases (existing: string[], extra: string[]): string[] {
  return [...new Set([...existing, ...extra])]
}

export function applyUnitOverrides (units: KitchenUnit[], overrides: CatalogOverrides): void {
  const byId = new Map(units.map((unit) => [unit.id, unit]))
  for (const [id, override] of Object.entries(overrides.units ?? {})) {
    let unit = byId.get(id)
    if (!unit) {
      if (!override.name) continue
      unit = {
        id,
        name: override.name,
        category: override.category ?? 'other',
        measure: override.measure ?? 'unit',
        aliases: [],
        isKitchen: override.isKitchen ?? true,
        active: override.active ?? true,
      }
      units.push(unit)
      byId.set(id, unit)
    }
    if (override.name) unit.name = override.name
    if (override.description) unit.description = override.description
    if (override.note) unit.note = override.note
    if (override.aliases) unit.aliases = mergeAliases(unit.aliases, override.aliases)
    if (override.category) unit.category = override.category
    if (override.measure) unit.measure = override.measure
    if (typeof override.isKitchen === 'boolean') unit.isKitchen = override.isKitchen
    if (typeof override.active === 'boolean') unit.active = override.active
  }
}

export function applyProductOverrides (products: Product[], overrides: CatalogOverrides): void {
  for (const product of products) {
    const override = overrides.products?.[product.id]
    if (!override) continue
    if (override.name) product.name = override.name
    if (override.description) product.description = override.description
    if (override.aliases) product.aliases = mergeAliases(product.aliases, override.aliases)
    if (typeof override.active === 'boolean') product.active = override.active
    if (typeof override.isKitchen === 'boolean') product.isKitchen = override.isKitchen
  }
}

export function applyContainerOverrides (
  containers: Container[],
  overrides: CatalogOverrides
): void {
  const byId = new Map(containers.map((container) => [container.id, container]))
  for (const [id, override] of Object.entries(overrides.containers ?? {})) {
    let container = byId.get(id)
    if (!container) {
      if (!override.name) continue
      container = { id, name: override.name, aliases: [], active: override.active ?? true }
      containers.push(container)
      byId.set(id, container)
    }
    if (override.name) container.name = override.name
    if (override.aliases) container.aliases = mergeAliases(container.aliases, override.aliases)
    if (typeof override.active === 'boolean') container.active = override.active
  }
}

export function applyRecipeOverrides (recipes: Recipe[], overrides: CatalogOverrides): void {
  for (const [productId, override] of Object.entries(overrides.recipes ?? {})) {
    const existing = recipes.find((recipe) => recipe.productId === productId)
    if (existing) {
      existing.components = override.components
      continue
    }
    recipes.push({ productId, components: override.components })
  }
}

export function applyOptionOverrides (recipes: Recipe[], overrides: CatalogOverrides): void {
  const optionOverrides = overrides.optionOverrides
  if (!optionOverrides) return
  for (const recipe of recipes) {
    for (const component of recipe.components) {
      if (component.kind !== 'choice') continue
      for (const option of component.options) {
        const override = optionOverrides[option.id]
        if (!override || typeof override.quantity !== 'number') continue
        for (const fixed of option.components) fixed.quantity = override.quantity
      }
    }
  }
}

function rewriteUnitId (unitId: string, merges: Record<string, string>): string {
  return merges[unitId] ?? unitId
}

export function applyUnitMerges (recipes: Recipe[], overrides: CatalogOverrides): void {
  const merges = overrides.unitMerges ?? {}
  for (const recipe of recipes) {
    for (const component of recipe.components) {
      if (component.kind === 'fixed') {
        component.unitId = rewriteUnitId(component.unitId, merges)
        continue
      }
      for (const option of component.options) {
        for (const fixed of option.components) {
          fixed.unitId = rewriteUnitId(fixed.unitId, merges)
        }
      }
    }
  }
  consolidateFixedComponents(recipes)
}

/** A unit stated twice is not twice the food. Keep the highest quantity. */
function consolidateFixedComponents (recipes: Recipe[]): void {
  for (const recipe of recipes) {
    const byUnit = new Map<string, FixedComponent>()
    const consolidated: RecipeComponent[] = []
    for (const component of recipe.components) {
      if (component.kind !== 'fixed') {
        consolidated.push(component)
        continue
      }
      const existing = byUnit.get(component.unitId)
      if (existing) {
        existing.quantity = Math.max(existing.quantity, component.quantity)
        continue
      }
      byUnit.set(component.unitId, component)
      consolidated.push(component)
    }
    recipe.components = consolidated
  }
}

export function applyUnitRewrites (recipes: Recipe[], overrides: CatalogOverrides): void {
  const rewritesByProduct = overrides.unitRewrites
  if (!rewritesByProduct) return
  for (const recipe of recipes) {
    const rewrites = rewritesByProduct[recipe.productId]
    if (!rewrites) continue
    const rewriteFixed = (fixed: { unitId: string; quantity: number }): void => {
      const rewrite = rewrites[fixed.unitId]
      if (!rewrite) return
      fixed.unitId = rewrite.unitId
      if (typeof rewrite.quantity === 'number') fixed.quantity = rewrite.quantity
    }
    for (const component of recipe.components) {
      if (component.kind === 'fixed') {
        rewriteFixed(component)
        continue
      }
      for (const option of component.options) {
        for (const fixed of option.components) rewriteFixed(fixed)
      }
    }
  }
  consolidateFixedComponents(recipes)
}

/** A recipe defined in overrides closes its product_without_recipe queue item. */
export function resolveRecipeQueueItems (queue: ReviewQueue, overrides: CatalogOverrides): void {
  const recipeOverrides = overrides.recipes ?? {}
  for (const item of queue.items) {
    if (item.type !== 'product_without_recipe') continue
    if (!item.refs.some((ref) => recipeOverrides[ref])) continue
    item.status = 'resolved'
    item.resolution = 'Recipe defined in catalog/overrides.json.'
  }
}

export function applyQueueOverrides (queue: ReviewQueue, overrides: CatalogOverrides): void {
  for (const item of queue.items) {
    const override = overrides.queue?.[item.id]
    if (!override) continue
    if (override.status === 'resolved') item.status = 'resolved'
    if (override.note) item.resolution = override.note
  }
}
