import type { KitchenList } from '../engine/aggregate'
import { templateLabels, unresolvedReasonLabels } from '../i18n/es'
import { escapeHtml } from './html'

/**
 * Printed kitchen sheet (the paper template the kitchen fills by hand today).
 * Labels are simplified on purpose, so every row maps manually to one or more
 * catalog unit ids. Units with no row land in the extra "OTROS" section.
 */
export interface TemplateSubRow {
  key: string
  label: string
  unitIds?: string[]
}

export interface TemplateBlock {
  key: string
  label: string
  unitIds?: string[]
  subRows?: TemplateSubRow[]
}

export interface PrintTemplate {
  left: TemplateBlock[]
  right: TemplateBlock[]
}

export const kitchenPrintTemplate: PrintTemplate = {
  left: [
    { key: 'jugo-pequeno', label: 'JUGO PEQUEÑO', unitIds: ['jugo-de-naranja-semi-natural-pequeno'] },
    { key: 'jugo-grande', label: 'JUGO GRANDE', unitIds: ['jugo-de-naranja-semi-natural-grande'] },
    {
      key: 'jugo-100',
      label: 'JUGO 100% NATURAL',
      unitIds: [
        'jugo-de-naranja-100-natural-0-azucar',
        'jugo-de-naranja-100-natural',
        'jugo-de-naranja-natural-grande',
      ],
    },
    { key: 'yogurt-pequeno', label: 'YOGURT PEQUEÑO', unitIds: ['yogurt-pequeno'] },
    { key: 'yogurt-grande', label: 'YOGURT GRANDE', unitIds: ['yogurt-grande'] },
    {
      key: 'kumis',
      label: 'KUMIS',
      subRows: [
        { key: 'kumis-grande', label: 'GRANDE', unitIds: ['kumis-grande'] },
        { key: 'kumis-pequeno', label: 'PEQUEÑO', unitIds: ['kumis-pequeno'] },
      ],
    },
    { key: 'jugo-classic', label: 'JUGO CLASSIC', unitIds: ['jugo-de-lata'] },
    { key: 'smoothie', label: 'SMOOTHIE FRUTOS ROJOS', unitIds: ['smoothie-de-frutos-rojos'] },
    { key: 'jugo-verde', label: 'JUGO VERDE', unitIds: ['jugo-verde', 'jugo-verde-grande'] },
    { key: 'sandwich-sencillo', label: 'SANDWICH SENCILLO', unitIds: ['sandwich-sencillo'] },
    {
      key: 'sandwich-croissant',
      label: 'SANDWICH CROISSANT',
      subRows: [
        { key: 'bolso', label: 'BOLSO', unitIds: ['sandwich-en-croissant-para-bolso'] },
        { key: 'mini-box', label: 'MINI BOX', unitIds: ['sandwich-mini-box'] },
        { key: 'glow', label: 'GLOW', unitIds: ['sandwich-en-croissant-para-glow'] },
        { key: 'brunch-chill', label: 'BRUNCH CHILL', unitIds: ['sandwich-en-croissant-para-brunch-chill'] },
        { key: 'classic', label: 'CLASSIC:', unitIds: ['sandwich-classic'] },
        { key: 'cristal', label: 'CRISTAL:', unitIds: ['sandwich-en-croissant-para-cristal'] },
        { key: 'maleta-pro', label: 'MALETA PRO', unitIds: ['sandwich-en-croissant-para-maleta-pro'] },
        { key: 'morning', label: 'MORNING', unitIds: ['sandwich-en-croissant-para-morning'] },
        { key: 'libro', label: 'LIBRO', unitIds: ['sandwich-en-croissant-para-libro'] },
      ],
    },
    { key: 'sandwich-saludable', label: 'SANDWICH SALUDABLE', unitIds: ['sandwich-saludable'] },
    {
      key: 'waffle-pequeno-arequipe',
      label: 'WAFFLE PEQUEÑO AREQUIPE',
      unitIds: ['waffle-con-arequipe-y-fresas'],
    },
    { key: 'waffle-pequeno-morning', label: 'WAFFLE PEQUEÑO MORNING', unitIds: ['waffle-suelto'] },
    {
      key: 'waffle-grande-arequipe',
      label: 'WAFFLE GRANDE AREQUIPE',
      unitIds: ['waffles-con-arequipe-y-fresas-porcion-grande'],
    },
    {
      key: 'waffle-cofres-nutella',
      label: 'WAFFLE COFRES NUTELLA',
      unitIds: ['waffles-con-nutella-fresas', 'waffle-con-nutella', 'waffle-para-cofres-de-nutella'],
    },
    {
      key: 'quesadillas-nutella',
      label: 'QUESADILLAS DE NUTELLA',
      unitIds: ['quesadillas-de-nutella-con-fresas-y-banano', 'quesadilla-de-nutella'],
    },
    { key: 'pancakes-maleta-pro', label: 'PANCAKES MALETA PRO', unitIds: ['pancakes-para-maleta-pro'] },
    {
      key: 'mini-pancakes',
      label: 'MINI PANCAKES',
      subRows: [
        { key: 'mini-pancakes-nino', label: 'NIÑO', unitIds: ['mini-pancakes-con-fresas-y-nutella'] },
        { key: 'mini-pancakes-glow', label: 'GLOW', unitIds: ['mini-pancakes-para-glow'] },
        { key: 'mini-pancakes-game-box', label: 'GAME BOX', unitIds: [] },
      ],
    },
  ],
  right: [
    { key: 'parfait', label: 'PARFAIT', unitIds: ['parfait'] },
    { key: 'parfait-classic', label: 'PARFAIT CLASSIC', unitIds: ['parfait-classic'] },
    { key: 'bowl-copa-cristal', label: 'BOWL COPA CRISTAL', unitIds: ['bowl-en-copa-de-cristal'] },
    {
      key: 'bowl-pequeno',
      label: 'BOWL PEQUEÑO',
      subRows: [
        { key: 'bowl-morning', label: 'MORNING', unitIds: ['bowl-pequeno-de-morning'] },
        { key: 'bowl-brunch', label: 'BRUNCH', unitIds: ['bowl-pequeno-de-brunch'] },
        { key: 'bowl-glow', label: 'GLOW', unitIds: ['bowl-pequeno-de-glow'] },
        { key: 'bowl-maleta-pro', label: 'MALETA PRO', unitIds: ['bowl-para-maleta-pro'] },
        { key: 'bowl-mujer', label: 'MUJER', unitIds: ['bowl-mujer'] },
      ],
    },
    { key: 'bowl-saludable', label: 'BOWL SALUDABLE', unitIds: ['bowl-saludable'] },
    { key: 'bowl-libro', label: 'BOWL LIBRO', unitIds: ['bowl-grande'] },
    { key: 'quesos-frasco-largo', label: 'QUESOS FRASCO LARGO', unitIds: ['frasco-largo-de-quesos'] },
    { key: 'quesos-casita', label: 'QUESOS CASITA', unitIds: ['quesos-casita'] },
    {
      key: 'pinchos-quesos',
      label: 'PINCHOS DE QUESOS',
      unitIds: [
        'un-pincho-de-queso-mozzarella-y-chorizo-espanol',
        'un-pincho-de-queso-mozzarella-chorizo-espanol-y-aceituna',
      ],
    },
    {
      key: 'tablas-de-quesos',
      label: 'TABLAS DE QUESOS',
      subRows: [
        { key: 'tabla-morning', label: 'MORNING', unitIds: ['tablas-de-quesos-para-morning'] },
        { key: 'tabla-brunch', label: 'BRUNCH', unitIds: ['tablas-de-quesos-para-brunch'] },
        { key: 'tabla-cristal', label: 'CRISTAL (BARQUITO)', unitIds: ['tablas-de-quesos-barquito'] },
        {
          key: 'tabla-libro',
          label: 'LIBRO',
          unitIds: ['mini-tabla-de-madera-con-quesos-jamon-y-chocolate-blanco'],
        },
        { key: 'tabla-picnic', label: 'PICNIC', unitIds: ['tablas-de-quesos-para-picnic'] },
      ],
    },
    { key: 'cristal-zona-dulce', label: 'CRISTAL ZONA DULCE', unitIds: ['cristal-zone-dulce'] },
    { key: 'cristal-zona-sal', label: 'CRISTAL ZONA SAL', unitIds: ['cristal-zone-salado'] },
    { key: 'mocca-grande', label: 'MOCCA GRANDE', unitIds: ['mocca-grande', 'cafe-mocca'] },
    { key: 'arepas-rancheras', label: 'AREPAS RANCHERAS', unitIds: ['arepas-rancheras'] },
    {
      key: 'arepas-pollo',
      label: 'AREPAS DE POLLO',
      unitIds: ['arepas-de-pollo', 'arepa-asada-con-pechuga-asada-aguacate-y-parmesano'],
    },
    { key: 'deditos-queso', label: 'DEDITOS DE QUESO', unitIds: ['deditos-de-queso'] },
    { key: 'empanaditas', label: 'EMPANADITAS', unitIds: ['empanaditas'] },
    { key: 'tinto-luxury', label: 'TINTO LUXURY', unitIds: [] },
    { key: 'fruta-grande', label: 'FRUTA GRANDE', unitIds: ['ensalada-de-frutas-grande'] },
    { key: 'fruta-pequena', label: 'FRUTA PEQUEÑA', unitIds: ['ensalada-de-frutas-pequena'] },
  ],
}

export interface TemplateFormatOptions {
  template?: PrintTemplate
  date?: string
  /** Data URI for the header logo (Palorosa brand mark). */
  logo?: string
}

function blockUnitIds (block: TemplateBlock): string[] {
  if (block.subRows) return block.subRows.flatMap(row => row.unitIds ?? [])
  return block.unitIds ?? []
}

function renderBlock (block: TemplateBlock, quantities: ReadonlyMap<string, number>): string {
  const sum = (unitIds: string[] | undefined): number =>
    (unitIds ?? []).reduce((total, unitId) => total + (quantities.get(unitId) ?? 0), 0)

  if (block.subRows) {
    const cells = block.subRows
      .map((row, index) => {
        const quantity = sum(row.unitIds)
        const value = quantity > 0 ? String(quantity) : ''
        const label = index === 0
          ? `<td class="label" rowspan="${block.subRows!.length}">${escapeHtml(block.label)}</td>`
          : ''
        return `<tr>${label}<td class="sub"><span>${escapeHtml(row.label)}</span><span class="inline-qty">${value}</span></td></tr>`
      })
      .join('')
    return cells
  }

  const quantity = sum(block.unitIds)
  const value = quantity > 0 ? String(quantity) : ''
  return `<tr><td class="label">${escapeHtml(block.label)}</td><td class="qty">${value}</td></tr>`
}

export function formatKitchenTemplateHtml (
  list: KitchenList,
  options: TemplateFormatOptions = {}
): string {
  const template = options.template ?? kitchenPrintTemplate
  const quantities = new Map(list.entries.map(entry => [entry.unitId, entry.quantity]))
  const mapped = new Set([...template.left, ...template.right].flatMap(blockUnitIds))
  const extras = list.entries.filter(entry => !mapped.has(entry.unitId))

  const leftTable = `<table class="panel">${template.left.map(block => renderBlock(block, quantities)).join('')}</table>`
  const rightTable = `<table class="panel">${template.right.map(block => renderBlock(block, quantities)).join('')}</table>`

  const extrasSection = extras.length > 0
    ? `<section class="extra">
  <h2>${escapeHtml(templateLabels.others)}</h2>
  <table class="extra-table">${extras
    .map(entry => `<tr><td>${escapeHtml(entry.name)}</td><td class="qty">${entry.quantity}</td></tr>`)
    .join('')}</table>
</section>`
    : ''

  const unresolvedSection = list.unresolved.length > 0
    ? `<section class="extra">
  <h2>${escapeHtml(templateLabels.unresolved)}</h2>
  <table class="extra-table">${list.unresolved
    .map(entry => `<tr><td>${escapeHtml(entry.productText)} <span class="reason">${escapeHtml(unresolvedReasonLabels[entry.reason])}</span></td><td class="qty">${entry.quantity}</td></tr>`)
    .join('')}</table>
</section>`
    : ''

  const dateLine = `<header class="sheet-header"><span class="title">${escapeHtml(templateLabels.title)}</span>${options.date ? `<span class="date">${escapeHtml(options.date)}</span>` : ''}</header>`
  const logo = options.logo
    ? `<div class="sheet-footer"><img class="logo" src="${escapeHtml(options.logo)}" alt=""></div>`
    : ''

  return `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>${escapeHtml(templateLabels.title)}</title>
<style>
  @page { size: letter portrait; margin: 0; }
  * { box-sizing: border-box; }
  body { font-family: Arial, sans-serif; margin: 0; padding: 0.375in; color: #6a4b20; }
  .sheet-header { display: flex; width: 744px; justify-content: space-between; align-items: center; margin: 0 0 10px; }
  .sheet-header .title, .sheet-header .date { background: #f6d5cd; color: #6a4b20; font-size: 12pt; font-weight: bold; padding: 2px 8px; }
  .sheet-footer { position: fixed; right: 10px; bottom: 10px; text-align: right; }
  .sheet-footer .logo { width: 36px; height: 36px; display: inline-block; vertical-align: bottom; }
  .sheet { display: flex; width: 744px; justify-content: space-between; align-items: flex-start; }
  table { border-collapse: collapse; table-layout: fixed; }
  table.panel { width: 344px; }
  table.panel td { border: 1px solid #c4b3aa; height: 24px; vertical-align: middle; }
  table.panel td.label { width: 200px; font-size: 9pt; font-weight: bold; text-align: center; padding: 2px 4px; }
  table.panel td.sub { width: 144px; font-size: 8.5pt; padding-left: 8px; position: relative; }
  td.sub span { display: inline-block; }
  td.sub .inline-qty { position: absolute; right: 8px; top: 50%; transform: translateY(-50%); font-weight: bold; }
  td.qty { text-align: center; font-weight: bold; }
  .extra { margin-top: 18px; }
  .extra h2 { font-size: 10pt; margin: 0 0 4px; color: #a55d26; }
  table.extra-table { width: 344px; }
  table.extra-table td { border: 1px solid #c4b3aa; font-size: 9pt; padding: 3px 6px; height: 20px; }
  table.extra-table td.qty { width: 60px; text-align: center; font-weight: bold; }
  .reason { font-weight: normal; font-size: 8pt; color: #8a6a54; }
</style>
</head>
<body>
${dateLine}
<div class="sheet">
${leftTable}
${rightTable}
</div>
${extrasSection}
${unresolvedSection}
${logo}
</body>
</html>`
}
