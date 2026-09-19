export interface AuthUser {
  id: number
  username: string
  email: string
  admin: boolean
}

export interface SignupRequest {
  username: string
  email: string
  password: string
  password_confirmation: string
}

// Go authUserResponse: signup and login share the same body.
export interface SignupResponse {
  id: number
  username: string
  email: string
  admin: boolean
  token: string
}

export interface LoginRequest {
  email: string
  password: string
}

export type LoginResponse = SignupResponse
