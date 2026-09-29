import type { CatalogIndex } from '../catalog'
import { normalizeName } from '../normalize'
import type { IgnoredReason, MeasureUnit, UnitCategory, UnresolvedReason } from '../schema'
import type { ResolvedLine } from './resolve'

export const unitCategoryOrder: UnitCategory[] = [
  'drink',
  'main',
  'side',
  'dessert',
  'fruit',
  'condiment',
  'other',
]

export interface KitchenListEntry {
  unitId: string
  name: string
  measure: MeasureUnit
  category: UnitCategory
  note?: string
  quantity: number
  references: string[]
}

export interface UnresolvedEntry {
  productText: string
  quantity: number
  count: number
  reason: UnresolvedReason
  detail?: string
  references: string[]
}

export interface IgnoredEntry {
  id: string
  name: string
  quantity: number
  count: number
  reason: IgnoredReason
  references: string[]
}

export interface WarningEntry {
  raw: string
  productText: string
  quantity: number
  count: number
  references: string[]
}

export interface KitchenList {
  entries: KitchenListEntry[]
  unresolved: UnresolvedEntry[]
  ignoredProducts: IgnoredEntry[]
  ignoredUnits: IgnoredEntry[]
  warnings: WarningEntry[]
}

function referenceOf (line: ResolvedLine): string {
  return line.line.orderNumber ?? line.line.raw
}

function pushReference (references: string[], reference: string): void {
  if (reference && !references.includes(reference)) references.push(reference)
}

function addEntry (
  entries: Map<string, KitchenListEntry>,
  unitId: string,
  fields: Omit<KitchenListEntry, 'quantity' | 'references'>,
  quantity: number,
  reference: string
): void {
  const existing = entries.get(unitId)
  if (existing) {
    existing.quantity += quantity
    pushReference(existing.references, reference)
    return
  }
  entries.set(unitId, {
    ...fields,
    quantity,
    references: reference ? [reference] : [],
  })
}

function addIgnored (
  entries: Map<string, IgnoredEntry>,
  id: string,
  name: string,
  reason: IgnoredReason,
  quantity: number,
  reference: string
): void {
  const key = `${reason}|${id}`
  const existing = entries.get(key)
  if (existing) {
    existing.quantity += quantity
    existing.count += 1
    pushReference(existing.references, reference)
    return
  }
  entries.set(key, {
    id,
    name,
    quantity,
    count: 1,
    reason,
    references: reference ? [reference] : [],
  })
}

function addUnresolved (entries: Map<string, UnresolvedEntry>, line: ResolvedLine): void {
  const reason = line.unresolvedReason ?? 'unknown_product'
  const key = `${reason}|${line.unresolvedDetail ?? ''}|${normalizeName(line.line.productText)}`
  const reference = referenceOf(line)
  const existing = entries.get(key)
  if (existing) {
    existing.quantity += line.line.quantity
    existing.count += 1
    pushReference(existing.references, reference)
    return
  }
  entries.set(key, {
    productText: line.line.productText,
    quantity: line.line.quantity,
    count: 1,
    reason,
    ...(line.unresolvedDetail ? { detail: line.unresolvedDetail } : {}),
    references: reference ? [reference] : [],
  })
}

function addWarning (entries: Map<string, WarningEntry>, line: ResolvedLine): void {
  const reference = referenceOf(line)
  const existing = entries.get(line.line.raw)
  if (existing) {
    existing.quantity += line.line.quantity
    existing.count += 1
    pushReference(existing.references, reference)
    return
  }
  entries.set(line.line.raw, {
    raw: line.line.raw,
    productText: line.line.productText,
    quantity: line.line.quantity,
    count: 1,
    references: reference ? [reference] : [],
  })
}

function byCategoryThenName (a: KitchenListEntry, b: KitchenListEntry): number {
  const categoryDiff = unitCategoryOrder.indexOf(a.category) - unitCategoryOrder.indexOf(b.category)
  if (categoryDiff !== 0) return categoryDiff
  return a.name.localeCompare(b.name, 'es')
}

export function aggregateUnits (resolved: ResolvedLine[], index: CatalogIndex): KitchenList {
  const entries = new Map<string, KitchenListEntry>()
  const unresolvedEntries = new Map<string, UnresolvedEntry>()
  const ignoredProducts = new Map<string, IgnoredEntry>()
  const ignoredUnits = new Map<string, IgnoredEntry>()
  const warnings = new Map<string, WarningEntry>()

  for (const line of resolved) {
    if (line.line.warning) addWarning(warnings, line)

    if (line.status === 'ignored') {
      addIgnored(
        ignoredProducts,
        line.productId ?? line.line.productText,
        line.productName ?? line.line.productText,
        line.ignoredReason ?? 'not_kitchen',
        line.line.quantity,
        referenceOf(line)
      )
      continue
    }
    if (line.status === 'unresolved') {
      // Known contributions of a partially resolved line still reach the list.
      addUnresolved(unresolvedEntries, line)
    }

    for (const contribution of line.contributions) {
      const unit = index.unitsById.get(contribution.unitId)
      const reference = referenceOf(line)
      if (!unit) {
        // A missing unit is already reported by the resolver.
        continue
      }
      if (!unit.isKitchen || !unit.active) {
        addIgnored(
          ignoredUnits,
          unit.id,
          unit.name,
          unit.isKitchen ? 'inactive_unit' : 'not_kitchen',
          contribution.quantity,
          reference
        )
        continue
      }
      addEntry(
        entries,
        unit.id,
        {
          unitId: unit.id,
          name: unit.name,
          measure: unit.measure,
          category: unit.category,
          ...(unit.note ? { note: unit.note } : {}),
        },
        contribution.quantity,
        reference
      )
    }
  }

  return {
    entries: [...entries.values()].sort(byCategoryThenName),
    unresolved: [...unresolvedEntries.values()].sort((a, b) =>
      a.reason.localeCompare(b.reason) || a.productText.localeCompare(b.productText, 'es')
    ),
    ignoredProducts: [...ignoredProducts.values()].sort((a, b) => a.name.localeCompare(b.name, 'es')),
    ignoredUnits: [...ignoredUnits.values()].sort((a, b) => a.name.localeCompare(b.name, 'es')),
    warnings: [...warnings.values()],
  }
}

export function groupEntriesByCategory (list: KitchenList): { category: UnitCategory, entries: KitchenListEntry[] }[] {
  const groups: { category: UnitCategory, entries: KitchenListEntry[] }[] = []
  for (const entry of list.entries) {
    const last = groups[groups.length - 1]
    if (last && last.category === entry.category) last.entries.push(entry)
    else groups.push({ category: entry.category, entries: [entry] })
  }
  return groups
}
