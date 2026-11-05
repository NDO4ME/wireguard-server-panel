import logoFull from '../theme/logo.webp'
import logoMark from '../theme/logo-mark.webp'
import { useEffect, useState, ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { Icon } from './Icon'
import { api } from '../api/client'

interface NavItem {
  to: string
  label: string
  icon: string
  title: string
  desc: string
}

export const NAV: NavItem[] = [
  { to: '/', label: 'Clients', icon: 'users', title: 'Clients', desc: 'WireGuard clients, live connection state and traffic.' },
  { to: '/tailscale', label: 'Tailscale', icon: 'network', title: 'Tailscale', desc: 'Connect this server to your tailnet, route clients into it and keep Tailscale up to date.' },
  { to: '/api-access', label: 'API access', icon: 'key', title: 'API access', desc: 'Token and endpoints for automating the panel.' },
  { to: '/settings', label: 'Settings', icon: 'sliders', title: 'Settings', desc: 'Client defaults, logging and admin password.' },
]

const SB_KEY = 'ndo4.sidebar'

interface LayoutProps {
  onLogout: () => void
  actions?: ReactNode
  children: ReactNode
}

export default function Layout({ onLogout, actions, children }: LayoutProps) {
  const { pathname } = useLocation()
  const current = NAV.find(n => n.to === pathname) ?? NAV[0]

  const [mini, setMini] = useState(() => {
    try {
      return localStorage.getItem(SB_KEY) === 'mini'
    } catch {
      return false
    }
  })
  const [drawer, setDrawer] = useState(false)
  const [menu, setMenu] = useState(false)
  const [online, setOnline] = useState(true)

  useEffect(() => {
    try {
      localStorage.setItem(SB_KEY, mini ? 'mini' : 'full')
    } catch {
      /* storage blocked */
    }
  }, [mini])

  useEffect(() => {
    setDrawer(false)
    setMenu(false)
    document.title = `${current.title} · NDO4ME`
  }, [pathname, current.title])

  useEffect(() => {
    const up = () => setOnline(true)
    const down = () => setOnline(false)
    window.addEventListener('online', up)
    window.addEventListener('offline', down)
    return () => {
      window.removeEventListener('online', up)
      window.removeEventListener('offline', down)
    }
  }, [])

  useEffect(() => {
    if (!menu) return
    const close = (e: MouseEvent) => {
      if (!(e.target as HTMLElement).closest('.menu')) setMenu(false)
    }
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setMenu(false)
    document.addEventListener('click', close)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', esc)
    }
  }, [menu])

  const rootClass = ['app', mini ? 'sb-mini' : '', drawer ? 'sb-open' : ''].filter(Boolean).join(' ')

  const signOut = async () => {
    await api.logout()
    onLogout()
  }

  return (
    <div className={rootClass}>
      <a className="skip" href="#main">Skip to content</a>
      <aside className="sb" aria-label="Sidebar">
        <NavLink to="/" className="sb-brand" aria-label="NDO4ME" data-label="NDO4ME">
          <img className="sb-logo-full" src={logoFull} width={124} height={31} alt="" draggable={false} />
          <img className="sb-logo-mark" src={logoMark} width={34} height={20} alt="" draggable={false} />
        </NavLink>
        <nav className="sb-nav" aria-label="Main">
          <div className="sb-group">Manage</div>
          {NAV.slice(0, 2).map(n => (
            <NavLink key={n.to} to={n.to} end className="sb-item" data-label={n.label}>
              <Icon name={n.icon} />
              <span>{n.label}</span>
            </NavLink>
          ))}
          <div className="sb-group">Configure</div>
          {NAV.slice(2).map(n => (
            <NavLink key={n.to} to={n.to} end className="sb-item" data-label={n.label}>
              <Icon name={n.icon} />
              <span>{n.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="sb-foot">
          <span className="sb-ver">WireGuard Panel</span>
          <button
            type="button"
            className="sb-collapse"
            aria-label={mini ? 'Expand sidebar' : 'Collapse sidebar'}
            title={mini ? 'Expand sidebar' : 'Collapse sidebar'}
            onClick={() => setMini(m => !m)}
          >
            <Icon name="chevrons-left" />
          </button>
        </div>
      </aside>
      <div className="scrim" aria-hidden="true" onClick={() => setDrawer(false)} />

      <div className="main">
        <header className="tb">
          <button type="button" className="tb-btn tb-menu" aria-label="Open menu" onClick={() => setDrawer(true)}>
            <Icon name="menu" />
          </button>
          <nav className="crumbs" aria-label="Breadcrumb">
            <span>NDO4ME</span>
            <Icon name="chevron-right" />
            <span aria-current="page">{current.title}</span>
          </nav>
          <div className="tb-right">
            <span className={online ? 'conn' : 'conn on'} role="status" aria-live="polite">
              <i />
              Offline
            </span>
            <div className="menu">
              <button
                type="button"
                className="tb-user"
                aria-label="Account menu"
                aria-expanded={menu}
                onClick={() => setMenu(m => !m)}
              >
                <span className="tb-avatar" aria-hidden="true">N</span>
                <span className="hide-sm">Administrator</span>
                <Icon name="chevron-down" />
              </button>
              {menu && (
                <div className="menu-pop">
                  <button type="button" className="menu-item" onClick={signOut}>
                    <Icon name="logout" />
                    Sign out
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>

        <main className="page" id="main" tabIndex={-1}>
          <div className="ph">
            <div className="ph-text">
              <h1>{current.title}</h1>
              <p className="ph-desc">{current.desc}</p>
            </div>
            {actions && <div className="ph-actions">{actions}</div>}
          </div>
          {children}
        </main>
      </div>
    </div>
  )
}
