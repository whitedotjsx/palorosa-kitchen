import { readFileSync, statSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig, type Plugin } from 'vite'
import { viteSingleFile } from 'vite-plugin-singlefile'

const dataDir = resolve(import.meta.dirname, '../..', 'data')

/**
 * Embeds the accepted catalog seed (D12) and its file date so the app can show
 * how old the snapshot is.
 */
function catalogSnapshot (): Plugin {
  const virtualId = 'virtual:catalog-snapshot'
  const resolvedId = `\0${virtualId}`
  const snapshotPath = join(dataDir, 'seed.json')

  return {
    name: 'catalog-snapshot',
    resolveId (id) {
      return id === virtualId ? resolvedId : null
    },
    load (id) {
      if (id !== resolvedId) return null
      const raw = readFileSync(snapshotPath, 'utf8')
      const meta = { mtime: statSync(snapshotPath).mtime.toISOString() }
      return `export const catalog = ${raw}\nexport const catalogMeta = ${JSON.stringify(meta)}`
    },
  }
}

export default defineConfig({
  plugins: [
    svelte({ compilerOptions: { runes: true } }),
    catalogSnapshot(),
    viteSingleFile(),
  ],
  build: {
    target: 'es2022',
  },
  server: {
    port: 5198,
    strictPort: true,
  },
})
