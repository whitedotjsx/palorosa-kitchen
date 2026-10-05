import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import {
  groupEntriesByCategory,
  measureLabels,
  unitCategoryLabels,
  unresolvedReasonLabels,
  type Catalog,
} from '@palorosa-kitchen/core'
import { configFromEnv } from './env'
import { buildDayList, publishList } from './lib'

interface Args {
  date?: string
  publish?: string
  catalog?: string
  json: boolean
  help: boolean
}

function parseArgs (argv: string[]): Args {
  const args: Args = { json: false, help: false }
  for (let index = 0; index < argv.length; index++) {
    const token = argv[index]
    if (token === '--json') args.json = true
    else if (token === '--help' || token === '-h') args.help = true
    else if (token === '--publish') {
      const value = argv[++index]
      if (value !== undefined) args.publish = value
    } else if (token === '--catalog') {
      const value = argv[++index]
      if (value !== undefined) args.catalog = value
    } else if (token && !token.startsWith('-') && !args.date) args.date = token
  }
  return args
}

const USAGE = [
  'Usage: pnpm orders <YYYY-MM-DD> [--publish <serverUrl>] [--catalog <seed.json>] [--json]',
  '',
  'Pulls the delivery day\'s orders from the store WP All Export (HTTP only) and prints the kitchen list.',
  '',
  'Env (.env): WP_ADMIN_URL, WP_ADMIN_USER, WP_ADMIN_PASSWORD, WP_EXPORT_ID,',
  '            WP_EXPORT_CRON_KEY, WP_EXPORT_TOKEN (optional, derived from the cron key).',
].join('\n')

async function main (): Promise<void> {
  const args = parseArgs(process.argv.slice(2))
  if (args.help) {
    console.log(USAGE)
    return
  }
  if (!args.date) {
    console.log(USAGE)
    process.exitCode = 1
    return
  }
  const catalogPath = args.catalog ?? fileURLToPath(new URL('../../../data/seed.json', import.meta.url))
  const catalog = JSON.parse(await readFile(catalogPath, 'utf8')) as Catalog
  const config = configFromEnv()

  const result = await buildDayList(config, args.date, catalog)

  console.log(`Delivery ${args.date}: ${result.orderCount} orders, ${result.lines.length} lines`)
  for (const group of groupEntriesByCategory(result.list)) {
    console.log(`\n${unitCategoryLabels[group.category]}`)
    for (const entry of group.entries) {
      console.log(`  ${entry.quantity} x ${entry.name} (${measureLabels[entry.measure]})`)
    }
  }
  if (result.list.unresolved.length > 0) {
    console.log('\nSIN RESOLVER')
    for (const entry of result.list.unresolved) {
      const detail = entry.detail ? ` · ${entry.detail}` : ''
      console.log(`  ${entry.quantity} x ${entry.productText} — ${unresolvedReasonLabels[entry.reason]}${detail} [${entry.references.join(', ')}]`)
    }
  }

  if (args.publish) {
    await publishList(args.publish, args.date, result.list, 'store orders (WP All Export)', result.orderCount)
    console.log(`\nPublished to ${args.publish}`)
  }
  if (args.json) console.log(`\n${JSON.stringify(result.list, null, 2)}`)
}

main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : error)
  process.exit(1)
})
