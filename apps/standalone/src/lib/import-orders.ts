import {
  appLabels,
  extractLabelsFromPdf,
  isWideOrderHeader,
  parseDelimitedText,
  parseLabelFields,
  parseOrdersExport,
  parseWideOrderRows,
  tableToRecords,
  type LabelParsingRules,
  type ParsedOrderLine,
  type SkippedText,
} from '@palorosa-kitchen/core'
import { extractLabelsWithPdfjs } from './pdfjs'
import { readXlsxRows } from './xlsx'

export interface ImportResult {
  lines: ParsedOrderLine[]
  skipped: SkippedText[]
  deliveryDate?: string
  source: string
  pdfFallback: boolean
  unreadableStreams: number
}

async function importPdf (file: File, rules: LabelParsingRules): Promise<ImportResult> {
  const buffer = await file.arrayBuffer()
  const extraction = await extractLabelsFromPdf(new Uint8Array(buffer))
  let labels = extraction.labels
  let pdfFallback = false

  if (labels.length === 0) {
    try {
      labels = await extractLabelsWithPdfjs(buffer)
      pdfFallback = true
    } catch {
      throw new Error(appLabels.pdfFailed)
    }
  }
  if (labels.length === 0) throw new Error(appLabels.pdfEmpty)

  const lines: ParsedOrderLine[] = []
  const skipped: SkippedText[] = []
  for (const label of labels) {
    const parsed = parseLabelFields(label, rules)
    lines.push(...parsed.lines)
    skipped.push(...parsed.skipped)
  }

  return {
    lines,
    skipped,
    source: file.name,
    pdfFallback,
    unreadableStreams: extraction.skippedStreams,
  }
}

async function importJson (file: File, rules: LabelParsingRules): Promise<ImportResult> {
  let parsed: unknown
  try {
    parsed = JSON.parse(await file.text())
  } catch {
    throw new Error(appLabels.jsonFailed)
  }

  const result = parseOrdersExport(parsed, rules)
  if (result.lines.length === 0 && result.skipped.length === 0) throw new Error(appLabels.jsonEmpty)

  return {
    lines: result.lines,
    skipped: result.skipped,
    ...(result.deliveryDate ? { deliveryDate: result.deliveryDate } : {}),
    source: file.name,
    pdfFallback: false,
    unreadableStreams: 0,
  }
}

function importTable (rows: string[][], rules: LabelParsingRules, source: string): ImportResult {
  if (!isWideOrderHeader((rows[0] ?? []).map(cell => String(cell)))) {
    throw new Error(appLabels.unsupportedTable)
  }
  const result = parseWideOrderRows(tableToRecords(rows), { rules })
  if (result.lines.length === 0 && result.skipped.length === 0) {
    throw new Error(appLabels.jsonEmpty)
  }
  return {
    lines: result.lines,
    skipped: result.skipped,
    ...(result.deliveryDate ? { deliveryDate: result.deliveryDate } : {}),
    source,
    pdfFallback: false,
    unreadableStreams: 0,
  }
}

export async function importOrdersFile (
  file: File,
  rules: LabelParsingRules
): Promise<ImportResult> {
  const name = file.name.toLowerCase()
  if (name.endsWith('.pdf')) return importPdf(file, rules)
  if (name.endsWith('.json')) return importJson(file, rules)
  if (name.endsWith('.csv')) {
    return importTable(parseDelimitedText(await file.text()), rules, file.name)
  }
  if (name.endsWith('.xlsx')) {
    let rows: string[][]
    try {
      rows = await readXlsxRows(await file.arrayBuffer())
    } catch {
      throw new Error(appLabels.xlsxFailed)
    }
    return importTable(rows, rules, file.name)
  }
  throw new Error(appLabels.unsupportedFile)
}
