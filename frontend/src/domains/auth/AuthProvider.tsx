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
import { userApiClient } from "../users/api/userApiClient";
import type { AuthUser, LoginRequest, SignupRequest } from "./types";

interface AuthContextValue {
  user: AuthUser | null;
  token: string | null;
  isLoading: boolean;
  login(data: LoginRequest): Promise<void>;
  signup(data: SignupRequest): Promise<void>;
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

async function fetchUserById(userId: number): Promise<AuthUser | null> {
  try {
    const res = await userApiClient.get<AuthUser[]>("/users");
    return res.data.find((u) => u.id === userId) ?? null;
  } catch {
    return null;
  }
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
        setUser(storedUser);
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

  const signup = useCallback(
    async (data: SignupRequest) => {
      const res = await authApi.signup(data);
      setToken(res.token);
      setTokenAtom(res.token);
      const authUser: AuthUser = { id: res.id, username: res.username, email: res.email };
      setUser(authUser);
      setStoredUser(authUser);
    },
    [setUser, setTokenAtom]
  );

  const login = useCallback(
    async (data: LoginRequest) => {
      const res = await authApi.login(data);
      setToken(res.token);
      setTokenAtom(res.token);
      const payload = decodeJwtPayload(res.token);
      const userId = payload.user_id as number;
      const fetchedUser = await fetchUserById(userId);
      setUser(fetchedUser);
      if (fetchedUser) setStoredUser(fetchedUser);
    },
    [setUser, setTokenAtom]
  );

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch {
      // clear local state regardless
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
    <AuthContext.Provider value={{ user, token, isLoading, login, signup, logout, refreshUser }}>
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
