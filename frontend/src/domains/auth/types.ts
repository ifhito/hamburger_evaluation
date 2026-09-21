export interface AuthUser {
  id: number;
  username: string;
  email: string;
  admin: boolean;
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

// ログインと、signup の確認(POST /signup/confirm)が返す本文。
export interface AuthUserResponse {
  id: number;
  username: string;
  email: string;
  admin: boolean;
  token: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

// Go の authUserResponse: login と signup の確認は同じ本文を返す。
export type LoginResponse = AuthUserResponse;
