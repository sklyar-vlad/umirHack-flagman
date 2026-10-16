import { describe, expect, it } from 'vitest'
import { canAccess, permittedReturnPath, sections, startPath, visibleSections, type Role } from './navigation'

const cases: { role: Role; main: string[]; recommendations: string[]; extra: string[] }[] = [
  { role: 'admin', main: ['overview', 'inventory', 'supplies', 'orders', 'stop-list', 'recommendations'], recommendations: ['offers', 'procurement'], extra: ['write-offs', 'movements', 'analytics', 'forecast', 'dishes'] },
  { role: 'manager', main: ['overview', 'inventory', 'supplies', 'orders', 'stop-list', 'recommendations'], recommendations: ['offers', 'procurement'], extra: ['write-offs', 'movements', 'analytics', 'forecast', 'dishes'] },
  { role: 'storekeeper', main: ['overview', 'inventory', 'supplies', 'recommendations'], recommendations: ['procurement'], extra: ['write-offs', 'movements'] },
  { role: 'order_operator', main: ['overview', 'inventory', 'orders', 'stop-list', 'recommendations'], recommendations: ['offers'], extra: ['dishes'] },
]
describe('README role permissions', () => {
  for (const { role, main, recommendations, extra } of cases) {
    it(`${role}: navigation and route access match all capabilities`, () => {
      expect(visibleSections(role, 'main').map((item) => item.id)).toEqual(main)
      expect(visibleSections(role, 'recommendations').map((item) => item.id)).toEqual(recommendations)
      expect(visibleSections(role, 'extra').map((item) => item.id)).toEqual(extra)
      const administration = ['products', 'categories', 'recipes', 'users', 'settings', 'integrations', 'audit']
      expect(visibleSections(role, 'admin').map((item) => item.id)).toEqual(role === 'admin' ? administration : [])
      const allowed = [...main, ...recommendations, ...extra, ...(role === 'admin' ? administration : [])]
      for (const section of sections) expect(canAccess(role, section)).toBe(allowed.includes(section.id))
      expect(startPath(role)).toBe('/')
    })
  }
  it('uses the most specific permission for return URLs and chooses an available fallback', () => {
    expect(permittedReturnPath('storekeeper', '/recommendations/offers')).toBe('/')
    expect(permittedReturnPath('storekeeper', '/recommendations/procurement')).toBe('/recommendations/procurement')
    expect(permittedReturnPath('order_operator', '/supplies')).toBe('/')
    expect(permittedReturnPath('order_operator', '/dishes/123')).toBe('/dishes/123')
    expect(permittedReturnPath('storekeeper', '/dishes/123')).toBe('/')
    expect(permittedReturnPath('manager', '/admin/users')).toBe('/')
    expect(permittedReturnPath('admin', '/admin')).toBe('/admin/products')
    expect(permittedReturnPath('manager', '/unknown')).toBe('/')
    expect(permittedReturnPath('manager', '/forbidden')).toBe('/')
    expect(permittedReturnPath('order_operator', '/inventory?filter=expired&page=2')).toBe('/inventory?filter=expired&page=2')
  })
})
