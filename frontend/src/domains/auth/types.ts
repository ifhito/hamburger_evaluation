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

export interface SignupResponse {
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

// Go の authUserResponse: signup と login は同じ本文を返す。
export type LoginResponse = SignupResponse;
