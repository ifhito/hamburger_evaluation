// bio(自己紹介文。空文字は未設定)は、誰が閲覧しても返る。
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
