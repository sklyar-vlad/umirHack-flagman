import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { decodeEvent } from './event-contracts'
import { createLiveEvents, type EventStream } from './live-events'

const data = JSON.stringify({ occurred_at: '2026-10-13T17:00:00Z', topics: ['inventory', 'dishes'] })
class Stream implements EventStream {
  readyState = 0
  listeners = new Map<string, ((event: MessageEvent<string>) => void)[]>()
  close = vi.fn()
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) { this.listeners.set(type, [...this.listeners.get(type) ?? [], listener]) }
  emit(type: string, id = '1', body = data) { for (const listener of this.listeners.get(type) ?? []) listener({ lastEventId: id, data: body } as MessageEvent<string>) }
}
function setup() {
  const streams: Stream[] = [], refresh = vi.fn(), confirmSession = vi.fn(async () => true)
  const state = { online: true, visible: true }
  const connect = vi.fn(() => { const stream = new Stream(); streams.push(stream); return stream })
  const live = createLiveEvents({ connect, refresh, confirmSession, online: () => state.online, visible: () => state.visible })
  return { live, streams, refresh, confirmSession, state, connect }
}
beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())
describe('published SSE decoding', () => {
  it('preserves cursor precision and strips extra fields', () => {
    expect(decodeEvent('inventory.changed', '9007199254740993', data)?.cursor).toBe(9007199254740993n)
    expect(decodeEvent('inventory.changed', '1', JSON.stringify({ ...JSON.parse(data), request_id: 'hidden', password: 'hidden' }))).not.toHaveProperty('password')
  })
  it.each(['', '-1', '1e2', '9223372036854775808', '1\n2', '0'])('rejects invalid published cursor %s', (cursor) => { expect(decodeEvent('inventory.changed', cursor, data)).toBeNull() })
  it.each(['{', '{}', '{"occurred_at":"bad","topics":[]}', '{"occurred_at":"2026-10-13T17:00:00Z","topics":["passwords"]}'])('rejects malformed payload %s', (payload) => { expect(decodeEvent('inventory.changed', '1', payload)).toBeNull() })
})
describe('live connection lifecycle', () => {
  it('coalesces bursts, ignores duplicate/older cursors and uses exact high int64 IDs', async () => {
    const { live, streams: [stream], refresh } = setup()
    stream.emit('inventory.changed', '9007199254740993')
    stream.emit('inventory.changed', '9007199254740993')
    stream.emit('inventory.changed', '9007199254740992')
    stream.emit('dishes.changed', '9007199254740994')
    await vi.advanceTimersByTimeAsync(150)
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(refresh.mock.calls[0][0]).toHaveLength(2)
    expect(refresh.mock.calls[0][1]).toBe(false)
    live.stop()
  })
  it('full resync/reconnect refresh does not open a second stream and replay stays deduplicated', async () => {
    const { live, streams: [stream], refresh, connect } = setup()
    stream.emit('inventory.changed', '10'); await vi.advanceTimersByTimeAsync(150)
    stream.emit('error'); stream.emit('open'); stream.emit('inventory.changed', '10')
    await vi.advanceTimersByTimeAsync(150)
    expect(refresh).toHaveBeenLastCalledWith([], true)
    stream.emit('resync.required', '20'); await vi.advanceTimersByTimeAsync(150)
    expect(refresh).toHaveBeenLastCalledWith([], true)
    expect(connect).toHaveBeenCalledTimes(1)
    live.stop()
  })
  it('polls only visible online reads every ten seconds and bounds six attempts', async () => {
    const { live, streams: [stream], refresh, state } = setup()
    stream.emit('error')
    state.visible = false; live.wake(); await vi.advanceTimersByTimeAsync(20_000)
    expect(refresh).not.toHaveBeenCalled()
    state.visible = true; state.online = false; live.wake(); await vi.advanceTimersByTimeAsync(20_000)
    expect(refresh).not.toHaveBeenCalled()
    state.online = true; live.wake(); await vi.advanceTimersByTimeAsync(150)
    refresh.mockClear(); await vi.advanceTimersByTimeAsync(70_000)
    expect(refresh).toHaveBeenCalledTimes(6)
    await vi.advanceTimersByTimeAsync(60_000); expect(refresh).toHaveBeenCalledTimes(6)
    live.stop()
  })
  it('stops fallback immediately on recovery; bounds native errors and restarts on focus/online', async () => {
    const { live, streams, refresh, connect } = setup(), stream = streams[0]
    stream.emit('error'); await vi.advanceTimersByTimeAsync(10_150)
    expect(refresh).toHaveBeenCalledTimes(1)
    stream.emit('open'); await vi.advanceTimersByTimeAsync(150); refresh.mockClear()
    await vi.advanceTimersByTimeAsync(30_000); expect(refresh).not.toHaveBeenCalled()
    for (let i = 0; i < 5; i++) stream.emit('error')
    expect(stream.close).toHaveBeenCalledTimes(1)
    live.wake(); expect(connect).toHaveBeenCalledTimes(2)
    stream.emit('inventory.changed', '999'); await vi.advanceTimersByTimeAsync(150)
    expect(refresh).toHaveBeenLastCalledWith([], true)
    live.stop()
  })
  it('coalesces session confirmations and discards events, timers and wake after disposal', async () => {
    let resolve!: () => void
    const confirmSession = vi.fn(() => new Promise<void>((ok) => { resolve = ok }))
    const stream = new Stream(), refresh = vi.fn()
    const live = createLiveEvents({ connect: () => stream, refresh, confirmSession, visible: () => true, online: () => true })
    stream.emit('error'); stream.emit('error'); stream.emit('users.changed', '1')
    expect(confirmSession).toHaveBeenCalledTimes(1)
    live.stop(); live.stop(); live.wake(); resolve()
    stream.emit('inventory.changed', '2'); await vi.advanceTimersByTimeAsync(60_000)
    expect(refresh).not.toHaveBeenCalled(); expect(stream.close).toHaveBeenCalledTimes(1)
  })
  it('does not consume invalid payload cursors, so a valid replay remains usable', async () => {
    const { live, streams: [stream], refresh } = setup()
    stream.emit('inventory.changed', '20', '{}'); stream.emit('inventory.changed', '20')
    await vi.advanceTimersByTimeAsync(150)
    expect(refresh.mock.calls[0][0]).toHaveLength(1)
    live.stop()
  })
  it('restarts permanently closed native streams on focus and verifies session settings on resync', async () => {
    const { live, streams, connect, confirmSession } = setup()
    streams[0].readyState = 2; streams[0].emit('error')
    expect(streams[0].close).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(150)
    live.wake(); expect(connect).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(150)
    confirmSession.mockClear()
    streams[1].emit('resync.required', '5')
    expect(confirmSession).toHaveBeenCalledTimes(1)
    live.stop()
  })
})
