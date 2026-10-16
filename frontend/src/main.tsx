import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from '@/app'
import { MockStartupError } from '@/app/mocks/MockStartupError'
import { env } from '@/shared/config'
import '@/app/styles/index.css'

const root = createRoot(document.getElementById('root')!)

function renderApp() {
  root.render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}

if (import.meta.env.DEV && env.mockModeEnabled) {
  import('@/app/mocks/browser')
    .then(({ startMocking }) => startMocking())
    .then(renderApp)
    .catch((error: unknown) => {
      console.error('Не удалось запустить MSW:', error)
      root.render(<MockStartupError />)
    })
} else {
  renderApp()
}
