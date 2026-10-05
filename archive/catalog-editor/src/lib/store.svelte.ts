import type { Catalog, KitchenUnit, Product } from '@palorosa-kitchen/core'
import { computePatch, type OverridesPatch } from './diff'

interface EditorState {
  loading: boolean
  error: string
  seed: Catalog | null
  catalog: Catalog | null
  merges: Record<string, string>
  patch: OverridesPatch
  message: string
  saving: boolean
}

export const editor = $state<EditorState>({
  loading: true,
  error: '',
  seed: null,
  catalog: null,
  merges: {},
  patch: {},
  message: '',
  saving: false,
})

function plain<T> (value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export async function load (): Promise<void> {
  editor.loading = true
  editor.error = ''
  try {
    const response = await fetch('/api/catalog')
    const seed = (await response.json()) as Catalog
    editor.seed = plain(seed)
    editor.catalog = seed
    editor.merges = {}
    recompute()
  } catch (error) {
    editor.error = (error as Error).message
  } finally {
    editor.loading = false
  }
}

export function recompute (): void {
  if (!editor.seed || !editor.catalog) return
  editor.patch = plain(computePatch(editor.seed, editor.catalog, editor.merges))
}

export async function save (): Promise<void> {
  recompute()
  editor.saving = true
  editor.message = 'Guardando y regenerando la semilla...'
  try {
    const response = await fetch('/api/overrides', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(editor.patch),
    })
    const data = (await response.json()) as { ok: boolean; migrate?: string; error?: string }
    if (data.ok) {
      await load()
      editor.message = `Guardado en catalog/overrides.json y semilla regenerada.\n\n${data.migrate ?? ''}`
    } else {
      editor.message = `Error: ${data.error ?? 'desconocido'}`
    }
  } catch (error) {
    editor.message = `Error: ${(error as Error).message}`
  } finally {
    editor.saving = false
  }
}

export function mergeUnit (sourceId: string, targetId: string): void {
  const source = unitById(sourceId)
  const target = unitById(targetId)
  if (!source || !target || source.id === target.id) return
  source.active = false
  if (!target.aliases.includes(source.name)) target.aliases.push(source.name)
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
