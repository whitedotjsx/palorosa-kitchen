/**
 * Minimal, dependency-free XLSX reader for Node. Same scope as the browser
 * reader in the standalone app (shared strings, first worksheet, cell values)
 * but with a regex XML pass instead of `DOMParser`, which Node lacks.
 * ZIP is inflated with `DecompressionStream('deflate-raw')` (Node 18+).
 */

const EOCD_SIGNATURE = 0x06054b50
const CENTRAL_SIGNATURE = 0x02014b50
const LOCAL_SIGNATURE = 0x04034b50

interface ZipEntry {
  method: number
  compressedSize: number
  offset: number
}

function view (bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
}

function findEndOfCentralDirectory (bytes: Uint8Array): number {
  const data = view(bytes)
  const min = Math.max(0, bytes.length - 65557)
  for (let index = bytes.length - 22; index >= min; index--) {
    if (data.getUint32(index, true) === EOCD_SIGNATURE) return index
  }
  throw new Error('xlsx: end of central directory not found')
}

function readEntries (bytes: Uint8Array): Map<string, ZipEntry> {
  const data = view(bytes)
  const decoder = new TextDecoder()
  const eocd = findEndOfCentralDirectory(bytes)
  const count = data.getUint16(eocd + 10, true)
  let offset = data.getUint32(eocd + 16, true)
  const entries = new Map<string, ZipEntry>()
  for (let index = 0; index < count; index++) {
    if (data.getUint32(offset, true) !== CENTRAL_SIGNATURE) break
    const method = data.getUint16(offset + 10, true)
    const compressedSize = data.getUint32(offset + 20, true)
    const nameLength = data.getUint16(offset + 28, true)
    const extraLength = data.getUint16(offset + 30, true)
    const commentLength = data.getUint16(offset + 32, true)
    const localOffset = data.getUint32(offset + 42, true)
    const name = decoder.decode(bytes.subarray(offset + 46, offset + 46 + nameLength))
    entries.set(name, { method, compressedSize, offset: localOffset })
    offset += 46 + nameLength + extraLength + commentLength
  }
  return entries
}

async function inflateRaw (bytes: Uint8Array): Promise<Uint8Array> {
  const decompressor = new DecompressionStream('deflate-raw')
  const writer = decompressor.writable.getWriter()
  await writer.write(bytes as Uint8Array<ArrayBuffer>)
  await writer.close()
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
  for (const part of chunks) {
    output.set(part, offset)
    offset += part.length
  }
  return output
}

async function readEntry (bytes: Uint8Array, entry: ZipEntry): Promise<Uint8Array> {
  const data = view(bytes)
  if (data.getUint32(entry.offset, true) !== LOCAL_SIGNATURE) throw new Error('xlsx: bad local header')
  const nameLength = data.getUint16(entry.offset + 26, true)
  const extraLength = data.getUint16(entry.offset + 28, true)
  const start = entry.offset + 30 + nameLength + extraLength
  const payload = bytes.subarray(start, start + entry.compressedSize)
  if (entry.method === 0) return payload
  if (entry.method === 8) return inflateRaw(payload)
  throw new Error(`xlsx: unsupported compression method ${entry.method}`)
}

function decodeEntities (text: string): string {
  return text.replace(/&(#x?[0-9a-fA-F]+|amp|lt|gt|quot|apos);/g, (_all, entity: string) => {
    if (entity === 'amp') return '&'
    if (entity === 'lt') return '<'
    if (entity === 'gt') return '>'
    if (entity === 'quot') return '"'
    if (entity === 'apos') return "'"
    if (entity[0] === '#') {
      const code = entity[1] === 'x' || entity[1] === 'X' ? Number.parseInt(entity.slice(2), 16) : Number.parseInt(entity.slice(1), 10)
      return Number.isFinite(code) ? String.fromCodePoint(code) : ''
    }
    return ''
  })
}

function sharedStrings (xml: string): string[] {
  return [...xml.matchAll(/<si>([\s\S]*?)<\/si>/g)].map((match) =>
    [...(match[1] ?? '').matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)].map((text) => decodeEntities(text[1] ?? '')).join('')
  )
}

function columnIndex (reference: string): number {
  const letters = /^([A-Z]+)/.exec(reference)?.[1] ?? 'A'
  let index = 0
  for (const char of letters) index = index * 26 + (char.charCodeAt(0) - 64)
  return index - 1
}

function sheetRows (xml: string, strings: string[]): string[][] {
  const rows: string[][] = []
  for (const rowMatch of xml.matchAll(/<row[^>]*\br="(\d+)"[^>]*>([\s\S]*?)<\/row>/g)) {
    const rowIndex = Number(rowMatch[1]) - 1
    const body = rowMatch[2] ?? ''
    const cells: string[] = []
    let position = 0
    for (const cellMatch of body.matchAll(/<c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/c>)/g)) {
      const attrs = cellMatch[1] ?? ''
      const inner = cellMatch[2] ?? ''
      const reference = /\br="([A-Z]+)\d+"/.exec(attrs)?.[1]
      const type = /\bt="([^"]+)"/.exec(attrs)?.[1]
      const index = reference ? columnIndex(reference) : position
      let value = ''
      if (type === 's') {
        const shared = /<v>([\s\S]*?)<\/v>/.exec(inner)?.[1]
        value = strings[Number(shared ?? '-1')] ?? ''
      } else if (type === 'inlineStr') {
        value = [...inner.matchAll(/<t[^>]*>([\s\S]*?)<\/t>/g)].map((text) => decodeEntities(text[1] ?? '')).join('')
      } else {
        value = decodeEntities(/<v>([\s\S]*?)<\/v>/.exec(inner)?.[1] ?? '')
      }
      while (cells.length < index) cells.push('')
      cells[index] = value
      position = index + 1
    }
    while (rows.length < rowIndex) rows.push([])
    rows[rowIndex] = cells
  }
  return rows
}

export async function readXlsxRows (data: Uint8Array): Promise<string[][]> {
  const entries = readEntries(data)
  const decoder = new TextDecoder()
  const readText = async (path: string): Promise<string> => {
    const entry = entries.get(path)
    if (!entry) throw new Error(`xlsx: missing ${path}`)
    return decoder.decode(await readEntry(data, entry))
  }

  let sheetPath = 'xl/worksheets/sheet1.xml'
  if (entries.has('xl/workbook.xml') && entries.has('xl/_rels/workbook.xml.rels')) {
    const workbook = await readText('xl/workbook.xml')
    const relationId = /<sheet\b[^>]*\br:id="([^"]+)"/.exec(workbook)?.[1]
    if (relationId) {
      const rels = await readText('xl/_rels/workbook.xml.rels')
      const relationship = new RegExp(`<Relationship\\b[^>]*\\bId="${relationId}"[^>]*>`).exec(rels)?.[0]
      const target = relationship ? /\bTarget="([^"]+)"/.exec(relationship)?.[1] : undefined
      if (target) sheetPath = target.startsWith('/') ? target.slice(1) : `xl/${target.replace(/^\.\//, '')}`
    }
  }

  const strings = entries.has('xl/sharedStrings.xml') ? sharedStrings(await readText('xl/sharedStrings.xml')) : []
  return sheetRows(await readText(sheetPath), strings)
}
