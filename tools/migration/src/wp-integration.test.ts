import { describe, expect, it } from 'vitest'
import { buildProducts } from './build-products'
import { buildRecipes } from './build-recipes'
import { ReviewQueue } from './review'
import type { SourceBreakfast } from './source-types'
import type { WpSnapshot } from './wp-types'
import type { KitchenUnit, Product, UnitCategory } from '@palorosa-kitchen/core'

function breakfast (name: string): SourceBreakfast {
  return {
    id: name.toUpperCase().replace(/ /g, '_'),
    name,
    description: '',
    decorations: [],
    foodItems: [],
    container: 'Caja de prueba',
  }
}

const snapshot: WpSnapshot = {
  fetchedAt: '2026-09-26T00:00:00.000Z',
  categories: [],
  products: [
    {
      id: 1,
      name: 'Box Mujer',
      sku: 'BMJ',
      type: 'variable',
      status: 'publish',
      price: '85000',
      parent_id: 0,
      categories: [{ id: 10, name: 'Desayunos' }],
      short_description: '<p>Con <b>jugo</b></p>',
      description: '',
    },
    {
      id: 2,
      name: 'Globo con helio',
      sku: '',
      type: 'simple',
      status: 'publish',
      price: '10000',
      parent_id: 0,
      categories: [{ id: 11, name: 'Adicionales' }],
    },
    {
      id: 3,
      name: 'Classic',
      sku: 'CL',
      type: 'variable',
      status: 'publish',
      price: '150000',
      parent_id: 0,
      categories: [{ id: 10, name: 'Desayunos' }],
    },
  ],
  variations: [
    { id: 100, parent_id: 1, name: 'Jugo de naranja', sku: 'BMJ1', status: 'publish', price: '85000' },
    { id: 101, parent_id: 1, name: 'Zona dulce', sku: 'BMZ1', status: 'publish', price: '85000' },
  ],
}

describe('buildProducts with a WordPress snapshot', () => {
  it('uses WP as the product master and keeps evaluation only products inactive', () => {
    const result = buildProducts({
      breakfasts: {
        BOX_MUJER: breakfast('Box Mujer'),
        BOLSO_FITNESS: breakfast('Bolso Fitness'),
      },
      additions: {},
      additionPrices: {},
      storeProducts: {},
      cajaSaludDrinks: {},
      wpSnapshot: snapshot,
    })

    const box = result.products.find((product) => product.name === 'Box Mujer')
    expect(box?.wpProductId).toBe(1)
    expect(box?.priceCop).toBe(85000)
    expect(box?.description).toBe('Con jugo')
    expect(box?.active).toBe(true)

    const classic = result.products.find((product) => product.name === 'Classic')
    expect(classic?.category).toBe('breakfast')
    expect(classic?.isKitchen).toBe(true)

    expect(result.excludedWp.map((product) => product.name)).toContain('Globo con helio')
    expect(result.missingInWp.map((product) => product.name)).toContain('Bolso Fitness')
    const fitness = result.products.find((product) => product.name === 'Bolso Fitness')
    expect(fitness?.active).toBe(false)
  })
})

describe('buildRecipes with WordPress variations', () => {
  const products: Product[] = [
    {
      id: 'box-mujer',
      name: 'Box Mujer',
      aliases: [],
      category: 'breakfast',
      isKitchen: true,
      wpProductId: 1,
      active: true,
    },
  ]

  const unit = (name: string): KitchenUnit => ({
    id: name.toLowerCase().replace(/ /g, '-'),
    name,
    category: 'drink' as UnitCategory,
    measure: 'unit',
    aliases: [name],
    isKitchen: true,
    active: true,
  })

  it('suggests a choice group when variations exist and the recipe has none', () => {
    const queue = new ReviewQueue()
    buildRecipes({
      breakfasts: { BOX_MUJER: breakfast('Box Mujer') },
      products,
      units: [],
      resolve: () => null,
      queue,
      wpSnapshot: snapshot,
    })
    expect(queue.countByType().variation_suggests_choice).toBe(1)
  })

  it('matches a variation to an option and queues the rest', () => {
    const queue = new ReviewQueue()
    const breakfastWithChoice = breakfast('Box Mujer')
    breakfastWithChoice.foodItems = [
      {
        type: 'choose_between',
        values: ['Jugo de naranja', 'Yogurt con granola'],
      },
    ]
    const result = buildRecipes({
      breakfasts: { BOX_MUJER: breakfastWithChoice },
      products,
      units: [unit('Jugo de naranja'), unit('Yogurt con granola')],
      resolve: (rawName) => {
        const name = rawName === 'Jugo de naranja' ? 'Jugo de naranja' : 'Yogurt con granola'
        return { unit: unit(name), quantity: 1 }
      },
      queue,
      wpSnapshot: snapshot,
    })
    const recipe = result.recipes[0]
    const group = recipe?.components.find((component) => component.kind === 'choice')
    const option = group?.kind === 'choice' ? group.options[0] : undefined
    expect(option?.matchAliases).toContain('Jugo de naranja')
    expect(queue.countByType().variation_unmapped).toBe(1)
  })
})
