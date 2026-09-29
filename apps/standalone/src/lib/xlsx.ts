/**
 * Minimal XLSX reader: ZIP central directory + `DecompressionStream` for the
 * deflated parts, `DOMParser` for the XML. No dependencies. Only what the
 * store order export needs: shared strings, the first worksheet, cell values.
 */

const EOCD_SIGNATURE = 0x06054b50
const CENTRAL_SIGNATURE = 0x02014b50
const LOCAL_SIGNATURE = 0x04034b50

interface ZipEntry {
  method: number
  compressedSize: number
  offset: number
}

function dataView (bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
}

function findEndOfCentralDirectory (bytes: Uint8Array): number {
  const view = dataView(bytes)
  const min = Math.max(0, bytes.length - 65557)
  for (let index = bytes.length - 22; index >= min; index--) {
    if (view.getUint32(index, true) === EOCD_SIGNATURE) return index
  }
  throw new Error('xlsx: end of central directory not found')
}

function readEntries (bytes: Uint8Array): Map<string, ZipEntry> {
  const view = dataView(bytes)
  const decoder = new TextDecoder()
  const eocd = findEndOfCentralDirectory(bytes)
  const count = view.getUint16(eocd + 10, true)
  let offset = view.getUint32(eocd + 16, true)
  const entries = new Map<string, ZipEntry>()

  for (let index = 0; index < count; index++) {
    if (view.getUint32(offset, true) !== CENTRAL_SIGNATURE) break
    const method = view.getUint16(offset + 10, true)
    const compressedSize = view.getUint32(offset + 20, true)
    const nameLength = view.getUint16(offset + 28, true)
    const extraLength = view.getUint16(offset + 30, true)
    const commentLength = view.getUint16(offset + 32, true)
    const localOffset = view.getUint32(offset + 42, true)
    const name = decoder.decode(bytes.subarray(offset + 46, offset + 46 + nameLength))
    entries.set(name, { method, compressedSize, offset: localOffset })
    offset += 46 + nameLength + extraLength + commentLength
  }
  return entries
}

async function inflateRaw (bytes: Uint8Array): Promise<Uint8Array> {
  const decompressor = new DecompressionStream('deflate-raw')
  const writer = decompressor.writable.getWriter()
  const chunk = bytes as unknown as Parameters<typeof writer.write>[0]
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
  for (const part of chunks) {
    output.set(part, offset)
    offset += part.length
  }
  return output
}

async function readEntry (bytes: Uint8Array, entry: ZipEntry): Promise<Uint8Array> {
  const view = dataView(bytes)
  if (view.getUint32(entry.offset, true) !== LOCAL_SIGNATURE) {
    throw new Error('xlsx: bad local header')
  }
  const nameLength = view.getUint16(entry.offset + 26, true)
  const extraLength = view.getUint16(entry.offset + 28, true)
  const start = entry.offset + 30 + nameLength + extraLength
  const data = bytes.subarray(start, start + entry.compressedSize)
  if (entry.method === 0) return data
  if (entry.method === 8) return inflateRaw(data)
  throw new Error(`xlsx: unsupported compression method ${entry.method}`)
}

function parseXml (text: string): Document {
  const document = new DOMParser().parseFromString(text, 'application/xml')
  if (document.getElementsByTagName('parsererror').length > 0) {
    throw new Error('xlsx: malformed XML')
  }
  return document
}

function sharedStrings (document: Document): string[] {
  return [...document.getElementsByTagName('si')].map(item =>
    [...item.getElementsByTagName('t')].map(text => text.textContent ?? '').join('')
  )
}

function columnIndex (reference: string): number {
  const letters = /^([A-Z]+)/.exec(reference)?.[1] ?? 'A'
  let index = 0
  for (const char of letters) index = index * 26 + (char.charCodeAt(0) - 64)
  return index - 1
}

function sheetRows (document: Document, strings: string[]): string[][] {
  const rows: string[][] = []
  for (const rowElement of document.getElementsByTagName('row')) {
    const rowIndex = Number(rowElement.getAttribute('r') ?? '0') - 1
    const cells: string[] = []
    for (const cell of rowElement.getElementsByTagName('c')) {
      const index = columnIndex(cell.getAttribute('r') ?? 'A1')
      const type = cell.getAttribute('t')
      let value = ''
      if (type === 's') {
        const stringIndex = Number(cell.getElementsByTagName('v')[0]?.textContent ?? '-1')
        value = strings[stringIndex] ?? ''
      } else if (type === 'inlineStr') {
        value = [...cell.getElementsByTagName('t')].map(text => text.textContent ?? '').join('')
      } else {
        value = cell.getElementsByTagName('v')[0]?.textContent ?? ''
      }
      while (cells.length < index) cells.push('')
      cells[index] = value
    }
    while (rows.length < rowIndex) rows.push([])
    rows[rowIndex] = cells
  }
  return rows
}

export async function readXlsxRows (data: ArrayBuffer): Promise<string[][]> {
  const bytes = new Uint8Array(data)
  const entries = readEntries(bytes)
  const decoder = new TextDecoder()

  const readText = async (path: string): Promise<string> => {
    const entry = entries.get(path)
    if (!entry) throw new Error(`xlsx: missing ${path}`)
    return decoder.decode(await readEntry(bytes, entry))
  }

  let sheetPath = 'xl/worksheets/sheet1.xml'
  const workbookEntry = entries.get('xl/workbook.xml')
  const relsEntry = entries.get('xl/_rels/workbook.xml.rels')
  if (workbookEntry && relsEntry) {
    const workbook = parseXml(await readText('xl/workbook.xml'))
    const sheet = workbook.getElementsByTagName('sheet')[0]
    const relationId = sheet?.getAttribute('r:id')
    if (relationId) {
      const relationships = parseXml(await readText('xl/_rels/workbook.xml.rels'))
      const relationship = [...relationships.getElementsByTagName('Relationship')]
        .find(item => item.getAttribute('Id') === relationId)
      const target = relationship?.getAttribute('Target')
      if (target) sheetPath = target.startsWith('/') ? target.slice(1) : `xl/${target.replace(/^\.\//, '')}`
    }
  }

  const stringsEntry = entries.get('xl/sharedStrings.xml')
  const strings = stringsEntry ? sharedStrings(parseXml(decoder.decode(await readEntry(bytes, stringsEntry)))) : []
  const sheet = parseXml(await readText(sheetPath))
  return sheetRows(sheet, strings)
}
