// email / admin は API が本人の閲覧時だけ返す(他人・匿名の閲覧ではキー自体が無い)ため省略可能。
// canEdit は、閲覧者がこのプロフィールを編集・削除できるか(本人だけ true)。backend が返す。
export interface User {
  id: string;
  username: string;
  email?: string;
  admin?: boolean;
  canEdit: boolean;
}

export interface UserUpdateInput {
  username?: string;
  email?: string;
  password?: string;
  passwordConfirmation?: string;
}
