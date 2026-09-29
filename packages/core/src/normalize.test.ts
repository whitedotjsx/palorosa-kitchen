import { describe, expect, it } from 'vitest'
import { normalizeName, slugify, stripAccents } from './normalize'

describe('normalizeName', () => {
  it('removes accents and case', () => {
    expect(normalizeName('Jugo de Naranja Semi Natural Pequeño')).toBe(
      'jugo de naranja semi natural pequeno'
    )
  })

  it('collapses punctuation and extra spaces', () => {
    expect(normalizeName('  Sándwich (jamón, queso)  ')).toBe('sandwich jamon queso')
  })
})

describe('slugify', () => {
  it('produces kebab case ids', () => {
    expect(slugify('Jugo de naranja semi natural pequeño')).toBe(
      'jugo-de-naranja-semi-natural-pequeno'
    )
  })
})

describe('stripAccents', () => {
  it('keeps other characters intact', () => {
    expect(stripAccents("Buchanan's Deluxe 12 años")).toBe("Buchanan's Deluxe 12 anos")
  })
})
