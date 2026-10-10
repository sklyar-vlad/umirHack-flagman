import { createBrowserRouter, type RouteObject } from 'react-router'
import { AppHome } from './AppHome'
import { RouteError } from './RouteError'
import { RouteLoading } from './RouteLoading'

const routes: RouteObject[] = [
  { path: '/', Component: AppHome, ErrorBoundary: RouteError, HydrateFallback: RouteLoading },
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
