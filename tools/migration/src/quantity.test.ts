import { describe, expect, it } from 'vitest'
import { detectLeadingQuantity } from './quantity'

describe('detectLeadingQuantity', () => {
  it('detects a leading number word and suggests a base name', () => {
    expect(detectLeadingQuantity('Dos mini ensaladas de frutas')).toEqual({
      quantity: 2,
      baseName: 'mini ensalada de fruta',
    })
  })

  it('handles tres', () => {
    expect(detectLeadingQuantity('Tres fresas enteras')).toEqual({
      quantity: 3,
      baseName: 'fresa entera',
    })
  })

  it('returns null when there is no leading quantity', () => {
    expect(detectLeadingQuantity('Jugo de naranja semi natural pequeño')).toBeNull()
  })

  it('accepts una as quantity one', () => {
    expect(detectLeadingQuantity('Una porción de fruta picada')).toEqual({
      quantity: 1,
      baseName: 'porción de fruta picada',
    })
  })
})
