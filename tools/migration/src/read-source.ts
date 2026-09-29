import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import type { SourceBreakfast, SourceData, SourceStoreProduct } from './source-types'

export const sourceDir = fileURLToPath(new URL('../source/', import.meta.url))

type SourceModule = Record<string, unknown>

async function loadModule (file: string): Promise<SourceModule> {
  return (await import(pathToFileURL(join(sourceDir, file)).href)) as SourceModule
}

function stringEnum (mod: SourceModule, name: string): Record<string, string> {
  const value = mod[name]
  if (typeof value !== 'object' || value === null) {
    throw new Error(`Missing enum ${name} in vendored source`)
  }
  const result: Record<string, string> = {}
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry === 'string') result[key] = entry
  }
  return result
}

function objectValue<T> (mod: SourceModule, name: string): Record<string, T> {
  const value = mod[name]
  if (typeof value !== 'object' || value === null) {
    throw new Error(`Missing object ${name} in vendored source`)
  }
  return value as Record<string, T>
}

export async function readSource (): Promise<SourceData> {
  const [foods, decorations, containers, breakfasts, extras, products] = await Promise.all([
    loadModule('foods.ts'),
    loadModule('decorations.ts'),
    loadModule('containers.ts'),
    loadModule('breakfasts.ts'),
    loadModule('extras.ts'),
    loadModule('products.ts'),
  ])

  return {
    kitchenFoods: stringEnum(foods, 'KitchenFoods'),
    decorations: stringEnum(decorations, 'Decorations'),
    containers: stringEnum(containers, 'Containers'),
    breakfasts: objectValue<SourceBreakfast>(breakfasts, 'BREAKFASTS'),
    additions: stringEnum(extras, 'Additions'),
    additionPrices: objectValue<number>(extras, 'ADDITION_PRICES'),
    storeProducts: objectValue<SourceStoreProduct>(products, 'ALL_PRODUCTS'),
    productCategories: stringEnum(products, 'ProductCategory'),
    cajaSaludDrinks: stringEnum(products, 'CajaSaludDrink'),
  }
}
