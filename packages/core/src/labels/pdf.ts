/**
 * Lightweight rótulos PDF reader.
 *
 * Rótulos are generated with a very regular layout: one card per stream, one
 * text block per field line. This reader only needs to inflate the PDF
 * content streams (`DecompressionStream`, web standard) and pull the text
 * lines out, so it carries no PDF engine. PDFs with a more complex internal
 * structure are handled by the pdf.js fallback in the consumer app.
 */

import { labelsFromLines } from './label-text'
import type { LabelText } from './label-text'

export interface TextLine {
  x: number
  y: number
  text: string
}

export interface PdfExtractionResult {
  labels: LabelText[]
  streams: number
  skippedStreams: number
}

const BT_BLOCK_PATTERN = /BT([\s\S]*?)ET/g
const TD_PATTERN = /(-?[\d.]+)\s+(-?[\d.]+)\s+Td\b/
const TM_PATTERN = /(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+Tm\b/
const PDF_STRING_PATTERN = /\(((?:[^()\\]|\\.)*)\)/g
const TJ_PATTERN = /\(((?:[^()\\]|\\.)*)\)\s*Tj/g
const TJ_ARRAY_PATTERN = /\[((?:[^[\]\\]|\\.)*)\]\s*TJ/g

const textEncoder = new TextEncoder()
const latin1Decoder = new TextDecoder('latin1')

function indexOfSequence (haystack: Uint8Array, needle: string, from: number): number {
  const target = textEncoder.encode(needle)
  for (let i = from; i <= haystack.length - target.length; i++) {
    let match = true
    for (let j = 0; j < target.length; j++) {
      if (haystack[i + j] !== target[j]) {
        match = false
        break
      }
    }
    if (match) return i
  }
  return -1
}

function trimTrailingWhitespace (bytes: Uint8Array): Uint8Array {
  let end = bytes.length
  while (end > 0) {
    const byte = bytes[end - 1]!
    if (byte === 0x0a || byte === 0x0d || byte === 0x20 || byte === 0x09) end--
    else break
  }
  return bytes.subarray(0, end)
}

async function inflate (bytes: Uint8Array): Promise<Uint8Array | null> {
  const trimmed = trimTrailingWhitespace(bytes)
  if (trimmed.length === 0) return null

  for (const format of ['deflate', 'deflate-raw'] as const) {
    try {
      const decompressor = new DecompressionStream(format)
      const writer = decompressor.writable.getWriter()
      const chunk = trimmed as unknown as Parameters<typeof writer.write>[0]
      writer.write(chunk).catch(() => {})
      writer.close().catch(() => {})

      const reader = decompressor.readable.getReader()
      const chunks: Uint8Array[] = []
      let total = 0
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        chunks.push(value)
        total += value.length
      }

      const output = new Uint8Array(total)
      let offset = 0
      for (const chunk of chunks) {
        output.set(chunk, offset)
        offset += chunk.length
      }
      return output
    } catch {
      // Wrong format or a non Flate stream (image, font). Try the next one.
    }
  }
  return null
}

export function decodePdfString (raw: string): string {
  return raw
    .replace(/\\([0-7]{1,3})/g, (_, oct: string) => String.fromCharCode(Number.parseInt(oct, 8)))
    .replace(/\\n/g, ' ')
    .replace(/\\r/g, ' ')
    .replace(/\\t/g, ' ')
    .replace(/\\([()\\])/g, '$1')
}

export function extractTextLinesFromContent (content: string): TextLine[] {
  const lines: TextLine[] = []
  BT_BLOCK_PATTERN.lastIndex = 0
  let blockMatch: RegExpExecArray | null
  while ((blockMatch = BT_BLOCK_PATTERN.exec(content)) !== null) {
    const block = blockMatch[1]!
    const tm = TM_PATTERN.exec(block)
    const td = TD_PATTERN.exec(block)
    const x = tm ? Number(tm[5]) : td ? Number(td[1]) : null
    const y = tm ? Number(tm[6]) : td ? Number(td[2]) : null
    if (x === null || y === null) continue

    let text = ''
    TJ_PATTERN.lastIndex = 0
    let match: RegExpExecArray | null
    while ((match = TJ_PATTERN.exec(block)) !== null) {
      text += decodePdfString(match[1] ?? '')
    }
    TJ_ARRAY_PATTERN.lastIndex = 0
    while ((match = TJ_ARRAY_PATTERN.exec(block)) !== null) {
      PDF_STRING_PATTERN.lastIndex = 0
      let stringMatch: RegExpExecArray | null
      while ((stringMatch = PDF_STRING_PATTERN.exec(match[1]!)) !== null) {
        text += decodePdfString(stringMatch[1] ?? '')
      }
    }
    if (text) lines.push({ x, y, text })
  }

  // PDF Y grows upwards: larger Y comes first.
  return lines.sort((a, b) => b.y - a.y || a.x - b.x)
}

export async function extractLabelsFromPdf (bytes: Uint8Array): Promise<PdfExtractionResult> {
  const labels: LabelText[] = []
  const seenOrders = new Set<string>()
  let streams = 0
  let skippedStreams = 0
  let cursor = 0

  while (true) {
    const streamStart = indexOfSequence(bytes, 'stream', cursor)
    if (streamStart === -1) break
    let dataStart = streamStart + 'stream'.length
    if (bytes[dataStart] === 0x0d) dataStart++
    if (bytes[dataStart] === 0x0a) dataStart++

    const streamEnd = indexOfSequence(bytes, 'endstream', dataStart)
    if (streamEnd === -1) break
    cursor = streamEnd + 'endstream'.length

    const inflated = await inflate(bytes.subarray(dataStart, streamEnd))
    if (!inflated) continue

    const content = latin1Decoder.decode(inflated)
    if (!content.includes('Tj') && !content.includes('TJ')) continue

    const lines = extractTextLinesFromContent(content)
    if (lines.length === 0) continue

    streams++
    const streamLabels = labelsFromLines(lines.map(line => line.text))
    if (streamLabels.length === 0) {
      skippedStreams++
      continue
    }
    for (const label of streamLabels) {
      if (label.orderNumber) {
        if (seenOrders.has(label.orderNumber)) continue
        seenOrders.add(label.orderNumber)
      }
      labels.push(label)
    }
  }

  return { labels, streams, skippedStreams }
}
