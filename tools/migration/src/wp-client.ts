import type { WpEnv } from './env'

export interface WpPage<T> {
  data: T[]
  total: number
  totalPages: number
}

export interface WpClient {
  get<T>(path: string, params?: Record<string, string | number>): Promise<WpPage<T>>
  getAll<T>(path: string, params?: Record<string, string | number>): Promise<T[]>
}

export function createWpClient (env: WpEnv): WpClient {
  const base = env.url.replace(/\/+$/, '')
  const auth = Buffer.from(`${env.consumerKey}:${env.consumerSecret}`).toString('base64')

  async function get<T> (
    path: string,
    params: Record<string, string | number> = {}
  ): Promise<WpPage<T>> {
    const url = new URL(`${base}/wp-json/wc/v3/${path}`)
    for (const [key, value] of Object.entries(params)) url.searchParams.set(key, String(value))
    const response = await fetch(url, { headers: { Authorization: `Basic ${auth}` } })
    if (!response.ok) {
      throw new Error(`WP ${path} failed: ${response.status} ${await response.text()}`)
    }
    return {
      data: (await response.json()) as T[],
      total: Number(response.headers.get('x-wp-total') ?? '0'),
      totalPages: Number(response.headers.get('x-wp-totalpages') ?? '1'),
    }
  }

  async function getAll<T> (
    path: string,
    params: Record<string, string | number> = {}
  ): Promise<T[]> {
    const first = await get<T>(path, { ...params, page: 1 })
    const all = [...first.data]
    for (let page = 2; page <= first.totalPages; page += 1) {
      const next = await get<T>(path, { ...params, page })
      all.push(...next.data)
    }
    return all
  }

  return { get, getAll }
}
