import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../../domains/auth/AuthProvider'
import type { ReturnToState } from './returnTo'

export function ProtectedRoute() {
  const { user, isLoading } = useAuth()
  const location = useLocation()
  if (isLoading) return null
  // ログインのあとに、いまの画面(許可を尋ねる画面など)へ戻せるよう、戻り先を遷移の state に入れる。
  const state: ReturnToState = { from: location.pathname + location.search }
  if (!user) return <Navigate to="/signin" replace state={state} />
  return <Outlet />
}
