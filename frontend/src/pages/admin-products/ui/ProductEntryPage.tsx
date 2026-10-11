import { Link, useParams, useSearchParams } from 'react-router'
import { safeProductListUrl } from '@/entities/product'
import { EmptyState } from '@/shared/ui'
export function ProductEntryPage() {
  const { id } = useParams(), [params] = useSearchParams()
  const returnTo = safeProductListUrl(params.get('returnTo'))
  return <div><h1>{id ? 'Редактирование продукта' : 'Создание продукта'}</h1><EmptyState title="Форма продукта — следующий этап" description="Каталог и категории доступны. Создание, изменение и отключение продукта будут реализованы в F17." /><Link className="ui-button ui-button--secondary" to={returnTo}>В каталог продуктов</Link></div>
}
