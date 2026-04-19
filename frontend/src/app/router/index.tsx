import { createBrowserRouter, Navigate } from 'react-router-dom'
import { ProtectedRoute } from './ProtectedRoute'
import { GuestRoute } from './GuestRoute'

import ShopListPage from '../../domains/shops/pages/ShopListPage'
import ShopDetailPage from '../../domains/shops/pages/ShopDetailPage'
import ReviewListPage from '../../domains/reviews/pages/ReviewListPage'
import ReviewDetailPage from '../../domains/reviews/pages/ReviewDetailPage'
import ReviewNewPage from '../../domains/reviews/pages/ReviewNewPage'
import ReviewEditPage from '../../domains/reviews/pages/ReviewEditPage'
import SignupPage from '../../domains/auth/pages/SignupPage'
import SigninPage from '../../domains/auth/pages/SigninPage'
import SignoutPage from '../../domains/auth/pages/SignoutPage'
import UserDetailPage from '../../domains/users/pages/UserDetailPage'
import UserUpdatePage from '../../domains/users/pages/UserUpdatePage'

export const router = createBrowserRouter([
  { path: '/', element: <Navigate to="/shops" replace /> },

  // Public routes
  { path: '/shops', element: <ShopListPage /> },
  { path: '/shops/:id', element: <ShopDetailPage /> },
  { path: '/reviews', element: <ReviewListPage /> },
  { path: '/reviews/:id', element: <ReviewDetailPage /> },
  { path: '/users/:id', element: <UserDetailPage /> },

  // Guest-only routes
  {
    element: <GuestRoute />,
    children: [
      { path: '/signup', element: <SignupPage /> },
      { path: '/signin', element: <SigninPage /> },
    ],
  },

  // Protected routes
  {
    element: <ProtectedRoute />,
    children: [
      { path: '/reviews/new', element: <ReviewNewPage /> },
      { path: '/reviews/:id/edit', element: <ReviewEditPage /> },
      { path: '/signout', element: <SignoutPage /> },
      { path: '/users/:id/edit', element: <UserUpdatePage /> },
    ],
  },
])
