import { Link, useParams, useSearchParams } from 'react-router'
import { safeProductListUrl } from '@/entities/product'
import { ProductEditor } from '@/widgets/product-editor'
import styles from './AdminProducts.module.css'
export function ProductEntryPage() {
  const { id } = useParams(), [params] = useSearchParams()
  const returnTo = safeProductListUrl(params.get('returnTo'))
  return <div className={styles.page}><h1>{id ? 'Редактирование продукта' : 'Создание продукта'}</h1><ProductEditor key={id ?? 'new'} id={id} returnTo={returnTo} /><div><Link className="ui-button ui-button--secondary" to={returnTo}>В каталог продуктов</Link></div></div>
}
