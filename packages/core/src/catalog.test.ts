import { describe, expect, it } from 'vitest'
import { indexCatalog, lookupProducts, lookupUnits, validateCatalog } from './catalog'
import { SCHEMA_VERSION } from './schema'
import { makeProduct, makeRecipe, makeUnit, testCatalog } from './test-helpers'

describe('indexCatalog', () => {
  const index = indexCatalog(testCatalog())

  it('finds products by name and alias', () => {
    expect(lookupProducts(index, 'box hombre')[0]?.id).toBe('box-hombre')
    expect(lookupProducts(index, 'BOX HOMBRE')[0]?.id).toBe('box-hombre')
  })

  it('finds units by name and alias', () => {
    expect(lookupUnits(index, 'Jugo de naranja')[0]?.id).toBe('jugo-pequeno')
  })

  it('returns an empty list for unknown text', () => {
    expect(lookupProducts(index, 'No existe')).toEqual([])
  })
})

describe('validateCatalog', () => {
  it('accepts a coherent catalog', () => {
    expect(validateCatalog(testCatalog())).toEqual([])
  })

  it('reports a wrong schema version', () => {
    const issues = validateCatalog(testCatalog({ schemaVersion: 99 }))
    expect(issues.some(issue => issue.kind === 'schema_version')).toBe(true)
  })

  it('reports duplicate ids', () => {
    const issues = validateCatalog(
      testCatalog({ units: [makeUnit('duplicada', 'Una'), makeUnit('duplicada', 'Otra')] })
    )
    expect(issues.some(issue => issue.kind === 'duplicate_id')).toBe(true)
  })

  it('reports units referenced by recipes but missing', () => {
    const catalog = testCatalog({
      recipes: [makeRecipe('box-hombre', [{ kind: 'fixed', unitId: 'no-existe', quantity: 1 }])],
    })
    const issues = validateCatalog(catalog)
    expect(issues.some(issue => issue.kind === 'missing_unit' && issue.id === 'no-existe')).toBe(true)
  })

  it('reports recipe options referencing missing units', () => {
    const catalog = testCatalog({
      recipes: [
        makeRecipe('box-hombre', [
          {
            kind: 'choice',
            label: 'Bebida',
            required: true,
            options: [
              {
                id: 'opcion',
                label: 'Opción',
                matchAliases: [],
                components: [{ kind: 'fixed', unitId: 'no-existe', quantity: 1 }],
              },
            ],
          },
        ]),
      ],
    })
    const issues = validateCatalog(catalog)
    expect(issues.some(issue => issue.kind === 'missing_unit' && issue.message.includes('box-hombre/opcion'))).toBe(true)
  })

  it('reports ambiguous aliases across products', () => {
    const catalog = testCatalog({
      products: [...testCatalog().products, makeProduct('gemelo', 'Box Hombre')],
    })
    const issues = validateCatalog(catalog)
    expect(issues.some(issue => issue.kind === 'ambiguous_text')).toBe(true)
  })

  it('reports an active kitchen product without recipe', () => {
    const catalog = testCatalog({
      recipes: [],
      products: [makeProduct('huerfano', 'Huérfano')],
    })
    const issues = validateCatalog(catalog)
    expect(
      issues.some(issue => issue.kind === 'product_without_recipe' && issue.id === 'huerfano')
    ).toBe(true)
  })

  it('reports missing containers', () => {
    const catalog = testCatalog({
      products: [makeProduct('en-caja', 'En caja', { containerId: 'no-existe' })],
    })
    const issues = validateCatalog(catalog)
    expect(issues.some(issue => issue.kind === 'missing_container')).toBe(true)
  })

  it('uses the current schema version constant', () => {
    expect(testCatalog().schemaVersion).toBe(SCHEMA_VERSION)
  })
})
