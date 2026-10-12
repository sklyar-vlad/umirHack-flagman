import { Link, useSearchParams } from 'react-router'
import { useSelector } from 'react-redux'
import { changeUserSearch, defaultUserSearch, userSearchParams, readUserSearch, useUsersQuery, useRolesQuery, type UserSearch } from '@/entities/user'
import { roleNames, selectSession } from '@/entities/session'
import { ApiAccessBoundary } from '@/features/access'
import { isApiError } from '@/shared/api'
import { formatRestaurantDateTime } from '@/shared/lib/restaurant-time'
import { Button, EmptyState, ErrorState, NoResults, Pagination, Skeleton } from '@/shared/ui'
import { UserFilters } from './UserFilters'
import styles from './UsersPage.module.css'

export function UsersPage() {
  const [params, setParams] = useSearchParams(), values = readUserSearch(params)
  const timezone = useSelector(selectSession).settings?.timezone
  const options = { refetchOnMountOrArgChange: true, refetchOnFocus: true, refetchOnReconnect: true }
  const query = useUsersQuery(values, options), roles = useRolesQuery(undefined, options), data = query.currentData
  const change = (patch: Partial<UserSearch>) => setParams(userSearchParams(changeUserSearch(values, patch)))
  const normalized = userSearchParams(values), returnTo = `/admin/users${normalized.size ? `?${normalized}` : ''}`
  const reset = () => setParams(userSearchParams(defaultUserSearch))
  // Either resource may revoke access; never keep displaying cached personal records on a 403.
  const accessError = isApiError(roles.error) && roles.error.status === 403 ? roles.error : query.error
  return <div className={styles.page}>
    <h1>Пользователи и роли</h1>
    <ApiAccessBoundary error={accessError}>
      <div className={styles.actions}><Link className="ui-button ui-button--primary" to={`/admin/users/new?${new URLSearchParams({ returnTo })}`}>Создать пользователя</Link><Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch() }}>Обновить пользователей</Button></div>
      <p className="text-muted">Поиск по имени и email. Сортировка: по имени. Последний вход указан в часовом поясе ресторана: {timezone ?? 'Europe/Moscow'}. Email-приглашения не отправляются.</p>
      {roles.isFetching && <Skeleton label="Загрузка справочника ролей" rows={1} />}
      {roles.error && <ErrorState title="Не удалось загрузить роли" description="Названия показаны по фиксированному справочнику приложения. Фильтр ролей доступен после успешной загрузки." onRetry={() => { void roles.refetch() }} retrying={roles.isFetching} />}
      <UserFilters key={values.search} values={values} roles={roles.currentData} rolesUnavailable={!!roles.error} onChange={change} onReset={reset} />
      {query.isFetching && <Skeleton label="Загрузка пользователей" rows={4} />}
      {query.error && <ErrorState title="Не удалось загрузить пользователей" description={data ? 'Показан последний успешный список для этих фильтров. Данные могут быть устаревшими.' : 'Попробуйте ещё раз.'} onRetry={() => { void query.refetch() }} retrying={query.isFetching} />}
      {data && <><p role="status">Пользователей: {data.total}. Страница {data.page}.</p>
        {data.items.length ? <div className={styles.list}>{data.items.map((user) => <article className={styles.card} key={user.id} aria-label={`Пользователь: ${user.name}`}>
          <div className={styles.row}><h2>{user.name}</h2><span className="text-label">{user.is_active ? 'Активен' : 'Отключён'}</span></div>
          <p>{user.email}</p>
          <dl className={styles.settings}>
            <div><dt>Роль</dt><dd>{(!roles.error && roles.currentData?.items.find((role) => role.code === user.role)?.name) || roleNames[user.role]}</dd></div>
            <div><dt>Последний вход</dt><dd>{user.last_login_at === null ? 'Ещё не входил' : <time dateTime={user.last_login_at}>{formatRestaurantDateTime(user.last_login_at, timezone)}</time>}</dd></div>
          </dl>
          <div className={styles.actions}>
            <Link to={`/admin/users/${user.id}/edit?${new URLSearchParams({ returnTo })}`} aria-label={`Редактировать: ${user.name}`}>Редактировать</Link>
            <Link to={`/admin/users/${user.id}/password?${new URLSearchParams({ returnTo })}`} aria-label={`Сбросить пароль: ${user.name}`}>Сбросить пароль</Link>
          </div>
        </article>)}</div> : values.page > 1 ? <EmptyState title="На этой странице нет пользователей" description="Вернитесь к первой странице." /> : values.search || values.role !== 'all' || values.is_active !== 'all' ? <NoResults query={values.search || 'Выбранные фильтры'} onReset={reset} /> : <EmptyState title="Список пользователей пуст" description="Создайте первого пользователя." />}
        {data.total > data.page_size && <Pagination page={values.page} pageCount={Math.max(values.page, Math.ceil(data.total / data.page_size))} loading={query.isFetching} onPageChange={(page) => change({ page })} />}
        {!data.items.length && values.page > 1 && <Button variant="secondary" onClick={() => change({ page: 1 })}>К первой странице</Button>}
      </>}
    </ApiAccessBoundary>
  </div>
}
