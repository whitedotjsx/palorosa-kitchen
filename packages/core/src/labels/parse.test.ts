import { describe, expect, it } from 'vitest'
import { parseKitchenField, parseLabelFields } from './parse'
import { compileRules } from './rules'

const rules = compileRules()

describe('parseKitchenField', () => {
  it('parses product, quantity and options', () => {
    const result = parseKitchenField('Gold Basic X 1: Cumpleaños, Azul', { rules })
    expect(result.skipped).toEqual([])
    expect(result.lines).toHaveLength(1)
    expect(result.lines[0]).toMatchObject({
      productText: 'Gold Basic',
      quantity: 1,
      options: ['Cumpleaños', 'Azul'],
      source: 'breakfast',
    })
  })

  it('accepts a comma between quantity and options', () => {
    const result = parseKitchenField('12 ROSAS X 1, Fotos: 1', { rules })
    expect(result.lines[0]).toMatchObject({
      productText: '12 ROSAS',
      quantity: 1,
      options: ['Fotos: 1'],
    })
  })

  it('splits several products on the product separator', () => {
    const result = parseKitchenField('Gold Basic X 1: Cumpleaños | Morning X 2: Jugo de naranja', {
      rules,
    })
    expect(result.lines.map(line => [line.productText, line.quantity])).toEqual([
      ['Gold Basic', 1],
      ['Morning', 2],
    ])
  })

  it('keeps quantity-less entries visible with a warning', () => {
    const result = parseKitchenField('Classic', { rules })
    expect(result.lines[0]).toMatchObject({
      productText: 'Classic',
      quantity: 1,
      warning: 'missing_quantity',
    })
  })

  it('skips noise entries', () => {
    const result = parseKitchenField('DOMICILIO EXCLUSIVO X 1: Antes de 730am', { rules })
    expect(result.lines).toEqual([])
    expect(result.skipped).toEqual([
      { raw: 'DOMICILIO EXCLUSIVO X 1: Antes de 730am', reason: 'noise' },
    ])
  })

  it('collapses wrapped whitespace', () => {
    const result = parseKitchenField('Box Hombre X 1: Sándwich Jamón y\n   queso, Cumpleaños', {
      rules,
    })
    expect(result.lines[0]?.options).toEqual(['Sándwich Jamón y queso', 'Cumpleaños'])
  })
})

describe('parseLabelFields', () => {
  it('parses breakfast and additionals with the order number', () => {
    const result = parseLabelFields(
      {
        orderNumber: '73175',
        breakfastText: 'Gold Basic X 1: Cumpleaños, Azul',
        additionalsText: 'Mini torta personal con topper X 1',
      },
      undefined
    )
    expect(result.lines).toHaveLength(2)
    expect(result.lines[0]).toMatchObject({ orderNumber: '73175', source: 'breakfast' })
    expect(result.lines[1]).toMatchObject({ orderNumber: '73175', source: 'add_on' })
  })
})
