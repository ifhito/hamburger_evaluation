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

// GET /me の応答(現在のユーザー)。signup と login の応答は、これに token が付く。
export interface CurrentUserResponse {
  id: string;
  username: string;
  email: string;
  canModerate: boolean;
}

export interface SignupResponse extends CurrentUserResponse {
  token: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

// Go の authUserResponse: signup と login は同じ本文を返す。
export type LoginResponse = SignupResponse;
