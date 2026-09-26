import { Navigate } from 'react-router-dom'
import { useAuth } from '../../domains/auth/AuthProvider'

// ProtectedRouteの配下。復元された本人のプロフィールへ案内する。
export default function MyPage() {
  const { user } = useAuth()
  return user ? <Navigate to={`/users/${user.id}`} replace /> : null
}
