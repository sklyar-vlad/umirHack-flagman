import { Link, useParams, useSearchParams } from 'react-router'
import { z } from 'zod'
import { safeUserListUrl } from '@/entities/user'
import { EmptyState } from '@/shared/ui'
export function UserEntryPage({ password = false }: { password?: boolean }) {
  const { id } = useParams(), [params] = useSearchParams(), returnTo = safeUserListUrl(params.get('returnTo'))
  const valid = !id || z.uuid().safeParse(id).success
  const title = !valid ? 'Некорректный адрес пользователя' : password ? 'Сброс пароля пользователя' : id ? 'Редактирование пользователя' : 'Создание пользователя'
  return <div><h1>{title}</h1><EmptyState title={valid ? 'Форма будет доступна в следующей истории' : 'Проверьте ссылку'} description={valid ? password ? 'Сброс пароля реализуется в F23. Изменение не отправлено.' : 'Создание и редактирование пользователя реализуется в F22. Email-приглашения не отправляются.' : 'Идентификатор пользователя должен быть UUID.'} /><Link to={returnTo}>В список пользователей</Link></div>
}
