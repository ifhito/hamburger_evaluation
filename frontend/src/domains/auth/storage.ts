export const getToken = (): string | null => localStorage.getItem("token");
export const setToken = (token: string): void =>
  localStorage.setItem("token", token);
export const removeToken = (): void => localStorage.removeItem("token");

export const getStoredUser = (): unknown => {
  const raw = localStorage.getItem("auth_user");
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
};
export const setStoredUser = (user: unknown): void =>
  localStorage.setItem("auth_user", JSON.stringify(user));
export const removeStoredUser = (): void => localStorage.removeItem("auth_user");
