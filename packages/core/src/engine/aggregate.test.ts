import { describe, expect, it } from 'vitest'
import { indexCatalog } from '../catalog'
import { parseKitchenField } from '../labels/parse'
import { compileRules } from '../labels/rules'
import { makeProduct, makeRecipe, makeUnit, testCatalog } from '../test-helpers'
import { aggregateUnits, groupEntriesByCategory, unitCategoryOrder } from './aggregate'
import { resolveLines } from './resolve'

const rules = compileRules()

function resolveText (text: string, catalog = testCatalog()) {
  const parsed = parseKitchenField(text, { rules })
  return resolveLines(parsed.lines, indexCatalog(catalog))
}

describe('aggregateUnits', () => {
  it('sums quantities per unit and keeps order references', () => {
    const resolved = [
      ...resolveText('Box Hombre X 1: Jugo de naranja, Sándwich sencillo').map(item => ({
        ...item,
        line: { ...item.line, orderNumber: '1' },
      })),
      ...resolveText('Box Hombre X 2: Jugo de naranja, Sándwich sencillo').map(item => ({
        ...item,
        line: { ...item.line, orderNumber: '2' },
      })),
    ]
    const list = aggregateUnits(resolved, indexCatalog(testCatalog()))
    const juice = list.entries.find(entry => entry.unitId === 'jugo-pequeno')
    expect(juice?.quantity).toBe(3)
    expect(juice?.references).toEqual(['1', '2'])
    const sandwich = list.entries.find(entry => entry.unitId === 'sandwich-sencillo')
    expect(sandwich?.quantity).toBe(3)
  })

  it('moves non kitchen units to ignored', () => {
    const list = aggregateUnits(resolveText('Box Hombre X 1: Jugo de naranja, Sándwich sencillo'), indexCatalog(testCatalog()))
    expect(list.entries.some(entry => entry.unitId === 'nutella-15-gr')).toBe(false)
    expect(list.ignoredUnits).toEqual([
      expect.objectContaining({ id: 'nutella-15-gr', quantity: 1, reason: 'not_kitchen' }),
    ])
  })

  it('marks inactive units as ignored', () => {
    const catalog = testCatalog({
      recipes: [
        ...testCatalog().recipes.filter(recipe => recipe.productId !== 'box-hombre'),
        makeRecipe('box-hombre', [
          { kind: 'fixed', unitId: 'unidad-inactiva', quantity: 1 },
        ]),
      ],
    })
    const parsed = parseKitchenField('Box Hombre X 1', { rules })
    const list = aggregateUnits(resolveLines(parsed.lines, indexCatalog(catalog)), indexCatalog(catalog))
    expect(list.ignoredUnits).toEqual([
      expect.objectContaining({ id: 'unidad-inactiva', reason: 'inactive_unit' }),
    ])
  })

  it('aggregates unresolved entries by reason and orders', () => {
    const resolved = [
      ...resolveText('Producto fantasma X 2').map(item => ({
        ...item,
        line: { ...item.line, orderNumber: '10' },
      })),
      ...resolveText('Producto fantasma X 1').map(item => ({
        ...item,
        line: { ...item.line, orderNumber: '11' },
      })),
    ]
    const list = aggregateUnits(resolved, indexCatalog(testCatalog()))
    expect(list.unresolved).toEqual([
      expect.objectContaining({
        productText: 'Producto fantasma',
        quantity: 3,
        count: 2,
        reason: 'unknown_product',
        references: ['10', '11'],
      }),
    ])
  })

  it('aggregates ignored products across orders', () => {
    const resolved = resolveText('Globo con helio X 2')
    const list = aggregateUnits(resolved, indexCatalog(testCatalog()))
    expect(list.ignoredProducts).toEqual([
      expect.objectContaining({ id: 'globo-con-helio', quantity: 2, reason: 'not_kitchen' }),
    ])
  })

  it('keeps known units of a partially resolved line and flags the gap', () => {
    const resolved = resolveText('Box Hombre X 2: Jugo de naranja')
    const list = aggregateUnits(resolved, indexCatalog(testCatalog()))
    const juice = list.entries.find(entry => entry.unitId === 'jugo-pequeno')
    expect(juice?.quantity).toBe(2)
    expect(list.unresolved[0]).toMatchObject({
      productText: 'Box Hombre',
      quantity: 2,
      reason: 'missing_choice',
    })
  })

  it('collects quantity warnings', () => {
    const list = aggregateUnits(resolveText('Classic'), indexCatalog(testCatalog()))
    expect(list.warnings).toEqual([
      expect.objectContaining({ productText: 'Classic', quantity: 1 }),
    ])
  })

  it('does not include units outside the catalog', () => {
    const catalog = testCatalog()
    const broken = testCatalog({ units: [...catalog.units, makeUnit('huerfana', 'Huérfana')] })
    const withMissing = testCatalog({
      recipes: [
        ...broken.recipes.filter(recipe => recipe.productId !== 'parfait'),
        makeRecipe('parfait', [{ kind: 'fixed', unitId: 'no-existe', quantity: 1 }]),
      ],
    })
    const parsed = parseKitchenField('Parfait X 1', { rules })
    const list = aggregateUnits(resolveLines(parsed.lines, indexCatalog(withMissing)), indexCatalog(withMissing))
    expect(list.unresolved[0]).toMatchObject({ reason: 'missing_unit', detail: 'no-existe' })
  })

  it('keeps category order and group helper', () => {
    const catalog = testCatalog({
      products: [...testCatalog().products, makeProduct('jugo-suelto', 'Jugo suelto', { aliases: ['Jugo suelto'] })],
      recipes: [
        ...testCatalog().recipes,
        makeRecipe('jugo-suelto', [{ kind: 'fixed', unitId: 'jugo-pequeno', quantity: 1 }]),
      ],
    })
    const resolved = resolveText('Jugo suelto X 1', catalog)
    const list = aggregateUnits(resolved, indexCatalog(catalog))
    const groups = groupEntriesByCategory(list)
    expect(groups.map(group => group.category)).toEqual(['drink'])
    expect(unitCategoryOrder[0]).toBe('drink')
  })
})
