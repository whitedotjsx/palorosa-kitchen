import { describe, expect, it } from 'vitest'
import {
  detectSeparator,
  isWideOrderHeader,
  parseDelimitedText,
  parseOrdersExport,
  parseSpanishDeliveryDate,
  parseWideOrderRows,
  tableToRecords,
} from './orders'

describe('parseOrdersExport', () => {
  it('reads the canonical nested export', () => {
    const result = parseOrdersExport({
      deliveryDate: '2026-09-30',
      orders: [
        {
          orderNumber: '73175',
          lines: [
            { productText: 'Gold Basic', quantity: 1, options: ['Cumpleaños', 'Azul'] },
            { productText: 'Mini torta personal con topper', quantity: 1 },
          ],
        },
      ],
    })
    expect(result.deliveryDate).toBe('2026-09-30')
    expect(result.lines).toHaveLength(2)
    expect(result.lines[0]).toMatchObject({
      orderNumber: '73175',
      productText: 'Gold Basic',
      quantity: 1,
      options: ['Cumpleaños', 'Azul'],
      source: 'breakfast',
    })
    expect(result.skipped).toEqual([])
  })

  it('accepts a flat array of lines', () => {
    const result = parseOrdersExport([
      { product: 'Morning', quantity: '2', options: 'Jugo de naranja, Cumpleaños' },
    ])
    expect(result.lines[0]).toMatchObject({
      productText: 'Morning',
      quantity: 2,
      options: ['Jugo de naranja', 'Cumpleaños'],
    })
  })

  it('accepts additionals lines with their source', () => {
    const result = parseOrdersExport([
      { productText: 'Globo con helio', quantity: 2, source: 'add_on' },
      { productText: 'Mini torta', quantity: 1, field: 'additionals' },
    ])
    expect(result.lines.map(line => line.source)).toEqual(['add_on', 'add_on'])
  })

  it('warns when the quantity is missing', () => {
    const result = parseOrdersExport([{ productText: 'Classic' }])
    expect(result.lines[0]?.warning).toBe('missing_quantity')
  })

  it('reports malformed entries instead of dropping them', () => {
    const result = parseOrdersExport({ orders: ['nope', { quantity: 3 }] })
    expect(result.lines).toEqual([])
    expect(result.skipped).toHaveLength(2)
    expect(result.skipped.every(entry => entry.reason === 'malformed')).toBe(true)
  })

  it('reports a root value that is not an export', () => {
    const result = parseOrdersExport(42)
    expect(result.lines).toEqual([])
    expect(result.skipped[0]?.reason).toBe('malformed')
  })
})

describe('parseSpanishDeliveryDate', () => {
  it('reads the store export format', () => {
    expect(parseSpanishDeliveryDate('29 Septiembre, 2026')).toBe('2026-09-29')
    expect(parseSpanishDeliveryDate('3 de enero de 2027')).toBe('2027-01-03')
  })

  it('reads ISO text and Excel serial numbers', () => {
    expect(parseSpanishDeliveryDate('2026-09-29')).toBe('2026-09-29')
    expect(parseSpanishDeliveryDate(46304)).toBe('2026-10-09')
  })

  it('returns undefined for unknown values', () => {
    expect(parseSpanishDeliveryDate('')).toBeUndefined()
    expect(parseSpanishDeliveryDate('mañana')).toBeUndefined()
  })
})

describe('parseWideOrderRows', () => {
  const rows = [
    {
      'ID orden': 74112,
      'Fecha de Entrega': '29 Septiembre, 2026',
      Productos: 'Box Hombre X 1',
      Adicionales: '',
      Observaciones: '',
      motivo: 'cumpleanos',
      'Jugo de Naranja': 1,
      Sandwich: 1,
      'Fruta Grande': 1,
    },
    {
      'ID orden': 75597,
      'Fecha de Entrega': '29 Septiembre, 2026',
      Productos: 'Saludable X 1',
      Adicionales: '24 ROSAS X 1 | 3 mini deditos de queso X 1: ',
      Sandwich: 1,
    },
  ]

  it('maps products, add-ons and checked columns to lines', () => {
    const result = parseWideOrderRows(rows)
    expect(result.deliveryDate).toBe('2026-09-29')
    expect(result.lines).toHaveLength(4)
    expect(result.lines[0]).toMatchObject({
      orderNumber: '74112',
      productText: 'Box Hombre',
      quantity: 1,
      options: ['Jugo de Naranja', 'Sandwich', 'Fruta Grande'],
      source: 'breakfast',
    })
    expect(result.lines[1]).toMatchObject({
      orderNumber: '75597',
      productText: 'Saludable',
      options: ['Sandwich', 'Hatsu té', 'Café Mocca'],
      source: 'breakfast',
    })
    expect(result.lines[2]).toMatchObject({
      orderNumber: '75597',
      productText: '24 ROSAS',
      source: 'add_on',
    })
    expect(result.lines[3]).toMatchObject({
      orderNumber: '75597',
      productText: '3 mini deditos de queso',
      source: 'add_on',
    })
  })

  it('adds the drink fallbacks only when the juice column is empty', () => {
    const result = parseWideOrderRows(rows)
    expect(result.lines[0]?.options).not.toContain('Hatsu té')
    expect(result.lines[0]?.options).not.toContain('Café Mocca')
    expect(result.lines[1]?.options).toContain('Hatsu té')
    expect(result.lines[1]?.options).toContain('Café Mocca')
  })

  it('reports rows with no product text at all', () => {
    const result = parseWideOrderRows([
      { 'ID orden': 1, Productos: '', Adicionales: '', Observaciones: 'solo nota' },
    ])
    expect(result.lines).toEqual([])
    expect(result.skipped[0]?.reason).toBe('malformed')
  })
})

describe('isWideOrderHeader', () => {
  it('recognizes the store export header', () => {
    expect(isWideOrderHeader(['ID orden', 'Fecha de Entrega', 'Productos'])).toBe(true)
    expect(isWideOrderHeader(['Pedido', 'Barrio'])).toBe(false)
  })
})

describe('parseDelimitedText', () => {
  it('detects the separator and reads quoted fields', () => {
    const rows = parseDelimitedText('a,b\n"x, y",2')
    expect(rows).toEqual([['a', 'b'], ['x, y', '2']])
    expect(detectSeparator('a;b;c')).toBe(';')
  })

  it('handles escaped quotes and newlines inside quotes', () => {
    const rows = parseDelimitedText('a,b\n"dijo ""hola""","linea\nnueva"')
    expect(rows[1]).toEqual(['dijo "hola"', 'linea\nnueva'])
  })

  it('converts rows to header keyed records', () => {
    const records = tableToRecords([['ID orden', 'Productos'], ['1', 'Box Hombre X 1']])
    expect(records).toEqual([{ 'ID orden': '1', Productos: 'Box Hombre X 1' }])
  })
})
