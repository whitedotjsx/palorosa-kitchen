import {
  normalizeName,
  slugify,
  type Product,
  type ProductCategory,
} from '@palorosa-kitchen/core'
import { classifyName } from './classify'
import { findExactName, findByLooseName } from './match'
import type { SourceBreakfast, SourceStoreProduct } from './source-types'
import type { WpProduct, WpSnapshot } from './wp-types'

const EVALUATION_CATEGORY_MAP: Record<string, ProductCategory> = {
  Flores: 'flower',
  Peluches: 'stuffed_animal',
  Accesorios: 'accessory',
  'Detalles y Cajas': 'gift_box',
  Bolsos: 'bag',
  Adicionales: 'add_on',
}

const WP_CATEGORY_MAP: [string, ProductCategory][] = [
  ['Flores', 'flower'],
  ['Palorosa Flowers', 'flower'],
  ['Peluches', 'stuffed_animal'],
  ['Puffy Bag', 'bag'],
  ['Diario', 'accessory'],
  ['Accesorios', 'accessory'],
  ['Adicionales', 'add_on'],
  ['Detalles', 'gift_box'],
]

const CATEGORY_ORDER: ProductCategory[] = [
  'breakfast',
  'add_on',
  'flower',
  'stuffed_animal',
  'accessory',
  'gift_box',
  'bag',
]

const BREAKFAST_NAME_PATTERN =
  /\b(box|canasta|cofre|gold|luxury|morning|saludable|parejas|glow|cristal|libro|maleta|bolso|brunch|classic|picnic|level up|game box|endulzada|querida)\b/i
const NON_BREAKFAST_PATTERN =
  /photobook|ramo|rosas|flores|bouquet|peluche|oso|globo|burbuja|vaso|joyero|foto|diario|lapicero|suculenta|tote|puffy/i
const BALLOON_PATTERN = /globo|burbuja|helio/
const SHIPPING_NAMES = new Set(['domicilio exclusivo'])

export interface ProductDuplicateMerge {
  keptId: string
  keptName: string
  mergedName: string
}

export interface ExcludedWpProduct {
  id: number
  name: string
  reason: string
}

export interface BuildProductsInput {
  breakfasts: Record<string, SourceBreakfast>
  additions: Record<string, string>
  additionPrices: Record<string, number>
  storeProducts: Record<string, SourceStoreProduct>
  cajaSaludDrinks: Record<string, string>
  wpSnapshot: WpSnapshot | null
}

export interface BuildProductsResult {
  products: Product[]
  duplicateMerges: ProductDuplicateMerge[]
  missingInWp: { id: string; name: string }[]
  excludedWp: ExcludedWpProduct[]
}

type IdFactory = (name: string) => string

function createIdFactory (used = new Set<string>()): IdFactory {
  return (name: string): string => {
    const base = slugify(name) || 'product'
    let id = base
    let counter = 2
    while (used.has(id)) {
      id = `${base}-${counter}`
      counter += 1
    }
    used.add(id)
    return id
  }
}

function titleCase (key: string): string {
  return key
    .split('_')
    .map((word) =>
      word.length <= 3 ? word : word.charAt(0).toUpperCase() + word.slice(1).toLowerCase()
    )
    .join(' ')
}

function cleanHtml (value: string | undefined): string {
  return (value ?? '')
    .replace(/<[^>]*>/g, ' ')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/\s+/g, ' ')
    .trim()
}

function unique (values: string[]): string[] {
  return [...new Set(values.filter(Boolean))]
}

function sortProducts (products: Product[]): Product[] {
  return [...products].sort(
    (a, b) =>
      CATEGORY_ORDER.indexOf(a.category) - CATEGORY_ORDER.indexOf(b.category) ||
      a.name.localeCompare(b.name)
  )
}

function buildEvaluationProducts (
  input: BuildProductsInput,
  uniqueId: IdFactory
): { products: Product[]; duplicateMerges: ProductDuplicateMerge[] } {
  const duplicateMerges: ProductDuplicateMerge[] = []
  const breakfasts: Product[] = []
  const storeProducts: Product[] = []

  for (const breakfast of Object.values(input.breakfasts)) {
    const containerId = slugify(breakfast.container)
    breakfasts.push({
      id: uniqueId(breakfast.name),
      name: breakfast.name,
      aliases: [breakfast.name],
      category: 'breakfast',
      isKitchen: true,
      description: breakfast.description,
      ...(containerId ? { containerId } : {}),
      active: true,
    })
  }

  for (const storeProduct of Object.values(input.storeProducts)) {
    storeProducts.push({
      id: uniqueId(storeProduct.name),
      name: storeProduct.name,
      aliases: [storeProduct.name],
      category: EVALUATION_CATEGORY_MAP[storeProduct.category] ?? 'add_on',
      isKitchen: classifyName(storeProduct.name) === 'food',
      priceCop: storeProduct.price,
      description: storeProduct.description,
      active: true,
    })
  }

  const additions: Product[] = Object.entries(input.additions).map(([key, description]) => {
    const name = titleCase(key)
    const price = input.additionPrices[description]
    return {
      id: uniqueId(name),
      name,
      aliases: [name],
      category: 'add_on',
      isKitchen: classifyName(`${name} ${description}`) === 'food',
      ...(typeof price === 'number' ? { priceCop: price } : {}),
      description,
      active: true,
    }
  })

  for (const addition of additions) {
    const match = findByLooseName(addition.name, storeProducts, (product) => product.name)
    if (match) {
      if (!match.aliases.includes(addition.name)) match.aliases.push(addition.name)
      if (addition.isKitchen) match.isKitchen = true
      duplicateMerges.push({ keptId: match.id, keptName: match.name, mergedName: addition.name })
    } else {
      storeProducts.push(addition)
    }
  }

  for (const [key, name] of Object.entries(input.cajaSaludDrinks)) {
    storeProducts.push({
      id: uniqueId(name),
      name,
      aliases: [name],
      category: 'add_on',
      isKitchen: true,
      description: `Drink option for Caja Salud (${key})`,
      active: true,
    })
  }

  return { products: [...breakfasts, ...storeProducts], duplicateMerges }
}

function deriveWpCategory (wp: WpProduct): ProductCategory {
  const names = (wp.categories ?? []).map((category) => category.name)
  const normalized = normalizeName(wp.name)
  if (
    names.includes('Desayunos') &&
    BREAKFAST_NAME_PATTERN.test(normalized) &&
    !NON_BREAKFAST_PATTERN.test(normalized)
  ) {
    return 'breakfast'
  }
  for (const [wpName, category] of WP_CATEGORY_MAP) {
    if (names.includes(wpName)) return category
  }
  return classifyName(wp.name) === 'food' ? 'add_on' : 'gift_box'
}

function buildWpProduct (
  wp: WpProduct,
  evaluationMatch: Product | null,
  category: ProductCategory,
  uniqueId: IdFactory
): Product {
  const price = Number.parseFloat(wp.price)
  const description = cleanHtml(wp.short_description) || cleanHtml(wp.description)
  const isKitchen =
    category === 'breakfast' ||
    evaluationMatch?.isKitchen === true ||
    (category === 'add_on' && classifyName(wp.name) === 'food')
  return {
    id: uniqueId(wp.name),
    name: wp.name,
    aliases: unique([
      wp.name,
      ...(evaluationMatch ? [evaluationMatch.name, ...evaluationMatch.aliases] : []),
    ]),
    category,
    isKitchen,
    ...(Number.isFinite(price) && price > 0 ? { priceCop: Math.round(price) } : {}),
    ...(description ? { description } : {}),
    ...(evaluationMatch?.containerId ? { containerId: evaluationMatch.containerId } : {}),
    wpProductId: wp.id,
    ...(wp.sku ? { sku: wp.sku } : {}),
    active: wp.status === 'publish',
  }
}

export function buildProducts (input: BuildProductsInput): BuildProductsResult {
  const evaluation = buildEvaluationProducts(input, createIdFactory())
  if (!input.wpSnapshot) {
    return {
      products: sortProducts(evaluation.products),
      duplicateMerges: evaluation.duplicateMerges,
      missingInWp: [],
      excludedWp: [],
    }
  }

  const wpIdFactory = createIdFactory()
  const wpProducts: Product[] = []
  const excludedWp: ExcludedWpProduct[] = []
  const matchedEvaluationIds = new Set<string>()

  for (const wp of input.wpSnapshot.products) {
    if (wp.type === 'variation') continue
    const normalized = normalizeName(wp.name)
    if (BALLOON_PATTERN.test(normalized)) {
      excludedWp.push({ id: wp.id, name: wp.name, reason: 'balloons are out of scope (D6)' })
      continue
    }
    if (SHIPPING_NAMES.has(normalized.trim())) {
      excludedWp.push({ id: wp.id, name: wp.name, reason: 'shipping product, not food' })
      continue
    }
    const derived = deriveWpCategory(wp)
    const exactMatch = findExactName(wp.name, evaluation.products, (product) => product.name)
    const looseMatch = exactMatch
      ? null
      : findByLooseName(wp.name, evaluation.products, (product) => product.name)
    const evaluationMatch =
      exactMatch ?? (looseMatch && looseMatch.category === derived ? looseMatch : null)
    if (evaluationMatch) matchedEvaluationIds.add(evaluationMatch.id)
    const category = evaluationMatch?.category ?? derived
    wpProducts.push(buildWpProduct(wp, evaluationMatch, category, wpIdFactory))
  }

  const usedIds = new Set(wpProducts.map((product) => product.id))
  const missingIds = createIdFactory(usedIds)
  const missingInWp: { id: string; name: string }[] = []
  const inactiveEvaluation: Product[] = []
  for (const product of evaluation.products) {
    if (matchedEvaluationIds.has(product.id)) continue
    missingInWp.push({ id: product.id, name: product.name })
    inactiveEvaluation.push({ ...product, id: missingIds(product.name), active: false })
  }

  return {
    products: sortProducts([...wpProducts, ...inactiveEvaluation]),
    duplicateMerges: evaluation.duplicateMerges,
    missingInWp,
    excludedWp,
  }
}
