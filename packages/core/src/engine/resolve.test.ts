import { describe, expect, it } from 'vitest'
import { indexCatalog } from '../catalog'
import { parseKitchenField } from '../labels/parse'
import { compileRules } from '../labels/rules'
import { makeProduct, makeRecipe, testCatalog } from '../test-helpers'
import { resolveLine, resolveLines } from './resolve'

const rules = compileRules()

function line (text: string) {
  const parsed = parseKitchenField(text, { rules })
  const first = parsed.lines[0]
  if (!first) throw new Error(`Nothing parsed from: ${text}`)
  return first
}

describe('resolveLine', () => {
  const catalog = testCatalog()
  const index = indexCatalog(catalog)

  it('resolves fixed components and choice options', () => {
    const resolved = resolveLine(line('Box Hombre X 1: Jugo de naranja, Sándwich sencillo'), index)
    expect(resolved.status).toBe('resolved')
    expect(resolved.productName).toBe('Box Hombre')
    expect(resolved.contributions).toEqual([
      { unitId: 'nutella-15-gr', quantity: 1 },
      { unitId: 'jugo-pequeno', quantity: 1 },
      { unitId: 'sandwich-sencillo', quantity: 1 },
    ])
    expect(resolved.matchedOptions).toEqual(['Jugo de naranja', 'Sándwich sencillo'])
  })

  it('multiplies contributions by the line quantity', () => {
    const resolved = resolveLine(line('Box Hombre X 2: Jugo de naranja, Sándwich sencillo'), index)
    expect(resolved.contributions).toEqual([
      { unitId: 'nutella-15-gr', quantity: 2 },
      { unitId: 'jugo-pequeno', quantity: 2 },
      { unitId: 'sandwich-sencillo', quantity: 2 },
    ])
  })

  it('ignores personalization options when required groups match', () => {
    const resolved = resolveLine(line('Box Hombre X 1: Cumpleaños, Azul, Yogurt pequeño, Sándwich sencillo'), index)
    expect(resolved.status).toBe('resolved')
    expect(resolved.contributions).toContainEqual({ unitId: 'yogurt-pequeno', quantity: 1 })
  })

  it('marks a required group without a match as unresolved but keeps known components', () => {
    const resolved = resolveLine(line('Box Hombre X 1: Jugo de naranja'), index)
    expect(resolved.status).toBe('unresolved')
    expect(resolved.unresolvedReason).toBe('missing_choice')
    expect(resolved.unresolvedDetail).toContain('Sándwich o waffle')
    expect(resolved.contributions).toEqual([
      { unitId: 'nutella-15-gr', quantity: 1 },
      { unitId: 'jugo-pequeno', quantity: 1 },
    ])
  })

  it('falls back to the default option when a required group matches nothing', () => {
    const catalogWithDefault = testCatalog({
      recipes: [
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
                matchAliases: ['Yogurt pequeño'],
                default: true,
                components: [{ kind: 'fixed', unitId: 'yogurt-pequeno', quantity: 1 }],
              },
            ],
          },
        ]),
      ],
    })
    const resolved = resolveLine(line('Box Hombre X 1'), indexCatalog(catalogWithDefault))
    expect(resolved.status).toBe('resolved')
    expect(resolved.contributions).toContainEqual({ unitId: 'nutella-15-gr', quantity: 1 })
    expect(resolved.contributions).toContainEqual({ unitId: 'yogurt-pequeno', quantity: 1 })
  })

  it('prefers a matched option over the default option', () => {
    const catalogWithDefault = testCatalog({
      recipes: [
        makeRecipe('box-hombre', [
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
                matchAliases: ['Yogurt pequeño'],
                default: true,
                components: [{ kind: 'fixed', unitId: 'yogurt-pequeno', quantity: 1 }],
              },
            ],
          },
        ]),
      ],
    })
    const resolved = resolveLine(line('Box Hombre X 1: Jugo de naranja'), indexCatalog(catalogWithDefault))
    expect(resolved.status).toBe('resolved')
    expect(resolved.contributions).toContainEqual({ unitId: 'jugo-pequeno', quantity: 1 })
    expect(resolved.contributions).not.toContainEqual({ unitId: 'yogurt-pequeno', quantity: 1 })
  })

  it('prefers the longest alias', () => {
    const resolved = resolveLine(line('Box Hombre X 1: Jugo de naranja semi natural pequeño, Sándwich sencillo'), index)
    expect(resolved.contributions).toContainEqual({ unitId: 'jugo-pequeno', quantity: 1 })
  })

  it('reports unknown products', () => {
    const resolved = resolveLine(line('Producto fantasma X 1'), index)
    expect(resolved.status).toBe('unresolved')
    expect(resolved.unresolvedReason).toBe('unknown_product')
  })

  it('reports a kitchen product without recipe', () => {
    const catalogWithoutRecipe = testCatalog({
      recipes: testCatalog().recipes.filter(recipe => recipe.productId !== 'box-hombre'),
    })
    const resolved = resolveLine(line('Box Hombre X 1'), indexCatalog(catalogWithoutRecipe))
    expect(resolved.unresolvedReason).toBe('missing_recipe')
  })

  it('marks non kitchen products as ignored', () => {
    const resolved = resolveLine(line('Globo con helio X 2'), index)
    expect(resolved.status).toBe('ignored')
    expect(resolved.ignoredReason).toBe('not_kitchen')
  })

  it('reports ambiguous product text', () => {
    const catalogWithTwin = testCatalog({
      products: [
        ...testCatalog().products,
        makeProduct('box-hombre-espejo', 'Box Hombre', { aliases: ['Box Hombre'] }),
      ],
    })
    const resolved = resolveLine(line('Box Hombre X 1'), indexCatalog(catalogWithTwin))
    expect(resolved.unresolvedReason).toBe('ambiguous_match')
  })

  it('prefers the active product when duplicated text has an inactive twin', () => {
    const catalogWithTwin = testCatalog({
      products: [
        ...testCatalog().products,
        makeProduct('box-hombre-viejo', 'Box Hombre', { active: false }),
      ],
    })
    const resolved = resolveLine(
      line('Box Hombre X 1: Jugo de naranja, Sándwich sencillo'),
      indexCatalog(catalogWithTwin)
    )
    expect(resolved.status).toBe('resolved')
    expect(resolved.productId).toBe('box-hombre')
  })

  it('applies operator mappings before text lookup', () => {
    const catalogWithUnknown = testCatalog({
      products: [...testCatalog().products, makeProduct('postre-secreto', 'Postre secreto')],
      recipes: [
        ...testCatalog().recipes,
        makeRecipe('postre-secreto', [
          { kind: 'fixed', unitId: 'ensalada-de-frutas-grande', quantity: 1 },
        ]),
      ],
    })
    const resolved = resolveLine(line('Pomo X 1'), indexCatalog(catalogWithUnknown), {
      productMappings: { pomo: 'postre-secreto' },
    })
    expect(resolved.status).toBe('resolved')
    expect(resolved.productId).toBe('postre-secreto')
  })

  it('reports missing units referenced by a recipe', () => {
    const catalogWithBrokenRecipe = testCatalog({
      units: testCatalog().units.filter(unit => unit.id !== 'jugo-pequeno'),
    })
    const resolved = resolveLine(line('Box Hombre X 1: Jugo de naranja, Sándwich sencillo'), indexCatalog(catalogWithBrokenRecipe))
    expect(resolved.unresolvedReason).toBe('missing_unit')
    expect(resolved.unresolvedDetail).toBe('jugo-pequeno')
  })
})

describe('resolveLines', () => {
  it('resolves each line independently', () => {
    const index = indexCatalog(testCatalog())
    const lines = [
      line('Box Hombre X 1: Jugo de naranja, Sándwich sencillo'),
      line('Parfait X 1'),
    ]
    const resolved = resolveLines(lines, index)
    expect(resolved.map(item => item.status)).toEqual(['resolved', 'resolved'])
  })
})
