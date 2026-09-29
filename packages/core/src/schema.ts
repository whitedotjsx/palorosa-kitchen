export const SCHEMA_VERSION = 1

export type Id = string

export type UnitCategory = 'drink' | 'main' | 'side' | 'dessert' | 'fruit' | 'condiment' | 'other'

export type MeasureUnit = 'unit' | 'portion' | 'glass' | 'bottle' | 'slice' | 'spoon' | 'gram'

export interface KitchenUnit {
  id: Id
  name: string
  description?: string
  category: UnitCategory
  measure: MeasureUnit
  aliases: string[]
  note?: string
  isKitchen: boolean
  active: boolean
}

export type ProductCategory =
  | 'breakfast'
  | 'add_on'
  | 'flower'
  | 'stuffed_animal'
  | 'accessory'
  | 'gift_box'
  | 'bag'

export interface Product {
  id: Id
  name: string
  aliases: string[]
  category: ProductCategory
  isKitchen: boolean
  priceCop?: number
  description?: string
  containerId?: Id
  wpProductId?: number
  sku?: string
  active: boolean
}

export interface FixedComponent {
  kind: 'fixed'
  unitId: Id
  quantity: number
}

export interface ChoiceOption {
  id: Id
  label: string
  matchAliases: string[]
  /**
   * Option taken when a required group matches none of its options. The store
   * export signals a choice by the column of one option; an empty column means
   * the other option, which is the one flagged here.
   */
  default?: boolean
  components: FixedComponent[]
}

export interface ChoiceGroup {
  kind: 'choice'
  label: string
  required: boolean
  options: ChoiceOption[]
}

export type RecipeComponent = FixedComponent | ChoiceGroup

export interface Recipe {
  productId: Id
  components: RecipeComponent[]
}

export interface Container {
  id: Id
  name: string
  aliases: string[]
  active: boolean
}

export interface LabelParsingRules {
  quantityPatterns: string[]
  productSeparators: string[]
  optionSeparators: string[]
  noisePatterns: string[]
}

export interface Catalog {
  schemaVersion: number
  units: KitchenUnit[]
  products: Product[]
  recipes: Recipe[]
  containers: Container[]
  labelParsing: LabelParsingRules
}

export type UnresolvedReason =
  | 'unknown_product'
  | 'missing_recipe'
  | 'missing_choice'
  | 'ambiguous_match'
  | 'missing_unit'

export type IgnoredReason = 'not_kitchen' | 'inactive_unit'

export type LineWarning = 'missing_quantity'
