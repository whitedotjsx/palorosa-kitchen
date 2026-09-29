import { describe, expect, it } from 'vitest'
import { decodePdfString, extractLabelsFromPdf, extractTextLinesFromContent } from './pdf'

function concatBytes (parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((sum, part) => sum + part.length, 0)
  const output = new Uint8Array(total)
  let offset = 0
  for (const part of parts) {
    output.set(part, offset)
    offset += part.length
  }
  return output
}

async function deflate (text: string): Promise<Uint8Array> {
  const input = new TextEncoder().encode(text)
  const compressor = new CompressionStream('deflate')
  const writer = compressor.writable.getWriter()
  const chunks: Uint8Array[] = []
  const reader = compressor.readable.getReader()
  const read = (async () => {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
    }
  })()
  await writer.write(input)
  await writer.close()
  await read
  return concatBytes(chunks)
}

function textBlock (x: number, y: number, text: string): string {
  return `BT /F1 12 Tf ${x} ${y} Td (${text}) Tj ET`
}

describe('decodePdfString', () => {
  it('resolves escaped parentheses and octal sequences', () => {
    expect(decodePdfString('Calle 19 \\(CCI\\)')).toBe('Calle 19 (CCI)')
    expect(decodePdfString('Cumplea\\361os')).toBe('Cumpleaños')
  })
})

describe('extractTextLinesFromContent', () => {
  it('reads Td/Tj blocks ordered by descending Y', () => {
    const lines = extractTextLinesFromContent(
      ['BT /F1 12 Tf 17 75 Td (Adicionales: x) Tj ET', 'BT /F1 12 Tf 17 194 Td (Pedido: 73175) Tj ET'].join('\n')
    )
    expect(lines.map(line => line.text)).toEqual(['Pedido: 73175', 'Adicionales: x'])
  })

  it('reads Tm/TJ blocks with kerning fragments', () => {
    const lines = extractTextLinesFromContent('BT /F1 12 Tf 1 0 0 1 300 400 Tm [(RUTA)-20( 3:)] TJ ET')
    expect(lines).toHaveLength(1)
    expect(lines[0]?.text).toBe('RUTA 3:')
    expect(lines[0]).toMatchObject({ x: 300, y: 400 })
  })
})

describe('extractLabelsFromPdf', () => {
  it('inflates content streams and groups label fields', async () => {
    const content = [
      textBlock(17, 194, 'Pedido: 73175'),
      textBlock(17, 177, 'Desayuno: Gold Basic X 1: Cumplea\\361os, Azul'),
      textBlock(17, 75, 'Adicionales: Mini torta personal con topper X 1'),
    ].join('\n')
    const deflated = await deflate(content)
    const pdf = concatBytes([
      new TextEncoder().encode('%PDF-1.4\n1 0 obj << /Length 1 >> stream\n'),
      deflated,
      new TextEncoder().encode('\nendstream\nendobj\n%%EOF\n'),
    ])

    const result = await extractLabelsFromPdf(pdf)
    expect(result.labels).toHaveLength(1)
    expect(result.labels[0]).toMatchObject({
      orderNumber: '73175',
      breakfastText: 'Gold Basic X 1: Cumpleaños, Azul',
      additionalsText: 'Mini torta personal con topper X 1',
    })
    expect(result.streams).toBe(1)
  })

  it('deduplicates repeated rótulos and skips streams without labels', async () => {
    const labelContent = [textBlock(17, 194, 'Pedido: 1'), textBlock(17, 177, 'Desayuno: Morning X 1')].join('\n')
    const deflatedLabel = await deflate(labelContent)
    const deflatedJunk = await deflate('0 0 0 rg 10 10 m 20 20 l S')
    const pdf = concatBytes([
      new TextEncoder().encode('%PDF-1.4\n'),
      new TextEncoder().encode('1 0 obj << >> stream\n'),
      deflatedLabel,
      new TextEncoder().encode('\nendstream endobj\n'),
      new TextEncoder().encode('2 0 obj << >> stream\n'),
      deflatedLabel,
      new TextEncoder().encode('\nendstream endobj\n'),
      new TextEncoder().encode('3 0 obj << >> stream\n'),
      deflatedJunk,
      new TextEncoder().encode('\nendstream endobj\n'),
      new TextEncoder().encode('endstream\n%%EOF\n'),
    ])

    const result = await extractLabelsFromPdf(pdf)
    expect(result.labels.map(label => label.orderNumber)).toEqual(['1'])
    expect(result.streams).toBe(2)
    expect(result.skippedStreams).toBe(0)
  })
})
