import { z } from 'zod'
import { userRoleSchema } from './contracts'
const length = (value: string) => [...value].length
export const userNameSchema = z.string().trim().refine((value) => length(value) >= 1 && length(value) <= 200 && !/\p{Cc}/u.test(value), 'Имя: 1–200 символов без управляющих символов')
export const userEmailSchema = z.string().trim().toLowerCase().pipe(z.email('Введите корректный email')).refine((value) => new TextEncoder().encode(value).length <= 254, 'Email: не более 254 байт')
export const userPasswordSchema = z.string().refine((value) => length(value) >= 12 && length(value) <= 128, 'Пароль: от 12 до 128 символов')
const revision = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
export const createUserSchema = z.object({ name: userNameSchema, email: userEmailSchema, role: userRoleSchema, password: userPasswordSchema, is_active: z.boolean().optional() }).strict()
export const updateUserSchema = z.object({ name: userNameSchema.optional(), email: userEmailSchema.optional(), role: userRoleSchema.optional(), is_active: z.boolean().optional(), expected_revision: revision }).strict().refine((value) => Object.keys(value).length > 1, 'Нет изменений')
export const resetUserPasswordSchema = z.object({ password: userPasswordSchema, expected_revision: revision }).strict()
export type CreateUser = z.infer<typeof createUserSchema>
export type UpdateUser = z.infer<typeof updateUserSchema>
export type ResetUserPassword = z.infer<typeof resetUserPasswordSchema>
