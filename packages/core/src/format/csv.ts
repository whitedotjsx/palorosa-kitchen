import { groupEntriesByCategory } from '../engine/aggregate'
import type { KitchenList } from '../engine/aggregate'
import {
  listLabels,
  measureLabels,
  unitCategoryLabels,
  unresolvedReasonLabels,
} from '../i18n/es'

export interface CsvFormatOptions {
  separator?: string
  includeUnresolved?: boolean
}

export function escapeCsvField (value: string, separator: string): string {
  if (value.includes('"') || value.includes(separator) || /[\r\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`
  }
  return value
}

export function formatListCsv (list: KitchenList, options: CsvFormatOptions = {}): string {
  const separator = options.separator ?? ','
  const rows: string[][] = [
    [listLabels.category, listLabels.item, listLabels.quantity, listLabels.measure, listLabels.note],
  ]

  for (const group of groupEntriesByCategory(list)) {
    for (const entry of group.entries) {
      rows.push([
        unitCategoryLabels[group.category],
        entry.name,
        String(entry.quantity),
        measureLabels[entry.measure],
        entry.note ?? '',
      ])
    }
  }

  if (options.includeUnresolved === true) {
    for (const entry of list.unresolved) {
      rows.push([
        listLabels.unresolved,
        entry.productText,
        String(entry.quantity),
        '',
        unresolvedReasonLabels[entry.reason],
      ])
    }
  }

  return rows
    .map(row => row.map(field => escapeCsvField(field, separator)).join(separator))
    .join('\n')
}
