// The day shown by Lista and Pedidos. Shared so switching screens keeps the
// date the operator picked. Lista can widen that day to a consecutive range and
// show the combined list of every day in it.

/** YYYY-MM-DD in Bogotá (UTC-5, no daylight saving), `days` from today. */
export function bogotaDate (days = 0): string {
  const shifted = new Date(Date.now() + days * 24 * 60 * 60 * 1000 - 5 * 60 * 60 * 1000)
  return shifted.toISOString().slice(0, 10)
}

export const day = $state({
  date: bogotaDate(0),
  // Several days at once: Lista shows the sum of from..to inclusive.
  range: false,
  from: bogotaDate(0),
  to: bogotaDate(1),
})

/** Widest range offered, so one view cannot fan out into a year of requests. */
export const maxRangeDays = 14

/** UTC midnight of a YYYY-MM-DD date, or null when it is malformed. */
function midnight (date: string): number | null {
  const [year, month, dayOfMonth] = date.split('-').map(Number)
  if (!year || !month || !dayOfMonth) return null
  return Date.UTC(year, month - 1, dayOfMonth)
}

function isoDate (time: number): string {
  return new Date(time).toISOString().slice(0, 10)
}

/** `days` after a YYYY-MM-DD date. */
export function shiftDate (date: string, days: number): string {
  const base = midnight(date)
  if (base === null) return date
  return isoDate(base + days * 24 * 60 * 60 * 1000)
}

/** Every day from `from` to `to` inclusive, oldest first, capped at `max`. */
export function rangeDates (from: string, to: string, max = maxRangeDays): string[] {
  const start = midnight(from)
  const end = midnight(to)
  if (start === null || end === null) return [bogotaDate(0)]
  const first = Math.min(start, end)
  const last = Math.max(start, end)
  const dates: string[] = []
  for (let cursor = first; cursor <= last && dates.length < max; cursor += 24 * 60 * 60 * 1000) {
    dates.push(isoDate(cursor))
  }
  return dates
}

function formatDate (date: string, options: Intl.DateTimeFormatOptions): string {
  const time = midnight(date)
  if (time === null) return date
  // Noon avoids any timezone sliding the calendar day around midnight.
  return new Date(time + 12 * 60 * 60 * 1000).toLocaleDateString('es-CO', { timeZone: 'UTC', ...options })
}

/** Long Spanish label for a YYYY-MM-DD date ("viernes, 3 de octubre"). */
export function longDate (date: string): string {
  return formatDate(date, { weekday: 'long', day: 'numeric', month: 'long' })
}

/** Short Spanish label for a date chip ("mié, 8 oct"). */
export function shortDate (date: string): string {
  return formatDate(date, { weekday: 'short', day: 'numeric', month: 'short' })
}

/** Spanish label for an inclusive range ("3 – 5 de octubre"). */
export function rangeLabel (from: string, to: string): string {
  if (from === to) return longDate(from)
  if (midnight(from) === null || midnight(to) === null) return from
  const sameMonth = from.slice(0, 7) === to.slice(0, 7)
  const sameYear = from.slice(0, 4) === to.slice(0, 4)
  const year: Intl.DateTimeFormatOptions = sameYear ? {} : { year: 'numeric' }
  const start: Intl.DateTimeFormatOptions = sameMonth
    ? { day: 'numeric', ...year }
    : { day: 'numeric', month: 'long', ...year }
  const end: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'long', ...year }
  return `${formatDate(from, start)} – ${formatDate(to, end)}`
}
