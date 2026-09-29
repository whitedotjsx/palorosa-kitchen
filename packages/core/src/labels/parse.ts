import type { LabelParsingRules } from '../schema'
import { compileRules } from './rules'
import type { CompiledRules } from './rules'

export type OrderLineSource = 'breakfast' | 'add_on'

export interface ParsedOrderLine {
  raw: string
  productText: string
  quantity: number
  options: string[]
  source: OrderLineSource
  orderNumber?: string
  warning?: 'missing_quantity'
}

export interface SkippedText {
  raw: string
  reason: 'noise' | 'malformed'
  orderNumber?: string
}

export interface ParsedField {
  lines: ParsedOrderLine[]
  skipped: SkippedText[]
}

export interface LabelFields {
  orderNumber?: string
  breakfastText?: string
  additionalsText?: string
}

export function collapseWhitespace (text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

function escapeRegExp (text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function splitBySeparators (text: string, separators: string[]): string[] {
  if (!text) return []
  if (separators.length === 0) return [text]
  const pattern = new RegExp(separators.map(escapeRegExp).join('|'))
  return text.split(pattern)
}

function isNoise (text: string, rules: CompiledRules): boolean {
  return rules.noisePatterns.some(pattern => pattern.test(text))
}

function matchQuantity (
  text: string,
  rules: CompiledRules
): { productText: string, quantity: number, options: string[] } | null {
  for (const pattern of rules.quantityPatterns) {
    const match = pattern.exec(text)
    const groups = match?.groups
    if (!groups) continue
    const productText = collapseWhitespace(groups.name ?? '')
    const quantity = Number.parseInt(groups.quantity ?? '', 10)
    if (!productText || !Number.isFinite(quantity) || quantity <= 0) continue
    const options = splitBySeparators(collapseWhitespace(groups.options ?? ''), rules.optionSeparators)
      .map(option => option.trim())
      .filter(Boolean)
    return { productText, quantity, options }
  }
  return null
}

export interface ParseFieldOptions {
  rules?: CompiledRules | null
  source?: OrderLineSource
  orderNumber?: string
}

function contextFields (orderNumber?: string): { orderNumber: string } | Record<string, never> {
  return orderNumber ? { orderNumber } : {}
}

export function parseKitchenField (text: string, options: ParseFieldOptions = {}): ParsedField {
  const rules = options.rules ?? compileRules()
  const source = options.source ?? 'breakfast'
  const context = contextFields(options.orderNumber)
  const lines: ParsedOrderLine[] = []
  const skipped: SkippedText[] = []

  for (const part of splitBySeparators(collapseWhitespace(text), rules.productSeparators)) {
    const raw = part.trim()
    if (!raw) continue
    if (isNoise(raw, rules)) {
      skipped.push({ raw, reason: 'noise', ...context })
      continue
    }
    const parsed = matchQuantity(raw, rules)
    if (!parsed) {
      lines.push({
        raw,
        productText: raw,
        quantity: 1,
        options: [],
        source,
        warning: 'missing_quantity',
        ...context,
      })
      continue
    }
    lines.push({
      raw,
      productText: parsed.productText,
      quantity: parsed.quantity,
      options: parsed.options,
      source,
      ...context,
    })
  }

  return { lines, skipped }
}

export function parseLabelFields (
  fields: LabelFields,
  rules?: Partial<LabelParsingRules> | null
): ParsedField {
  const compiled = compileRules(rules)
  const breakfast = parseKitchenField(fields.breakfastText ?? '', {
    rules: compiled,
    source: 'breakfast',
    ...contextFields(fields.orderNumber),
  })
  const additionals = parseKitchenField(fields.additionalsText ?? '', {
    rules: compiled,
    source: 'add_on',
    ...contextFields(fields.orderNumber),
  })
  return {
    lines: [...breakfast.lines, ...additionals.lines],
    skipped: [...breakfast.skipped, ...additionals.skipped],
  }
}
