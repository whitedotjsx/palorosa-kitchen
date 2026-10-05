import {
  aggregateUnits,
  indexCatalog,
  parseWideOrderRows,
  resolveLines,
  tableToRecords,
  type Catalog,
  type KitchenList,
  type LabelParsingRules,
  type ParsedOrderLine,
  type SkippedText,
} from '@palorosa-kitchen/core'
import { toStoreDate } from './date'
import type { OrdersExportConfig } from './env'
import { WpAllExportClient } from './wp'

export * from './date'
export * from './env'
export { WpAllExportClient } from './wp'

export interface DayListResult {
  list: KitchenList
  lines: ParsedOrderLine[]
  skipped: SkippedText[]
  /** Number of distinct orders in the export. */
  orderCount: number
}

export function rowsToLines (
  rows: string[][],
  rules: LabelParsingRules
): { lines: ParsedOrderLine[], skipped: SkippedText[] } {
  const parsed = parseWideOrderRows(tableToRecords(rows), { rules })
  return { lines: parsed.lines, skipped: parsed.skipped }
}

/** Pulls the raw wide-export rows for one delivery date. */
export async function exportOrdersRows (config: OrdersExportConfig, isoDate: string): Promise<string[][]> {
  return new WpAllExportClient(config).exportRows(toStoreDate(isoDate))
}

/** Pulls the day's orders and aggregates them into the kitchen list. */
export async function buildDayList (
  config: OrdersExportConfig,
  isoDate: string,
  catalog: Catalog
): Promise<DayListResult> {
  const rows = await exportOrdersRows(config, isoDate)
  const { lines, skipped } = rowsToLines(rows, catalog.labelParsing)
  const index = indexCatalog(catalog)
  const list = aggregateUnits(resolveLines(lines, index), index)
  const orderCount = new Set(
    lines.map((line) => line.orderNumber).filter((value): value is string => Boolean(value))
  ).size
  return { list, lines, skipped, orderCount }
}

/** Publishes a computed list to the team server, same endpoint the app uses. */
export async function publishList (
  serverUrl: string,
  deliveryDate: string,
  list: KitchenList,
  source: string,
  orderCount: number
): Promise<void> {
  const response = await fetch(`${serverUrl.replace(/\/+$/, '')}/api/list`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      deliveryDate,
      source,
      orderCount,
      entries: list.entries,
      unresolved: list.unresolved,
    }),
  })
  if (!response.ok) throw new Error(`Publish failed: HTTP ${response.status}`)
}
