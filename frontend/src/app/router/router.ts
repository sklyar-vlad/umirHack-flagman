import { createBrowserRouter, type RouteObject } from 'react-router'
import { SessionGate } from '@/features/auth'
import { AppHome } from './AppHome'
import { RouteError } from './RouteError'
import { RouteLoading } from './RouteLoading'

const routes: RouteObject[] = [
  { path: '/login', lazy: async () => { const { LoginPage } = await import('@/pages/login'); return { Component: LoginPage } }, ErrorBoundary: RouteError, HydrateFallback: RouteLoading },
  { Component: SessionGate, ErrorBoundary: RouteError, children: [{ path: '/', Component: AppHome }] },
]

if (import.meta.env.DEV) {
  routes.push({
    path: '/dev/ui',
    lazy: async () => {
      const { UiKitPage } = await import('@/pages/ui-kit')
      return { Component: UiKitPage }
    },
    ErrorBoundary: RouteError,
    HydrateFallback: RouteLoading,
  })
  routes.push({
    path: '/dev/api',
    lazy: async () => {
      const { ApiStatusPage } = await import('@/pages/api-status')
      return { Component: ApiStatusPage }
    },
    ErrorBoundary: RouteError,
    HydrateFallback: RouteLoading,
  })
}

export const router = createBrowserRouter(routes)
