export interface User {
  id: number;
  username: string;
  email: string;
}

export interface UserUpdateInput {
  username?: string;
  email?: string;
  password?: string;
  passwordConfirmation?: string;
}
