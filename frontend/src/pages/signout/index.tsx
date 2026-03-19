import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { Layout } from '../../shared/ui/Layout'

export default function SignoutPage() {
  const { logout } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    void logout().then(() => navigate('/reviews'))
  }, [logout, navigate])

  return (
    <Layout title="Signing out…">
      <p style={{ color: 'var(--color-text-muted)' }}>You are being signed out…</p>
    </Layout>
  )
}
