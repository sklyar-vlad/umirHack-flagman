import { createBrowserRouter, type RouteObject } from 'react-router'
import { sections, type Section } from '@/entities/session'
import { SessionGate, SessionLogout } from '@/features/auth'
import { RoleGate } from '@/features/access'
import { AppShell } from '@/widgets/app-shell'
import { SectionLanding, SectionPage } from '@/pages/section'
import { RouteStatusPage } from '@/pages/route-status'
import { InventoryPage } from '@/pages/inventory'
import { RouteError } from './RouteError'
import { RouteLoading } from './RouteLoading'

function guarded(section: Section, children = <SectionPage section={section} />) {
  return <RoleGate section={section}>{children}</RoleGate>
}
const dishes = sections.find((section) => section.id === 'dishes')!
const administration = sections.find((section) => section.group === 'admin')!
const supplies = sections.find((section) => section.id === 'supplies')!
const protectedRoutes: RouteObject[] = [
  ...sections.map((section): RouteObject => ({
    path: section.path,
    element: guarded(section, section.id === 'inventory' ? <InventoryPage /> : section.id === 'recommendations'
      ? <SectionLanding group="recommendations" />
      : <SectionPage section={section} />),
  })),
  { path: '/admin', element: guarded(administration, <SectionLanding group="admin" />) },
  { path: '/dishes/:id', element: guarded(dishes) },
  { path: '/supplies/:id', element: guarded(supplies) },
  { path: '/forbidden', element: <RouteStatusPage forbidden /> },
  { path: '*', Component: RouteStatusPage },
]
const routes: RouteObject[] = [
  { path: '/login', lazy: async () => { const { LoginPage } = await import('@/pages/login'); return { Component: LoginPage } }, ErrorBoundary: RouteError, HydrateFallback: RouteLoading },
  { Component: SessionGate, ErrorBoundary: RouteError, children: [{ path: '/logout', Component: SessionLogout }, { Component: AppShell, children: protectedRoutes }] },
]

if (import.meta.env.DEV) {
  protectedRoutes.push({ path: '/dev/access', lazy: async () => { const { AccessCheckPage } = await import('@/pages/access-check'); return { Component: AccessCheckPage } }, HydrateFallback: RouteLoading })
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
