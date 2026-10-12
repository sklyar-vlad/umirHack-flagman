import { Link, useParams, useSearchParams } from 'react-router'
import { safeDishListUrl } from '@/entities/dish'
import { EmptyState } from '@/shared/ui'
export function DishEntryPage() {
  const { id } = useParams(), [params] = useSearchParams()
  return <div><h1>{id ? 'Редактирование блюда' : 'Создание блюда'}</h1><EmptyState title="Редактор рецептуры — следующий этап" description="Сохранение блюда и новой версии рецептуры пока недоступно. Вернитесь в каталог, чтобы просмотреть состав." /><Link to={safeDishListUrl(params.get('returnTo'))}>В каталог блюд</Link></div>
}
