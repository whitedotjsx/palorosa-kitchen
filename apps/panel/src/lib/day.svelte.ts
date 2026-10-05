// The day shown by Lista and Pedidos. Shared so switching screens keeps the
// date the operator picked.

/** YYYY-MM-DD in Bogotá (UTC-5, no daylight saving), `days` from today. */
export function bogotaDate (days = 0): string {
  const shifted = new Date(Date.now() + days * 24 * 60 * 60 * 1000 - 5 * 60 * 60 * 1000)
  return shifted.toISOString().slice(0, 10)
}

export const day = $state({ date: bogotaDate(0) })

/** Long Spanish label for a YYYY-MM-DD date ("viernes, 3 de octubre"). */
export function longDate (date: string): string {
  const [year = 1970, month = 1, dayOfMonth = 1] = date.split('-').map(Number)
  return new Date(Date.UTC(year, month - 1, dayOfMonth, 12)).toLocaleDateString('es-CO', {
    timeZone: 'UTC', weekday: 'long', day: 'numeric', month: 'long',
  })
}
