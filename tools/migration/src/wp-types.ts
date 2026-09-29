export interface WpCategory {
  id: number
  name: string
  count: number
}

export interface WpProduct {
  id: number
  name: string
  sku: string
  type: string
  status: string
  price: string
  parent_id: number
  categories?: { id: number; name: string }[]
  short_description?: string
  description?: string
}

export interface WpVariation {
  id: number
  parent_id: number
  name: string
  sku: string
  status: string
  price: string
}

export interface WpSnapshot {
  fetchedAt: string
  categories: WpCategory[]
  products: WpProduct[]
  variations: WpVariation[]
}
