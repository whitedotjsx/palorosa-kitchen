import { spawn, type ChildProcess } from 'node:child_process'
import { existsSync, readdirSync } from 'node:fs'
import { mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import {
  formatKitchenTemplateHtml,
  serverLabels,
  type KitchenList,
  type KitchenListEntry,
  type UnresolvedEntry,
} from '@palorosa-kitchen/core'

const appDir = resolve(import.meta.dirname, '..')
const rootDir = resolve(appDir, '../..')
try {
  process.loadEnvFile(join(rootDir, '.env'))
} catch {
  // No .env file: defaults apply.
}

const port = Number(process.env.KITCHEN_SERVER_PORT ?? 5200)
const host = process.env.KITCHEN_SERVER_HOST ?? '0.0.0.0'
const editorUrl = process.env.KITCHEN_EDITOR_URL ?? 'http://localhost:5199'
const stateDir = join(appDir, '.state')
const statePath = join(stateDir, 'lists.json')
const listHtmlPath = join(rootDir, 'apps', 'standalone', 'dist', 'index.html')
const editorAutoStart = process.env.KITCHEN_EDITOR_AUTOSTART !== 'off'

export interface PublishedList {
  deliveryDate: string
  source?: string
  orderCount?: number
  publishedAt: string
  entries: KitchenListEntry[]
  unresolved?: UnresolvedEntry[]
}

interface ServerState {
  lists: Record<string, PublishedList>
}

let state: ServerState = { lists: {} }
let editorProcess: ChildProcess | null = null

async function loadState (): Promise<void> {
  try {
    const raw = await readFile(statePath, 'utf8')
    state = JSON.parse(raw) as ServerState
    if (!state.lists) state = { lists: {} }
  } catch {
    state = { lists: {} }
  }
}

async function saveState (): Promise<void> {
  await mkdir(stateDir, { recursive: true })
  const temporary = `${statePath}.tmp`
  await writeFile(temporary, JSON.stringify(state, null, '\t'), 'utf8')
  await rename(temporary, statePath)
}

function bogotaDate (offsetDays = 0): string {
  const now = new Date(Date.now() + offsetDays * 86400 * 1000)
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Bogota' }).format(now)
}

function isLocal (req: IncomingMessage): boolean {
  const address = req.socket.remoteAddress ?? ''
  return address === '127.0.0.1' || address === '::1' || address === '::ffff:127.0.0.1'
}

async function readBody (req: IncomingMessage): Promise<Buffer> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  return Buffer.concat(chunks)
}

function sendJson (res: ServerResponse, status: number, data: unknown): void {
  res.statusCode = status
  res.setHeader('content-type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(data))
}

function page (message: string): string {
  return `<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>${serverLabels.title}</title>
<style>body{font-family:system-ui,sans-serif;background:#f6f5f2;color:#1a1a1a;display:grid;place-items:center;min-height:100vh;margin:0}
main{background:#fff;border:1px solid #e2e0da;border-radius:10px;padding:24px 28px;max-width:520px}
h1{font-size:18px;margin:0 0 8px}p{margin:0;color:#555}</style></head>
<body><main><h1>${serverLabels.title}</h1><p>${message}</p></main></body></html>`
}

function sendPage (res: ServerResponse, status: number, message: string): void {
  res.statusCode = status
  res.setHeader('content-type', 'text/html; charset=utf-8')
  res.end(page(message))
}

async function editorReachable (): Promise<boolean> {
  try {
    await fetch(editorUrl, { signal: AbortSignal.timeout(1500) })
    return true
  } catch {
    return false
  }
}

function findChromium (): string | undefined {
  if (process.env.KITCHEN_CHROMIUM_PATH) return process.env.KITCHEN_CHROMIUM_PATH
  if (process.env.WHATSAPP_CHROMIUM_PATH) return process.env.WHATSAPP_CHROMIUM_PATH
  const candidates: string[] = []
  if (process.platform === 'win32') {
    const local = process.env.LOCALAPPDATA
    if (local) {
      const playwright = join(local, 'ms-playwright')
      if (existsSync(playwright)) {
        const versions = readdirSync(playwright)
          .filter(entry => entry.startsWith('chromium-'))
          .sort()
          .reverse()
        for (const version of versions) {
          candidates.push(join(playwright, version, 'chrome-win64', 'chrome.exe'))
          candidates.push(join(playwright, version, 'chrome-win', 'chrome.exe'))
        }
      }
      candidates.push(join(local, 'Google', 'Chrome', 'Application', 'chrome.exe'))
      candidates.push(join(local, 'Microsoft', 'Edge', 'Application', 'msedge.exe'))
    }
  } else if (process.platform === 'darwin') {
    candidates.push('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
  } else {
    candidates.push('/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser')
  }
  return candidates.find(candidate => existsSync(candidate))
}

/** Renders the printed sheet with headless Chromium: no browser headers or footers. */
async function renderSheetPdf (list: PublishedList): Promise<Buffer> {
  const chromium = findChromium()
  if (!chromium) throw new Error('Chromium not found')
  const kitchenList: KitchenList = {
    entries: list.entries,
    unresolved: list.unresolved ?? [],
    ignoredProducts: [],
    ignoredUnits: [],
    warnings: [],
  }
  const logoPath = join(rootDir, 'packages', 'core', 'assets', 'palorosa-logo.png')
  let logo: string | undefined
  try {
    logo = `data:image/png;base64,${(await readFile(logoPath)).toString('base64')}`
  } catch {
    logo = undefined
  }
  const html = formatKitchenTemplateHtml(kitchenList, { date: list.deliveryDate, ...(logo ? { logo } : {}) })
  const dir = await mkdtemp(join(tmpdir(), 'palorosa-sheet-'))
  const htmlPath = join(dir, 'sheet.html')
  const pdfPath = join(dir, 'sheet.pdf')
  await writeFile(htmlPath, html, 'utf8')
  try {
    await new Promise<void>((resolve, reject) => {
      const child = spawn(
        chromium,
        [
          '--headless=new',
          '--disable-gpu',
          '--no-pdf-header-footer',
          `--print-to-pdf=${pdfPath}`,
          `file:///${htmlPath.replace(/\\/g, '/')}`,
        ],
        { stdio: 'ignore', windowsHide: true }
      )
      child.on('error', reject)
      child.on('exit', code => {
        if (code === 0) resolve()
        else reject(new Error(`Chromium exited with code ${code}`))
      })
    })
    return await readFile(pdfPath)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
}

async function ensureEditor (): Promise<void> {
  if (!editorAutoStart || editorProcess) return
  if (await editorReachable()) return
  editorProcess = spawn('pnpm', ['editor'], { cwd: rootDir, shell: true, stdio: 'inherit', windowsHide: true })
  editorProcess.on('exit', () => {
    editorProcess = null
  })
  console.log('Catalog editor starting on', editorUrl)
}

async function proxyEditor (req: IncomingMessage, res: ServerResponse): Promise<void> {
  const target = `${editorUrl}${req.url ?? '/'}`
  try {
    const body = req.method === 'GET' || req.method === 'HEAD' ? undefined : await readBody(req)
    const response = await fetch(target, {
      method: req.method ?? 'GET',
      headers: { 'content-type': req.headers['content-type'] ?? 'application/json' },
      ...(body ? { body } : {}),
      signal: AbortSignal.timeout(15000),
    })
    res.statusCode = response.status
    response.headers.forEach((value, key) => {
      const lower = key.toLowerCase()
      if (lower !== 'content-encoding' && lower !== 'content-length' && lower !== 'transfer-encoding') {
        res.setHeader(key, value)
      }
    })
    res.end(Buffer.from(await response.arrayBuffer()))
  } catch {
    sendPage(res, 503, serverLabels.editorUnavailable)
  }
}

function parsePublishedList (value: unknown): PublishedList | null {
  if (typeof value !== 'object' || value === null) return null
  const record = value as Record<string, unknown>
  const deliveryDate = typeof record.deliveryDate === 'string' ? record.deliveryDate : ''
  if (!/^\d{4}-\d{2}-\d{2}$/.test(deliveryDate)) return null
  if (!Array.isArray(record.entries)) return null
  return {
    deliveryDate,
    ...(typeof record.source === 'string' && record.source ? { source: record.source } : {}),
    ...(typeof record.orderCount === 'number' ? { orderCount: record.orderCount } : {}),
    publishedAt: new Date().toISOString(),
    entries: record.entries as KitchenListEntry[],
    ...(Array.isArray(record.unresolved) ? { unresolved: record.unresolved as UnresolvedEntry[] } : {}),
  }
}

async function handle (req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`)

  if (url.pathname === '/health') {
    sendJson(res, 200, {
      ok: true,
      listBuilt: existsSync(listHtmlPath),
      editor: await editorReachable(),
      dates: Object.keys(state.lists).sort(),
    })
    return
  }

  if (url.pathname === '/favicon.ico' && req.method === 'GET') {
    res.statusCode = 204
    res.end()
    return
  }

  if (url.pathname === '/' && req.method === 'GET') {
    if (!existsSync(listHtmlPath)) {
      sendPage(res, 503, serverLabels.listNotBuilt)
      return
    }
    res.statusCode = 200
    res.setHeader('content-type', 'text/html; charset=utf-8')
    res.end(await readFile(listHtmlPath))
    return
  }

  if (url.pathname === '/api/list' && req.method === 'GET') {
    const date = url.searchParams.get('date') ?? bogotaDate()
    const list = state.lists[date]
    if (!list) {
      sendJson(res, 404, { ok: false, error: `No list for ${date}` })
      return
    }
    sendJson(res, 200, list)
    return
  }

  if (url.pathname === '/api/pdf' && req.method === 'GET') {
    const date = url.searchParams.get('date') ?? bogotaDate()
    const list = state.lists[date]
    if (!list) {
      sendJson(res, 404, { ok: false, error: `No list for ${date}` })
      return
    }
    try {
      const pdf = await renderSheetPdf(list)
      res.statusCode = 200
      res.setHeader('content-type', 'application/pdf')
      res.setHeader('content-disposition', `inline; filename="cocina-palorosa-${date}.pdf"`)
      res.end(pdf)
    } catch (error) {
      console.error(error)
      sendPage(res, 503, serverLabels.pdfUnavailable)
    }
    return
  }

  if (url.pathname === '/api/lists' && req.method === 'GET') {
    sendJson(
      res,
      200,
      Object.values(state.lists)
        .sort((a, b) => a.deliveryDate.localeCompare(b.deliveryDate))
        .map(list => ({
          deliveryDate: list.deliveryDate,
          publishedAt: list.publishedAt,
          entries: list.entries.length,
          source: list.source ?? '',
        }))
    )
    return
  }

  if (url.pathname === '/api/list' && req.method === 'POST') {
    let parsed: PublishedList | null = null
    try {
      parsed = parsePublishedList(JSON.parse((await readBody(req)).toString('utf8')) as unknown)
    } catch {
      parsed = null
    }
    if (!parsed) {
      sendJson(res, 400, { ok: false, error: serverLabels.invalidRequest })
      return
    }
    state.lists[parsed.deliveryDate] = parsed
    await saveState()
    console.log(`List published: ${parsed.deliveryDate} (${parsed.entries.length} entries, ${parsed.source ?? 'unknown source'})`)
    sendJson(res, 200, { ok: true, publishedAt: parsed.publishedAt })
    return
  }

  if (url.pathname === '/editor' || url.pathname.startsWith('/editor/') || url.pathname === '/api/catalog' || url.pathname === '/api/overrides') {
    if (!isLocal(req)) {
      sendPage(res, 403, serverLabels.editorLocalOnly)
      return
    }
    await proxyEditor(req, res)
    return
  }

  sendJson(res, 404, { ok: false, error: serverLabels.notFound })
}

await loadState()
const server = createServer((req, res) => {
  handle(req, res).catch((error: unknown) => {
    console.error(error)
    if (!res.headersSent) sendJson(res, 500, { ok: false, error: 'Internal error' })
  })
})

server.listen(port, host, () => {
  console.log(`Kitchen list app:  http://localhost:${port}/`)
  console.log(`List API:          http://localhost:${port}/api/list?date=YYYY-MM-DD`)
  console.log(`Catalog editor:    http://localhost:${port}/editor/ (only from this machine)`)
  ensureEditor().catch((error: unknown) => console.error('Could not start the catalog editor', error))
})

function shutdown (): void {
  editorProcess?.kill()
  server.close(() => process.exit(0))
}

process.on('SIGINT', shutdown)
process.on('SIGTERM', shutdown)
