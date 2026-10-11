import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { safeOrderListUrl, type OrderDetail } from '@/entities/order'
import { OrderComposer } from '@/widgets/order-composer'
import { CreatedOrder } from './CreatedOrder'
import styles from './OrdersPage.module.css'

export function NewOrderPage() {
  const [params] = useSearchParams(), [created, setCreated] = useState<OrderDetail>()
  return <section className={styles.page}><header className={styles.header}><h1>{created ? 'Заказ создан' : 'Новый заказ'}</h1><Link className="ui-button ui-button--secondary" to={safeOrderListUrl(params.get('returnTo'))}>К истории заказов</Link></header>
    {created ? <CreatedOrder order={created} onNew={() => setCreated(undefined)} /> : <OrderComposer onCreated={setCreated} />}
  </section>
}
