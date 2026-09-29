import { describe, expect, it } from 'vitest'
import { indexCatalog } from '../catalog'
import { aggregateUnits } from '../engine/aggregate'
import { resolveLines } from '../engine/resolve'
import { parseKitchenField } from '../labels/parse'
import { compileRules } from '../labels/rules'
import { testCatalog } from '../test-helpers'
import { kitchenPrintTemplate, formatKitchenTemplateHtml } from './print-template'

const rules = compileRules()

function makeList (text: string) {
  const parsed = parseKitchenField(text, { rules })
  const index = indexCatalog(testCatalog())
  return aggregateUnits(resolveLines(parsed.lines, index), index)
}

describe('kitchenPrintTemplate', () => {
  it('keeps labels uppercase and unique keys', () => {
    const blocks = [...kitchenPrintTemplate.left, ...kitchenPrintTemplate.right]
    for (const block of blocks) {
      expect(block.label).toBe(block.label.toUpperCase())
      for (const sub of block.subRows ?? []) expect(sub.label).toBe(sub.label.toUpperCase())
    }
    const keys = blocks.flatMap(block => [block.key, ...(block.subRows ?? []).map(sub => sub.key)])
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('maps every unit id only once', () => {
    const ids = [...kitchenPrintTemplate.left, ...kitchenPrintTemplate.right].flatMap(block =>
      block.subRows ? block.subRows.flatMap(sub => sub.unitIds ?? []) : (block.unitIds ?? [])
    )
    expect(new Set(ids).size).toBe(ids.length)
  })
})

describe('formatKitchenTemplateHtml', () => {
  it('writes quantities into the mapped rows', () => {
    const list = makeList('Box Hombre X 3: Jugo de naranja, Sándwich sencillo')
    const html = formatKitchenTemplateHtml(list, {
      date: '2026-09-30',
      template: {
        left: [
          { key: 'jugo', label: 'JUGO PEQUEÑO', unitIds: ['jugo-pequeno'] },
          { key: 'sandwich', label: 'SANDWICH SENCILLO', unitIds: ['sandwich-sencillo'] },
        ],
        right: [],
      },
    })
    expect(html).toContain('Cocina en Palorosa')
    expect(html).toContain('2026-09-30')
    expect(html).toContain('JUGO PEQUEÑO')
    expect(html).toContain('<td class="qty">3</td>')
    expect(html).not.toContain('OTROS (SIN MAPEAR)')
    expect(html).not.toContain('Fecha de entrega')
  })

  it('sums several units into one row', () => {
    const list = makeList('Box Hombre X 1: Jugo de naranja, Sándwich sencillo')
    list.entries.push({
      unitId: 'jugo-extra',
      name: 'Jugo extra',
      measure: 'bottle',
      category: 'drink',
      quantity: 2,
      references: ['1'],
    })
    const template = {
      left: [{ key: 'jugos', label: 'JUGOS', unitIds: ['jugo-pequeno', 'jugo-extra'] }],
      right: [],
    }
    const html = formatKitchenTemplateHtml(list, { template })
    expect(html).toContain('JUGOS')
    expect(html).toContain('<td class="qty">3</td>')
  })

  it('sends unmapped units to the extra section', () => {
    const list = makeList('Box Hombre X 1: Jugo de naranja, Sándwich sencillo')
    list.entries.push({
      unitId: 'unidad-sin-mapear',
      name: 'Unidad sin mapear',
      measure: 'unit',
      category: 'other',
      quantity: 2,
      references: ['1'],
    })
    const html = formatKitchenTemplateHtml(list)
    expect(html).toContain('OTROS (SIN MAPEAR)')
    expect(html).toContain('Unidad sin mapear')
  })

  it('keeps unresolved entries visible', () => {
    const list = makeList('Producto fantasma X 2')
    const html = formatKitchenTemplateHtml(list)
    expect(html).toContain('SIN RESOLVER')
    expect(html).toContain('Producto fantasma')
  })
})
