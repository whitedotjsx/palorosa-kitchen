import {
  formatKitchenTemplateHtml,
  kitchenPrintTemplate,
  type KitchenList,
  type PrintTemplate,
  type UnresolvedReason,
} from '@palorosa-kitchen/core'
import logo from '../assets/palorosa-logo.png'
import { api } from './api'
import type { PanelList } from './types'

/** Deep copy of the built-in layout, safe to edit. */
export function defaultTemplate (): PrintTemplate {
  return JSON.parse(JSON.stringify(kitchenPrintTemplate)) as PrintTemplate
}

/** The saved layout, or the built-in one when nothing was saved. */
export async function loadTemplate (): Promise<{ template: PrintTemplate; custom: boolean }> {
  try {
    const data = await api<{ template: PrintTemplate | null }>('/api/panel/print-template')
    if (data.template && Array.isArray(data.template.left) && Array.isArray(data.template.right)) {
      return { template: data.template, custom: true }
    }
  } catch {
    // Fall back to the default layout.
  }
  return { template: defaultTemplate(), custom: false }
}

/** Saves the layout; null restores the built-in one. */
export async function saveTemplate (template: PrintTemplate | null): Promise<void> {
  await api('/api/panel/print-template', { method: 'PUT', body: JSON.stringify({ template }) })
}

function toKitchenList (list: PanelList): KitchenList {
  return {
    entries: list.entries.map((entry) => ({
      unitId: entry.unitId,
      name: entry.name,
      measure: entry.measure,
      category: entry.category,
      note: entry.note || undefined,
      quantity: entry.quantity,
      references: [],
    })) as KitchenList['entries'],
    unresolved: (list.unresolvedItems ?? []).map((entry) => ({
      productText: entry.productText,
      quantity: entry.quantity,
      count: 1,
      reason: entry.reason as UnresolvedReason,
      references: [],
    })) as KitchenList['unresolved'],
    ignoredProducts: [],
    ignoredUnits: [],
    warnings: [],
  }
}

/** Renders the printable sheet HTML for a list. */
export function sheetHtml (list: PanelList, template: PrintTemplate, date?: string): string {
  return formatKitchenTemplateHtml(toKitchenList(list), date ? { template, date, logo } : { template, logo })
}

/**
 * Opens the browser print dialog for the sheet. "Guardar como PDF" in that
 * dialog produces the PDF; nothing else needs to be installed.
 */
export function printSheet (html: string): void {
  const frame = document.createElement('iframe')
  frame.setAttribute('aria-hidden', 'true')
  frame.style.cssText = 'position:fixed;right:0;bottom:0;width:0;height:0;border:0;visibility:hidden'
  frame.srcdoc = html
  frame.onload = () => {
    const view = frame.contentWindow
    if (!view) return
    const cleanup = () => setTimeout(() => frame.remove(), 500)
    view.addEventListener('afterprint', cleanup, { once: true })
    // Images (the logo) must finish decoding before printing.
    const images = Array.from(frame.contentDocument?.images ?? [])
    Promise.all(images.map((image) => image.decode().catch(() => undefined)))
      .then(() => {
        view.focus()
        view.print()
      })
      .catch(() => undefined)
  }
  document.body.appendChild(frame)
}
