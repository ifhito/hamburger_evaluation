import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../domains/auth/AuthProvider'
import styles from './layout.module.css'

interface LayoutProps {
  children: ReactNode
  title?: string
}

export function Layout({ children, title }: LayoutProps) {
  const { t } = useTranslation()
  const { user } = useAuth()

  return (
    <div className={styles.wrapper}>
      <header className={styles.header}>
        <Link to="/shops" className={styles.brand}>
          🍔 {t('nav.brand')}
        </Link>
        <nav className={styles.nav}>
          <Link to="/shops" className={styles.navLink}>{t('nav.shops')}</Link>
          <Link to="/reviews" className={styles.navLink}>{t('nav.reviews')}</Link>
          {user ? (
            <>
              <Link to={`/users/${user.id}`} className={styles.navLink}>{user.username}</Link>
              <Link to="/signout" className={styles.navLink}>{t('nav.signout')}</Link>
            </>
          ) : (
            <>
              <Link to="/signin" className={styles.navLink}>{t('nav.signin')}</Link>
              <Link to="/signup" className={styles.navLink}>{t('nav.signup')}</Link>
            </>
          )}
        </nav>
      </header>
      <main className={styles.main}>
        {title && <h1 className={styles.heading}>{title}</h1>}
        {children}
      </main>
    </div>
  )
}
