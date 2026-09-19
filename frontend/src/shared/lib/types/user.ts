export interface User {
  id: number
  username: string
  email: string
  admin: boolean
}

export interface UserUpdateInput {
  username?: string
  email?: string
  password?: string
  password_confirmation?: string
}
