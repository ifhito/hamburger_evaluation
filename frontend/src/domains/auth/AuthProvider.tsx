import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useAtom } from "jotai";
import { authUserAtom, authTokenAtom } from "../../states/authAtom";
import { authApi } from "./api/authApiClient";
import { ApiError } from "../../api/client/buildApiClient";
import { getToken, setToken, removeToken } from "./storage";
import type {
  AuthUser,
  AuthUserResponse,
  CurrentUserResponse,
  LoginRequest,
  SignupRequest,
} from "./types";

interface AuthContextValue {
  user: AuthUser | null;
  token: string | null;
  isLoading: boolean;
  login(data: LoginRequest): Promise<void>;
  /** 確認メールの送信を申し込む。アカウントは確認メールのリンクを開くまで作られず、ログインもしない。 */
  signup(data: SignupRequest): Promise<void>;
  /** 確認メールのリンクのトークンでアカウントを作成し、そのままログイン状態にする。 */
  confirmSignup(token: string): Promise<void>;
  logout(): Promise<void>;
  // プロフィールの更新後に、表示する名前・メールを差し替える(権限 canModerate は変わらない)。
  refreshUser(updated: Pick<AuthUser, "username" | "email">): void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function toAuthUser(res: CurrentUserResponse): AuthUser {
  return { id: res.id, username: res.username, email: res.email, canModerate: res.canModerate };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useAtom(authUserAtom);
  const [token, setTokenAtom] = useAtom(authTokenAtom);
  // 保存済みのトークンがあるときだけ、backend に問い合わせて復元する(その間は isLoading)。
  const [isLoading, setIsLoading] = useState(() => getToken() !== null);

  useEffect(() => {
    const storedToken = getToken();
    if (!storedToken) return;
    let cancelled = false;
    authApi
      .me()
      .then((me) => {
        // 復元の問い合わせ中に、確認メールのリンクなどで別のトークンに切り替わっていたら、古い結果は捨てる。
        if (cancelled || getToken() !== storedToken) return;
        setUser(toAuthUser(me));
        setTokenAtom(storedToken);
      })
      .catch((e: unknown) => {
        // 無効・期限切れ(401)のときだけ、トークンを捨てる。通信エラーなど一時的な失敗では、
        // トークンを消さない(ログアウト状態で表示するだけ。再読み込みで復元できる)。
        if (!cancelled && getToken() === storedToken && e instanceof ApiError && e.status === 401) removeToken();
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [setUser, setTokenAtom]);

  // ログインと、signup の確認は、同じ本文(user と token)で認証状態にする。
  const applyAuthResponse = useCallback(
    (res: AuthUserResponse) => {
      setToken(res.token);
      setTokenAtom(res.token);
      setUser(toAuthUser(res));
    },
    [setUser, setTokenAtom]
  );

  const signup = useCallback(async (data: SignupRequest) => {
    await authApi.signup(data);
  }, []);

  const confirmSignup = useCallback(
    async (confirmToken: string) => {
      applyAuthResponse(await authApi.confirmSignup(confirmToken));
    },
    [applyAuthResponse]
  );

  const login = useCallback(
    async (data: LoginRequest) => {
      applyAuthResponse(await authApi.login(data));
    },
    [applyAuthResponse]
  );

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch {
      // エラーは無視し、いずれにせよローカルの状態をクリアする
    }
    removeToken();
    setTokenAtom(null);
    setUser(null);
  }, [setUser, setTokenAtom]);

  const refreshUser = useCallback(
    (updated: Pick<AuthUser, "username" | "email">) => {
      setUser((current) => (current ? { ...current, ...updated } : current));
    },
    [setUser]
  );

  return (
    <AuthContext.Provider value={{ user, token, isLoading, login, signup, confirmSignup, logout, refreshUser }}>
      {children}
    </AuthContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
