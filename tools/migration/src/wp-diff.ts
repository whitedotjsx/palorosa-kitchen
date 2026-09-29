import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { normalizeName, type Catalog, type Product } from '@palorosa-kitchen/core'
import { findByLooseName } from './match'
import { dataDir } from './paths'
import type { WpProduct, WpSnapshot } from './wp-types'

async function readJson<T> (name: string): Promise<T> {
  return JSON.parse(await readFile(join(dataDir, name), 'utf8')) as T
}

async function main (): Promise<void> {
  const snapshot = await readJson<WpSnapshot>('wp-products.snapshot.json')
  const catalog = await readJson<Catalog>('seed.json')
  const parents = snapshot.products.filter((product) => product.type !== 'variation')

  interface Match {
    wp: WpProduct
    product: Product
    exact: boolean
  }

  const BALLOON_PATTERN = /globo|burbuja|helio/
  const excluded = parents.filter(
    (wp) => BALLOON_PATTERN.test(normalizeName(wp.name)) || normalizeName(wp.name) === 'domicilio exclusivo'
  )
  const sellable = parents.filter((wp) => !excluded.includes(wp))

  const matches: Match[] = []
  const wpOnly: WpProduct[] = []
  for (const wp of sellable) {
    const exact =
      catalog.products.find((product) => product.wpProductId === wp.id) ??
      catalog.products.find((product) => normalizeName(product.name) === normalizeName(wp.name))
    if (exact) {
      matches.push({ wp, product: exact, exact: true })
      continue
    }
    const loose = findByLooseName(wp.name, catalog.products, (product) => product.name)
    if (loose) matches.push({ wp, product: loose, exact: false })
    else wpOnly.push(wp)
  }

  const matchedIds = new Set(matches.map((match) => match.product.id))
  const catalogOnly = catalog.products.filter((product) => !matchedIds.has(product.id))

  const lines: string[] = []
  lines.push('# WordPress diff')
  lines.push('')
  lines.push(`Snapshot: ${snapshot.fetchedAt}`)
  lines.push('')
  lines.push('| Metric | Count |')
  lines.push('| --- | --- |')
  lines.push(`| WP products (parents) | ${parents.length} |`)
  lines.push(`| WP products sellable | ${sellable.length} |`)
  lines.push(`| WP products excluded by design | ${excluded.length} |`)
  lines.push(`| WP variations | ${snapshot.variations.length} |`)
  lines.push(`| Catalog products | ${catalog.products.length} |`)
  lines.push(`| Exact matches | ${matches.filter((match) => match.exact).length} |`)
  lines.push(`| Fuzzy matches | ${matches.filter((match) => !match.exact).length} |`)
  lines.push(`| WP only | ${wpOnly.length} |`)
  lines.push(`| Catalog only | ${catalogOnly.length} |`)
  lines.push('')

  lines.push('## Catalog breakfasts vs WP')
  lines.push('')
  for (const breakfast of catalog.products.filter((product) => product.category === 'breakfast')) {
    const match = matches.find((entry) => entry.product.id === breakfast.id)
    if (match) {
      lines.push(
        `- "${breakfast.name}" -> WP #${match.wp.id} "${match.wp.name}" [${match.wp.status}, ${match.wp.type}, ${match.wp.price}]${match.exact ? '' : ' (fuzzy)'}`
      )
    } else {
      lines.push(`- MISS "${breakfast.name}" not found in WP`)
    }
  }
  lines.push('')

  lines.push('## Excluded by design')
  lines.push('')
  for (const wp of excluded.sort((a, b) => a.name.localeCompare(b.name))) {
    lines.push(`- #${wp.id} "${wp.name}" [${wp.status}]`)
  }
  lines.push('')

  lines.push('## WP only')
  lines.push('')
  for (const wp of wpOnly.sort((a, b) => a.name.localeCompare(b.name))) {
    const categories = (wp.categories ?? []).map((category) => category.name).join('/') || 'sin categoria'
    lines.push(`- #${wp.id} "${wp.name}" [${wp.status}, ${wp.type}, ${categories}]`)
  }
  lines.push('')

  lines.push('## Fuzzy matches')
  lines.push('')
  for (const match of matches.filter((entry) => !entry.exact)) {
    lines.push(`- WP "${match.wp.name}" ~ catalog "${match.product.name}" [${match.product.category}]`)
  }
  lines.push('')

  lines.push('## Catalog only')
  lines.push('')
  for (const product of catalogOnly.sort((a, b) => a.name.localeCompare(b.name))) {
    lines.push(`- "${product.name}" [${product.category}${product.active ? '' : ', inactive'}]`)
  }
  lines.push('')

  lines.push('## Variations per matched product')
  lines.push('')
  for (const match of matches.filter((entry) => entry.product.category === 'breakfast')) {
    const variations = snapshot.variations.filter(
      (variation) =>
        variation.parent_id === match.wp.id && variation.status === 'publish' && variation.name.trim()
    )
    if (variations.length === 0) continue
    const names = [...new Set(variations.map((variation) => variation.name.trim()))]
    lines.push(`- "${match.wp.name}": ${names.join(' | ')}`)
  }
  lines.push('')

  await writeFile(join(dataDir, 'wp-diff.md'), lines.join('\n'), 'utf8')

  console.log('WordPress diff written to data/wp-diff.md')
  console.log(
    `  exact ${matches.filter((match) => match.exact).length}, fuzzy ${matches.filter((match) => !match.exact).length}, wp only ${wpOnly.length}, catalog only ${catalogOnly.length}`
  )
}

await main()
