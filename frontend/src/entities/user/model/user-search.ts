import { userRoleSchema } from './contracts'
export interface UserSearch { search: string; role: 'all' | 'admin' | 'manager' | 'storekeeper' | 'order_operator'; is_active: 'all' | 'true' | 'false'; sort: 'name'; page: number; page_size: number }
export const defaultUserSearch: UserSearch = { search: '', role: 'all', is_active: 'all', sort: 'name', page: 1, page_size: 50 }
const positive = (raw: string | null, fallback: number, max = Number.MAX_SAFE_INTEGER) => raw && /^\d+$/.test(raw) && Number.isSafeInteger(Number(raw)) && Number(raw) >= 1 && Number(raw) <= max ? Number(raw) : fallback
export function readUserSearch(params: URLSearchParams): UserSearch {
  const role = userRoleSchema.safeParse(params.get('role')), active = params.get('is_active')
  return { search: params.get('search') ?? '', role: role.success ? role.data : 'all', is_active: active === 'true' || active === 'false' ? active : 'all', sort: 'name', page: positive(params.get('page'), 1), page_size: positive(params.get('page_size'), 50, 100) }
}
export function userSearchParams(values: UserSearch) {
  const params = new URLSearchParams()
  for (const key of Object.keys(defaultUserSearch) as (keyof UserSearch)[]) if (values[key] !== defaultUserSearch[key]) params.set(key, String(values[key]))
  return params
}
export function changeUserSearch(values: UserSearch, patch: Partial<UserSearch>): UserSearch { return { ...values, ...patch, page: patch.page ?? 1 } }
export function safeUserListUrl(value: string | null | undefined) {
  if (!value || !value.startsWith('/') || value.startsWith('//') || /[\\\s%]/u.test(value.split(/[?#]/)[0])) return '/admin/users'
  const url = new URL(value, 'https://internal.invalid')
  if (url.origin !== 'https://internal.invalid' || url.pathname !== '/admin/users') return '/admin/users'
  const params = userSearchParams(readUserSearch(url.searchParams))
  return url.pathname + (params.size ? `?${params}` : '')
}
