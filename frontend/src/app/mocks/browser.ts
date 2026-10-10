import { setupWorker } from 'msw/browser'
import { env } from '@/shared/config'
import { handlers } from './handlers'
import { setMockScenario } from './scenarios'

const worker = setupWorker(...handlers)

export async function startMocking() {
  setMockScenario(env.mockScenario)
  await worker.start({
    serviceWorker: { url: '/mockServiceWorker.js' },
    // Только API перехватывается handlers; ресурсы Vite проходят в сеть.
    onUnhandledFrame: 'bypass',
  })
}

if (import.meta.hot) {
  import.meta.hot.dispose(() => { void worker.stop() })
}
