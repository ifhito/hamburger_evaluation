import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'

interface LayoutProps {
  children: ReactNode
  title?: string
}

export function Layout({ children, title }: LayoutProps) {
  const { user } = useAuth()

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <header
        style={{
          background: 'var(--color-primary)',
          color: '#fff',
          padding: '0 24px',
          height: 56,
          display: 'flex',
          alignItems: 'center',
          gap: 24,
        }}
      >
        <Link to="/shops" style={{ color: '#fff', fontWeight: 700, fontSize: '1.1rem' }}>
          🍔 Hamburger Evaluation
        </Link>
        <nav style={{ marginLeft: 'auto', display: 'flex', gap: 16, alignItems: 'center' }}>
          <Link to="/shops" style={{ color: '#fff' }}>Shops</Link>
          <Link to="/reviews" style={{ color: '#fff' }}>Reviews</Link>
          {user ? (
            <>
              <Link to={`/users/${user.id}`} style={{ color: '#fff' }}>{user.username}</Link>
              <Link to="/signout" style={{ color: '#fff' }}>Sign out</Link>
            </>
          ) : (
            <>
              <Link to="/signin" style={{ color: '#fff' }}>Sign in</Link>
              <Link to="/signup" style={{ color: '#fff' }}>Sign up</Link>
            </>
          )}
        </nav>
      </header>
      <main style={{ flex: 1, padding: '24px', maxWidth: 800, margin: '0 auto', width: '100%' }}>
        {title && <h1 style={{ marginBottom: 24, fontSize: '1.5rem' }}>{title}</h1>}
        {children}
      </main>
    </div>
  )
}
