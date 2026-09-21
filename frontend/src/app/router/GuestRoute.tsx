import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../../domains/auth/AuthProvider'
import { returnPathFrom } from './returnTo'

export function GuestRoute() {
  const { user, isLoading } = useAuth()
  const location = useLocation()
  if (isLoading) return null
  // ログインが必要な画面から送られてきたときは、ログインのあとに、その画面へ戻す。
  if (user) return <Navigate to={returnPathFrom(location.state) ?? '/reviews'} replace />
  return <Outlet />
}
