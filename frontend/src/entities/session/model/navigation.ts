import type { IconName } from '@/shared/assets/icons'
import type { SessionUser } from './session'

export type Role = SessionUser['role']
const all = ['admin', 'manager', 'storekeeper', 'order_operator'] as const
const stock = ['admin', 'manager', 'storekeeper'] as const
const sales = ['admin', 'manager', 'order_operator'] as const
const management = ['admin', 'manager'] as const
const admin = ['admin'] as const
export interface Section { id: string; path: string; title: string; icon: IconName; roles: readonly Role[]; group: 'main' | 'extra' | 'recommendations' | 'admin' }
/** README role matrix. Navigation and route guards consume the same records. */
export const sections: readonly Section[] = [
  { id: 'overview', path: '/', title: 'Обзор', icon: 'grid', roles: all, group: 'main' },
  { id: 'inventory', path: '/inventory', title: 'Склад', icon: 'box', roles: all, group: 'main' },
  { id: 'supplies', path: '/supplies', title: 'Поставки', icon: 'truck', roles: stock, group: 'main' },
  { id: 'orders', path: '/orders', title: 'Заказы', icon: 'receipt', roles: sales, group: 'main' },
  { id: 'stop-list', path: '/stop-list', title: 'Стоп-лист', icon: 'stop', roles: sales, group: 'main' },
  { id: 'recommendations', path: '/recommendations', title: 'Рекомендации', icon: 'spark', roles: all, group: 'main' },
  { id: 'offers', path: '/recommendations/offers', title: 'Продвигать блюда', icon: 'spark', roles: sales, group: 'recommendations' },
  { id: 'procurement', path: '/recommendations/procurement', title: 'Закупить продукты', icon: 'truck', roles: stock, group: 'recommendations' },
  { id: 'write-offs', path: '/write-offs', title: 'Списания', icon: 'box', roles: stock, group: 'extra' },
  { id: 'movements', path: '/movements', title: 'История движений', icon: 'adminHistory', roles: stock, group: 'extra' },
  { id: 'analytics', path: '/analytics', title: 'Аналитика', icon: 'grid', roles: management, group: 'extra' },
  { id: 'forecast', path: '/forecast', title: 'Прогноз', icon: 'grid', roles: management, group: 'extra' },
  { id: 'dishes', path: '/dishes', title: 'Рецепты', icon: 'adminBook', roles: sales, group: 'extra' },
  { id: 'products', path: '/admin/products', title: 'Продукты', icon: 'box', roles: admin, group: 'admin' },
  { id: 'categories', path: '/admin/categories', title: 'Категории', icon: 'adminBook', roles: admin, group: 'admin' },
  { id: 'recipes', path: '/admin/dishes', title: 'Рецептуры', icon: 'adminBook', roles: admin, group: 'admin' },
  { id: 'users', path: '/admin/users', title: 'Пользователи', icon: 'adminUsers', roles: admin, group: 'admin' },
  { id: 'settings', path: '/admin/settings', title: 'Настройки', icon: 'adminSettings', roles: admin, group: 'admin' },
  { id: 'audit', path: '/admin/audit-log', title: 'Журнал действий', icon: 'adminHistory', roles: admin, group: 'admin' },
]
export const roleNames: Record<Role, string> = { admin: 'Администратор', manager: 'Менеджер', storekeeper: 'Кладовщик', order_operator: 'Оператор заказов' }
export function canAccess(role: Role, section: Section) { return section.roles.includes(role) }
export function visibleSections(role: Role, group: Section['group']) { return sections.filter((section) => section.group === group && canAccess(role, section)) }
export function sectionForPath(path: string) {
  if (path === '/admin/audit') return sections.find((section) => section.id === 'audit')
  if (path === '/admin/recipes') return sections.find((section) => section.id === 'recipes')
  if (path === '/dashboard') return sections.find((section) => section.id === 'overview')
  // More specific records must win over their parent section.
  return [...sections].sort((a, b) => b.path.length - a.path.length).find((section) => path === section.path || (section.path !== '/' && path.startsWith(`${section.path}/`)))
}
export function startPath(role: Role) { return visibleSections(role, 'main')[0]?.path ?? '/forbidden' }
/** The caller must first strip external URLs/query secrets using safeReturnUrl. */
export function permittedReturnPath(role: Role, path: string) {
  const section = sectionForPath(path.split(/[?#]/)[0])
  if (path === '/admin') return visibleSections(role, 'admin')[0]?.path ?? startPath(role)
  return section && canAccess(role, section) ? path : startPath(role)
}
