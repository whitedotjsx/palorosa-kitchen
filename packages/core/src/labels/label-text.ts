export interface LabelText {
  orderNumber: string
  breakfastText: string
  additionalsText: string
  rawLines: string[]
}

export const labelFieldPrefixes = [
  'Pedido:',
  'Barrio:',
  'Quien recibe:',
  'Teléfono de quien recibe:',
  'Teléfono de quien compró:',
  'Desayuno:',
  'Adicionales:',
  'Observaciones:',
] as const

function startsWithField (text: string): boolean {
  return labelFieldPrefixes.some(prefix => text.startsWith(prefix))
}

function fieldText (lines: string[], prefix: string): string {
  const start = lines.findIndex(line => line.startsWith(prefix))
  if (start === -1) return ''
  const parts = [lines[start]!.slice(prefix.length).trim()]
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i]!
    if (startsWithField(line)) break
    parts.push(line.trim())
  }
  return parts.filter(Boolean).join(' ')
}

export function labelsFromLines (lines: string[]): LabelText[] {
  const groups: string[][] = []
  for (const line of lines) {
    const text = line.trim()
    if (!text) continue
    if (text.startsWith('Pedido:') || groups.length === 0) groups.push([text])
    else groups[groups.length - 1]!.push(text)
  }

  return groups
    .map(group => ({
      orderNumber: fieldText(group, 'Pedido:'),
      breakfastText: fieldText(group, 'Desayuno:'),
      additionalsText: fieldText(group, 'Adicionales:'),
      rawLines: group,
    }))
    .filter(label => label.orderNumber || label.breakfastText || label.additionalsText)
}

export function labelsFromPages (pages: string[][]): LabelText[] {
  return pages.flatMap(page => labelsFromLines(page))
}
