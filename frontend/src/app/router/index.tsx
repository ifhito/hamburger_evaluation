import { createBrowserRouter, Navigate } from 'react-router-dom'
import { ProtectedRoute } from './ProtectedRoute'
import { GuestRoute } from './GuestRoute'

import ShopListPage from '../../pages/shop-list'
import ShopDetailPage from '../../pages/shop-detail'
import ReviewListPage from '../../pages/review-list'
import ReviewDetailPage from '../../pages/review-detail'
import ReviewNewPage from '../../pages/review-new'
import ReviewEditPage from '../../pages/review-edit'
import SignupPage from '../../pages/signup'
import SigninPage from '../../pages/signin'
import SignoutPage from '../../pages/signout'
import UserDetailPage from '../../pages/user-detail'
import UserUpdatePage from '../../pages/user-update'

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
