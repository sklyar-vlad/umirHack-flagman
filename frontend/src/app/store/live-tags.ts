import { canAccess, sectionForPath, type Role } from '@/entities/session'
import type { LiveEvent } from '@/shared/api/event-contracts'
import { baseApi } from '@/shared/api'

export const liveApi = baseApi.enhanceEndpoints({ addTagTypes: ['Inventory', 'Batches', 'Dashboard', 'Dishes', 'StopList', 'HotOffers', 'Procurement', 'Recommendations', 'Products', 'ProductCategories', 'Supplies', 'Orders', 'OrderAttempts', 'StockMovements', 'IncomingSupplies', 'ProductFlow', 'WriteOffReport', 'IngredientShortages', 'StopListForecast', 'Users', 'Roles', 'Settings', 'AuditLog'] })
const paths = {
  Inventory: '/inventory', Batches: '/inventory', Dashboard: '/', Dishes: '/dishes', StopList: '/stop-list', HotOffers: '/recommendations/offers', Procurement: '/recommendations/procurement', Recommendations: '/recommendations',
  Products: '/inventory', ProductCategories: '/inventory', Supplies: '/supplies', Orders: '/orders', OrderAttempts: '/orders', StockMovements: '/stock-movements', IncomingSupplies: '/supplies', ProductFlow: '/analytics', WriteOffReport: '/analytics', IngredientShortages: '/analytics', StopListForecast: '/analytics/stop-list-forecast', Users: '/admin/users', Roles: '/admin/users', Settings: '/admin/settings', AuditLog: '/admin/audit-log',
} as const
type Tag = keyof typeof paths
const topics: Record<string, readonly Tag[]> = {
  inventory: ['IncomingSupplies', 'ProductFlow', 'Inventory', 'Supplies', 'WriteOffReport', 'StopListForecast'], batches: ['Batches'], dashboard: ['Dashboard'], dishes: ['Dishes', 'StopListForecast'], 'stop-list': ['StopList'], 'hot-offers': ['HotOffers'], 'procurement-recommendations': ['Procurement', 'Recommendations'],
  products: ['Products', 'StopListForecast'], 'product-categories': ['ProductCategories'], settings: ['Settings', 'StopListForecast'], users: ['Users', 'Roles'], 'stock-movements': ['ProductFlow', 'StockMovements', 'WriteOffReport'], supplies: ['IncomingSupplies', 'Supplies'], orders: ['Orders', 'OrderAttempts', 'IngredientShortages'],
}
export function liveTags(role: Role, events: readonly LiveEvent[], full: boolean): Tag[] {
  const tags = new Set<Tag>(full ? Object.keys(paths) as Tag[] : events.flatMap((event) => event.topics.flatMap((topic) => topics[topic] ?? [])))
  if (events.some((event) => event.type !== 'inventory.time_changed')) tags.add('AuditLog')
  return [...tags].filter((tag) => { const section = sectionForPath(paths[tag]); return !!section && canAccess(role, section) })
}
