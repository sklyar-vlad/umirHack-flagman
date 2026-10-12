import { NavLink } from 'react-router'

export function AnalyticsNavigation() {
  return <nav aria-label="Отчёты аналитики" className="ui-actions">{[
    ['/analytics/write-offs', 'Потери от списаний'],
    ['/analytics/ingredient-shortages', 'Недостача ингредиентов'],
  ].map(([path, title]) => <NavLink key={path} to={path} className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`}>{title}</NavLink>)}</nav>
}
