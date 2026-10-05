import type { Catalog, Recipe } from '@palorosa-kitchen/core'

export interface OverridesPatch {
  units?: Record<string, Record<string, unknown>>
  unitMerges?: Record<string, string>
  products?: Record<string, Record<string, unknown>>
  containers?: Record<string, Record<string, unknown>>
  recipes?: Record<string, { components: Recipe['components'] }>
}

function changedFields<T extends object> (
  original: T,
  edited: T,
  fields: readonly (keyof T)[]
): Record<string, unknown> {
  const changes: Record<string, unknown> = {}
  for (const field of fields) {
    const before = JSON.stringify(original[field] ?? null)
    const after = JSON.stringify(edited[field] ?? null)
    if (before !== after) changes[field as string] = edited[field]
  }
  return changes
}

function definedFields<T extends object> (item: T, fields: readonly (keyof T)[]): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const field of fields) {
    const value = item[field]
    if (value !== undefined) result[field as string] = value
  }
  return result
}

const UNIT_FIELDS = [
  'name',
  'description',
  'note',
  'aliases',
  'isKitchen',
  'active',
  'category',
  'measure',
] as const
const PRODUCT_FIELDS = ['name', 'description', 'aliases', 'active', 'isKitchen'] as const
const CONTAINER_FIELDS = ['name', 'aliases', 'active'] as const

export function computePatch (
  seed: Catalog,
  edited: Catalog,
  merges: Record<string, string>
): OverridesPatch {
  const patch: OverridesPatch = {}
  if (Object.keys(merges).length > 0) patch.unitMerges = { ...merges }

  const seedUnits = new Map(seed.units.map((unit) => [unit.id, unit]))
  for (const unit of edited.units) {
    const original = seedUnits.get(unit.id)
    const changes = original
      ? changedFields(original, unit, UNIT_FIELDS)
      : definedFields(unit, UNIT_FIELDS)
    if (Object.keys(changes).length > 0) {
      patch.units ??= {}
      patch.units[unit.id] = changes
    }
  }
  for (const original of seed.units) {
    if (edited.units.some((unit) => unit.id === original.id)) continue
    patch.units ??= {}
    patch.units[original.id] = { ...(patch.units[original.id] ?? {}), active: false }
  }

  const seedProducts = new Map(seed.products.map((product) => [product.id, product]))
  for (const product of edited.products) {
    const original = seedProducts.get(product.id)
    if (!original) continue
    const changes = changedFields(original, product, PRODUCT_FIELDS)
    if (Object.keys(changes).length > 0) {
      patch.products ??= {}
      patch.products[product.id] = changes
    }
  }

  const seedContainers = new Map(seed.containers.map((container) => [container.id, container]))
  for (const container of edited.containers) {
    const original = seedContainers.get(container.id)
    const changes = original
      ? changedFields(original, container, CONTAINER_FIELDS)
      : definedFields(container, CONTAINER_FIELDS)
    if (Object.keys(changes).length > 0) {
      patch.containers ??= {}
      patch.containers[container.id] = changes
    }
  }

  for (const recipe of edited.recipes) {
    const original = seed.recipes.find((item) => item.productId === recipe.productId)
    if (original && JSON.stringify(original.components) === JSON.stringify(recipe.components)) {
      continue
    }
    patch.recipes ??= {}
    patch.recipes[recipe.productId] = {
      components: JSON.parse(JSON.stringify(recipe.components)) as Recipe['components'],
    }
  }

  return patch
}
