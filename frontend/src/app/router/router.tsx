import { MovementsPage } from '@/pages/stock-movements'
import { AuditPage } from '@/pages/audit-log'
import { AdminNavigation } from '@/widgets/admin-navigation'
import { SettingsPage } from '@/pages/admin-settings'
import { UsersPage, UserEntryPage } from '@/pages/admin-users'
import { DishesPage, DishEntryPage } from '@/pages/admin-dishes'
import { createBrowserRouter, type RouteObject } from 'react-router'
import { sections, type Section } from '@/entities/session'
import { SessionGate, SessionLogout } from '@/features/auth'
import { RoleGate } from '@/features/access'
import { AppShell } from '@/widgets/app-shell'
import { SectionLanding, SectionPage } from '@/pages/section'
import { RouteStatusPage } from '@/pages/route-status'
import { InventoryPage } from '@/pages/inventory'
import { SuppliesPage, SupplyEntryPage, SupplyDetailPage } from '@/pages/supplies'
import { OrdersPage, OrderDetailPage, NewOrderPage } from '@/pages/orders'
import { ProcurementPage, RecommendationsLanding } from '@/pages/procurement'
import { HotOffersPage } from '@/pages/hot-offers'
import { ProductsPage, CategoriesPage, ProductEntryPage } from '@/pages/admin-products'
import { DashboardPage } from '@/pages/dashboard'
import { StopListPage } from '@/pages/stop-list'
import { RouteError } from './RouteError'
import { RouteLoading } from './RouteLoading'

function guarded(section: Section, children = <SectionPage section={section} />) {
  return <RoleGate section={section}>{children}</RoleGate>
}
const dishes = sections.find((section) => section.id === 'dishes')!
const administration = sections.find((section) => section.group === 'admin')!
const users = sections.find((section) => section.id === 'users')!
const supplies = sections.find((section) => section.id === 'supplies')!
const orders = sections.find((section) => section.id === 'orders')!
const protectedRoutes: RouteObject[] = [
  { path: '/movements', element: guarded(sections.find((section) => section.id === 'movements')!, <MovementsPage />) },
  { path: '/admin/audit', element: guarded(sections.find((section) => section.id === 'audit')!, <><AdminNavigation /><AuditPage /></>) },
  ...sections.map((section): RouteObject => ({
    path: section.path,
    element: guarded(section, <>{section.group === 'admin' && <AdminNavigation />}{section.id === 'movements' ? <MovementsPage /> : section.id === 'audit' ? <AuditPage /> : section.id === 'settings' ? <SettingsPage /> : section.id === 'users' ? <UsersPage /> : section.id === 'recipes' ? <DishesPage /> : section.id === 'products' ? <ProductsPage /> : section.id === 'categories' ? <CategoriesPage /> : section.id === 'overview' ? <DashboardPage /> : section.id === 'inventory' ? <InventoryPage /> : section.id === 'supplies' ? <SuppliesPage /> : section.id === 'orders' ? <OrdersPage /> : section.id === 'stop-list' ? <StopListPage /> : section.id === 'offers' ? <HotOffersPage /> : section.id === 'procurement' ? <ProcurementPage /> : section.id === 'recommendations'
      ? <RecommendationsLanding />
      : <SectionPage section={section} />}</>),
  })),
  { path: '/admin/users/new', element: guarded(users, <UserEntryPage />) },
  { path: '/admin/users/:id', element: guarded(users, <UserEntryPage />) },
  { path: '/admin/users/:id/edit', element: guarded(users, <UserEntryPage />) },
  { path: '/admin/users/:id/password', element: guarded(users, <UserEntryPage password />) },
  { path: '/dashboard', element: guarded(sections.find((section) => section.id === 'overview')!, <DashboardPage />) },
  { path: '/admin/products/new', element: guarded(sections.find((section) => section.id === 'products')!, <ProductEntryPage />) },
  { path: '/admin/products/:id', element: guarded(sections.find((section) => section.id === 'products')!, <ProductEntryPage />) },
  { path: '/admin/products/:id/edit', element: guarded(sections.find((section) => section.id === 'products')!, <ProductEntryPage />) },
  { path: '/admin/recipes', element: guarded(sections.find((section) => section.id === 'recipes')!, <DishesPage />) },
  { path: '/admin/dishes/new', element: guarded(sections.find((section) => section.id === 'recipes')!, <DishEntryPage />) },
  { path: '/admin/dishes/:id', element: guarded(sections.find((section) => section.id === 'recipes')!, <DishEntryPage />) },
  { path: '/admin/dishes/:id/edit', element: guarded(sections.find((section) => section.id === 'recipes')!, <DishEntryPage />) },
  { path: '/admin', element: guarded(administration, <SectionLanding group="admin" />) },
  { path: '/dishes/:id', element: guarded(dishes) },
  { path: '/supplies/new', element: guarded(supplies, <SupplyEntryPage />) },
  { path: '/supplies/:id', element: guarded(supplies, <SupplyDetailPage />) },
  { path: '/orders/new', element: guarded(orders, <NewOrderPage />) },
  { path: '/orders/:id', element: guarded(orders, <OrderDetailPage />) },
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
