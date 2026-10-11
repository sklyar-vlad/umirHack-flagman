import { useEffect, useRef, useState } from 'react'
import { useSelector } from 'react-redux'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { roleNames, visibleSections, type SessionState } from '@/entities/session'
import { Button, Drawer, Icon } from '@/shared/ui'
import styles from './AppShell.module.css'

function Navigation({ role, onNavigate }: { role: keyof typeof roleNames; onNavigate?: () => void }) {
  const overviewAlias = useLocation().pathname === '/dashboard'
  return <nav aria-label="Разделы приложения" className={styles.navigation}>
    {(['main', 'extra'] as const).map((group) => <div key={group} className={styles.group}>
      {group === 'extra' && <p className="text-caption text-muted">Дополнительно</p>}
      {visibleSections(role, group).map((section) => <NavLink key={section.id} to={section.id === 'overview' && overviewAlias ? '/dashboard' : section.path} end={section.path === '/'} onClick={onNavigate}
        className={({ isActive }) => `${styles.navLink} ${isActive ? styles.active : ''}`}>
        <Icon name={section.icon} /><span>{section.title}</span>
      </NavLink>)}
    </div>)}
    {visibleSections(role, 'admin').length > 0 && <NavLink to="/admin" onClick={onNavigate} className={({ isActive }) => `${styles.navLink} ${isActive ? styles.active : ''}`}><Icon name="adminSettings" /><span>Админ-панель</span></NavLink>}
  </nav>
}

export function AppShell() {
  const { user, settings } = useSelector((state: { session: SessionState }) => state.session)
  const navigate = useNavigate()
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const mainRef = useRef<HTMLElement>(null)
  useEffect(() => { mainRef.current?.focus() }, [location.pathname])
  // SessionGate owns loading, errors, expiration and logout.
  if (!user || !settings) return null
  return <div className={styles.shell}>
    <a className={styles.skip} href="#app-content">К содержимому</a>
    <aside className={styles.sidebar} aria-label="Боковая панель">
      <Link to="/" className={styles.brand}><img src="/logo-wordmark.svg" alt="Кладовая" width="168" /></Link>
      <p className={styles.restaurant}>{settings.restaurantName}</p>
      <Navigation role={user.role} />
    </aside>
    <div className={styles.workspace}>
      <header className={styles.header}>
        <Button variant="secondary" className={styles.menuButton} aria-label="Открыть меню" aria-expanded={open} aria-haspopup="dialog" onClick={() => setOpen(true)}><Icon name="menu" /></Button>
        <div className={styles.headerRestaurant}><p className="text-strong">{settings.restaurantName}</p><p className="text-caption text-muted">Ресторанный склад</p></div>
        <div className={styles.user}><p className="text-strong">{user.name}</p><p className="text-caption text-muted">{roleNames[user.role]}</p></div>
        <Button variant="secondary" onClick={() => { void navigate('/logout') }}>Выйти</Button>
      </header>
      <main id="app-content" tabIndex={-1} ref={mainRef} className={styles.content}><Outlet /></main>
    </div>
    <Drawer open={open} onClose={() => setOpen(false)} title="Меню" closeLabel="Закрыть меню">
      <p className={styles.restaurant}>{settings.restaurantName}</p>
      <Navigation role={user.role} onNavigate={() => setOpen(false)} />
    </Drawer>
  </div>
}
