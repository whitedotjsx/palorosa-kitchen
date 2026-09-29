import type { KitchenList } from './aggregate'

export interface ListDeltaEntry {
  unitId: string
  name: string
  previous: number
  next: number
  delta: number
}

export interface ListDiff {
  added: ListDeltaEntry[]
  removed: ListDeltaEntry[]
  changed: ListDeltaEntry[]
}

function deltaEntry (
  unitId: string,
  name: string,
  previous: number,
  next: number
): ListDeltaEntry {
  return { unitId, name, previous, next, delta: next - previous }
}

export function diffLists (previous: KitchenList | null | undefined, next: KitchenList): ListDiff {
  const before = new Map((previous?.entries ?? []).map(entry => [entry.unitId, entry]))
  const after = new Map(next.entries.map(entry => [entry.unitId, entry]))
  const added: ListDeltaEntry[] = []
  const removed: ListDeltaEntry[] = []
  const changed: ListDeltaEntry[] = []

  for (const entry of after.values()) {
    const old = before.get(entry.unitId)
    if (!old) added.push(deltaEntry(entry.unitId, entry.name, 0, entry.quantity))
    else if (old.quantity !== entry.quantity) {
      changed.push(deltaEntry(entry.unitId, entry.name, old.quantity, entry.quantity))
    }
  }
  for (const entry of before.values()) {
    if (!after.has(entry.unitId)) removed.push(deltaEntry(entry.unitId, entry.name, entry.quantity, 0))
  }

  return { added, removed, changed }
}

export function isListDiffEmpty (diff: ListDiff): boolean {
  return diff.added.length === 0 && diff.removed.length === 0 && diff.changed.length === 0
}
