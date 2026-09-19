import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'
import {
  authApi,
  getToken,
  removeToken,
  setToken,
} from '../../shared/lib/api'
import type {
  AuthUser,
  LoginRequest,
  SignupRequest,
} from '../../shared/lib/types/auth'

interface AuthContextValue {
  user: AuthUser | null
  token: string | null
  isLoading: boolean
  login(data: LoginRequest): Promise<void>
  signup(data: SignupRequest): Promise<void>
  logout(): Promise<void>
  refreshUser(user: AuthUser): void
}

const AuthContext = createContext<AuthContextValue | null>(null)

function decodeJwtPayload(token: string): Record<string, unknown> {
  try {
    const base64 = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    return JSON.parse(atob(base64)) as Record<string, unknown>
  } catch {
    return {}
  }
}

function isTokenExpired(token: string): boolean {
  const payload = decodeJwtPayload(token)
  if (typeof payload.exp !== 'number') return true
  return Date.now() / 1000 > payload.exp
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [token, setTokenState] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  // Restore from localStorage on mount
  useEffect(() => {
    const storedToken = getToken()
    if (storedToken && !isTokenExpired(storedToken)) {
      const storedUser = localStorage.getItem('auth_user')
      if (storedUser) {
        try {
          const parsed = JSON.parse(storedUser) as AuthUser
          // Older stored users may lack admin; keep it a real boolean.
          setUser({ ...parsed, admin: Boolean(parsed.admin) })
          setTokenState(storedToken)
        } catch {
          removeToken()
          localStorage.removeItem('auth_user')
        }
      }
    } else if (storedToken) {
      removeToken()
      localStorage.removeItem('auth_user')
    }
    setIsLoading(false)
  }, [])

  const signup = useCallback(async (data: SignupRequest) => {
    const res = await authApi.signup(data)
    setToken(res.token)
    setTokenState(res.token)
    const authUser: AuthUser = { id: res.id, username: res.username, email: res.email, admin: res.admin }
    setUser(authUser)
    localStorage.setItem('auth_user', JSON.stringify(authUser))
  }, [])

  const login = useCallback(async (data: LoginRequest) => {
    const res = await authApi.login(data)
    setToken(res.token)
    setTokenState(res.token)
    const authUser: AuthUser = { id: res.id, username: res.username, email: res.email, admin: res.admin }
    setUser(authUser)
    localStorage.setItem('auth_user', JSON.stringify(authUser))
  }, [])

  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // Ignore errors — clear local state regardless
    }
    removeToken()
    localStorage.removeItem('auth_user')
    setTokenState(null)
    setUser(null)
  }, [])

  const refreshUser = useCallback((updatedUser: AuthUser) => {
    setUser(updatedUser)
    localStorage.setItem('auth_user', JSON.stringify(updatedUser))
  }, [])

  return (
    <AuthContext.Provider value={{ user, token, isLoading, login, signup, logout, refreshUser }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
