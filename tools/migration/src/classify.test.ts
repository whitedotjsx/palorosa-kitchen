import { describe, expect, it } from 'vitest'
import { classifyName } from './classify'

describe('classifyName', () => {
  it('does not treat tote products as food', () => {
    expect(classifyName('Tote Bag Primavera')).not.toBe('food')
    expect(classifyName('PRIMAVERA')).not.toBe('food')
  })

  it('treats tea based drinks as food', () => {
    expect(classifyName('Hatsu té')).toBe('food')
    expect(classifyName('Dos Hatsu té')).toBe('food')
  })

  it('treats wine add-ons as food', () => {
    expect(classifyName('JP Chenete 200ml')).toBe('food')
    expect(classifyName('Botella de vino tinto 750ml')).toBe('food')
  })

  it('classifies balloons as presentation', () => {
    expect(classifyName('Globo con helio')).toBe('presentation')
  })
})
