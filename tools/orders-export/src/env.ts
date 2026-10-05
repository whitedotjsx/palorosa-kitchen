import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

export interface OrdersExportConfig {
  /** Store base URL, e.g. https://palorosabreakfast.com */
  base: string
  /** WordPress admin user with permission to edit the export. */
  user: string
  /** WordPress admin password (wp-login, not an application password). */
  password: string
  /** WP All Export saved export id. */
  exportId: string
  /** WP All Export "Cron Job Key" (Settings). */
  cronKey: string
  /** Download token: substr(md5(cronKey + exportId), 0, 16). */
  token: string
}

function readEnvFile (): Record<string, string> {
  const result: Record<string, string> = {}
  let raw = ''
  try {
    raw = readFileSync(fileURLToPath(new URL('../../../.env', import.meta.url)), 'utf8')
  } catch {
    return result
  }
  for (const line of raw.split(/\r?\n/)) {
    const match = /^\s*([A-Za-z0-9_]+)\s*=\s*(.*)$/.exec(line)
    if (match) result[match[1]!] = (match[2] ?? '').trim()
  }
  return result
}

const fileEnv = readEnvFile()

function value (env: NodeJS.ProcessEnv, key: string, fallback: string): string {
  const fromProcess = env[key]
  if (fromProcess !== undefined && fromProcess !== '') return fromProcess
  const fromFile = fileEnv[key]
  if (fromFile !== undefined && fromFile !== '') return fromFile
  return fallback
}

export function exportToken (cronKey: string, exportId: string): string {
  return createHash('md5').update(`${cronKey}${exportId}`).digest('hex').slice(0, 16)
}

export function configFromEnv (env: NodeJS.ProcessEnv = process.env): OrdersExportConfig {
  const base = value(env, 'WP_ADMIN_URL', 'https://palorosabreakfast.com').replace(/\/+$/, '')
  const exportId = value(env, 'WP_EXPORT_ID', '1')
  const cronKey = value(env, 'WP_EXPORT_CRON_KEY', '')
  const token = value(env, 'WP_EXPORT_TOKEN', '') || (cronKey ? exportToken(cronKey, exportId) : '')
  return {
    base,
    user: value(env, 'WP_ADMIN_USER', ''),
    password: value(env, 'WP_ADMIN_PASSWORD', ''),
    exportId,
    cronKey,
    token,
  }
}
