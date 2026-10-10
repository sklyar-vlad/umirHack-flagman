import { RouterProvider } from 'react-router/dom'
import { env } from '@/shared/config'
import { MockModeBanner } from './mocks/MockModeBanner'
import { AppProviders } from './providers'
import { router } from './router'

export function App() {
  return (
    <AppProviders>
      {import.meta.env.DEV && env.mockModeEnabled && <MockModeBanner />}
      <RouterProvider router={router} />
    </AppProviders>
  )
}
