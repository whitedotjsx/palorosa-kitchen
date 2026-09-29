import { defaultLabelParsingRules } from './labels/rules'
import { SCHEMA_VERSION } from './schema'
import type { Catalog, KitchenUnit, Product, Recipe } from './schema'

export function makeUnit (id: string, name: string, extra: Partial<KitchenUnit> = {}): KitchenUnit {
  return {
    id,
    name,
    category: 'other',
    measure: 'unit',
    aliases: [],
    isKitchen: true,
    active: true,
    ...extra,
  }
}

export function makeProduct (id: string, name: string, extra: Partial<Product> = {}): Product {
  return {
    id,
    name,
    aliases: [],
    category: 'breakfast',
    isKitchen: true,
    active: true,
    ...extra,
  }
}

export function makeRecipe (productId: string, components: Recipe['components']): Recipe {
  return { productId, components }
}

export function testCatalog (overrides: Partial<Catalog> = {}): Catalog {
  const units: KitchenUnit[] = [
    makeUnit('jugo-pequeno', 'Jugo de naranja semi natural pequeño', {
      category: 'drink',
      measure: 'bottle',
      aliases: ['Jugo de naranja', 'Jugo de naranja semi natural pequeño'],
    }),
    makeUnit('yogurt-pequeno', 'Yogurt pequeño', {
      category: 'drink',
      aliases: ['Yogurt pequeño', 'Yogur'],
    }),
    makeUnit('sandwich-sencillo', 'Sándwich sencillo', {
      category: 'main',
      aliases: ['Sándwich sencillo', 'Sándwich: Jamón y queso'],
    }),
    makeUnit('waffle-con-arequipe-y-fresas', 'Waffles con arequipe y fresas', {
      category: 'main',
      measure: 'portion',
      aliases: ['Waffle con arequipe y fresas', 'Waffle: Base de arequipe'],
    }),
    makeUnit('ensalada-de-frutas-grande', 'Ensalada de frutas grande', {
      category: 'fruit',
      measure: 'portion',
    }),
    makeUnit('nutella-15-gr', 'Nutella 15 gr', { category: 'dessert', isKitchen: false }),
    makeUnit('unidad-inactiva', 'Unidad inactiva', { active: false }),
  ]

  const products: Product[] = [
    makeProduct('box-hombre', 'Box Hombre', { aliases: ['Box Hombre'] }),
    makeProduct('parfait', 'Parfait', { category: 'add_on', aliases: ['Parfait'] }),
    makeProduct('globo-con-helio', 'Globo con helio', {
      category: 'add_on',
      isKitchen: false,
      aliases: ['Globo con helio'],
    }),
  ]

  const recipes: Recipe[] = [
    makeRecipe('box-hombre', [
      { kind: 'fixed', unitId: 'nutella-15-gr', quantity: 1 },
      {
        kind: 'choice',
        label: 'Jugo de naranja o yogurt',
        required: true,
        options: [
          {
            id: 'jugo-de-naranja',
            label: 'Jugo de naranja',
            matchAliases: ['Jugo de naranja'],
            components: [{ kind: 'fixed', unitId: 'jugo-pequeno', quantity: 1 }],
          },
          {
            id: 'yogurt-pequeno',
            label: 'Yogurt pequeño',
            matchAliases: ['Yogurt pequeño', 'Yogur'],
            components: [{ kind: 'fixed', unitId: 'yogurt-pequeno', quantity: 1 }],
          },
        ],
      },
      {
        kind: 'choice',
        label: 'Sándwich o waffle',
        required: true,
        options: [
          {
            id: 'sandwich-sencillo',
            label: 'Sándwich sencillo',
            matchAliases: ['Sándwich sencillo', 'Sándwich: Jamón y queso'],
            components: [{ kind: 'fixed', unitId: 'sandwich-sencillo', quantity: 1 }],
          },
          {
            id: 'waffle-con-arequipe-y-fresas',
            label: 'Waffles con arequipe y fresas',
            matchAliases: ['Waffles con arequipe y fresas', 'Waffle con arequipe y fresas'],
            components: [{ kind: 'fixed', unitId: 'waffle-con-arequipe-y-fresas', quantity: 1 }],
          },
        ],
      },
    ]),
    makeRecipe('parfait', [{ kind: 'fixed', unitId: 'ensalada-de-frutas-grande', quantity: 1 }]),
  ]

  return {
    schemaVersion: SCHEMA_VERSION,
    units,
    products,
    recipes,
    containers: [],
    labelParsing: defaultLabelParsingRules,
    ...overrides,
  }
}
