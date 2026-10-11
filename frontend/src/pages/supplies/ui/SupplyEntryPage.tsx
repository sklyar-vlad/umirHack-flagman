import { useState } from 'react'
import { useSelector } from 'react-redux'
import { Link, useSearchParams } from 'react-router'
import { safeSupplyListUrl, type SupplyDetail } from '@/entities/supply'
import { selectSession } from '@/entities/session'
import { ReceiveSupplyForm } from '@/widgets/supply-reception'
import { SupplyReceptionResult } from './SupplyReceptionResult'
import styles from './SuppliesPage.module.css'

export function SupplyEntryPage() {
  const [params] = useSearchParams()
  const [received, setReceived] = useState<SupplyDetail | null>(null)
  const timezone = useSelector(selectSession).settings?.timezone ?? 'Europe/Moscow'
  const returnTo = safeSupplyListUrl(params.get('returnTo'))
  return <section className={styles.page}><header className={styles.header}><h1>Новая поставка</h1>
    <Link className="ui-button ui-button--secondary" to={returnTo}>К истории поставок</Link>
  </header>
    {received ? <SupplyReceptionResult supply={received} timezone={timezone} returnTo={returnTo} /> : <ReceiveSupplyForm onReceived={setReceived} />}
  </section>
}
