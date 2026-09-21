export const getToken = (): string | null => localStorage.getItem("token");
export const setToken = (token: string): void =>
  localStorage.setItem("token", token);
// 以前は、ログイン中のユーザーも保存していた(auth_user)。ユーザーは、起動時に GET /me で
// backend から取り直すので、もう保存しない。残っている古い値は、トークンを消すときに一緒に消す。
export const removeToken = (): void => {
  localStorage.removeItem("token");
  localStorage.removeItem("auth_user");
};
