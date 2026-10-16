export const DEFAULT_RESTAURANT_TIMEZONE = 'Europe/Moscow'
export type RestaurantTimeErrorCode = 'invalid_datetime' | 'invalid_timezone' | 'nonexistent_time' | 'ambiguous_time'

export class RestaurantTimeError extends Error {
  constructor(public readonly code: RestaurantTimeErrorCode, message: string) {
    super(message)
    this.name = 'RestaurantTimeError'
  }
}

const localPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?$/
const apiPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/

function invalidDateTime(): never {
  throw new RestaurantTimeError('invalid_datetime', 'Укажите существующую дату и время в правильном формате.')
}

function wallTime(match: RegExpMatchArray): number {
  const [, year, month, day, hour, minute, second = '00', fraction = ''] = match
  const parts = [year, month, day, hour, minute, second].map(Number)
  if (parts[0] < 1 || parts[0] > 9999 || parts[1] < 1 || parts[1] > 12 || parts[2] < 1
    || parts[3] > 23 || parts[4] > 59 || parts[5] > 59) invalidDateTime()
  const date = new Date(0)
  date.setUTCFullYear(parts[0], parts[1] - 1, parts[2])
  date.setUTCHours(parts[3], parts[4], parts[5], Number(fraction.slice(0, 3).padEnd(3, '0')))
  if (date.getUTCFullYear() !== parts[0] || date.getUTCMonth() !== parts[1] - 1 || date.getUTCDate() !== parts[2]) invalidDateTime()
  return date.getTime()
}

export function parseApiDateTime(raw: string): Date {
  const match = raw.match(apiPattern)
  if (!match || match[0] !== raw) return invalidDateTime()
  const local = wallTime(match)
  const offset = match[8]
  const hours = offset === 'Z' ? 0 : Number(offset.slice(1, 3))
  const minutes = offset === 'Z' ? 0 : Number(offset.slice(4, 6))
  if (hours > 23 || minutes > 59) invalidDateTime()
  const direction = offset.startsWith('-') ? -1 : 1
  return new Date(local - direction * (hours * 60 + minutes) * 60_000)
}

function formatter(timeZone: string): Intl.DateTimeFormat {
  try {
    if (/^[+-]/.test(timeZone)) throw new RangeError('Expected IANA timezone')
    return new Intl.DateTimeFormat('en-CA', {
      timeZone, year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
    })
  } catch {
    throw new RestaurantTimeError('invalid_timezone', 'Укажите корректный часовой пояс IANA.')
  }
}

function localValue(date: Date, format: Intl.DateTimeFormat): string {
  const parts = Object.fromEntries(format.formatToParts(date).map(({ type, value }) => [type, value]))
  return `${parts.year.padStart(4, '0')}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}:${parts.second}`
}

export function restaurantDateTimeToInput(raw: string | null, timeZone = DEFAULT_RESTAURANT_TIMEZONE): string | null {
  if (raw === null) return null
  const date = parseApiDateTime(raw)
  const fraction = raw.match(apiPattern)![7]
  return localValue(date, formatter(timeZone)) + (fraction ? `.${fraction}` : '')
}

export function restaurantDateTimeFromInput(raw: string | null, timeZone = DEFAULT_RESTAURANT_TIMEZONE): string | null {
  if (raw === null || raw === '') return null
  const match = raw.match(localPattern)
  if (!match || match[0] !== raw) return invalidDateTime()
  const wall = wallTime(match)
  const format = formatter(timeZone)
  const expected = localValue(new Date(wall), formatter('UTC'))
  const offsets = new Set<number>()
  for (const hours of [-48, -24, -12, 0, 12, 24, 48]) {
    const sample = wall + hours * 3_600_000
    const local = localValue(new Date(sample), format).match(localPattern)!
    offsets.add(wallTime(local) - Math.floor(sample / 1000) * 1000)
  }
  const candidates = [...offsets].map((offset) => new Date(wall - offset))
    .filter((date) => localValue(date, format) === expected)
  if (candidates.length === 0) {
    throw new RestaurantTimeError('nonexistent_time', 'Это время отсутствует при переходе на летнее время. Выберите другое время.')
  }
  if (candidates.length > 1) {
    throw new RestaurantTimeError('ambiguous_time', 'Это время повторяется при смене часового пояса. Выберите однозначное время.')
  }
  const fraction = match[7]
  return candidates[0].toISOString().slice(0, 19) + (fraction ? `.${fraction}` : '') + 'Z'
}

export function formatRestaurantDateTime(raw: string | null, timeZone = DEFAULT_RESTAURANT_TIMEZONE): string {
  if (raw === null) return '—'
  const local = restaurantDateTimeToInput(raw, timeZone)!
  const [date, time] = local.split('T')
  const [year, month, day] = date.split('-')
  return `${day}.${month}.${year}, ${time.slice(0, 5)}`
}
