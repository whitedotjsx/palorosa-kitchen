import { describe, expect, it } from 'vitest'
import { indexCatalog } from '../catalog'
import { parseKitchenField } from '../labels/parse'
import { compileRules } from '../labels/rules'
import { testCatalog } from '../test-helpers'
import { aggregateUnits } from './aggregate'
import { diffLists, isListDiffEmpty } from './diff'
import { resolveLines } from './resolve'

const rules = compileRules()

function listOf (text: string) {
  const parsed = parseKitchenField(text, { rules })
  const index = indexCatalog(testCatalog())
  return aggregateUnits(resolveLines(parsed.lines, index), index)
}

describe('diffLists', () => {
  it('reports added, removed and changed units', () => {
    const previous = listOf('Box Hombre X 1: Jugo de naranja, Sándwich sencillo')
    const next = listOf('Box Hombre X 3: Jugo de naranja, Waffle con arequipe y fresas')
    const diff = diffLists(previous, next)

    expect(diff.added.map(entry => [entry.unitId, entry.next])).toEqual([
      ['waffle-con-arequipe-y-fresas', 3],
    ])
    expect(diff.removed.map(entry => entry.unitId)).toEqual(['sandwich-sencillo'])
    expect(diff.changed.map(entry => [entry.unitId, entry.previous, entry.next])).toEqual([
      ['jugo-pequeno', 1, 3],
    ])
    expect(isListDiffEmpty(diff)).toBe(false)
  })

  it('returns an empty diff for equal lists', () => {
    const list = listOf('Box Hombre X 1: Jugo de naranja, Sándwich sencillo')
    expect(isListDiffEmpty(diffLists(list, list))).toBe(true)
  })

  it('treats a missing previous list as all added', () => {
    const next = listOf('Box Hombre X 1: Jugo de naranja, Sándwich sencillo')
    const diff = diffLists(null, next)
    expect(diff.added.length).toBeGreaterThan(0)
    expect(diff.removed).toEqual([])
    expect(diff.changed).toEqual([])
  })
})
