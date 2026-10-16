import { decodeEvent, eventTypes, type LiveEvent } from './event-contracts'

export interface EventStream {
  readonly readyState?: number
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void
  close(): void
}
interface LivePorts {
  connect: () => EventStream
  refresh: (events: readonly LiveEvent[], full: boolean) => void
  confirmSession: () => Promise<unknown>
  visible: () => boolean
  online: () => boolean
}

export function createLiveEvents(ports: LivePorts) {
  let source: EventStream | null = null, stopped = false, connected = false
  let cursor = -1n, failures = 0, polls = 0
  let online = ports.online()
  let batch: ReturnType<typeof setTimeout> | undefined, poll: ReturnType<typeof setTimeout> | undefined
  let events: LiveEvent[] = [], full = false, checking = false
  function confirm() {
    if (checking || stopped || !ports.online()) return
    checking = true
    void ports.confirmSession().catch(() => {}).finally(() => { checking = false })
  }
  function queue(event?: LiveEvent) {
    if (stopped) return
    if (event) events.push(event)
    else full = true
    if (batch) return
    batch = setTimeout(() => {
      batch = undefined
      const pending = events, refreshAll = full
      events = []; full = false
      if (!stopped) ports.refresh(pending, refreshAll)
    }, 150)
  }
  function stopPolling() { clearTimeout(poll); poll = undefined }
  function fallback() {
    if (poll || stopped || connected || !ports.online() || !ports.visible() || polls >= 6) return
    poll = setTimeout(() => {
      poll = undefined
      if (stopped || connected || !ports.online() || !ports.visible()) return
      polls++; queue(); confirm(); fallback()
    }, 10_000)
  }
  function connect() {
    if (source || stopped || !ports.online()) return
    cursor = -1n
    const current = ports.connect()
    source = current
    let opened = false
    current.addEventListener('open', () => {
      if (stopped || source !== current) return
      const recovering = opened || failures > 0
      opened = true; connected = true; failures = 0; polls = 0; stopPolling()
      if (recovering) queue()
      confirm()
    })
    current.addEventListener('error', () => {
      if (stopped || source !== current) return
      connected = false; failures++; confirm(); fallback()
      if (failures >= 5 || current.readyState === 2) { current.close(); source = null }
    })
    for (const type of eventTypes) current.addEventListener(type, (message) => {
      if (stopped || source !== current) return
      const event = decodeEvent(type, message.lastEventId, message.data)
      if (!event || event.cursor <= cursor) return
      cursor = event.cursor
      if (type === 'resync.required') { queue(); confirm() }
      else queue(event)
      if (type === 'users.changed' || type === 'settings.changed') confirm()
    })
  }
  function wake() {
    if (stopped) return
    const recovered = !online && ports.online()
    online = ports.online()
    if (!online || !ports.visible()) { stopPolling(); return }
    if (connected && !recovered) return
    polls = 0; failures = 0; queue(); confirm(); connect(); fallback()
  }
  function stop() {
    if (stopped) return
    stopped = true; source?.close(); source = null; connected = false
    clearTimeout(batch); stopPolling(); events = []; full = false
  }
  connect()
  fallback()
  return { wake, stop }
}
