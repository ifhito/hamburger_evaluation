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
import {
  getToken,
  setToken,
  removeToken,
  getStoredUser,
  setStoredUser,
  removeStoredUser,
} from "./storage";
import type { AuthUser, AuthUserResponse, LoginRequest, SignupRequest } from "./types";

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
  refreshUser(user: AuthUser): void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function decodeJwtPayload(token: string): Record<string, unknown> {
  try {
    const base64 = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(atob(base64)) as Record<string, unknown>;
  } catch {
    return {};
  }
}

function isTokenExpired(token: string): boolean {
  const payload = decodeJwtPayload(token);
  if (typeof payload.exp !== "number") return true;
  return Date.now() / 1000 > payload.exp;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useAtom(authUserAtom);
  const [token, setTokenAtom] = useAtom(authTokenAtom);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const storedToken = getToken();
    if (storedToken && !isTokenExpired(storedToken)) {
      const storedUser = getStoredUser() as AuthUser | null;
      if (storedUser) {
        // 古い保存ユーザーには admin がないことがあるため、必ず boolean にそろえる。
        setUser({ ...storedUser, admin: Boolean(storedUser.admin) });
        setTokenAtom(storedToken);
      } else {
        removeToken();
      }
    } else if (storedToken) {
      removeToken();
      removeStoredUser();
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setIsLoading(false);
  }, [setUser, setTokenAtom]);

  // ログインと、signup の確認は、同じ本文(user と token)で認証状態にする。
  const applyAuthResponse = useCallback(
    (res: AuthUserResponse) => {
      setToken(res.token);
      setTokenAtom(res.token);
      const authUser: AuthUser = { id: res.id, username: res.username, email: res.email, admin: res.admin };
      setUser(authUser);
      setStoredUser(authUser);
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
    removeStoredUser();
    setTokenAtom(null);
    setUser(null);
  }, [setUser, setTokenAtom]);

  const refreshUser = useCallback(
    (updatedUser: AuthUser) => {
      setUser(updatedUser);
      setStoredUser(updatedUser);
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
