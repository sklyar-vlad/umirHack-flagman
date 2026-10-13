import { http } from 'msw/http'
import { getMockScenario } from './scenarios'

/** Only registered by the explicit development/test MSW worker. No production fallback. */
export function createMockEventResponse(request: Request) {
  let timer: ReturnType<typeof setInterval> | undefined
  const encoder = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      let cursor = BigInt(request.headers.get('Last-Event-ID') ?? '100'), previous = '', previousScenario = '', ticks = 0
      controller.enqueue(encoder.encode(': mock heartbeat\n\n'))
      const stop = () => { clearInterval(timer); try { controller.close() } catch { /* Already cancelled. */ } }
      request.signal.addEventListener('abort', stop, { once: true })
      timer = setInterval(() => {
        const scenario = getMockScenario()
        if (scenario !== previousScenario) { ticks = 0; previousScenario = scenario }
        ticks++
        try {
          if (scenario === 'events-disconnect') { stop(); return }
          if (scenario === 'events-reconnect' && ticks === 2) { stop(); return }
          if (scenario === 'events-replay' && ticks === 2) { controller.enqueue(encoder.encode(previous)); return }
          if (ticks % 30 === 0 || (scenario === 'events-replay' || scenario === 'events-resync' || scenario === 'events-reconnect') && ticks === 1) {
            cursor++
            previous = `id: ${cursor}\nevent: ${scenario === 'events-resync' && ticks === 1 ? 'resync.required' : 'inventory.time_changed'}\ndata: ${JSON.stringify({ occurred_at: new Date().toISOString(), topics: ['inventory', 'batches', 'dashboard', 'dishes', 'stop-list', 'hot-offers', 'procurement-recommendations'] })}\n\n`
            controller.enqueue(encoder.encode(previous))
          } else controller.enqueue(encoder.encode(': mock heartbeat\n\n'))
        } catch { clearInterval(timer) }
      }, 1_000)
    },
    cancel() { clearInterval(timer) },
  })
  return new Response(stream, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache', 'X-Mock-Mode': 'true' } })
}

export const eventHandlers = [http.get('/api/v1/events', ({ request }) => createMockEventResponse(request))]
