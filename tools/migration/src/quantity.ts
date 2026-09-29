const NUMBER_WORDS: Record<string, number> = {
  un: 1,
  una: 1,
  dos: 2,
  tres: 3,
  cuatro: 4,
  cinco: 5,
  seis: 6,
  siete: 7,
  ocho: 8,
  nueve: 9,
  diez: 10,
}

export interface QuantityMatch {
  quantity: number
  baseName: string
}

/**
 * Detects a leading Spanish number word in a unit name.
 * Example: "Dos mini ensaladas de frutas" -> quantity 2, base "Mini ensaladas de frutas".
 * Singularization is approximate on purpose. The reviewer confirms the base name.
 */
export function detectLeadingQuantity (name: string): QuantityMatch | null {
  const match = /^(un|una|dos|tres|cuatro|cinco|seis|siete|ocho|nueve|diez)\s+(.+)$/i.exec(
    name.trim()
  )
  if (!match) return null
  const word = match[1]
  const rest = match[2]
  if (!word || !rest) return null
  const quantity = NUMBER_WORDS[word.toLowerCase()]
  if (!quantity) return null
  return { quantity, baseName: singularizeTrailingS(rest) }
}

function singularizeTrailingS (name: string): string {
  return name
    .split(' ')
    .map((word) => (word.length > 3 && word.endsWith('s') ? word.slice(0, -1) : word))
    .join(' ')
}
