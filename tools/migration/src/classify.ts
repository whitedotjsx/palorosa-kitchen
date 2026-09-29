import { normalizeName, type MeasureUnit, type UnitCategory } from '@palorosa-kitchen/core'

export type FoodClassification = 'food' | 'presentation' | 'unknown'

const PRESENTATION_KEYWORDS = [
  'globo',
  'burbuja',
  'carta',
  'mensaje',
  'lazo',
  'mantel',
  'cubierto',
  'cuchara',
  'lapicero',
  'motivo',
  'menu',
  'topper',
  'sticker',
  'stikers',
  'cinta',
  'separador',
  'domo',
  'suculenta',
  'bouquet',
  'rosas',
  'diario',
  'binocular',
  'bolso',
  'vaso',
  'joyero',
  'foto',
  'planta',
  'panol',
  'seda',
  'servilleta',
  'tarjeta',
  'iman',
  'llavero',
  'helio',
  'cucharita',
]

const FOOD_KEYWORDS = [
  'jugo',
  'bebida',
  'yogur',
  'yogurt',
  'kumis',
  'cafe',
  'mocca',
  'smoothie',
  'smothie',
  'hatsu',
  'soda',
  'waffle',
  'pancake',
  'quesadilla',
  'sandwich',
  'arepa',
  'wrap',
  'empanada',
  'pastel',
  'croissant',
  'bowl',
  'parfait',
  'copa',
  'granola',
  'tabla',
  'tablita',
  'frasco',
  'pincho',
  'deditos',
  'queso',
  'jamon',
  'fruta',
  'ensalada',
  'fresa',
  'uva',
  'manzana',
  'durazno',
  'mango',
  'kiwi',
  'banano',
  'miel',
  'mantequilla',
  'arequipe',
  'nutella',
  'chocolate',
  'chocolatina',
  'bombon',
  'galleta',
  'muffin',
  'torta',
  'brownie',
  'gomita',
  'kinder',
  'cheesecake',
  'mani',
  'frutos secos',
  'papas',
  'rosquilla',
  'tosh',
  'huevo',
  'salchicha',
  'chorizo',
  'mozzarella',
  'parmesano',
  'pollo',
  'carne',
  'vino',
  'cerveza',
  'whisky',
  'buchanan',
  'tequila',
  'chenet',
]

export function classifyName (rawName: string): FoodClassification {
  const name = normalizeName(rawName)
  const hasFood = FOOD_KEYWORDS.some((keyword) => name.includes(keyword))
  const hasPresentation = PRESENTATION_KEYWORDS.some((keyword) => name.includes(keyword))
  if (hasFood) return 'food'
  if (hasPresentation) return 'presentation'
  return 'unknown'
}

const DRINK_KEYWORDS = [
  'jugo',
  'kumis',
  'cafe',
  'mocca',
  'smoothie',
  'smothie',
  'hatsu',
  'soda',
  'vino',
  'cerveza',
  'whisky',
  'buchanan',
  'tequila',
]

const DESSERT_KEYWORDS = [
  'waffle',
  'pancake',
  'muffin',
  'torta',
  'galleta',
  'brownie',
  'gomita',
  'chocolate',
  'chocolatina',
  'bombon',
  'nutella',
  'cheesecake',
  'kinder',
  'tosh',
  'arequipe',
  'quesadilla',
]

const FRUIT_KEYWORDS = [
  'ensalada de fruta',
  'fruta',
  'fresa',
  'uva',
  'manzana',
  'durazno',
  'mango',
  'kiwi',
  'banano',
]

const MAIN_KEYWORDS = ['sandwich', 'arepa', 'wrap', 'empanada', 'pastel', 'croissant']

const SIDE_KEYWORDS = [
  'bowl',
  'parfait',
  'copa',
  'tabla',
  'tablita',
  'frasco',
  'pincho',
  'deditos',
  'papas',
  'rosquilla',
]

const CONDIMENT_KEYWORDS = ['miel', 'mantequilla', 'salsa', 'aderezo', 'mani', 'granola']

function firstMatch (name: string, keywords: string[]): boolean {
  return keywords.some((keyword) => name.includes(keyword))
}

export function inferUnitCategory (rawName: string): UnitCategory {
  const name = normalizeName(rawName)
  if (firstMatch(name, DRINK_KEYWORDS)) return 'drink'
  if (firstMatch(name, FRUIT_KEYWORDS)) return 'fruit'
  if (firstMatch(name, DESSERT_KEYWORDS)) return 'dessert'
  if (firstMatch(name, MAIN_KEYWORDS)) return 'main'
  if (firstMatch(name, SIDE_KEYWORDS)) return 'side'
  if (firstMatch(name, CONDIMENT_KEYWORDS)) return 'condiment'
  return 'other'
}

export function inferMeasure (rawName: string): MeasureUnit {
  const name = normalizeName(rawName)
  if (name.includes('porcion')) return 'portion'
  if (name.includes('vaso') || name.includes('copa')) return 'glass'
  if (name.includes('botella') || name.includes('frasco')) return 'bottle'
  if (name.includes('tajada')) return 'slice'
  if (name.includes('cucharada')) return 'spoon'
  if (name.includes('gramo') || name.includes('gr ')) return 'gram'
  return 'unit'
}
