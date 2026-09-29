import { labelsFromLines, type LabelText } from '@palorosa-kitchen/core'
import * as pdfjsLib from 'pdfjs-dist/legacy/build/pdf.mjs'
// The worker ships as text and becomes a Blob URL at runtime: the single HTML
// file has no URL of its own for pdf.js to fetch a worker from.
import pdfWorkerSource from 'pdfjs-dist/legacy/build/pdf.worker.mjs?raw'

interface TextItem {
  str?: string
  transform?: number[]
  width?: number
}

export async function extractLabelsWithPdfjs (data: ArrayBuffer): Promise<LabelText[]> {
  if (!pdfjsLib.GlobalWorkerOptions.workerSrc) {
    const blob = new Blob([pdfWorkerSource], { type: 'text/javascript' })
    pdfjsLib.GlobalWorkerOptions.workerSrc = URL.createObjectURL(blob)
  }

  const pdf = await pdfjsLib.getDocument({ data }).promise
  const labels: LabelText[] = []

  for (let pageNumber = 1; pageNumber <= pdf.numPages; pageNumber++) {
    const page = await pdf.getPage(pageNumber)
    const textContent = await page.getTextContent()

    const items: { x: number, y: number, width: number, text: string }[] = []
    for (const raw of textContent.items) {
      const item = raw as unknown as TextItem
      const str = item.str
      const transform = item.transform
      if (!str?.trim() || !transform) continue
      items.push({
        x: transform[4] ?? 0,
        y: transform[5] ?? 0,
        width: item.width ?? 0,
        text: str,
      })
    }

    const sorted = [...items].sort((a, b) => b.y - a.y || a.x - b.x)
    const lines: { y: number, text: string, endX: number }[] = []
    const Y_TOLERANCE = 2
    const GAP_TOLERANCE = 1.5
    for (const item of sorted) {
      const last = lines[lines.length - 1]
      if (last && Math.abs(last.y - item.y) < Y_TOLERANCE) {
        const gap = item.x - last.endX
        last.text += (gap > GAP_TOLERANCE ? ' ' : '') + item.text
        last.endX = item.x + item.width
      } else {
        lines.push({ y: item.y, text: item.text, endX: item.x + item.width })
      }
    }

    labels.push(...labelsFromLines(lines.map(line => line.text.trim()).filter(Boolean)))
  }

  pdf.destroy().catch(() => {})
  return labels
}
