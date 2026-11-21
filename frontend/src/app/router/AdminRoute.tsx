import { Navigate, Outlet, useLocation } from "react-router-dom";
import { useAuth } from "../../domains/auth/AuthProvider";
import type { ReturnToState } from "./returnTo";

export function AdminRoute() {
  const { user, isLoading } = useAuth();
  const location = useLocation();
  if (isLoading) return null;
  // サインインのあとに管理の画面へ戻せるよう、戻り先を遷移の state に入れる。
  const state: ReturnToState = { from: location.pathname + location.search };
  if (!user) return <Navigate to="/signin" replace state={state} />;
  // 管理画面に入れるかは、backend が返す canModerate で決める(権限の判断は backend だけが持つ)
  if (!user.canModerate) return <Navigate to="/shops" replace />;
  return <Outlet />;
}
