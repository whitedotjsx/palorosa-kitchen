import type { Catalog, KitchenUnit, Product, RecipeComponent } from '@palorosa-kitchen/core'
import { api } from './api'

interface EditorState {
  loading: boolean
  error: string
  path: string
  catalog: Catalog | null
  /** Units merged this session: source id → target id. Display only. */
  merges: Record<string, string>
  dirty: boolean
  saving: boolean
  problems: string[]
}

export const editor = $state<EditorState>({
  loading: true,
  error: '',
  path: '',
  catalog: null,
  merges: {},
  dirty: false,
  saving: false,
  problems: [],
})

/** The catalog as last loaded or saved, to detect and discard changes. */
let saved = ''

function snapshot (): string {
  return editor.catalog ? JSON.stringify(editor.catalog) : ''
}

export async function load (): Promise<void> {
  editor.loading = true
  editor.error = ''
  try {
    const data = await api<{ path: string; catalog: Catalog }>('/api/panel/catalog')
    editor.path = data.path
    editor.catalog = data.catalog
    editor.merges = {}
    editor.problems = []
    saved = snapshot()
    editor.dirty = false
  } catch (error) {
    editor.error = (error as Error).message
  } finally {
    editor.loading = false
  }
}

/** Marks the catalog as edited. Components call it after every change. */
export function recompute (): void {
  editor.dirty = snapshot() !== saved
}

export function discard (): void {
  if (!saved) return
  editor.catalog = JSON.parse(saved) as Catalog
  editor.merges = {}
  editor.problems = []
  editor.dirty = false
}

/** Saves the whole catalog. The kitchen uses it from the next order on. */
export async function save (): Promise<{ products: number; recipes: number }> {
  if (!editor.catalog) throw new Error('Sin catálogo')
  editor.saving = true
  editor.problems = []
  try {
    const result = await fetch('/api/panel/catalog', {
      method: 'PUT',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ catalog: $state.snapshot(editor.catalog) }),
    })
    const data = (await result.json().catch(() => ({}))) as {
      ok?: boolean
      error?: string
      problems?: string[]
      products?: number
      recipes?: number
    }
    if (!result.ok || !data.ok) {
      editor.problems = data.problems ?? []
      throw new Error(data.error ?? `HTTP ${result.status}`)
    }
    saved = snapshot()
    editor.merges = {}
    editor.dirty = false
    return { products: data.products ?? 0, recipes: data.recipes ?? 0 }
  } finally {
    editor.saving = false
  }
}

function rewriteComponents (components: RecipeComponent[], from: string, to: string): void {
  for (const component of components) {
    if (component.kind === 'fixed') {
      if (component.unitId === from) component.unitId = to
    } else {
      for (const option of component.options) rewriteComponents(option.components, from, to)
    }
  }
}

/**
 * Merges a duplicate unit into another: the source is deactivated, its name
 * becomes an alias of the target and every recipe pointing at it is rewritten.
 */
export function mergeUnit (sourceId: string, targetId: string): void {
  const source = unitById(sourceId)
  const target = unitById(targetId)
  if (!editor.catalog || !source || !target || source.id === target.id) return
  source.active = false
  for (const alias of [source.name, ...source.aliases]) {
    if (alias && alias !== target.name && !target.aliases.includes(alias)) target.aliases.push(alias)
  }
  for (const recipe of editor.catalog.recipes) rewriteComponents(recipe.components, source.id, target.id)
  editor.merges[sourceId] = targetId
  recompute()
}

export function download (filename: string, data: unknown): void {
  const blob = new Blob([`${JSON.stringify(data, null, '\t')}\n`], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function unitName (id: string): string {
  return editor.catalog?.units.find((unit) => unit.id === id)?.name ?? id
}

export function unitById (id: string): KitchenUnit | undefined {
  return editor.catalog?.units.find((unit) => unit.id === id)
}

export function productById (id: string): Product | undefined {
  return editor.catalog?.products.find((product) => product.id === id)
}
