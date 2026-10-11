import { useSelector } from 'react-redux'
import { Link, Navigate, NavLink } from 'react-router'
import { visibleSections, type Section, type SessionState } from '@/entities/session'
import { EmptyState } from '@/shared/ui'
import styles from './SectionPage.module.css'

function SectionLinks({ group }: { group: 'recommendations' | 'admin' }) {
  const role = useSelector((state: { session: SessionState }) => state.session.user!.role)
  return <nav aria-label={group === 'admin' ? 'Административные разделы' : 'Вкладки рекомендаций'} className={styles.links}>
    {visibleSections(role, group).map((item) => <NavLink className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`} key={item.id} to={item.path}>{item.title}</NavLink>)}
  </nav>
}

export function SectionLanding({ group }: { group: 'recommendations' | 'admin' }) {
  const role = useSelector((state: { session: SessionState }) => state.session.user!.role)
  const first = visibleSections(role, group)[0]
  return <Navigate to={first?.path ?? '/forbidden'} replace />
}

export function SectionPage({ section }: { section: Section }) {
  const title = section.group === 'recommendations' ? 'Рекомендации' : section.group === 'admin' ? 'Админ-панель' : section.title
  return <div className={styles.page}>
    <header className={styles.heading}><p className="text-caption text-muted">Ресторанный склад</p><h1>{title}</h1></header>
    {(section.group === 'recommendations' || section.group === 'admin') && <SectionLinks group={section.group} />}
    <div className="ui-panel"><EmptyState title={section.group === 'recommendations' || section.group === 'admin' ? section.title : 'Содержимое ещё не добавлено'}
      description="Содержимое этого раздела появится на следующем этапе разработки." /></div>
    {import.meta.env.DEV && section.id === 'overview' && <div className={styles.links}>
      <Link to="/dev/access">Проверить доступ и переходы</Link>
      <Link to="/dev/api">Проверить подключение к API</Link><Link to="/dev/ui">Компоненты интерфейса</Link>
    </div>}
  </div>
}
