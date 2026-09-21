import { createBrowserRouter, Navigate } from 'react-router-dom'
import { ProtectedRoute } from './ProtectedRoute'
import { GuestRoute } from './GuestRoute'
import { AdminRoute } from './AdminRoute'

import ShopListPage from '../../domains/shops/pages/ShopListPage'
import ShopDetailPage from '../../domains/shops/pages/ShopDetailPage'
import ShopNewPage from '../../domains/shops/pages/ShopNewPage'
import AdminShopListPage from '../../domains/shops/pages/admin/AdminShopListPage'
import AdminShopEditPage from '../../domains/shops/pages/admin/AdminShopEditPage'
import ReviewListPage from '../../domains/reviews/pages/ReviewListPage'
import ReviewDetailPage from '../../domains/reviews/pages/ReviewDetailPage'
import ReviewNewPage from '../../domains/reviews/pages/ReviewNewPage'
import ReviewEditPage from '../../domains/reviews/pages/ReviewEditPage'
import SignupPage from '../../domains/auth/pages/SignupPage'
import SignupConfirmPage from '../../domains/auth/pages/SignupConfirmPage'
import SigninPage from '../../domains/auth/pages/SigninPage'
import SignoutPage from '../../domains/auth/pages/SignoutPage'
import UserDetailPage from '../../domains/users/pages/UserDetailPage'
import UserUpdatePage from '../../domains/users/pages/UserUpdatePage'

export const router = createBrowserRouter([
  { path: '/', element: <Navigate to="/shops" replace /> },

  // 公開ルート
  { path: '/shops', element: <ShopListPage /> },
  { path: '/shops/:id', element: <ShopDetailPage /> },
  { path: '/reviews', element: <ReviewListPage /> },
  { path: '/reviews/:id', element: <ReviewDetailPage /> },
  { path: '/users/:id', element: <UserDetailPage /> },
  // 確認メールのリンクの受け皿(ログイン中でも開ける。成功すると確認したアカウントのログイン状態になる)
  { path: '/signup/confirm', element: <SignupConfirmPage /> },

  // ゲスト専用ルート
  {
    element: <GuestRoute />,
    children: [
      { path: '/signup', element: <SignupPage /> },
      { path: '/signin', element: <SigninPage /> },
    ],
  },

  // 保護されたルート
  {
    element: <ProtectedRoute />,
    children: [
      { path: '/shops/new', element: <ShopNewPage /> },
      { path: '/reviews/new', element: <ReviewNewPage /> },
      { path: '/reviews/:id/edit', element: <ReviewEditPage /> },
      { path: '/signout', element: <SignoutPage /> },
      { path: '/users/:id/edit', element: <UserUpdatePage /> },
    ],
  },

  // Admin-only routes
  {
    element: <AdminRoute />,
    children: [
      { path: '/admin/shops', element: <AdminShopListPage /> },
      { path: '/admin/shops/:id/edit', element: <AdminShopEditPage /> },
    ],
  },
])
