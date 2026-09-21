// bio は自己紹介文(biography の略)で、書かれていなければ空文字。誰が閲覧しても返る。
// email / admin は API が本人の閲覧時だけ返す(他人・匿名の閲覧ではキー自体が無い)ため省略可能。
// canEdit は、閲覧者がこのプロフィールを編集・削除できるか(本人だけ true)。backend が返す。
export interface User {
  id: string;
  username: string;
  bio: string;
  email?: string;
  admin?: boolean;
  canEdit: boolean;
}

export interface UserUpdateInput {
  username?: string;
  bio?: string;
  email?: string;
  password?: string;
  passwordConfirmation?: string;
}
