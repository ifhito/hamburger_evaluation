import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '../../domains/auth/AuthProvider'

export function GuestRoute() {
  const { user, isLoading } = useAuth()
  if (isLoading) return null
  if (user) return <Navigate to="/reviews" replace />
  return <Outlet />
}
