import { z } from 'zod'
import { userNameSchema, userEmailSchema, userPasswordSchema, userRoleSchema, type User, type UpdateUser } from '@/entities/user'
export const userFormSchema = z.object({ name: userNameSchema, email: userEmailSchema, role: userRoleSchema, is_active: z.boolean() })
export const newUserFormSchema = userFormSchema.extend({ initial_password: userPasswordSchema })
export type UserValues = z.infer<typeof userFormSchema>
export type NewUserValues = z.infer<typeof newUserFormSchema>
export const emptyUser: UserValues = { name: '', email: '', role: 'manager', is_active: true }
export function userValues(user: User): UserValues { return { name: user.name, email: user.email, role: user.role, is_active: user.is_active } }
export function userPatch(values: UserValues, user: User): UpdateUser {
  const patch: UpdateUser = { expected_revision: user.revision }
  if (values.name !== user.name) patch.name = values.name
  if (values.email !== user.email) patch.email = values.email
  if (values.role !== user.role) patch.role = values.role
  if (values.is_active !== user.is_active) patch.is_active = values.is_active
  return patch
}
export function revokesAccess(values: UserValues, user: User) { return values.role !== user.role || !values.is_active && user.is_active }
