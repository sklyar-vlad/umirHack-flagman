// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createMockEventResponse } from './event-handlers'
import { setMockScenario } from './scenarios'

beforeEach(() => { vi.useFakeTimers(); setMockScenario('success') })
afterEach(() => vi.useRealTimers())
async function open(cursor?: string) {
  const abort = new AbortController()
  const response = createMockEventResponse(new Request(`${window.location.origin}/api/v1/events`, { signal: abort.signal, headers: cursor ? { 'Last-Event-ID': cursor } : {} }))
  const reader = response.body!.getReader()
  const read = async () => new TextDecoder().decode((await reader.read()).value)
  expect(response.headers.get('Content-Type')).toBe('text/event-stream')
  expect(await read()).toContain(': mock heartbeat')
  return { read, close: async () => {
    abort.abort()
    await reader.cancel()
  } }
}
it('explicit mock streams emit the published periodic time marker without any write', async () => {
  const stream = await open('9007199254740993')
  await vi.advanceTimersByTimeAsync(30_000)
  for (let i = 1; i < 30; i++) expect(await stream.read()).toContain(': mock heartbeat')
  const message = await stream.read()
  expect(message).toContain('id: 9007199254740994'); expect(message).toContain('event: inventory.time_changed')
  expect(message).toContain('"topics":["inventory","batches","dashboard","dishes","stop-list","hot-offers","procurement-recommendations"]')
  await stream.close(); expect(vi.getTimerCount()).toBe(0)
})
it('explicit replay produces an identical cursor and resync produces a new high-water', async () => {
  setMockScenario('events-replay')
  const stream = await open('20')
  await vi.advanceTimersByTimeAsync(2_000)
  const first = await stream.read(); expect(first).toContain('id: 21'); expect(await stream.read()).toBe(first)
  setMockScenario('events-resync'); await vi.advanceTimersByTimeAsync(1_000)
  const resync = await stream.read(); expect(resync).toContain('id: 22'); expect(resync).toContain('event: resync.required')
  await stream.close(); expect(vi.getTimerCount()).toBe(0)
})
