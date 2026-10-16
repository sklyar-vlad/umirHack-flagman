import { useForm } from 'react-hook-form'
import type { RoleList, UserSearch } from '@/entities/user'
import { roleNames } from '@/entities/session'
import { Button, FormField, Input, Select } from '@/shared/ui'
import styles from './UsersPage.module.css'

export function UserFilters({ values, roles, rolesUnavailable, onChange, onReset }: { values: UserSearch; roles: RoleList | undefined; rolesUnavailable: boolean; onChange: (patch: Partial<UserSearch>) => void; onReset: () => void }) {
  const { register, handleSubmit, reset } = useForm({ defaultValues: { search: values.search } })
  return <div className={styles.filters}>
    <form className={styles.actions} role="search" aria-label="Поиск пользователей" onSubmit={handleSubmit(({ search }) => onChange({ search: search.trim() }))}>
      <FormField label="Поиск по имени или email">{(props) => <Input {...props} {...register('search')} type="search" />}</FormField>
      <Button type="submit" variant="secondary">Найти</Button><Button variant="ghost" onClick={() => { reset({ search: '' }); onReset() }}>Сбросить фильтры</Button>
    </form>
    <FormField label="Роль" hint={rolesUnavailable ? 'Справочник недоступен. Повторите загрузку ролей.' : undefined}>{(props) => <Select {...props} disabled={rolesUnavailable || !roles} value={values.role} onChange={(event) => onChange({ role: event.target.value as UserSearch['role'] })}>
      <option value="all">Все роли</option>
      {roles ? roles.items.map((role) => <option key={role.code} value={role.code}>{role.name}</option>) : values.role !== 'all' && <option value={values.role}>{roleNames[values.role]}</option>}
    </Select>}</FormField>
    <FormField label="Активность">{(props) => <Select {...props} value={values.is_active} onChange={(event) => onChange({ is_active: event.target.value as UserSearch['is_active'] })}><option value="all">Все пользователи</option><option value="true">Активные</option><option value="false">Отключённые</option></Select>}</FormField>
    <FormField label="Пользователей на странице">{(props) => <Select {...props} value={values.page_size} onChange={(event) => onChange({ page_size: Number(event.target.value) })}>{[...new Set([20, 50, 100, values.page_size])].sort((a, b) => a - b).map((size) => <option key={size} value={size}>{size}</option>)}</Select>}</FormField>
  </div>
}
