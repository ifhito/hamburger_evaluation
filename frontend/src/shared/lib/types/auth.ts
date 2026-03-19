export interface AuthUser {
  id: number
  username: string
  email: string
}

export interface SignupRequest {
  username: string
  email: string
  password: string
  password_confirmation: string
}

export interface SignupResponse {
  id: number
  username: string
  email: string
  token: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  token: string
}
