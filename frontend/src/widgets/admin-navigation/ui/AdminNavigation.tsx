import { useSelector } from 'react-redux'
import { NavLink } from 'react-router'
import { selectSession, visibleSections } from '@/entities/session'

/** Keep the existing admin directory reachable when a placeholder becomes a business page. */
export function AdminNavigation() {
  const role = useSelector(selectSession).user?.role
  return <nav aria-label="Административные разделы" className="ui-actions">{role && visibleSections(role, 'admin').map((item) => <NavLink key={item.id} to={item.path} className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`}>{item.title}</NavLink>)}</nav>
}
