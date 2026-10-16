import type { User, RoleList, UserSearch } from '@/entities/user'
export const mockRoles: RoleList = { items: [
  { code: 'admin', name: 'Администратор' }, { code: 'manager', name: 'Менеджер' },
  { code: 'storekeeper', name: 'Кладовщик' }, { code: 'order_operator', name: 'Оператор заказов' },
] }
export const mockUsers: User[] = [
  { id: '00000000-0000-4000-8000-000000000901', name: 'Анна Администратор', email: 'anna@example.test', role: 'admin', is_active: true, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: '2026-10-12T06:15:00Z' },
  { id: '00000000-0000-4000-8000-000000000902', name: 'Борис Кладовщик', email: 'boris@example.test', role: 'storekeeper', is_active: false, revision: 2, created_at: '2026-01-01T00:00:00Z', last_login_at: null },
  { id: '00000000-0000-4000-8000-000000000903', name: 'Вера Оператор', email: 'vera@example.test', role: 'order_operator', is_active: true, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: '2026-10-11T09:30:00+03:00' },
  ...Array.from({ length: 22 }, (_, index): User => ({ id: `00000000-0000-4000-8000-${String(910 + index).padStart(12, '0')}`, name: `Менеджер ${String(index + 1).padStart(2, '0')}${index === 0 ? ' %_\\' : ''}`, email: `manager${index + 1}@example.test`, role: 'manager', is_active: index % 3 !== 0, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: null })),
]
export function mockUserList(values: UserSearch, empty = false) {
  const search = values.search.toLocaleLowerCase('ru')
  const filtered = (empty ? [] : [...users.values()]).filter((user) => (user.name.toLocaleLowerCase('ru').includes(search) || user.email.toLocaleLowerCase('ru').includes(search)) && (values.role === 'all' || user.role === values.role) && (values.is_active === 'all' || String(user.is_active) === values.is_active)).sort((a, b) => a.name.localeCompare(b.name, 'ru') || a.id.localeCompare(b.id))
  return { items: filtered.slice((values.page - 1) * values.page_size, values.page * values.page_size), total: filtered.length, page: values.page, page_size: values.page_size, meta: {} }
}

let users = new Map(mockUsers.map((user) => [user.id, { ...user }]))
let ownUser: User | undefined
export const mockActorId = '11111111-1111-4111-8111-111111111111'
export function resetMockUsers() { users = new Map(mockUsers.map((user) => [user.id, { ...user }])); ownUser = undefined }
export function getMockSessionUser() { return ownUser }
export function getMockUser(id: string): User | undefined {
  if (id.toLowerCase() === mockActorId) return ownUser ?? { id: mockActorId, name: 'Тестовый сотрудник', email: 'employee@example.test', role: 'admin', is_active: true, revision: 1, created_at: '2026-01-01T00:00:00Z', last_login_at: null }
  return users.get(id.toLowerCase())
}
export function storeMockUser(user: User) { if (user.id === mockActorId) ownUser = user; else users.set(user.id, user) }
export function mockEmailUsed(email: string, except?: string) { return [...users.values(), ...(ownUser ? [ownUser] : [])].some((user) => user.email === email && user.id !== except) }
export function mockLastAdmin(user: User, role: User['role'], active: boolean) {
  return user.role === 'admin' && user.is_active && (role !== 'admin' || !active) && ![...users.values(), ...(ownUser ? [ownUser] : [])].some((other) => other.id !== user.id && other.role === 'admin' && other.is_active)
}
