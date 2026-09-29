export function stripAccents (text: string): string {
  return text.normalize('NFD').replace(/[\u0300-\u036f]/g, '')
}

export function normalizeName (text: string): string {
  return stripAccents(text).toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim()
}

export function slugify (text: string): string {
  return normalizeName(text).replace(/ /g, '-')
}
