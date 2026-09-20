import { Provider as JotaiProvider } from 'jotai'
import { RouterProvider } from 'react-router-dom'
import { AuthProvider } from '../domains/auth/AuthProvider'
import { router } from './router'

export default function App() {
  return (
    <JotaiProvider>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </JotaiProvider>
  )
}
