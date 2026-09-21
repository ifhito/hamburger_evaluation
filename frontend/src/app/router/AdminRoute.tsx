import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../../domains/auth/AuthProvider";

export function AdminRoute() {
  const { user, isLoading } = useAuth();
  if (isLoading) return null;
  if (!user) return <Navigate to="/signin" replace />;
  // 管理画面に入れるかは、backend が返す canModerate で決める(権限の判断は backend だけが持つ)
  if (!user.canModerate) return <Navigate to="/shops" replace />;
  return <Outlet />;
}
