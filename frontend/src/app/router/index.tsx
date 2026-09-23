import { createBrowserRouter } from 'react-router-dom'
import { ProtectedRoute } from './ProtectedRoute'
import { GuestRoute } from './GuestRoute'
import { AdminRoute } from './AdminRoute'

import HomePage from '../../domains/home/pages/HomePage'
import ShopListPage from '../../domains/shops/pages/ShopListPage'
import ShopDetailPage from '../../domains/shops/pages/ShopDetailPage'
import ShopNewPage from '../../domains/shops/pages/ShopNewPage'
import AdminShopListPage from '../../domains/shops/pages/admin/AdminShopListPage'
import AdminShopEditPage from '../../domains/shops/pages/admin/AdminShopEditPage'
import ReviewListPage from '../../domains/reviews/pages/ReviewListPage'
import ReviewDetailPage from '../../domains/reviews/pages/ReviewDetailPage'
import ReviewNewPage from '../../domains/reviews/pages/ReviewNewPage'
import ReviewEditPage from '../../domains/reviews/pages/ReviewEditPage'
import BurgerListPage from '../../domains/burgers/pages/BurgerListPage'
import BurgerDetailPage from '../../domains/burgers/pages/BurgerDetailPage'
import SignupPage from '../../domains/auth/pages/SignupPage'
import SignupConfirmPage from '../../domains/auth/pages/SignupConfirmPage'
import GoogleCompletePage from '../../domains/auth/pages/GoogleCompletePage'
import SigninPage from '../../domains/auth/pages/SigninPage'
import SignoutPage from '../../domains/auth/pages/SignoutPage'
import UserDetailPage from '../../domains/users/pages/UserDetailPage'
import UserUpdatePage from '../../domains/users/pages/UserUpdatePage'
import OAuthConsentPage from '../../domains/oauth/pages/OAuthConsentPage'

export const router = createBrowserRouter([
  { path: '/', element: <HomePage /> },

  // 公開ルート
  { path: '/shops', element: <ShopListPage /> },
  { path: '/shops/:id', element: <ShopDetailPage /> },
  { path: '/reviews', element: <ReviewListPage /> },
  { path: '/reviews/:id', element: <ReviewDetailPage /> },
  { path: '/burgers', element: <BurgerListPage /> },
  { path: '/burgers/:id', element: <BurgerDetailPage /> },
  { path: '/users/:id', element: <UserDetailPage /> },
  // 確認メールのリンクの受け皿(ログイン中でも開ける。成功すると確認したアカウントのログイン状態になる)
  { path: '/signup/confirm', element: <SignupConfirmPage /> },
  // Google でのサインインの結果の受け皿(ログイン中でも開ける。成功するとサインインした状態になる)
  { path: '/auth/google/complete', element: <GoogleCompletePage /> },

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
      // AI アプリが、ログインと許可だけでつなぐための、許可を尋ねる画面(backend の認可の URL から渡される)
      { path: '/oauth/authorize', element: <OAuthConsentPage /> },
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
