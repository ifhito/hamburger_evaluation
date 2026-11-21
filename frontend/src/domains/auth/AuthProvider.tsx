import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useAtom } from "jotai";
import { useSWRConfig } from "swr";
import { authUserAtom, authTokenAtom } from "../../states/authAtom";
import { authApi } from "./api/authApiClient";
import { ApiError, onUnauthorized } from "../../api/client/buildApiClient";
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
  /** すでに受け取ったログインの結果(Google でのサインインなど)で、ログイン状態にする。 */
  signInWithResponse(res: AuthUserResponse): void;
  logout(): Promise<void>;
  /** API を呼ばずにローカルの認証状態とキャッシュだけを消す(退会直後など、サーバー側で既に失効しているとき)。 */
  clearSession(): void;
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
  const { cache, mutate } = useSWRConfig();
  // 保存済みのトークンがあるときだけ、backend に問い合わせて復元する(その間は isLoading)。
  const [isLoading, setIsLoading] = useState(() => getToken() !== null);

  const clearSwrCache = useCallback(() => {
    // 公開の固定メタ情報(/meta)だけ残し、閲覧者や認証状態に依存しうるキャッシュを消す。
    // 先に全対象キーを Map から消してから mutate(key) を呼ぶことで、未マウントのキャッシュは消えたまま保ちつつ、
    // 画面に表示中でキーが変わらない購読者(ランキングなど)には再取得を走らせる。
    const entries: Array<[string, unknown]> = [];
    for (const key of Array.from(cache.keys())) {
      if (key !== "/meta") {
        entries.push([key, cache.get(key)]);
        cache.delete(key);
      }
    }
    for (const [key, prev] of entries) {
      void mutate(key);
      const current = cache.get(key) as Record<string, unknown> | undefined;
      if (
        current &&
        prev &&
        typeof prev === "object" &&
        "_k" in (prev as Record<string, unknown>) &&
        !("_k" in current)
      ) {
        cache.set(key, { ...current, _k: (prev as Record<string, unknown>)._k } as never);
      }
    }
  }, [cache, mutate]);

  const clearSession = useCallback(() => {
    removeToken();
    setTokenAtom(null);
    setUser(null);
    setIsLoading(false);
    clearSwrCache();
  }, [clearSwrCache, setTokenAtom, setUser]);

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
        if (!cancelled && getToken() === storedToken && e instanceof ApiError && e.status === 401) {
          removeToken();
          setTokenAtom(null);
          setUser(null);
        }
      })
      .finally(() => {
        // 復元中に別タブで新しいトークンへ切り替わっていたときは、そちらの問い合わせが終わるまで isLoading を保つ。
        if (!cancelled && (getToken() === storedToken || getToken() === null)) {
          setIsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [setUser, setTokenAtom]);

  // セッション途中でトークンが期限切れ・失効(401)になったとき、ログイン表示のまま残らないよう状態を消す。
  useEffect(() => {
    return onUnauthorized((expiredToken) => {
      if (getToken() === expiredToken) {
        clearSession();
      }
    });
  }, [clearSession]);

  // 別のタブでサインアウト・アカウント切り替えが起きたとき、このタブの認証状態とキャッシュも揃える。
  useEffect(() => {
    let cancelled = false;
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== null && event.key !== "token") return;
      const nextToken = getToken();
      if (!nextToken) {
        setTokenAtom(null);
        setUser(null);
        setIsLoading(false);
        clearSwrCache();
        return;
      }
      // 新しいトークンの持ち主が確定するまでは前の user を残さない(前の利用者の表示で新しいトークンを送る不整合を防ぐ)。
      setTokenAtom(null);
      setUser(null);
      setIsLoading(true);
      clearSwrCache();
      authApi
        .me()
        .then((me) => {
          if (cancelled || getToken() !== nextToken) return;
          setTokenAtom(nextToken);
          setUser(toAuthUser(me));
        })
        .catch((e: unknown) => {
          if (!cancelled && getToken() === nextToken && e instanceof ApiError && e.status === 401) {
            removeToken();
            setTokenAtom(null);
            setUser(null);
          }
        })
        .finally(() => {
          if (!cancelled && (getToken() === nextToken || getToken() === null)) {
            setIsLoading(false);
          }
        });
    };
    window.addEventListener("storage", handleStorage);
    return () => {
      cancelled = true;
      window.removeEventListener("storage", handleStorage);
    };
  }, [clearSwrCache, setTokenAtom, setUser]);

  // ログインと、signup の確認は、同じ本文(user と token)で認証状態にする。
  const applyAuthResponse = useCallback(
    (res: AuthUserResponse) => {
      setToken(res.token);
      setTokenAtom(res.token);
      setUser(toAuthUser(res));
      setIsLoading(false);
      clearSwrCache();
    },
    [clearSwrCache, setUser, setTokenAtom]
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
    if (getToken() !== null) {
      try {
        await authApi.logout();
      } catch {
        // エラーは無視し、いずれにせよローカルの状態をクリアする
      }
    }
    clearSession();
  }, [clearSession]);

  const refreshUser = useCallback(
    (updated: Pick<AuthUser, "username" | "email">) => {
      setUser((current) => (current ? { ...current, ...updated } : current));
    },
    [setUser]
  );

  const contextValue = useMemo(
    () => ({
      user,
      token,
      isLoading,
      login,
      signup,
      confirmSignup,
      signInWithResponse: applyAuthResponse,
      logout,
      clearSession,
      refreshUser,
    }),
    [user, token, isLoading, login, signup, confirmSignup, applyAuthResponse, logout, clearSession, refreshUser]
  );

  return (
    <AuthContext.Provider value={contextValue}>
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
