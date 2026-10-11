import { Link } from 'react-router'
import { AccessDenied } from '@/features/access'

export function RouteStatusPage({ forbidden = false }: { forbidden?: boolean }) {
  if (forbidden) return <AccessDenied />
  return <section className="ui-state"><h1>Страница не найдена</h1><p>Проверьте адрес или откройте доступный раздел в меню.</p><Link to="/" className="ui-button ui-button--secondary">К обзору</Link></section>
}
