import type { ThunkAction, UnknownAction } from '@reduxjs/toolkit'
import { baseQuery } from '@/shared/api/base-query'
import { type ApiTransport, type ApiError } from '@/shared/api'
import { userSchema, type User } from '../model/contracts'
import { createUserSchema, updateUserSchema, resetUserPasswordSchema, type CreateUser, type UpdateUser, type ResetUserPassword } from '../model/user-work'
import { userApi } from './user-api'
export type UserWrite = { kind: 'create'; body: CreateUser } | { kind: 'update'; id: string; body: UpdateUser } | { kind: 'password'; id: string; body: ResetUserPassword }
export function writeUser(input: UserWrite, signal: AbortSignal): ThunkAction<Promise<User>, unknown, ApiTransport, UnknownAction> {
  return async (dispatch, getState, extra) => {
    const schema = input.kind === 'create' ? createUserSchema : input.kind === 'update' ? updateUserSchema : resetUserPasswordSchema
    const body = schema.safeParse(input.body)
    if (!body.success) throw { status: 422, code: 'VALIDATION_ERROR', message: 'Проверьте поля формы.', details: {} } satisfies ApiError
    const result = await baseQuery({ url: input.kind === 'create' ? '/users' : `/users/${input.id}${input.kind === 'password' ? '/password' : ''}`, method: input.kind === 'update' ? 'PATCH' : 'POST', body: body.data },
      { signal, abort: () => {}, dispatch, getState, extra, endpoint: 'privateUserWrite', type: 'mutation' }, {})
    if (result.error) {
      const error = result.error
      const allowed = ['name', 'email', 'role', 'is_active', 'password']
      throw { status: error.status, code: ['CONFLICT', 'REVISION_CONFLICT', 'LAST_ADMIN_PROTECTED', 'LAST_ADMIN', 'VALIDATION_ERROR', 'FORBIDDEN', 'CSRF_INVALID', 'UNAUTHORIZED', 'NOT_FOUND'].includes(error.code) ? error.code : 'USER_WRITE_FAILED', message: 'Не удалось сохранить пользователя.', details: {
        fields: (Array.isArray(error.details.fields) ? error.details.fields : []).flatMap((field) => {
          if (!field || typeof field.path !== 'string') return []
          const path = field.path.replace(/^\/?(?:body[./])?/, '')
          return allowed.includes(path) ? [{ path, code: 'INVALID', message: 'Проверьте значение поля.' }] : []
        }),
      } } satisfies ApiError
    }
    const parsed = userSchema.safeParse(result.data)
    if (!parsed.success || input.kind !== 'create' && parsed.data.id.toLowerCase() !== input.id.toLowerCase()) throw { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сохранения не подтверждён.', details: {} } satisfies ApiError
    const fields = Object.entries(body.data).filter(([key]) => !['password', 'expected_revision'].includes(key))
    if (fields.some(([key, value]) => parsed.data[key as keyof User] !== value) || 'expected_revision' in body.data && parsed.data.revision !== body.data.expected_revision + 1) throw { status: 'INVALID_RESPONSE', code: 'INVALID_RESPONSE', message: 'Ответ сохранения не соответствует отправленным изменениям.', details: {} } satisfies ApiError
    dispatch(userApi.util.invalidateTags(['Users']))
    return parsed.data
  }
}
