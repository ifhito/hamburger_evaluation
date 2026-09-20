// email / admin は API が本人の閲覧時だけ返す(他人・匿名の閲覧ではキー自体が無い)ため省略可能。
export interface User {
  id: number;
  username: string;
  email?: string;
  admin?: boolean;
}

export interface UserUpdateInput {
  username?: string;
  email?: string;
  password?: string;
  passwordConfirmation?: string;
}
