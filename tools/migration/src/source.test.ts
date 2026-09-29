import { describe, expect, it } from 'vitest'
import { readSource } from './read-source'

describe('readSource', () => {
  it('loads the vendored catalog', async () => {
    const source = await readSource()
    expect(Object.keys(source.breakfasts)).toHaveLength(20)
    expect(Object.keys(source.kitchenFoods).length).toBeGreaterThanOrEqual(51)
    expect(Object.keys(source.decorations).length).toBeGreaterThanOrEqual(150)
    expect(Object.keys(source.containers).length).toBeGreaterThanOrEqual(19)
    expect(Object.keys(source.additions).length).toBeGreaterThanOrEqual(21)
    expect(Object.keys(source.storeProducts).length).toBeGreaterThanOrEqual(35)
  })
})
