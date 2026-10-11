import { Link, useSearchParams } from 'react-router'
import { safeSupplyListUrl } from '@/entities/supply'
import styles from './SuppliesPage.module.css'

/** Entry points until the detail and reception stories implement their business content. */
export function SupplyEntryPage({ reception = false }: { reception?: boolean }) {
  const [params] = useSearchParams()
  return <section className={styles.page}><h1>Поставки</h1><h2>{reception ? 'Новая поставка' : 'Детали поставки'}</h2>
    <p className="text-muted">{reception ? 'Форма приёмки будет доступна после реализации соответствующей истории.' : 'Состав и партии поставки будут доступны после реализации истории деталей.'}</p>
    <Link className="ui-button ui-button--secondary" to={safeSupplyListUrl(params.get('returnTo'))}>К истории поставок</Link>
  </section>
}
