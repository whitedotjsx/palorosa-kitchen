import type { PanelEntry, PanelEntrySource, PanelList, PanelUnresolved } from './types'

/** One day's list inside a combined several-days view. */
export interface DayList {
  date: string
  list: PanelList
}

function unique (values: string[]): string[] {
  return [...new Set(values)]
}

function tagSources (sources: PanelEntrySource[] | undefined, date: string): PanelEntrySource[] | undefined {
  if (!sources || sources.length === 0) return sources
  return sources.map((source) => ({ ...source, date }))
}

/**
 * Combines consecutive days into one list: quantities add up, every provenance
 * line keeps the day it came from and unresolved lines merge by product, reason
 * and detail. Days must come oldest first so sources stay in calendar order.
 */
export function mergeDayLists (days: DayList[]): PanelList {
  const entries = new Map<string, PanelEntry>()
  const unresolved = new Map<string, PanelUnresolved>()

  for (const { date, list } of days) {
    for (const entry of list.entries ?? []) {
      const sources = tagSources(entry.sources, date)
      const current = entries.get(entry.unitId)
      if (!current) {
        entries.set(entry.unitId, {
          ...entry,
          references: unique(entry.references ?? []),
          ...(sources ? { sources } : {}),
        })
        continue
      }
      current.quantity += entry.quantity
      if (sources) current.sources = [...(current.sources ?? []), ...sources]
      current.references = unique([...(current.references ?? []), ...(entry.references ?? [])])
    }

    for (const item of list.unresolvedItems ?? []) {
      const key = `${item.reason}|${item.productText}|${item.detail ?? ''}`
      const current = unresolved.get(key)
      if (!current) {
        unresolved.set(key, { ...item, references: unique(item.references ?? []) })
        continue
      }
      current.quantity += item.quantity
      current.count += item.count
      current.references = unique([...(current.references ?? []), ...(item.references ?? [])])
    }
  }

  const mergedEntries = [...entries.values()]
  const unresolvedItems = [...unresolved.values()]
  return {
    entries: mergedEntries,
    unresolved: unresolvedItems.length,
    total: mergedEntries.reduce((sum, entry) => sum + entry.quantity, 0),
    unresolvedItems,
  }
}
