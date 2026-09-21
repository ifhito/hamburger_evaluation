// frontend が持つ認証ユーザー。moderation(管理画面・承認・却下)ができるかは、backend が
// 返す canModerate で決める(admin の値から権限を導かない)。
export interface AuthUser {
  id: string;
  username: string;
  email: string;
  canModerate: boolean;
}

export interface SignupRequest {
  username: string;
  email: string;
  password: string;
  passwordConfirmation: string;
}

// POST /signup の 202 の本文。登録済みの email でも未登録の email でも同じ値が返る
// (応答から登録の有無を判別できないようにするため)。アカウントは確認メールのリンクを
// 開いて初めて作られ、この時点ではログインしない。
export interface SignupAcceptedResponse {
  message: string;
}

// GET /me の応答(現在のユーザー)。ログインと signup の確認の応答は、これに token が付く。
export interface CurrentUserResponse {
  id: string;
  username: string;
  email: string;
  canModerate: boolean;
}

// ログインと、signup の確認(POST /signup/confirm)が返す本文。
export interface AuthUserResponse extends CurrentUserResponse {
  token: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

// Go の authUserResponse: login と signup の確認は同じ本文を返す。
export type LoginResponse = AuthUserResponse;
