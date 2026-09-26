import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../../domains/auth/AuthProvider'
import { Logo } from '../../components/Logo'
import { LanguageSwitcher } from '../../components/LanguageSwitcher'
import { NavigationIcon } from './NavigationIcon'
import styles from './navigation.module.css'

export function Layout({ children, title }: { children: ReactNode; title?: string }) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { pathname, search } = useLocation()
  const main = useRef<HTMLElement>(null)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const record = pathname === '/record' || (pathname.startsWith('/shops/') && new URLSearchParams(search).get('from') === 'record') || pathname === '/reviews/new' || /^\/reviews\/[^/]+\/edit$/.test(pathname)
  const profile = pathname === '/me' || pathname === '/signout' || (!!user && (pathname === `/users/${user.id}` || pathname === `/users/${user.id}/edit`))
  const discover = !record && /^\/(discover|shops|burgers|reviews)(\/|$)/.test(pathname)
  const active = record ? 'record' : profile ? 'profile' : discover ? 'discover' : pathname === '/' ? 'home' : null

  useEffect(() => { main.current?.focus({ preventScroll: true }) }, [pathname, search])
  useEffect(() => {
    const viewport = window.visualViewport
    if (!viewport) return
    const update = () => {
      const focused = document.activeElement
      const editing = focused instanceof HTMLElement && (focused.matches('input, textarea, select') || focused.isContentEditable)
      setKeyboardOpen(editing && window.innerHeight - viewport.height > 120)
    }
    viewport.addEventListener('resize', update)
    document.addEventListener('focusin', update)
    document.addEventListener('focusout', update)
    return () => {
      viewport.removeEventListener('resize', update)
      document.removeEventListener('focusin', update)
      document.removeEventListener('focusout', update)
    }
  }, [])

  const items = [
    { kind: 'home', to: '/', label: t('navigation.home'), name: t('navigation.home') },
    { kind: 'discover', to: '/discover', label: t('navigation.discoverShort'), name: t('navigation.discover') },
    { kind: 'record', to: '/record', label: t('navigation.record'), name: t('navigation.record') },
    { kind: 'profile', to: '/me', label: t('navigation.profile'), name: t('navigation.profile') },
  ] as const

  return <div className={styles.wrapper}>
    <a className={styles.skip} href="#main-content">{t('navigation.skip')}</a>
    <header className={styles.header}>
      <div className={styles.headerInner}>
        <Link to="/" className={styles.brand} aria-label={t('nav.brand')}><Logo name={t('nav.brand')} /></Link>
        <Link to="/about" className={styles.about} aria-current={pathname === '/about' ? 'page' : undefined}>{t('nav.aboutHeader')}</Link>
        <nav className={styles.desktop} aria-label={t('navigation.main')}>
          <Link to="/discover" className={styles.discover} aria-current={active === 'discover' ? 'page' : undefined}>{t('navigation.discover')}</Link>
          <Link to="/record" className={styles.record} aria-current={active === 'record' ? 'page' : undefined}><NavigationIcon kind="record" />{t('navigation.recordDesktop')}</Link>
          <Link to="/me" className={styles.profile} aria-label={t('navigation.profile')} aria-current={active === 'profile' ? 'page' : undefined}><NavigationIcon kind="profile" /></Link>
        </nav>
      </div>
    </header>
    <main id="main-content" ref={main} tabIndex={-1} className={styles.main}>
      {title && <h1 className={styles.heading}>{title}</h1>}
      {children}
    </main>
    <footer className={styles.footer}>
      <div className={styles.footerInner}>
        <div className={styles.footerBrand}><Link to="/" className={styles.brand}><Logo name={t('nav.brand')} /></Link><p>{t('navigation.message')}</p></div>
        <nav className={styles.footerLinks} aria-label={t('navigation.information')}><h2>Burger Stack</h2><Link to="/about">{t('nav.aboutHeader')}</Link><Link to="/mcp">{t('navigation.ai')}</Link></nav>
        <div className={styles.footerBottom}><small>© Burger Stack</small><LanguageSwitcher fullLabels /></div>
      </div>
    </footer>
    <nav className={`${styles.bottom} ${keyboardOpen ? styles.keyboardOpen : ''}`} aria-label={t('navigation.main')}>
      {items.map(({ kind, to, label, name }) => <Link key={kind} to={to} aria-label={name} aria-current={active === kind ? 'page' : undefined}>
        <span className={styles.icon}><NavigationIcon kind={kind} /></span><span>{label}</span>
      </Link>)}
    </nav>
  </div>
}
