import { existsSync, readdirSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import { join, resolve } from 'node:path'
import {
  aggregateUnits,
  botLabels,
  diffLists,
  formatListDiffText,
  formatListText,
  indexCatalog,
  isListDiffEmpty,
  resolveLines,
  stripAccents,
  type Catalog,
  type KitchenList,
  type KitchenListEntry,
  type ListDiff,
  type ParsedOrderLine,
  type UnresolvedEntry,
} from '@palorosa-kitchen/core'
import qrcode from 'qrcode-terminal'
import whatsapp from 'whatsapp-web.js'
import type { Message } from 'whatsapp-web.js'
import { loadState, saveState } from './state'

// whatsapp-web.js is CommonJS: the named exports are not statically detectable
// by the ESM loader, so they are read from the default import.
const { Client, LocalAuth } = whatsapp

const appDir = resolve(import.meta.dirname, '..')
const rootDir = resolve(appDir, '../..')
try {
  process.loadEnvFile(join(rootDir, '.env'))
} catch {
  // No .env file: defaults apply.
}

function digits (value: string): string {
  return value.replace(/\D+/g, '')
}

const port = Number(process.env.BOT_PORT ?? 5210)
const hookToken = process.env.BOT_HOOK_TOKEN ?? ''
const allowlist = (process.env.WHATSAPP_ALLOWLIST ?? '')
  .split(',')
  .map(item => digits(item))
  .filter(Boolean)
const kitchenServerUrl = (process.env.KITCHEN_SERVER_URL ?? 'http://127.0.0.1:5200').replace(/\/$/, '')
const catalogPath = process.env.KITCHEN_CATALOG_PATH ?? join(rootDir, 'data', 'seed.json')
const pairingPhone = digits(process.env.WHATSAPP_PAIRING_PHONE ?? '')

const catalog = JSON.parse(await readFile(catalogPath, 'utf8')) as Catalog
const catalogIndex = indexCatalog(catalog)
const state = await loadState()
const whatsappStatus = { ready: false, lastError: '' }

function bogotaDate (offsetDays = 0): string {
  const now = new Date(Date.now() + offsetDays * 86400 * 1000)
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Bogota' }).format(now)
}

function emptyList (): KitchenList {
  return { entries: [], unresolved: [], ignoredProducts: [], ignoredUnits: [], warnings: [] }
}

function isAllowed (from: string): boolean {
  const number = digits(from.split('@')[0] ?? '')
  return allowlist.includes(number)
}

function chatId (number: string): string {
  return `${number}@c.us`
}

async function listFromServer (date: string): Promise<KitchenList | null> {
  try {
    const response = await fetch(`${kitchenServerUrl}/api/list?date=${date}`, {
      signal: AbortSignal.timeout(2500),
    })
    if (!response.ok) return null
    const published = (await response.json()) as { entries?: KitchenListEntry[], unresolved?: UnresolvedEntry[] }
    if (!Array.isArray(published.entries)) return null
    return {
      entries: published.entries,
      unresolved: published.unresolved ?? [],
      ignoredProducts: [],
      ignoredUnits: [],
      warnings: [],
    }
  } catch {
    return null
  }
}

function listFromState (date: string): KitchenList | null {
  const orders = state.orders[date]
  if (!orders) return null
  const lines = Object.values(orders).flat()
  if (lines.length === 0) return null
  return aggregateUnits(resolveLines(lines, catalogIndex), catalogIndex)
}

async function listForDate (date: string): Promise<KitchenList | null> {
  return (await listFromServer(date)) ?? listFromState(date)
}

function findChromium (): string | undefined {
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

const chromiumPath = findChromium()
const client = new Client({
  authStrategy: new LocalAuth({ dataPath: join(appDir, '.wwebjs_auth') }),
  puppeteer: {
    headless: true,
    ...(chromiumPath ? { executablePath: chromiumPath } : {}),
    args: ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'],
  },
})

let pairingRequested = false
client.on('qr', (qr: string) => {
  if (pairingPhone) {
    if (pairingRequested) return
    pairingRequested = true
    client
      .requestPairingCode(pairingPhone)
      .then(code => console.log(`WhatsApp pairing code for ${pairingPhone}: ${code}`))
      .catch(error => console.error('Pairing code failed', error))
    return
  }
  console.log('Scan this QR with the WhatsApp account the bot will use:')
  qrcode.generate(qr, { small: true })
})
client.on('ready', () => {
  whatsappStatus.ready = true
  console.log('WhatsApp client ready')
})
client.on('auth_failure', (message: string) => {
  whatsappStatus.lastError = message
  console.error('WhatsApp auth failure:', message)
})
client.on('disconnected', (reason: string) => {
  whatsappStatus.ready = false
  console.warn('WhatsApp disconnected:', reason)
})
client.on('message', (message: Message) => {
  handleMessage(message).catch(error => console.error('Message handler failed', error))
})

function normalizeCommand (text: string): string {
  return stripAccents(text.toLowerCase())
    .replace(/[^a-z0-9\s]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

async function listReply (date: string): Promise<string> {
  const list = await listForDate(date)
  if (!list || list.entries.length === 0) return botLabels.noList.replace('{date}', date)
  return formatListText(list, { date })
}

async function statusReply (): Promise<string> {
  const today = bogotaDate(0)
  const dates = Object.keys(state.lists).sort()
  if (dates.length === 0) return botLabels.statusNoLists.replace('{date}', today)
  return botLabels.statusReady.replace('{date}', today).replace('{dates}', dates.join(', '))
}

async function handleMessage (message: Message): Promise<void> {
  if (message.fromMe) return
  const from = message.from
  if (!isAllowed(from)) {
    console.log(`Ignored message from ${from}`)
    return
  }
  const command = normalizeCommand(message.body ?? '')
  if (command === 'lista' || command === 'lista hoy') {
    await client.sendMessage(from, await listReply(bogotaDate(0)))
    return
  }
  if (command === 'lista manana' || command === 'lista de manana') {
    await client.sendMessage(from, await listReply(bogotaDate(1)))
    return
  }
  if (command === 'estado') {
    await client.sendMessage(from, await statusReply())
    return
  }
  if (command === 'ayuda' || command === 'help' || command === 'comandos') {
    await client.sendMessage(from, botLabels.commands)
  }
}

async function notifyDiff (diff: ListDiff, date: string, title: string): Promise<number> {
  if (isListDiffEmpty(diff)) return 0
  if (!whatsappStatus.ready) {
    console.warn('WhatsApp is not ready, notification skipped')
    return 0
  }
  const text = formatListDiffText(diff, { title, date })
  let sent = 0
  for (const number of allowlist) {
    try {
      await client.sendMessage(chatId(number), text)
      sent++
    } catch (error) {
      console.error(`Notification to ${number} failed`, error)
    }
  }
  return sent
}

function isRecord (value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isHookAuthorized (req: IncomingMessage): boolean {
  if (!hookToken) return true
  return req.headers['x-bot-token'] === hookToken
}

async function readJson (req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  const raw = Buffer.concat(chunks).toString('utf8')
  return raw ? JSON.parse(raw) : null
}

function sendJson (res: ServerResponse, status: number, data: unknown): void {
  res.statusCode = status
  res.setHeader('content-type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(data))
}

interface HookResult {
  statusCode: number
  body: unknown
}

async function handleHookOrder (record: Record<string, unknown>): Promise<HookResult> {
  const date = typeof record.deliveryDate === 'string' ? record.deliveryDate : ''
  const orderNumber = record.orderNumber !== undefined ? String(record.orderNumber) : ''
  const lines = Array.isArray(record.lines) ? (record.lines as ParsedOrderLine[]) : null
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !orderNumber || !lines) {
    return { statusCode: 400, body: { ok: false, error: 'Expected deliveryDate, orderNumber and lines' } }
  }

  const previous = state.lists[date] ?? null
  state.orders[date] = { ...(state.orders[date] ?? {}), [orderNumber]: lines }
  const next = listFromState(date)
  if (next) state.lists[date] = next
  const diff = diffLists(previous, next ?? emptyList())
  await saveState(state)
  const notified = await notifyDiff(diff, date, botLabels.newOrder)
  return {
    statusCode: 200,
    body: {
      ok: true,
      notified,
      added: diff.added.length,
      changed: diff.changed.length,
      removed: diff.removed.length,
    },
  }
}

async function handleHookOrders (record: Record<string, unknown>): Promise<HookResult> {
  const date = typeof record.deliveryDate === 'string' ? record.deliveryDate : ''
  const orders = Array.isArray(record.orders) ? record.orders.filter(isRecord) : null
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !orders) {
    return { statusCode: 400, body: { ok: false, error: 'Expected deliveryDate and orders' } }
  }

  const map: Record<string, ParsedOrderLine[]> = {}
  orders.forEach((order, index) => {
    const number = order.orderNumber !== undefined ? String(order.orderNumber) : String(index + 1)
    map[number] = Array.isArray(order.lines) ? (order.lines as ParsedOrderLine[]) : []
  })

  const previous = state.lists[date] ?? null
  state.orders[date] = map
  const next = listFromState(date)
  if (next) state.lists[date] = next
  const diff = diffLists(previous, next ?? emptyList())
  await saveState(state)
  const notified = await notifyDiff(diff, date, botLabels.updatedOrder)
  return {
    statusCode: 200,
    body: {
      ok: true,
      notified,
      added: diff.added.length,
      changed: diff.changed.length,
      removed: diff.removed.length,
    },
  }
}

async function route (req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`)

  if (url.pathname === '/health') {
    sendJson(res, 200, {
      ok: true,
      whatsapp: whatsappStatus,
      dates: Object.keys(state.lists).sort(),
    })
    return
  }

  if ((url.pathname === '/hook/order' || url.pathname === '/hook/orders') && req.method === 'POST') {
    if (!isHookAuthorized(req)) {
      sendJson(res, 401, { ok: false, error: 'Unauthorized' })
      return
    }
    let body: unknown
    try {
      body = await readJson(req)
    } catch {
      sendJson(res, 400, { ok: false, error: 'Invalid JSON body' })
      return
    }
    if (!isRecord(body)) {
      sendJson(res, 400, { ok: false, error: 'Invalid JSON body' })
      return
    }
    const result = url.pathname === '/hook/order' ? await handleHookOrder(body) : await handleHookOrders(body)
    sendJson(res, result.statusCode, result.body)
    return
  }

  sendJson(res, 404, { ok: false, error: 'Not found' })
}

const server = createServer((req, res) => {
  route(req, res).catch((error: unknown) => {
    console.error(error)
    if (!res.headersSent) sendJson(res, 500, { ok: false, error: 'Internal error' })
  })
})

server.listen(port, () => {
  console.log(`WhatsApp bot hooks: http://localhost:${port}/hook/order`)
  console.log(`Kitchen server:     ${kitchenServerUrl}`)
  console.log(`Catalog:            ${catalogPath}`)
  if (allowlist.length === 0) {
    console.warn('WHATSAPP_ALLOWLIST is empty: commands are ignored and notifications are not sent')
  } else {
    console.log(`Allowlist:          ${allowlist.join(', ')}`)
  }
  console.log(chromiumPath ? `Chromium:           ${chromiumPath}` : 'Chromium:           puppeteer default (set WHATSAPP_CHROMIUM_PATH if it fails)')
  client.initialize().catch((error: unknown) => {
    whatsappStatus.lastError = String(error)
    console.error('WhatsApp initialization failed', error)
  })
})

function shutdown (): void {
  client.destroy().catch(() => {})
  server.close(() => process.exit(0))
}

process.on('SIGINT', shutdown)
process.on('SIGTERM', shutdown)
