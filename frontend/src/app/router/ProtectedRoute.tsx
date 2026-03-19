import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from '../providers/AuthProvider'

export function ProtectedRoute() {
  const { user, isLoading } = useAuth()
  if (isLoading) return null
  if (!user) return <Navigate to="/signin" replace />
  return <Outlet />
}
