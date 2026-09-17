import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "../../domains/auth/AuthProvider";

export function AdminRoute() {
  const { user, isLoading } = useAuth();
  if (isLoading) return null;
  if (!user) return <Navigate to="/signin" replace />;
  if (!user.admin) return <Navigate to="/shops" replace />;
  return <Outlet />;
}
