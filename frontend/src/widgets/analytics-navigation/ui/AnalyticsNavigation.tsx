import { NavLink } from 'react-router'

export function AnalyticsNavigation() {
  return <nav aria-label="Отчёты аналитики" className="ui-actions">{[
    ['/analytics/product-flow', 'Движение продуктов'],
    ['/analytics/write-offs', 'Потери от списаний'],
    ['/analytics/ingredient-shortages', 'Недостача ингредиентов'],
    ['/analytics/stop-list-forecast', 'Прогноз стоп-листа'],
  ].map(([path, title]) => <NavLink key={path} to={path} className={({ isActive }) => `ui-button ui-button--${isActive ? 'primary' : 'secondary'}`}>{title}</NavLink>)}</nav>
}
