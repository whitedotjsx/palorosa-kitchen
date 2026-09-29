import { compileRules } from './labels/rules'
import type { CompiledRules } from './labels/rules'
import { collapseWhitespace, parseKitchenField, splitBySeparators } from './labels/parse'
import type { OrderLineSource, ParsedOrderLine, SkippedText } from './labels/parse'
import { normalizeName, stripAccents } from './normalize'
import type { LabelParsingRules } from './schema'

/**
 * Canonical order export, produced by the WordPress plugin (Phase 2) or any
 * other consumer:
 *
 * ```json
 * {
 *   "deliveryDate": "2026-09-30",
 *   "orders": [
 *     { "orderNumber": "73175", "lines": [
 *       { "productText": "Gold Basic", "quantity": 1, "options": ["Cumpleaños", "Azul"] }
 *     ] }
 *   ]
 * }
 * ```
 *
 * A flat array of lines is also accepted. Malformed entries are reported in
 * `skipped`, never dropped silently.
 */
export interface OrdersExport {
  deliveryDate?: string
  lines: ParsedOrderLine[]
  skipped: SkippedText[]
}

type RecordValue = Record<string, unknown>

function isRecord (value: unknown): value is RecordValue {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function safeStringify (value: unknown): string {
  try {
    return (JSON.stringify(value) ?? String(value)).slice(0, 300)
  } catch {
    return String(value)
  }
}

function firstString (values: unknown[]): string {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return collapseWhitespace(value)
    if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  }
  return ''
}

function toNumber (value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return null
}

function toOptions (value: unknown, rules: CompiledRules): string[] {
  if (Array.isArray(value)) {
    return value
      .filter((item): item is string => typeof item === 'string')
      .map(item => collapseWhitespace(item))
      .filter(Boolean)
  }
  if (typeof value === 'string') {
    return splitBySeparators(collapseWhitespace(value), rules.optionSeparators)
      .map(part => part.trim())
      .filter(Boolean)
  }
  return []
}

function sourceOf (record: RecordValue): OrderLineSource {
  const source = record.source ?? record.field
  return source === 'add_on' || source === 'additionals' ? 'add_on' : 'breakfast'
}

function lineFromRecord (
  record: RecordValue,
  rules: CompiledRules,
  fallbackOrderNumber: string
): ParsedOrderLine | null {
  const productText = firstString([
    record.productText,
    record.product,
    record.name,
    record.productName,
  ])
  if (!productText) return null

  const quantityValue = toNumber(record.quantity)
  const quantity = quantityValue !== null && quantityValue > 0 ? quantityValue : 1
  const orderNumber = firstString([record.orderNumber, record.order]) || fallbackOrderNumber

  return {
    raw: productText,
    productText,
    quantity,
    options: toOptions(record.options, rules),
    source: sourceOf(record),
    ...(orderNumber ? { orderNumber } : {}),
    ...(quantityValue === null ? { warning: 'missing_quantity' } : {}),
  }
}

export function parseOrdersExport (
  input: unknown,
  rulesInput?: Partial<LabelParsingRules> | null
): OrdersExport {
  const rules = compileRules(rulesInput)
  const lines: ParsedOrderLine[] = []
  const skipped: SkippedText[] = []

  const addRecord = (value: unknown, fallbackOrderNumber = ''): void => {
    if (!isRecord(value)) {
      skipped.push({ raw: safeStringify(value), reason: 'malformed' })
      return
    }
    const orderNumber = firstString([value.orderNumber, value.order]) || fallbackOrderNumber
    const nested = value.lines ?? value.items
    if (Array.isArray(nested)) {
      for (const item of nested) addRecord(item, orderNumber)
      return
    }
    const line = lineFromRecord(value, rules, orderNumber)
    if (line) lines.push(line)
    else skipped.push({ raw: safeStringify(value), reason: 'malformed' })
  }

  let deliveryDate = ''
  if (Array.isArray(input)) {
    for (const item of input) addRecord(item)
  } else if (isRecord(input)) {
    if (typeof input.deliveryDate === 'string') deliveryDate = input.deliveryDate.trim()
    const root = input.lines ?? input.orders
    if (Array.isArray(root)) for (const item of root) addRecord(item)
    else skipped.push({ raw: safeStringify(input), reason: 'malformed' })
  } else {
    skipped.push({ raw: safeStringify(input), reason: 'malformed' })
  }

  return {
    ...(deliveryDate ? { deliveryDate } : {}),
    lines,
    skipped,
  }
}

/**
 * Wide order export: the store's order export plugin writes one row per order,
 * with the breakfast text in `Productos`, add-ons in `Adicionales` and one
 * column per food choice holding a quantity (1, 2, or empty). The column name
 * is the option text, so checked columns feed `ParsedOrderLine.options` and the
 * resolver maps them with the catalog like any other option text.
 */
export const defaultWideExportMetadataColumns = [
  'ID orden',
  'Fecha de Entrega',
  'Productos',
  'Elige los Globos',
  'Fotos',
  'Adicionales',
  'Observaciones',
  'color',
  'stiker',
  'motivo',
  'vegetariano',
  'Tiene Carta',
  'Barrio',
]

export interface WideExportOptions {
  rules?: Partial<LabelParsingRules> | null
  metadataColumns?: string[]
  /** Column that marks the juice choice. */
  juiceColumn?: string
  /**
   * Option texts added when the juice column is not checked. The store export
   * only checks the juice box; its absence means the other drink of the recipe
   * (Hatsu té, Café Mocca). Unmatched texts are ignored by the resolver.
   */
  juiceFallbackOptions?: string[]
}

const spanishMonths: Record<string, string> = {
  enero: '01',
  febrero: '02',
  marzo: '03',
  abril: '04',
  mayo: '05',
  junio: '06',
  julio: '07',
  agosto: '08',
  septiembre: '09',
  setiembre: '09',
  octubre: '10',
  noviembre: '11',
  diciembre: '12',
}

export function parseSpanishDeliveryDate (value: unknown): string | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) {
    // Excel serial date, days since 1899-12-30.
    const date = new Date(Math.round((value - 25569) * 86400 * 1000))
    if (Number.isNaN(date.getTime())) return undefined
    return date.toISOString().slice(0, 10)
  }
  if (typeof value !== 'string') return undefined
  const text = collapseWhitespace(value).toLowerCase()
  const iso = /^(\d{4})-(\d{2})-(\d{2})$/.exec(text)
  if (iso) return `${iso[1]}-${iso[2]}-${iso[3]}`
  const match = /^(\d{1,2})\s*(?:de\s+)?([a-z]+),?\s*(?:de\s+)?(\d{4})$/.exec(text)
  if (!match) return undefined
  const month = spanishMonths[stripAccents(match[2]!)]
  if (!month) return undefined
  return `${match[3]}-${month}-${match[1]!.padStart(2, '0')}`
}

function isPositive (value: unknown): boolean {
  if (typeof value === 'number') return value > 0
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') {
    const text = value.trim().toLowerCase()
    if (!text || text === '0' || text === 'no' || text === 'false') return false
    return true
  }
  return false
}

export function isWideOrderHeader (header: string[]): boolean {
  const normalized = header.map(cell => normalizeName(cell))
  return normalized.includes('id orden') && normalized.includes('productos')
}

export function tableToRecords (rows: string[][]): Record<string, unknown>[] {
  const header = (rows[0] ?? []).map(cell => collapseWhitespace(cell))
  const records: Record<string, unknown>[] = []
  for (const row of rows.slice(1)) {
    const record: Record<string, unknown> = {}
    for (let index = 0; index < header.length; index++) {
      const key = header[index]
      if (key) record[key] = row[index] ?? ''
    }
    records.push(record)
  }
  return records
}

export function parseWideOrderRows (
  rows: Array<Record<string, unknown>>,
  options: WideExportOptions = {}
): OrdersExport {
  const rules = compileRules(options.rules)
  const metadata = new Set(options.metadataColumns ?? defaultWideExportMetadataColumns)
  const juiceColumn = options.juiceColumn ?? 'Jugo de Naranja'
  const juiceFallbacks = options.juiceFallbackOptions ?? ['Hatsu té', 'Café Mocca']
  const lines: ParsedOrderLine[] = []
  const skipped: SkippedText[] = []
  let deliveryDate = ''

  for (const record of rows) {
    const rawOrder = record['ID orden'] ?? record.ID ?? record.orderNumber
    const orderNumber = typeof rawOrder === 'number' ? String(rawOrder) : firstString([rawOrder])
    const context = orderNumber ? { orderNumber } : {}
    if (!deliveryDate) deliveryDate = parseSpanishDeliveryDate(record['Fecha de Entrega']) ?? ''

    const optionTexts: string[] = []
    for (const [key, value] of Object.entries(record)) {
      if (metadata.has(key)) continue
      if (isPositive(value)) optionTexts.push(key)
    }
    if (!isPositive(record[juiceColumn])) optionTexts.push(...juiceFallbacks)

    const breakfast = parseKitchenField(String(record['Productos'] ?? ''), {
      rules,
      source: 'breakfast',
      ...context,
    })
    for (const line of breakfast.lines) {
      lines.push({ ...line, options: [...line.options, ...optionTexts] })
    }
    skipped.push(...breakfast.skipped)

    const additionals = parseKitchenField(String(record['Adicionales'] ?? ''), {
      rules,
      source: 'add_on',
      ...context,
    })
    lines.push(...additionals.lines)
    skipped.push(...additionals.skipped)

    if (breakfast.lines.length === 0 && additionals.lines.length === 0) {
      skipped.push({ raw: safeStringify(record), reason: 'malformed', ...context })
    }
  }

  return {
    ...(deliveryDate ? { deliveryDate } : {}),
    lines,
    skipped,
  }
}

export function detectSeparator (headerLine: string): string {
  const candidates = [',', ';', '\t']
  let best = ','
  let bestCount = -1
  for (const candidate of candidates) {
    const count = headerLine.split(candidate).length - 1
    if (count > bestCount) {
      best = candidate
      bestCount = count
    }
  }
  return best
}

/** RFC 4180 style reader for CSV exports. Quotes, escaped quotes and newlines inside quotes. */
export function parseDelimitedText (text: string, separator?: string): string[][] {
  const headerLine = text.split(/\r?\n/, 1)[0] ?? ''
  const delimiter = separator ?? detectSeparator(headerLine)
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let inQuotes = false

  for (let index = 0; index < text.length; index++) {
    const char = text[index]!
    if (inQuotes) {
      if (char === '"') {
        if (text[index + 1] === '"') {
          field += '"'
          index++
        } else {
          inQuotes = false
        }
      } else {
        field += char
      }
      continue
    }
    if (char === '"') {
      inQuotes = true
      continue
    }
    if (char === delimiter) {
      row.push(field)
      field = ''
      continue
    }
    if (char === '\n') {
      row.push(field)
      rows.push(row)
      row = []
      field = ''
      continue
    }
    if (char === '\r') continue
    field += char
  }
  if (field !== '' || row.length > 0) {
    row.push(field)
    rows.push(row)
  }
  return rows
}
