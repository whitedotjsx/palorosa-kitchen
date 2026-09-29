export type SourceSelection<T> =
  | { type: 'none' }
  | { type: 'single'; value: T }
  | { type: 'choose_between'; values: T[] }
  | { type: 'optional'; values: T[] }
  | { type: 'choose_between_multiple'; values: SourceSelection<T>[] }
  | { type: 'multiple'; values: T[] }

export interface SourceBreakfast {
  id: string
  name: string
  description: string
  decorations: SourceSelection<string>[]
  foodItems: SourceSelection<string>[]
  container: string
}

export interface SourceStoreProduct {
  id: string
  name: string
  description: string
  price: number
  category: string
}

export interface SourceData {
  kitchenFoods: Record<string, string>
  decorations: Record<string, string>
  containers: Record<string, string>
  breakfasts: Record<string, SourceBreakfast>
  additions: Record<string, string>
  additionPrices: Record<string, number>
  storeProducts: Record<string, SourceStoreProduct>
  productCategories: Record<string, string>
  cajaSaludDrinks: Record<string, string>
}
