import { describe, expect, it } from 'vitest'
import {
  RestaurantTimeError, formatRestaurantDateTime, parseApiDateTime,
  restaurantDateTimeFromInput, restaurantDateTimeToInput,
} from './index'

describe('API instants and restaurant wall time', () => {
  it('uses Moscow by default, independent of the browser timezone', () => {
    expect(parseApiDateTime('2026-10-10T15:30:00+03:00').toISOString()).toBe('2026-10-10T12:30:00.000Z')
    expect(restaurantDateTimeToInput('2026-10-10T12:30:00Z')).toBe('2026-10-10T15:30:00')
    expect(restaurantDateTimeFromInput('2026-10-10T15:30')).toBe('2026-10-10T12:30:00Z')
    expect(formatRestaurantDateTime('2026-10-10T12:30:00Z')).toBe('10.10.2026, 15:30')
  })

  it('preserves null and treats a blank optional datetime input as null', () => {
    expect(restaurantDateTimeToInput(null)).toBeNull()
    expect(restaurantDateTimeFromInput(null)).toBeNull()
    expect(restaurantDateTimeFromInput('')).toBeNull()
    expect(formatRestaurantDateTime(null)).toBe('—')
  })

  it('preserves nanoseconds in string conversions', () => {
    const raw = '2026-10-10T12:30:00.123456789Z'
    expect(restaurantDateTimeToInput(raw)).toBe('2026-10-10T15:30:00.123456789')
    expect(restaurantDateTimeFromInput(restaurantDateTimeToInput(raw))).toBe(raw)
    expect(parseApiDateTime(raw).getUTCMilliseconds()).toBe(123)
  })

  it('handles date rollovers, leap days, midnight and years below 100', () => {
    expect(restaurantDateTimeToInput('2026-12-31T22:00:00Z')).toBe('2027-01-01T01:00:00')
    expect(restaurantDateTimeFromInput('2027-01-01T00:00')).toBe('2026-12-31T21:00:00Z')
    expect(restaurantDateTimeFromInput('2024-02-29T00:00', 'UTC')).toBe('2024-02-29T00:00:00Z')
    expect(parseApiDateTime('0026-02-28T12:00:00Z').getUTCFullYear()).toBe(26)
    expect(restaurantDateTimeFromInput('0026-02-28T12:00', 'UTC')).toBe('0026-02-28T12:00:00Z')
  })

  it('uses a configured timezone, including fractional-hour offsets', () => {
    expect(restaurantDateTimeFromInput('2026-10-10T15:30', 'Asia/Kathmandu')).toBe('2026-10-10T09:45:00Z')
    expect(restaurantDateTimeToInput('2026-10-10T09:45:00Z', 'Asia/Kathmandu')).toBe('2026-10-10T15:30:00')
    expect(restaurantDateTimeFromInput('2026-07-10T12:00', 'America/New_York')).toBe('2026-07-10T16:00:00Z')
    expect(restaurantDateTimeFromInput('2026-01-10T12:00', 'America/New_York')).toBe('2026-01-10T17:00:00Z')
  })

  it.each([
    '', '2026-10-10T15:30:00', '2026-02-30T12:30:00Z', '2026-02-29T12:30:00Z',
    '2026-13-01T12:30:00Z', '2026-01-00T12:30:00Z', '2026-10-10T24:00:00Z',
    '2026-10-10T12:60:00Z', '2026-10-10T12:30:60Z', '2026-10-10T12:30:00+24:00',
    '2026-10-10T12:30:00+03:60', '2026-10-10T12:30:00Z\n',
  ])('rejects invalid API timestamp %j instead of normalizing it', (raw) => {
    expect(() => parseApiDateTime(raw)).toThrow(RestaurantTimeError)
  })

  it.each(['2026-02-30T12:00', '2026-10-10T24:00', '2026-10-10', '2026-10-10T12:00Z', '2026-10-10T12:00\n'])(
    'rejects invalid datetime-local %j', (raw) => {
      expect(() => restaurantDateTimeFromInput(raw)).toThrow(RestaurantTimeError)
    },
  )

  it('rejects invalid timezone names', () => {
    expect(() => restaurantDateTimeToInput('2026-10-10T12:30:00Z', 'Mars/Olympus'))
      .toThrow(expect.objectContaining({ code: 'invalid_timezone' }))
    expect(() => restaurantDateTimeFromInput('2026-10-10T12:30', 'Mars/Olympus')).toThrow(RestaurantTimeError)
    expect(() => restaurantDateTimeFromInput('2026-10-10T12:30', '+03:00'))
      .toThrow(expect.objectContaining({ code: 'invalid_timezone' }))
  })
})

describe('DST transitions', () => {
  it('rejects nonexistent spring times', () => {
    expect(() => restaurantDateTimeFromInput('2026-03-08T02:30', 'America/New_York'))
      .toThrow(expect.objectContaining({ code: 'nonexistent_time' }))
    expect(restaurantDateTimeFromInput('2026-03-08T03:30', 'America/New_York')).toBe('2026-03-08T07:30:00Z')
  })

  it('rejects ambiguous fall times and still displays both API instants correctly', () => {
    expect(() => restaurantDateTimeFromInput('2026-11-01T01:30', 'America/New_York'))
      .toThrow(expect.objectContaining({ code: 'ambiguous_time' }))
    expect(restaurantDateTimeToInput('2026-11-01T05:30:00Z', 'America/New_York')).toBe('2026-11-01T01:30:00')
    expect(restaurantDateTimeToInput('2026-11-01T06:30:00Z', 'America/New_York')).toBe('2026-11-01T01:30:00')
    expect(restaurantDateTimeFromInput('2026-11-01T02:30', 'America/New_York')).toBe('2026-11-01T07:30:00Z')
  })

  it('handles half-hour DST transitions', () => {
    expect(() => restaurantDateTimeFromInput('2026-10-04T02:15', 'Australia/Lord_Howe'))
      .toThrow(expect.objectContaining({ code: 'nonexistent_time' }))
    expect(() => restaurantDateTimeFromInput('2026-04-05T01:45', 'Australia/Lord_Howe'))
      .toThrow(expect.objectContaining({ code: 'ambiguous_time' }))
    expect(restaurantDateTimeFromInput('2026-10-04T02:30', 'Australia/Lord_Howe')).toBe('2026-10-03T15:30:00Z')
  })
})
