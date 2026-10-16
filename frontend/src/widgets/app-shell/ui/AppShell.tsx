import { useRef, useState } from 'react'
import { useSelector } from 'react-redux'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'
import { roleNames, sectionForPath, visibleSections, type SessionState } from '@/entities/session'
import { Button, Drawer, Icon } from '@/shared/ui'
import styles from './AppShell.module.css'

function Navigation({ role, onNavigate }: { role: keyof typeof roleNames; onNavigate?: () => void }) {
  const pathname = useLocation().pathname
  const selected = sectionForPath(pathname)
  const activeId = selected?.id === 'offers' ? 'offers' : selected?.group === 'recommendations' ? 'recommendations' : selected?.id
  return <nav aria-label="Разделы приложения" className={styles.navigation}>
    {(['main', 'extra'] as const).map((group) => <div key={group} className={styles.group}>
      {group === 'extra' && <p className="text-caption text-muted">Дополнительно</p>}
      {[...visibleSections(role, group).filter((section) => group !== 'extra' || section.id !== 'dishes'), ...(group === 'main' ? [...visibleSections(role, 'extra').filter((section) => section.id === 'dishes'), ...visibleSections(role, 'recommendations').filter((section) => section.id === 'offers')] : [])].map((section) => <Link key={section.id} to={section.path} onClick={onNavigate}
        aria-current={activeId === section.id ? 'page' : undefined} className={`${styles.navLink} ${activeId === section.id ? styles.active : ''}`}>
        <Icon name={section.icon} /><span>{section.title}</span>
      </Link>)}
    </div>)}
    {visibleSections(role, 'admin').length > 0 && <Link to="/admin" onClick={onNavigate} aria-current={pathname === '/admin' || selected?.group === 'admin' ? 'page' : undefined} className={`${styles.navLink} ${pathname === '/admin' || selected?.group === 'admin' ? styles.active : ''}`}><Icon name="adminSettings" /><span>Админ-панель</span></Link>}
  </nav>
}

export function AppShell() {
  const { user, settings } = useSelector((state: { session: SessionState }) => state.session)
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const menuRef = useRef<HTMLButtonElement>(null)
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
        <Button ref={menuRef} variant="secondary" className={styles.menuButton} aria-label="Открыть меню" aria-expanded={open} aria-haspopup="dialog" onClick={() => setOpen(true)}><Icon name="menu" /></Button>
        <div className={styles.headerRestaurant}><p className="text-strong">{settings.restaurantName}</p><p className="text-caption text-muted">Ресторанный склад</p></div>
        <div className={styles.user}><p className="text-strong">{user.name}</p><p className="text-caption text-muted">{roleNames[user.role]}</p></div>
        <Button variant="secondary" onClick={() => { void navigate('/logout') }}>Выйти</Button>
      </header>
      <main id="app-content" tabIndex={-1} className={styles.content}><Outlet /></main>
    </div>
    <Drawer side="left" open={open} returnFocusRef={menuRef} onClose={() => setOpen(false)} title="Меню" closeLabel="Закрыть меню">
      <p className={styles.restaurant}>{settings.restaurantName}</p>
      <Navigation role={user.role} onNavigate={() => setOpen(false)} />
    </Drawer>
  </div>
}
