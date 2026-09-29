import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { kitchenPrintTemplate, type Catalog, type TemplateSubRow } from '@palorosa-kitchen/core'
import { describe, expect, it } from 'vitest'
import { dataDir } from './paths'

const seed = JSON.parse(readFileSync(join(dataDir, 'seed.json'), 'utf8')) as Catalog
const unitsById = new Map(seed.units.map(unit => [unit.id, unit]))

describe('kitchen print template against the seed', () => {
  const rows: TemplateSubRow[] = [...kitchenPrintTemplate.left, ...kitchenPrintTemplate.right].flatMap(
    block =>
      block.subRows ??
      [{ key: block.key, label: block.label, ...(block.unitIds ? { unitIds: block.unitIds } : {}) }]
  )

  it('only maps unit ids that exist in the seed', () => {
    const missing: string[] = []
    for (const row of rows) {
      for (const unitId of row.unitIds ?? []) {
        if (!unitsById.has(unitId)) missing.push(`${row.key}: ${unitId}`)
      }
    }
    expect(missing).toEqual([])
  })

  it('maps each unit id once', () => {
    const ids = rows.flatMap(row => row.unitIds ?? [])
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('every mapped unit is a kitchen unit', () => {
    const notKitchen: string[] = []
    for (const row of rows) {
      for (const unitId of row.unitIds ?? []) {
        const unit = unitsById.get(unitId)
        if (unit && !unit.isKitchen) notKitchen.push(unitId)
      }
    }
    expect(notKitchen).toEqual([])
  })
})
