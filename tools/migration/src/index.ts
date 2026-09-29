import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import {
  defaultLabelParsingRules,
  SCHEMA_VERSION,
  slugify,
  type Catalog,
  type Container,
  type LabelParsingRules,
} from '@palorosa-kitchen/core'
import { buildProducts } from './build-products'
import { buildRecipes } from './build-recipes'
import { buildUnits } from './build-units'
import {
  applyContainerOverrides,
  applyOptionOverrides,
  applyProductOverrides,
  applyQueueOverrides,
  applyRecipeOverrides,
  applyUnitMerges,
  applyUnitOverrides,
  applyUnitRewrites,
  resolveRecipeQueueItems,
  type CatalogOverrides,
} from './overrides'
import { dataDir } from './paths'
import { readSource, sourceDir } from './read-source'
import { buildReport } from './report'
import { ReviewQueue, type ReviewDecisions } from './review'
import type { WpSnapshot } from './wp-types'

const labelParsing: LabelParsingRules = defaultLabelParsingRules

async function loadDecisions (): Promise<ReviewDecisions> {
  try {
    const raw = await readFile(join(dataDir, 'review-decisions.json'), 'utf8')
    return JSON.parse(raw) as ReviewDecisions
  } catch {
    return {}
  }
}

async function loadWpSnapshot (): Promise<WpSnapshot | null> {
  try {
    const raw = await readFile(join(dataDir, 'wp-products.snapshot.json'), 'utf8')
    return JSON.parse(raw) as WpSnapshot
  } catch {
    return null
  }
}

async function loadOverrides (): Promise<CatalogOverrides> {
  try {
    const raw = await readFile(join(sourceDir, 'overrides.json'), 'utf8')
    return JSON.parse(raw) as CatalogOverrides
  } catch {
    return {}
  }
}

export async function runMigration (): Promise<void> {
  const source = await readSource()
  const decisions = await loadDecisions()
  const overrides = await loadOverrides()
  const wpSnapshot = await loadWpSnapshot()
  const queue = new ReviewQueue()

  const units = buildUnits({
    kitchenFoods: source.kitchenFoods,
    decorations: source.decorations,
    decisions,
    queue,
  })
  applyUnitOverrides(units.units, overrides)

  const products = buildProducts({
    breakfasts: source.breakfasts,
    additions: source.additions,
    additionPrices: source.additionPrices,
    storeProducts: source.storeProducts,
    cajaSaludDrinks: source.cajaSaludDrinks,
    wpSnapshot,
  })
  applyProductOverrides(products.products, overrides)

  const recipes = buildRecipes({
    breakfasts: source.breakfasts,
    products: products.products,
    units: units.units,
    resolve: units.resolve,
    queue,
    wpSnapshot,
  })
  applyRecipeOverrides(recipes.recipes, overrides)
  applyOptionOverrides(recipes.recipes, overrides)
  applyUnitMerges(recipes.recipes, overrides)
  applyUnitRewrites(recipes.recipes, overrides)

  for (const missing of products.missingInWp) {
    queue.add({
      type: 'evaluation_product_missing_in_wp',
      severity: 'info',
      title: missing.name,
      detail: 'Evaluation product without a matching WordPress product. Kept inactive.',
      refs: [missing.id],
    })
  }

  for (const excluded of products.excludedWp) {
    queue.add({
      type: 'excluded_wp_product',
      severity: 'info',
      title: excluded.name,
      detail: `Excluded from the catalog: ${excluded.reason}.`,
      refs: [String(excluded.id)],
    })
  }

  resolveRecipeQueueItems(queue, overrides)
  applyQueueOverrides(queue, overrides)

  const containers: Container[] = []
  const containerIds = new Set<string>()
  for (const name of Object.values(source.containers)) {
    const id = slugify(name)
    if (containerIds.has(id)) continue
    containerIds.add(id)
    containers.push({ id, name, aliases: [name], active: true })
  }
  containers.sort((a, b) => a.id.localeCompare(b.id))
  applyContainerOverrides(containers, overrides)

  const catalog: Catalog = {
    schemaVersion: SCHEMA_VERSION,
    units: units.units,
    products: products.products,
    recipes: recipes.recipes,
    containers,
    labelParsing,
  }

  await mkdir(dataDir, { recursive: true })
  await writeFile(join(dataDir, 'seed.json'), `${JSON.stringify(catalog, null, 2)}\n`, 'utf8')
  await writeFile(
    join(dataDir, 'review-queue.json'),
    `${JSON.stringify({ items: queue.sorted() }, null, 2)}\n`,
    'utf8'
  )
  await writeFile(
    join(dataDir, 'migration-report.md'),
    buildReport({ units, products, recipes, queue }),
    'utf8'
  )

  const counts = queue.countByType()
  console.log('Migration finished')
  console.log(
    `  source: ${wpSnapshot ? `WordPress snapshot of ${wpSnapshot.fetchedAt}` : 'evaluation constants only (no snapshot)'}`
  )
  console.log(`  units: ${units.units.length}`)
  console.log(
    `  products: ${products.products.length} (${products.products.filter((p) => p.active).length} active, ${products.products.filter((p) => !p.active).length} inactive)`
  )
  console.log(`  recipes: ${recipes.recipes.length}`)
  console.log(`  containers: ${containers.length}`)
  console.log(`  review items: ${queue.items.length} ${JSON.stringify(counts)}`)
  console.log('  output: data/seed.json, data/review-queue.json, data/migration-report.md')
}

await runMigration()
