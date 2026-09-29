import { mkdir, readFile, rename, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import type { KitchenList, ParsedOrderLine } from '@palorosa-kitchen/core'

const appDir = resolve(import.meta.dirname, '..')
const stateDir = join(appDir, '.state')
const statePath = join(stateDir, 'state.json')

export interface BotState {
  /** Delivery date to order number to parsed lines, fed by the order hooks. */
  orders: Record<string, Record<string, ParsedOrderLine[]>>
  /** Last computed list per delivery date, used for the notification delta. */
  lists: Record<string, KitchenList>
}

export function emptyState (): BotState {
  return { orders: {}, lists: {} }
}

export async function loadState (): Promise<BotState> {
  try {
    const raw = await readFile(statePath, 'utf8')
    const parsed = JSON.parse(raw) as Partial<BotState>
    return {
      orders: parsed.orders ?? {},
      lists: parsed.lists ?? {},
    }
  } catch {
    return emptyState()
  }
}

export async function saveState (state: BotState): Promise<void> {
  await mkdir(stateDir, { recursive: true })
  const temporary = `${statePath}.tmp`
  await writeFile(temporary, JSON.stringify(state, null, '\t'), 'utf8')
  await rename(temporary, statePath)
}
