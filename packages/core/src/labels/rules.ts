import type { LabelParsingRules } from '../schema'

export const defaultLabelParsingRules: LabelParsingRules = {
  quantityPatterns: ['^(?<name>.+?)\\s+X\\s+(?<quantity>\\d+)(?:\\s*[:,]\\s*(?<options>.*))?$'],
  productSeparators: ['|'],
  optionSeparators: [',', '|'],
  noisePatterns: [
    '^DOMICILIO EXCLUSIVO',
    '^Observaciones:',
    '^es oficina',
    '^es un local',
  ],
}

export interface CompiledRules {
  quantityPatterns: RegExp[]
  productSeparators: string[]
  optionSeparators: string[]
  noisePatterns: RegExp[]
}

function safeRegex (source: string, flags = ''): RegExp | null {
  try {
    return new RegExp(source, flags)
  } catch {
    return null
  }
}

function choose (values: string[] | undefined, fallback: string[]): string[] {
  return values && values.length > 0 ? [...values] : [...fallback]
}

export function resolveRules (
  rules?: Partial<LabelParsingRules> | null
): LabelParsingRules {
  return {
    quantityPatterns: choose(rules?.quantityPatterns, defaultLabelParsingRules.quantityPatterns),
    productSeparators: choose(rules?.productSeparators, defaultLabelParsingRules.productSeparators),
    optionSeparators: choose(rules?.optionSeparators, defaultLabelParsingRules.optionSeparators),
    noisePatterns: choose(rules?.noisePatterns, defaultLabelParsingRules.noisePatterns),
  }
}

export function compileRules (rules?: Partial<LabelParsingRules> | null): CompiledRules {
  const resolved = resolveRules(rules)
  return {
    quantityPatterns: resolved.quantityPatterns
      .map(source => safeRegex(source))
      .filter((pattern): pattern is RegExp => pattern !== null),
    productSeparators: resolved.productSeparators,
    optionSeparators: resolved.optionSeparators,
    noisePatterns: resolved.noisePatterns
      .map(source => safeRegex(source, 'i'))
      .filter((pattern): pattern is RegExp => pattern !== null),
  }
}
