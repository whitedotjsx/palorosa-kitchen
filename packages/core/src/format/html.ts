import { groupEntriesByCategory } from '../engine/aggregate'
import type { KitchenList } from '../engine/aggregate'
import {
  listLabels,
  measureLabels,
  unitCategoryLabels,
  unresolvedReasonLabels,
} from '../i18n/es'

export interface HtmlFormatOptions {
  title?: string
  date?: string
  includeUnresolved?: boolean
}

export function escapeHtml (text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function formatListHtml (list: KitchenList, options: HtmlFormatOptions = {}): string {
  const title = escapeHtml(options.title ?? listLabels.title)
  const dateLine = options.date
    ? `<p class="date">${escapeHtml(listLabels.deliveryDate)}: ${escapeHtml(options.date)}</p>`
    : ''

  const sections = groupEntriesByCategory(list)
    .map((group) => {
      const rows = group.entries
        .map(entry => `<tr>
        <td class="quantity">${entry.quantity}</td>
        <td>${escapeHtml(entry.name)}</td>
        <td class="measure">${escapeHtml(measureLabels[entry.measure])}</td>
        <td class="note">${escapeHtml(entry.note ?? '')}</td>
      </tr>`)
        .join('\n')
      return `<h2>${escapeHtml(unitCategoryLabels[group.category])}</h2>
    <table>
      <thead><tr><th>${escapeHtml(listLabels.quantity)}</th><th>${escapeHtml(listLabels.item)}</th><th>${escapeHtml(listLabels.measure)}</th><th>${escapeHtml(listLabels.note)}</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>`
    })
    .join('\n')

  const unresolved = options.includeUnresolved !== false && list.unresolved.length > 0
    ? `<section class="unresolved">
    <h2>${escapeHtml(listLabels.unresolved)}</h2>
    <ul>${list.unresolved
      .map(entry => `<li><span class="quantity">${entry.quantity}</span> ${escapeHtml(entry.productText)} <span class="reason">${escapeHtml(unresolvedReasonLabels[entry.reason])}</span></li>`)
      .join('')}</ul>
  </section>`
    : ''

  return `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>${title}</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, "Segoe UI", sans-serif; color: #1a1a1a; margin: 0; padding: 24px; }
  h1 { font-size: 20px; margin: 0 0 4px; }
  .date { color: #555; margin: 0 0 16px; }
  h2 { font-size: 14px; text-transform: uppercase; letter-spacing: 0.04em; color: #8a1c1c; margin: 18px 0 6px; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; font-size: 11px; text-transform: uppercase; color: #777; border-bottom: 1px solid #ccc; padding: 2px 6px 4px 0; }
  td { padding: 4px 6px 4px 0; border-bottom: 1px solid #eee; vertical-align: top; }
  td.quantity { font-weight: 600; width: 60px; }
  td.measure, td.note { color: #555; font-size: 12px; }
  .unresolved { margin-top: 24px; border-top: 2px dashed #c9a227; padding-top: 8px; }
  .unresolved h2 { color: #7a5b00; }
  .unresolved ul { margin: 0; padding-left: 18px; }
  .unresolved .quantity { font-weight: 600; }
  .unresolved .reason { color: #7a5b00; font-size: 12px; }
  @media print {
    body { padding: 0; }
    @page { size: letter; margin: 12mm; }
  }
</style>
</head>
<body>
<h1>${title}</h1>
${dateLine}
${list.entries.length === 0 ? `<p>${escapeHtml(listLabels.empty)}</p>` : sections}
${unresolved}
</body>
</html>`
}
