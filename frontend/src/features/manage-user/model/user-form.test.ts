import { describe, it, expect } from 'vitest'
import { userPasswordSchema, createUserSchema, updateUserSchema } from '@/entities/user'
import { userFormSchema, newUserFormSchema, userValues, userPatch, revokesAccess } from './user-form'
import { mockUsers } from '@/app/mocks/user-fixtures'
describe('F22 published user input', () => {
  it('normalizes metadata but preserves password exactly and maps initial_password only at submission', () => {
    const parsed = newUserFormSchema.parse({ name: ' Анна ', email: ' ANNA@EXAMPLE.TEST ', role: 'admin', is_active: true, initial_password: '  secret-value  ' })
    expect(parsed.name).toBe('Анна'); expect(parsed.email).toBe('anna@example.test'); expect(parsed.initial_password).toBe('  secret-value  ')
    expect(createUserSchema.safeParse(parsed).success).toBe(false)
    expect(createUserSchema.parse({ name: parsed.name, email: parsed.email, role: parsed.role, password: parsed.initial_password }).is_active).toBeUndefined()
  })
  it.each([11, 129])('rejects password with %i Unicode characters', (size) => expect(userPasswordSchema.safeParse('🙂'.repeat(size)).success).toBe(false))
  it.each([12, 128])('accepts password with %i Unicode characters', (size) => expect(userPasswordSchema.parse('🙂'.repeat(size))).toBe('🙂'.repeat(size)))
  it('checks names, bytes, role enums and required activity without null', () => {
    const initial = userValues(mockUsers[0])
    for (const patch of [{ name: '' }, { name: 'a\nb' }, { name: '🙂'.repeat(201) }, { email: 'bad' }, { email: `${'a'.repeat(255)}@example.test` }, { role: 'owner' }, { is_active: null }]) expect(userFormSchema.safeParse({ ...initial, ...patch }).success).toBe(false)
    expect(userFormSchema.parse({ ...initial, name: '🙂'.repeat(200) }).name).toBe('🙂'.repeat(200))
  })
  it('uses only changed published PATCH fields and exact owned revision', () => {
    const user = mockUsers[0], values = userValues(user)
    expect(userPatch(values, user)).toEqual({ expected_revision: 1 })
    expect(updateUserSchema.safeParse(userPatch(values, user)).success).toBe(false)
    expect(userPatch({ ...values, name: 'Изменён', is_active: false }, user)).toEqual({ name: 'Изменён', is_active: false, expected_revision: 1 })
    expect(revokesAccess({ ...values, name: 'Изменён' }, user)).toBe(false)
    expect(revokesAccess({ ...values, role: 'manager' }, user)).toBe(true)
    expect(revokesAccess({ ...values, is_active: false }, user)).toBe(true)
    expect(updateUserSchema.safeParse({ expected_revision: 1, password: 'a'.repeat(12) }).success).toBe(false)
  })
})
