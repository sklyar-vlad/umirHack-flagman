import { Link, useParams, useSearchParams } from 'react-router'
import { z } from 'zod'
import { safeOrderListUrl } from '@/entities/order'
import { ErrorState } from '@/shared/ui'
import styles from './OrdersPage.module.css'

export function OrderEntryPage() {
  const { id } = useParams(), [params] = useSearchParams()
  const invalid = id !== undefined && !z.uuid().safeParse(id).success
  return <section className={styles.page}><header className={styles.header}><h1>{id ? 'Детали заказа' : 'Новый заказ'}</h1><Link className="ui-button ui-button--secondary" to={safeOrderListUrl(params.get('returnTo'))}>К истории заказов</Link></header>
    {invalid ? <ErrorState title="Некорректный идентификатор заказа" description="Вернитесь к истории и выберите заказ." /> : <p>{id ? 'Просмотр списаний по партиям пока недоступен.' : 'Создание заказа пока недоступно.'}</p>}
  </section>
}
