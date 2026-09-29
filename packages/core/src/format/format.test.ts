import { describe, expect, it } from 'vitest'
import { indexCatalog } from '../catalog'
import { aggregateUnits } from '../engine/aggregate'
import { resolveLines } from '../engine/resolve'
import { parseKitchenField } from '../labels/parse'
import { compileRules } from '../labels/rules'
import { testCatalog } from '../test-helpers'
import { escapeCsvField, formatListCsv } from './csv'
import { escapeHtml, formatListHtml } from './html'
import { formatListDiffText, formatListText } from './text'

const rules = compileRules()

function testList () {
  const parsed = parseKitchenField(
    'Box Hombre X 3: Jugo de naranja, Sándwich sencillo | Producto raro X 1',
    { rules }
  )
  const index = indexCatalog(testCatalog())
  return aggregateUnits(resolveLines(parsed.lines, index), index)
}

describe('formatListText', () => {
  it('renders grouped quantities and unresolved entries', () => {
    const text = formatListText(testList(), { date: '2026-09-30' })
    expect(text).toContain('Lista de cocina · 2026-09-30')
    expect(text).toContain('Bebidas')
    expect(text).toContain('- 3 x Jugo de naranja semi natural pequeño')
    expect(text).toContain('Sin resolver')
    expect(text).toContain('- 1 x Producto raro (Producto no reconocido)')
  })

  it('can omit unresolved entries', () => {
    const text = formatListText(testList(), { includeUnresolved: false })
    expect(text).not.toContain('Sin resolver')
  })
})

describe('formatListCsv', () => {
  it('renders a header and escaped rows', () => {
    const csv = formatListCsv(testList())
    const lines = csv.split('\n')
    expect(lines[0]).toBe('Categoría,Ítem,Cantidad,Medida,Nota')
    expect(lines[1]).toBe('Bebidas,Jugo de naranja semi natural pequeño,3,Botella,')
  })

  it('escapes separators, quotes and newlines', () => {
    expect(escapeCsvField('Con "comillas"', ',')).toBe('"Con ""comillas"""')
    expect(escapeCsvField('Con, coma', ',')).toBe('"Con, coma"')
    expect(escapeCsvField('Con\nsalto', ',')).toBe('"Con\nsalto"')
    expect(escapeCsvField('Limpio', ',')).toBe('Limpio')
  })

  it('supports a custom separator and unresolved rows', () => {
    const csv = formatListCsv(testList(), { separator: ';', includeUnresolved: true })
    expect(csv.split('\n')[0]).toBe('Categoría;Ítem;Cantidad;Medida;Nota')
    expect(csv).toContain('Sin resolver;Producto raro;1;;Producto no reconocido')
  })
})

describe('formatListHtml', () => {
  it('renders a printable document', () => {
    const html = formatListHtml(testList(), { date: '2026-09-30' })
    expect(html).toContain('<!doctype html>')
    expect(html).toContain('<h1>Lista de cocina</h1>')
    expect(html).toContain('Jugo de naranja semi natural pequeño')
    expect(html).toContain('Sin resolver')
    expect(html).toContain('@media print')
  })

  it('escapes html characters', () => {
    expect(escapeHtml('<b>"x" & y</b>')).toBe('&lt;b&gt;&quot;x&quot; &amp; y&lt;/b&gt;')
  })
})

describe('formatListDiffText', () => {
  it('renders added, changed and removed units', () => {
    const diff = {
      added: [{ unitId: 'nuevo', name: 'Nuevo ítem', previous: 0, next: 2, delta: 2 }],
      changed: [{ unitId: 'jugo', name: 'Jugo', previous: 1, next: 3, delta: 2 }],
      removed: [{ unitId: 'viejo', name: 'Viejo ítem', previous: 1, next: 0, delta: -1 }],
    }
    const text = formatListDiffText(diff, { title: 'Pedido nuevo', date: '2026-09-29' })
    expect(text).toContain('Pedido nuevo · 2026-09-29')
    expect(text).toContain('+ 2 x Nuevo ítem')
    expect(text).toContain('~ Jugo: 1 → 3')
    expect(text).toContain('- 1 x Viejo ítem')
  })

  it('says when there are no changes', () => {
    const text = formatListDiffText({ added: [], changed: [], removed: [] })
    expect(text).toContain('Sin cambios en la lista')
  })
})
