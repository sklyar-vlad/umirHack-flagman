import { Link, useParams, useSearchParams } from 'react-router'
import { safeProductListUrl } from '@/entities/product'
import { ProductCategoryDraft } from '@/widgets/product-category-draft'
import { EmptyState } from '@/shared/ui'
import styles from './AdminProducts.module.css'
export function ProductEntryPage() {
  const { id } = useParams(), [params] = useSearchParams()
  const returnTo = safeProductListUrl(params.get('returnTo'))
  return <div><h1>{id ? 'Редактирование продукта' : 'Создание продукта'}</h1><EmptyState title="Форма продукта — следующий этап" description="Каталог и категории доступны. Создание, изменение и отключение продукта будут реализованы в F17." />{!id && <div className={styles.draft}><ProductCategoryDraft /></div>}<Link className="ui-button ui-button--secondary" to={returnTo}>В каталог продуктов</Link></div>
}
