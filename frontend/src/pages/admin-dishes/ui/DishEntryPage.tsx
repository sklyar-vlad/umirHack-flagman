import { Link, useParams, useSearchParams } from 'react-router'
import { safeDishListUrl } from '@/entities/dish'
import { DishEditor } from '@/widgets/dish-editor'
export function DishEntryPage() {
  const { id } = useParams(), [params] = useSearchParams(), returnTo = safeDishListUrl(params.get('returnTo'))
  return <div><h1>{id ? 'Редактирование блюда' : 'Создание блюда'}</h1><p><Link to={returnTo}>В каталог блюд</Link></p><DishEditor key={id ?? 'new'} id={id} returnTo={returnTo} /></div>
}
