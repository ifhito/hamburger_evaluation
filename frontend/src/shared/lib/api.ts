import type { ApiError } from './types/api'
import type {
  LoginRequest,
  LoginResponse,
  SignupRequest,
  SignupResponse,
} from './types/auth'
import type { Review, ReviewCreateInput, ReviewListParams, ReviewUpdateInput } from './types/review'
import type { Shop, ShopDetail } from './types/shop'
import type { User, UserUpdateInput } from './types/user'

const BASE_URL = '/api'

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly apiError: ApiError,
  ) {
    super(apiError.error ?? apiError.errors?.join(', ') ?? apiError.message ?? 'Request failed')
    this.name = 'ApiRequestError'
  }
}

export const getToken = (): string | null => localStorage.getItem('token')
export const setToken = (token: string): void => localStorage.setItem('token', token)
export const removeToken = (): void => localStorage.removeItem('token')

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const { headers: optHeaders, ...restOptions } = options ?? {}
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json', ...optHeaders },
    ...restOptions,
  })
  if (!res.ok) {
    const body: ApiError = await res.json().catch(() => ({}))
    throw new ApiRequestError(res.status, body)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

async function authRequest<T>(path: string, options?: RequestInit): Promise<T> {
  const token = getToken()
  return request<T>(path, {
    ...options,
    headers: {
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  })
}

export const authApi = {
  signup(data: SignupRequest): Promise<SignupResponse> {
    return request('/signup', { method: 'POST', body: JSON.stringify(data) })
  },
  login(data: LoginRequest): Promise<LoginResponse> {
    return request('/login', { method: 'POST', body: JSON.stringify(data) })
  },
  logout(): Promise<{ message: string }> {
    return authRequest('/logout', { method: 'POST' })
  },
}

export const reviewsApi = {
  list(params?: ReviewListParams): Promise<Review[]> {
    const qs = params ? '?' + new URLSearchParams(
      Object.fromEntries(
        Object.entries(params)
          .filter(([, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    ) : ''
    return request(`/reviews${qs}`)
  },
  get(id: number): Promise<Review> {
    return request(`/reviews/${id}`)
  },
  create(data: ReviewCreateInput): Promise<Review> {
    return authRequest('/reviews', { method: 'POST', body: JSON.stringify({ review: data }) })
  },
  update(id: number, data: ReviewUpdateInput): Promise<Review> {
    return authRequest(`/reviews/${id}`, { method: 'PUT', body: JSON.stringify({ review: data }) })
  },
  delete(id: number): Promise<void> {
    return authRequest(`/reviews/${id}`, { method: 'DELETE' })
  },
}

export const shopsApi = {
  list(params?: { keyword?: string }): Promise<Shop[]> {
    const qs = params?.keyword ? `?keyword=${encodeURIComponent(params.keyword)}` : ''
    return request(`/shops${qs}`)
  },
  get(id: number): Promise<ShopDetail> {
    return request(`/shops/${id}`)
  },
}

export const usersApi = {
  list(): Promise<User[]> {
    return request('/users')
  },
  update(id: number, data: UserUpdateInput): Promise<User> {
    return authRequest(`/users/${id}`, { method: 'PUT', body: JSON.stringify({ user: data }) })
  },
  delete(id: number): Promise<void> {
    return authRequest(`/users/${id}`, { method: 'DELETE' })
  },
}
