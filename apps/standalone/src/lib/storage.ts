const STORAGE_KEY = 'palorosa-kitchen.product-mappings.v1'

export function loadProductMappings (): Record<string, string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as unknown
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return {}
    const mappings: Record<string, string> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === 'string') mappings[key] = value
    }
    return mappings
  } catch {
    return {}
  }
}

export function saveProductMappings (mappings: Record<string, string>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(mappings))
  } catch {
    // Private mode or storage full: the mapping still works for this session.
  }
}
