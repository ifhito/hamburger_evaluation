import { ApiError, buildApiClient } from "../../../api/client/buildApiClient";
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

// Google の交換の失敗(409・400 など)。backend は、失敗の応答にも、検証済みの戻り先(return_to。手続きを始めた画面)を
// 含める。この項目は、この失敗にだけあるので、共有の ApiError には持たせず、ここで、本文から読む。なければ空。
export class GoogleExchangeError extends ApiError {
  readonly returnTo: string;

  constructor(cause: ApiError) {
    super(cause.messages, cause.status, cause.body);
    this.name = "GoogleExchangeError";
    const returnTo = (cause.body as { return_to?: unknown } | null | undefined)?.return_to;
    this.returnTo = typeof returnTo === "string" ? returnTo : "";
  }
}

// Google の手続きの cookie(手続きの cookie・交換の cookie)を送受信する要求に付ける。同一オリジン(/api の転送)の前提で、
// API を別のオリジンに置くと、cookie は届かない(その構成は、対応しない。CLAUDE.md を参照)。
const withCookies = { withCredentials: true } as const;
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
    try {
      const res = await authApiClient.post<GoogleExchangeResponse>("/auth/google/exchange", { code }, withCookies);
      return res.data;
    } catch (e) {
      throw e instanceof ApiError ? new GoogleExchangeError(e) : e;
    }
  },
  // ログイン済みの利用者が、Google アカウントを結び付ける手続きを、このブラウザで始める。応答の Set-Cookie で、
  // このブラウザに手続きの cookie が設定され(同一オリジンの要求なので、そのまま保存される)、Google の認可の画面の
  // URL(redirectUrl)が返る。画面は、その URL へ移動する(別のブラウザで開いても、cookie がないので失敗する)。
  // returnTo は、手続きのあとに戻る画面(アプリの中のパス。省略できる)。
  async startGoogleLink(returnTo?: string): Promise<{ redirectUrl: string }> {
    const res = await authApiClient.post<{ redirectUrl: string }>(
      "/me/identities/google/link",
      returnTo ? { returnTo } : undefined,
      withCookies,
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
