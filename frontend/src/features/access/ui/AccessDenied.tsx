import { Link } from 'react-router'
import { Icon } from '@/shared/ui'

export function AccessDenied() {
  return <section className="ui-state" role="alert">
    <Icon name="alert" /><h1>Недостаточно прав</h1>
    <p>Этот раздел или действие недоступны. Обратитесь к администратору, если доступ нужен для работы.</p>
    <Link className="ui-button ui-button--secondary" to="/">К обзору</Link>
  </section>
}
