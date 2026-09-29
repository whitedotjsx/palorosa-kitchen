import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { loadWpEnv } from './env'
import { dataDir } from './paths'
import { createWpClient } from './wp-client'
import type { WpCategory, WpProduct, WpSnapshot, WpVariation } from './wp-types'

async function main (): Promise<void> {
  const client = createWpClient(loadWpEnv())
  const categories = await client.getAll<WpCategory>('products/categories', { per_page: 100 })
  const products = await client.getAll<WpProduct>('products', { status: 'any', per_page: 100 })

  const variables = products.filter((product) => product.type === 'variable')
  const variations: WpVariation[] = []
  for (const parent of variables) {
    const parentVariations = await client.getAll<WpVariation>(`products/${parent.id}/variations`, {
      status: 'any',
      per_page: 100,
    })
    variations.push(...parentVariations)
  }

  const snapshot: WpSnapshot = {
    fetchedAt: new Date().toISOString(),
    categories,
    products,
    variations,
  }
  const target = join(dataDir, 'wp-products.snapshot.json')
  await writeFile(target, `${JSON.stringify(snapshot, null, 2)}\n`, 'utf8')

  console.log('WordPress snapshot saved')
  console.log(`  categories: ${categories.length}`)
  console.log(`  products: ${products.length} (${variables.length} variable)`)
  console.log(`  variations: ${variations.length}`)
  console.log(`  file: ${target}`)
}

await main()
