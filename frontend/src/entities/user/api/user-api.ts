import { baseApi, type ApiError } from '@/shared/api'
import { userListSchema, roleListSchema, type UserList, type RoleList, userSchema, type User } from '../model/contracts'
import type { UserSearch } from '../model/user-search'

const invalid: ApiError = { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Некорректный ответ списка пользователей или ролей.', details: {} }
export const userApi = baseApi.enhanceEndpoints({ addTagTypes: ['Users', 'Roles'] }).injectEndpoints({
  endpoints: (build) => ({
    user: build.query<User, string>({
      providesTags: ['Users'],
      queryFn: async (id, _api, _options, query) => {
        const result = await query(`/users/${id}`)
        if (result.error) return { error: result.error }
        const parsed = userSchema.safeParse(result.data)
        return parsed.success && parsed.data.id.toLowerCase() === id.toLowerCase() ? { data: parsed.data } : { error: invalid }
      },
    }),
    users: build.query<UserList, UserSearch>({
      providesTags: ['Users'],
      queryFn: async ({ search, ...params }, _api, _options, query) => {
        const result = await query({ url: '/users', params: { ...params, ...(search && { search }) } })
        if (result.error) return { error: result.error }
        const parsed = userListSchema.safeParse(result.data)
        if (!parsed.success) return { error: invalid }
        const data = parsed.data
        if (data.page !== params.page || data.page_size !== params.page_size || data.items.length > params.page_size || data.items.length > data.total || new Set(data.items.map((item) => item.id.toLowerCase())).size !== data.items.length || data.items.some((item) => params.role !== 'all' && item.role !== params.role || params.is_active !== 'all' && String(item.is_active) !== params.is_active)) return { error: invalid }
        return { data }
      },
    }),
    roles: build.query<RoleList, void>({
      providesTags: ['Roles'],
      queryFn: async (_arg, _api, _options, query) => {
        const result = await query('/roles')
        if (result.error) return { error: result.error }
        const parsed = roleListSchema.safeParse(result.data)
        return parsed.success ? { data: parsed.data } : { error: invalid }
      },
    }),
  }),
})
export const { useUsersQuery, useRolesQuery, useLazyUserQuery } = userApi
