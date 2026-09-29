import { normalizeName } from './normalize'
import { SCHEMA_VERSION } from './schema'
import type { Catalog, Container, KitchenUnit, Product, Recipe } from './schema'

export type CatalogIssueKind =
  | 'schema_version'
  | 'duplicate_id'
  | 'missing_unit'
  | 'missing_product'
  | 'missing_container'
  | 'ambiguous_text'
  | 'product_without_recipe'

export interface CatalogIssue {
  kind: CatalogIssueKind
  id: string
  message: string
}

export interface CatalogIndex {
  catalog: Catalog
  productsById: ReadonlyMap<string, Product>
  unitsById: ReadonlyMap<string, KitchenUnit>
  recipesByProductId: ReadonlyMap<string, Recipe>
  containersById: ReadonlyMap<string, Container>
  productsByText: ReadonlyMap<string, Product[]>
  unitsByText: ReadonlyMap<string, KitchenUnit[]>
}

function pushText<T> (map: Map<string, T[]>, text: string, value: T): void {
  const key = normalizeName(text)
  if (!key) return
  const list = map.get(key)
  if (!list) {
    map.set(key, [value])
    return
  }
  if (!list.includes(value)) list.push(value)
}

export function indexCatalog (catalog: Catalog): CatalogIndex {
  const productsById = new Map<string, Product>()
  const unitsById = new Map<string, KitchenUnit>()
  const recipesByProductId = new Map<string, Recipe>()
  const containersById = new Map<string, Container>()
  const productsByText = new Map<string, Product[]>()
  const unitsByText = new Map<string, KitchenUnit[]>()

  for (const unit of catalog.units) {
    unitsById.set(unit.id, unit)
    pushText(unitsByText, unit.name, unit)
    for (const alias of unit.aliases) pushText(unitsByText, alias, unit)
  }
  for (const product of catalog.products) {
    productsById.set(product.id, product)
    pushText(productsByText, product.name, product)
    for (const alias of product.aliases) pushText(productsByText, alias, product)
  }
  for (const recipe of catalog.recipes) recipesByProductId.set(recipe.productId, recipe)
  for (const container of catalog.containers) containersById.set(container.id, container)

  return {
    catalog,
    productsById,
    unitsById,
    recipesByProductId,
    containersById,
    productsByText,
    unitsByText,
  }
}

export function lookupProducts (index: CatalogIndex, text: string): Product[] {
  return index.productsByText.get(normalizeName(text)) ?? []
}

export function lookupUnits (index: CatalogIndex, text: string): KitchenUnit[] {
  return index.unitsByText.get(normalizeName(text)) ?? []
}

function duplicatedIds (ids: string[]): string[] {
  const seen = new Set<string>()
  const duplicates = new Set<string>()
  for (const id of ids) {
    if (seen.has(id)) duplicates.add(id)
    else seen.add(id)
  }
  return [...duplicates]
}

function collectAmbiguousText<T extends { id: string, name: string }> (
  map: ReadonlyMap<string, T[]>,
  kind: 'product' | 'unit',
  issues: CatalogIssue[]
): void {
  for (const [text, values] of map) {
    const ids = new Set(values.map(value => value.id))
    if (ids.size > 1) {
      issues.push({
        kind: 'ambiguous_text',
        id: text,
        message: `${kind} text "${text}" maps to ${ids.size} different ids: ${[...ids].join(', ')}`,
      })
    }
  }
}

export function validateCatalog (catalog: Catalog): CatalogIssue[] {
  const issues: CatalogIssue[] = []

  if (catalog.schemaVersion !== SCHEMA_VERSION) {
    issues.push({
      kind: 'schema_version',
      id: String(catalog.schemaVersion),
      message: `Expected schema version ${SCHEMA_VERSION}, got ${catalog.schemaVersion}`,
    })
  }

  for (const id of duplicatedIds(catalog.units.map(unit => unit.id))) {
    issues.push({ kind: 'duplicate_id', id, message: `Duplicate kitchen unit id: ${id}` })
  }
  for (const id of duplicatedIds(catalog.products.map(product => product.id))) {
    issues.push({ kind: 'duplicate_id', id, message: `Duplicate product id: ${id}` })
  }
  for (const id of duplicatedIds(catalog.containers.map(container => container.id))) {
    issues.push({ kind: 'duplicate_id', id, message: `Duplicate container id: ${id}` })
  }
  for (const id of duplicatedIds(catalog.recipes.map(recipe => recipe.productId))) {
    issues.push({ kind: 'duplicate_id', id, message: `Duplicate recipe for product: ${id}` })
  }

  const unitIds = new Set(catalog.units.map(unit => unit.id))
  const productIds = new Set(catalog.products.map(product => product.id))
  const containerIds = new Set(catalog.containers.map(container => container.id))

  const checkUnit = (unitId: string, owner: string): void => {
    if (!unitIds.has(unitId)) {
      issues.push({
        kind: 'missing_unit',
        id: unitId,
        message: `Unit "${unitId}" referenced by ${owner} does not exist`,
      })
    }
  }

  for (const recipe of catalog.recipes) {
    if (!productIds.has(recipe.productId)) {
      issues.push({
        kind: 'missing_product',
        id: recipe.productId,
        message: `Recipe references unknown product "${recipe.productId}"`,
      })
    }
    for (const component of recipe.components) {
      if (component.kind === 'fixed') {
        checkUnit(component.unitId, `recipe ${recipe.productId}`)
        continue
      }
      for (const option of component.options) {
        for (const detail of option.components) {
          checkUnit(detail.unitId, `option ${recipe.productId}/${option.id}`)
        }
      }
    }
  }

  for (const product of catalog.products) {
    if (product.containerId !== undefined && !containerIds.has(product.containerId)) {
      issues.push({
        kind: 'missing_container',
        id: product.containerId,
        message: `Product "${product.id}" references unknown container "${product.containerId}"`,
      })
    }
    const needsRecipe =
      product.category === 'breakfast' && product.isKitchen && product.active
    if (needsRecipe && !catalog.recipes.some(recipe => recipe.productId === product.id)) {
      issues.push({
        kind: 'product_without_recipe',
        id: product.id,
        message: `Active kitchen product "${product.id}" has no recipe`,
      })
    }
  }

  const index = indexCatalog(catalog)
  collectAmbiguousText(index.productsByText, 'product', issues)
  collectAmbiguousText(index.unitsByText, 'unit', issues)

  return issues
}
