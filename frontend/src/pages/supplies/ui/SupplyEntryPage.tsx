import { Link, useSearchParams } from 'react-router'
import { safeSupplyListUrl } from '@/entities/supply'
import styles from './SuppliesPage.module.css'

/** Reception entry point until F08 implements its business content. */
export function SupplyEntryPage() {
  const [params] = useSearchParams()
  return <section className={styles.page}><h1>Поставки</h1><h2>Новая поставка</h2>
    <p className="text-muted">Форма приёмки будет доступна после реализации соответствующей истории.</p>
    <Link className="ui-button ui-button--secondary" to={safeSupplyListUrl(params.get('returnTo'))}>К истории поставок</Link>
  </section>
}
