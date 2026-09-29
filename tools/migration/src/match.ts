import { normalizeName } from '@palorosa-kitchen/core'

const NUMBER_WORDS: Record<string, string> = {
  1: 'un',
  2: 'dos',
  3: 'tres',
  4: 'cuatro',
  5: 'cinco',
  6: 'seis',
  7: 'siete',
  8: 'ocho',
  9: 'nueve',
  10: 'diez',
}

// Treat spelling variants that mean the same food as equal.
function canonical (name: string): string {
  return normalizeName(name)
    .replace(/\byogurt\b/g, 'yogur')
    .replace(/\b(\d{1,2})\b/g, (match) => NUMBER_WORDS[match] ?? match)
}

export function squashName (name: string): string {
  return canonical(name).replace(/ /g, '')
}

export function findExactName<T> (
  name: string,
  candidates: T[],
  getName: (item: T) => string
): T | null {
  const normalized = canonical(name)
  if (!normalized) return null
  return candidates.find((candidate) => canonical(getName(candidate)) === normalized) ?? null
}

/**
 * Finds a candidate by loose name: exact normalized, containment either way,
 * or squash equality and containment guarded by a minimum length.
 * Prefers the shortest matching name to avoid broad candidates.
 */
export function findByLooseName<T> (
  name: string,
  candidates: T[],
  getName: (item: T) => string
): T | null {
  const normalized = canonical(name)
  if (!normalized) return null
  let best: T | null = null
  let bestLength = Number.POSITIVE_INFINITY

  for (const candidate of candidates) {
    const candidateName = canonical(getName(candidate))
    if (!candidateName) continue
    if (
      candidateName === normalized ||
      candidateName.includes(normalized) ||
      normalized.includes(candidateName)
    ) {
      const length = getName(candidate).length
      if (length < bestLength) {
        best = candidate
        bestLength = length
      }
    }
  }
  if (best) return best

  const squashed = squashName(name)
  if (squashed.length < 8) return null
  for (const candidate of candidates) {
    const candidateSquashed = squashName(getName(candidate))
    if (!candidateSquashed) continue
    if (
      candidateSquashed === squashed ||
      candidateSquashed.includes(squashed) ||
      squashed.includes(candidateSquashed)
    ) {
      const length = getName(candidate).length
      if (length < bestLength) {
        best = candidate
        bestLength = length
      }
    }
  }
  return best
}
