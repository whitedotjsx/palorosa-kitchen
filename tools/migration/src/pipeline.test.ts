import { describe, expect, it } from 'vitest'
import { buildProducts } from './build-products'
import { buildRecipes } from './build-recipes'
import { buildUnits } from './build-units'
import { readSource } from './read-source'
import { ReviewQueue } from './review'

describe('full migration pipeline', () => {
  it('produces a recipe for every breakfast', async () => {
    const source = await readSource()
    const queue = new ReviewQueue()
    const units = buildUnits({
      kitchenFoods: source.kitchenFoods,
      decorations: source.decorations,
      decisions: {},
      queue,
    })
    const products = buildProducts({
      breakfasts: source.breakfasts,
      additions: source.additions,
      additionPrices: source.additionPrices,
      storeProducts: source.storeProducts,
      cajaSaludDrinks: source.cajaSaludDrinks,
      wpSnapshot: null,
    })
    const recipes = buildRecipes({
      breakfasts: source.breakfasts,
      products: products.products,
      units: units.units,
      resolve: units.resolve,
      queue,
      wpSnapshot: null,
    })

    const breakfastProducts = products.products.filter(
      (product) => product.category === 'breakfast'
    )
    expect(breakfastProducts).toHaveLength(20)

    const recipeIds = new Set(recipes.recipes.map((recipe) => recipe.productId))
    for (const product of breakfastProducts) {
      expect(recipeIds.has(product.id)).toBe(true)
    }
    expect(units.units.length).toBeGreaterThanOrEqual(51)
  })
})
