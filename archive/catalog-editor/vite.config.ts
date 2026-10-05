import { spawnSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig, type Plugin } from 'vite'

const rootDir = resolve(import.meta.dirname, '../..')
const dataDir = join(rootDir, 'data')

function envValue (key: string): string {
  if (process.env[key]) return process.env[key]!
  try {
    for (const line of readFileSync(join(rootDir, '.env'), 'utf8').split(/\r?\n/)) {
      const match = /^([A-Z_]+)\s*=\s*(.*)$/.exec(line.trim())
      if (match && match[1] === key) return match[2]!.trim()
    }
  } catch {
    // no env file
  }
  return ''
}

function overridesPath (): string {
  const repo = envValue('EVALUATION_REPO')
  if (!repo) throw new Error('EVALUATION_REPO missing in .env')
  return join(repo, 'catalog', 'overrides.json')
}

function readJson (path: string): Record<string, unknown> {
  return JSON.parse(readFileSync(path, 'utf8')) as Record<string, unknown>
}

function mergeOverrides (
  existing: Record<string, unknown>,
  patch: Record<string, unknown>
): Record<string, unknown> {
  const merged = structuredClone(existing)
  const mapKeys = ['units', 'products', 'containers', 'optionOverrides', 'queue']
  for (const key of mapKeys) {
    const entries = patch[key] as Record<string, Record<string, unknown>> | undefined
    if (!entries) continue
    const target = (merged[key] as Record<string, Record<string, unknown>>) ?? {}
    for (const [id, value] of Object.entries(entries)) {
      target[id] = { ...(target[id] ?? {}), ...value }
    }
    merged[key] = target
  }
  const mergeKeys = ['unitMerges', 'unitRewrites']
  for (const key of mergeKeys) {
    const entries = patch[key] as Record<string, unknown> | undefined
    if (!entries) continue
    merged[key] = { ...((merged[key] as Record<string, unknown>) ?? {}), ...entries }
  }
  const recipes = patch.recipes as Record<string, unknown> | undefined
  if (recipes) {
    merged.recipes = { ...((merged.recipes as Record<string, unknown>) ?? {}), ...recipes }
  }
  return merged
}

function send (res: import('node:http').ServerResponse, status: number, data: unknown): void {
  res.statusCode = status
  res.setHeader('content-type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(data))
}

function apiPlugin (): Plugin {
  return {
    name: 'catalog-editor-api',
    configureServer (server) {
      server.middlewares.use((req, res, next) => {
        if (!req.url?.startsWith('/api/')) {
          next()
          return
        }
        try {
          if (req.method === 'GET' && req.url.startsWith('/api/catalog')) {
            send(res, 200, readJson(join(dataDir, 'seed.json')))
            return
          }
          if (req.method === 'GET' && req.url.startsWith('/api/overrides')) {
            send(res, 200, readJson(overridesPath()))
            return
          }
          if (req.method === 'POST' && req.url.startsWith('/api/overrides')) {
            let body = ''
            req.on('data', (chunk) => {
              body += chunk
            })
            req.on('end', () => {
              try {
                const patch = JSON.parse(body) as Record<string, unknown>
                const merged = mergeOverrides(readJson(overridesPath()), patch)
                writeFileSync(overridesPath(), `${JSON.stringify(merged, null, '\t')}\n`, 'utf8')
                const run = (script: string): string => {
                  const result = spawnSync(
                    'pnpm',
                    ['--filter', '@palorosa-kitchen/migration', 'run', script],
                    { cwd: rootDir, shell: true, encoding: 'utf8' }
                  )
                  return `${result.stdout ?? ''}${result.stderr ?? ''}`
                }
                const syncOutput = run('sync:source')
                const migrateOutput = run('migrate')
                const output = `${syncOutput}${migrateOutput}`.trim()
                send(res, 200, {
                  ok: true,
                  migrate: output.split('\n').slice(-8).join('\n'),
                })
              } catch (error) {
                send(res, 500, { ok: false, error: (error as Error).message })
              }
            })
            return
          }
        } catch (error) {
          send(res, 500, { ok: false, error: (error as Error).message })
          return
        }
        next()
      })
    },
  }
}

export default defineConfig({
  plugins: [svelte({ compilerOptions: { runes: true } }), apiPlugin()],
  server: { port: 5199, strictPort: true },
})
