const MONTHS = [
  'enero',
  'febrero',
  'marzo',
  'abril',
  'mayo',
  'junio',
  'julio',
  'agosto',
  'septiembre',
  'octubre',
  'noviembre',
  'diciembre',
]

/**
 * The store keeps the delivery date as a WooCommerce order meta whose value is
 * a Spanish string, e.g. `1 octubre, 2026`. The Order Delivery Date plugin and
 * the WP All Export filter both use that shape, so the date we send has to match.
 */
export function toStoreDate (iso: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso.trim())
  if (!match) throw new Error(`Invalid date "${iso}". Expected YYYY-MM-DD`)
  const year = match[1]!
  const month = Number(match[2]!)
  const day = Number(match[3]!)
  if (month < 1 || month > 12 || day < 1 || day > 31) {
    throw new Error(`Invalid date "${iso}"`)
  }
  return `${day} ${MONTHS[month - 1]}, ${year}`
}
