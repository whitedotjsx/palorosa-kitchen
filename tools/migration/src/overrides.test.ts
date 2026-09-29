import { describe, expect, it } from 'vitest'
import type { KitchenUnit, Product, Recipe } from '@palorosa-kitchen/core'
import {
  applyContainerOverrides,
  applyOptionOverrides,
  applyProductOverrides,
  applyQueueOverrides,
  applyRecipeOverrides,
  applyUnitMerges,
  applyUnitOverrides,
  applyUnitRewrites,
  type CatalogOverrides,
} from './overrides'
import { ReviewQueue } from './review'

function unit (id: string, name: string): KitchenUnit {
  return {
    id,
    name,
    category: 'main',
    measure: 'unit',
    aliases: [name],
    isKitchen: true,
    active: true,
  }
}

describe('catalog overrides', () => {
  it('applies unit descriptions, aliases and active state', () => {
    const units = [unit('sandwich-sencillo', 'Sándwich sencillo')]
    const overrides: CatalogOverrides = {
      units: {
        'sandwich-sencillo': {
          description: 'Pan cuadrado artesanal.',
          aliases: ['Sándwich: Jamón y queso'],
          isKitchen: false,
        },
      },
    }
    applyUnitOverrides(units, overrides)
    expect(units[0]?.description).toBe('Pan cuadrado artesanal.')
    expect(units[0]?.aliases).toContain('Sándwich: Jamón y queso')
    expect(units[0]?.isKitchen).toBe(false)
  })

  it('merges units in fixed components and options', () => {
    const recipes: Recipe[] = [
      {
        productId: 'parejas',
        components: [
          { kind: 'fixed', unitId: 'waffle-source', quantity: 4 },
          {
            kind: 'choice',
            label: 'A o B',
            required: true,
            options: [
              {
                id: 'a',
                label: 'A',
                matchAliases: [],
                components: [{ kind: 'fixed', unitId: 'waffle-source', quantity: 2 }],
              },
            ],
          },
        ],
      },
    ]
    applyUnitMerges(recipes, { unitMerges: { 'waffle-source': 'waffle-target' } })
    expect(recipes[0]?.components[0]).toEqual({
      kind: 'fixed',
      unitId: 'waffle-target',
      quantity: 4,
    })
    const group = recipes[0]?.components[1]
    expect(group?.kind === 'choice' ? group.options[0]?.components[0]?.unitId : null).toBe(
      'waffle-target'
    )
  })

  it('deduplicates repeated fixed components keeping the highest quantity', () => {
    const recipes: Recipe[] = [
      {
        productId: 'luxury',
        components: [
          { kind: 'fixed', unitId: 'sandwich', quantity: 1 },
          { kind: 'fixed', unitId: 'sandwich', quantity: 1 },
        ],
      },
    ]
    applyUnitMerges(recipes, {})
    expect(recipes[0]?.components).toHaveLength(1)
    expect(recipes[0]?.components[0]).toEqual({
      kind: 'fixed',
      unitId: 'sandwich',
      quantity: 1,
    })
  })

  it('sets option quantities and resolves queue items', () => {
    const recipes: Recipe[] = [
      {
        productId: 'box-mujer',
        components: [
          {
            kind: 'choice',
            label: 'Waffle',
            required: true,
            options: [
              {
                id: 'waffle',
                label: 'Waffle',
                matchAliases: [],
                components: [{ kind: 'fixed', unitId: 'waffle', quantity: 1 }],
              },
            ],
          },
        ],
      },
    ]
    applyOptionOverrides(recipes, { optionOverrides: { waffle: { quantity: 2 } } })
    const group = recipes[0]?.components[0]
    expect(group?.kind === 'choice' ? group.options[0]?.components[0]?.quantity : null).toBe(2)

    const queue = new ReviewQueue()
    queue.add({ type: 'ambiguous_unit', severity: 'decision', title: 'X', detail: 'd', refs: [] })
    const item = queue.items[0]
    applyQueueOverrides(queue, {
      queue: { [item!.id]: { status: 'resolved', note: 'Defined by owner.' } },
    })
    expect(item?.status).toBe('resolved')
    expect(item?.resolution).toBe('Defined by owner.')
  })

  it('creates units from overrides when the id does not exist', () => {
    const units: KitchenUnit[] = []
    applyUnitOverrides(units, {
      units: {
        'porcion-grande': {
          name: 'Waffles con arequipe y fresas porción grande',
          category: 'dessert',
          description: 'Cuatro waffles.',
        },
      },
    })
    expect(units).toHaveLength(1)
    expect(units[0]?.name).toBe('Waffles con arequipe y fresas porción grande')
    expect(units[0]?.category).toBe('dessert')
    expect(units[0]?.active).toBe(true)
  })

  it('rewrites unit references per product and consolidates afterwards', () => {
    const recipes: Recipe[] = [
      {
        productId: 'parejas',
        components: [
          { kind: 'fixed', unitId: 'waffle-base', quantity: 4 },
          { kind: 'fixed', unitId: 'porcion-grande', quantity: 1 },
        ],
      },
      {
        productId: 'otro',
        components: [{ kind: 'fixed', unitId: 'waffle-base', quantity: 2 }],
      },
    ]
    applyUnitRewrites(recipes, {
      unitRewrites: {
        parejas: { 'waffle-base': { unitId: 'porcion-grande', quantity: 1 } },
      },
    })
    expect(recipes[0]?.components).toHaveLength(1)
    expect(recipes[0]?.components[0]).toEqual({
      kind: 'fixed',
      unitId: 'porcion-grande',
      quantity: 1,
    })
    expect(recipes[1]?.components[0]).toEqual({
      kind: 'fixed',
      unitId: 'waffle-base',
      quantity: 2,
    })
  })

  it('creates and updates containers', () => {
    const containers = [{ id: 'caja', name: 'Caja', aliases: ['Caja'], active: true }]
    applyContainerOverrides(containers, {
      containers: {
        caja: { name: 'Caja de prueba' },
        nueva: { name: 'Canasta nueva' },
      },
    })
    expect(containers[0]?.name).toBe('Caja de prueba')
    expect(containers).toHaveLength(2)
    expect(containers[1]?.id).toBe('nueva')
  })

  it('replaces recipe components and product overrides', () => {
    const recipes: Recipe[] = [{ productId: 'cristal', components: [] }]
    applyRecipeOverrides(recipes, {
      recipes: {
        cristal: { components: [{ kind: 'fixed', unitId: 'cristal-zone-dulce', quantity: 1 }] },
      },
    })
    expect(recipes[0]?.components).toHaveLength(1)

    applyRecipeOverrides(recipes, {
      recipes: {
        'add-on': { components: [{ kind: 'fixed', unitId: 'tabla', quantity: 1 }] },
      },
    })
    expect(recipes).toHaveLength(2)
    expect(recipes[1]?.productId).toBe('add-on')

    const products: Product[] = [
      {
        id: 'bolso-fitness',
        name: 'Bolso Fitness',
        aliases: [],
        category: 'breakfast',
        isKitchen: true,
        active: true,
      },
    ]
    applyProductOverrides(products, {
      products: {
        'bolso-fitness': {
          active: false,
          description: 'Agotado por el momento.',
          isKitchen: false,
        },
      },
    })
    expect(products[0]?.active).toBe(false)
    expect(products[0]?.description).toBe('Agotado por el momento.')
    expect(products[0]?.isKitchen).toBe(false)
  })
})
