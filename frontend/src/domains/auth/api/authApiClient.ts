import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../storage";
import type {
  AuthUserResponse,
  CurrentUserResponse,
  GoogleExchangeResponse,
  IdentitiesResponse,
  LoginRequest,
  LoginResponse,
  SignupAcceptedResponse,
  SignupRequest,
} from "../types";

export const authApiClient = buildApiClient(getToken);

export const authApi = {
  async signup(data: SignupRequest): Promise<SignupAcceptedResponse> {
    const res = await authApiClient.post<SignupAcceptedResponse>("/signup", data);
    return res.data;
  },
  async confirmSignup(token: string): Promise<AuthUserResponse> {
    const res = await authApiClient.post<AuthUserResponse>("/signup/confirm", { token });
    return res.data;
  },
  async login(data: LoginRequest): Promise<LoginResponse> {
    const res = await authApiClient.post<LoginResponse>("/login", data);
    return res.data;
  },
  // 保存済みのトークンで、現在のユーザーを取得する。トークンの有効性は backend が判断する(無効・期限切れは 401)。
  async me(): Promise<CurrentUserResponse> {
    const res = await authApiClient.get<CurrentUserResponse>("/me");
    return res.data;
  },
  // Google でのサインインの結果(1 回限りのコード)を交換する。サインインの成功は、ログインと同じ本文(token つき)で返る。
  // 重複・失敗・無効なコードは、サーバーの文言を持つ ApiError(409・400)になる。コードは 1 回しか使えない。
  async exchangeGoogleCode(code: string): Promise<GoogleExchangeResponse> {
    const res = await authApiClient.post<GoogleExchangeResponse>("/auth/google/exchange", { code });
    return res.data;
  },
  // ログイン済みの利用者が、Google アカウントを結び付ける手続きを、このブラウザで始める。応答の Set-Cookie で、
  // このブラウザに手続きの cookie が設定され(同一オリジンの要求なので、そのまま保存される)、Google の認可の画面の
  // URL(redirectUrl)が返る。画面は、その URL へ移動する(別のブラウザで開いても、cookie がないので失敗する)。
  // returnTo は、手続きのあとに戻る画面(アプリの中のパス。省略できる)。
  async startGoogleLink(returnTo?: string): Promise<{ redirectUrl: string }> {
    const res = await authApiClient.post<{ redirectUrl: string }>(
      "/me/identities/google/link",
      returnTo ? { returnTo } : undefined,
    );
    return res.data;
  },
  async listIdentities(): Promise<IdentitiesResponse> {
    const res = await authApiClient.get<IdentitiesResponse>("/me/identities");
    return res.data;
  },
  // 解除するとサインインする方法がなくなるときは、サーバーの文言を持つ ApiError(422)になる。
  async unlinkGoogle(): Promise<void> {
    await authApiClient.delete("/me/identities/google");
  },
  async logout(): Promise<{ message: string }> {
    const res = await authApiClient.post<{ message: string }>("/logout");
    return res.data;
  },
};
