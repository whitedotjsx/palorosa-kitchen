import { groupEntriesByCategory } from '../engine/aggregate'
import type { KitchenList } from '../engine/aggregate'
import type { ListDiff } from '../engine/diff'
import { diffLabels, listLabels, unitCategoryLabels, unresolvedReasonLabels } from '../i18n/es'

export interface ListFormatOptions {
  title?: string
  date?: string
  includeUnresolved?: boolean
}

export function formatListText (list: KitchenList, options: ListFormatOptions = {}): string {
  const title = options.title ?? listLabels.title
  const lines: string[] = [options.date ? `${title} · ${options.date}` : title]

  if (list.entries.length === 0) lines.push(listLabels.empty)
  for (const group of groupEntriesByCategory(list)) {
    lines.push('', unitCategoryLabels[group.category])
    for (const entry of group.entries) {
      const note = entry.note ? ` (${entry.note})` : ''
      lines.push(`- ${entry.quantity} x ${entry.name}${note}`)
    }
  }

  if (options.includeUnresolved !== false && list.unresolved.length > 0) {
    lines.push('', listLabels.unresolved)
    for (const entry of list.unresolved) {
      lines.push(`- ${entry.quantity} x ${entry.productText} (${unresolvedReasonLabels[entry.reason]})`)
    }
  }

  return lines.join('\n')
}

export interface DiffFormatOptions {
  title?: string
  date?: string
}

export function formatListDiffText (diff: ListDiff, options: DiffFormatOptions = {}): string {
  const title = options.title ?? diffLabels.title
  const lines: string[] = [options.date ? `${title} · ${options.date}` : title]
  for (const entry of diff.added) lines.push(`+ ${entry.next} x ${entry.name}`)
  for (const entry of diff.changed) lines.push(`~ ${entry.name}: ${entry.previous} → ${entry.next}`)
  for (const entry of diff.removed) lines.push(`- ${entry.previous} x ${entry.name}`)
  if (lines.length === 1) lines.push(diffLabels.empty)
  return lines.join('\n')
}
