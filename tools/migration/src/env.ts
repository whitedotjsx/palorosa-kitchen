import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { rootDir } from './paths'

export interface WpEnv {
  url: string
  consumerKey: string
  consumerSecret: string
}

let cachedFileValues: Record<string, string> | null = null

function parseEnvFile (path: string): Record<string, string> {
  try {
    const values: Record<string, string> = {}
    for (const line of readFileSync(path, 'utf8').split(/\r?\n/)) {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#')) continue
      const separator = trimmed.indexOf('=')
      if (separator === -1) continue
      const key = trimmed.slice(0, separator).trim()
      let value = trimmed.slice(separator + 1).trim()
      if (
        (value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'"))
      ) {
        value = value.slice(1, -1)
      }
      values[key] = value
    }
    return values
  } catch {
    return {}
  }
}

function envFileValues (): Record<string, string> {
  if (!cachedFileValues) cachedFileValues = parseEnvFile(join(rootDir, '.env'))
  return cachedFileValues
}

export function readEnv (key: string): string {
  return process.env[key] ?? envFileValues()[key] ?? ''
}

export function loadWpEnv (): WpEnv {
  const env: WpEnv = {
    url: readEnv('WORDPRESS_URL'),
    consumerKey: readEnv('WOOCOMERCE_CONSUMER_KEY'),
    consumerSecret: readEnv('WOOCOMERCE_CONSUMER_SECRET'),
  }
  for (const [key, value] of Object.entries(env)) {
    if (!value) throw new Error(`Missing environment variable: ${key}`)
  }
  return env
}
