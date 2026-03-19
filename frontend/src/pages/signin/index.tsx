import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { ApiRequestError } from '../../shared/lib/api'
import { useLoginForm } from '../../shared/lib/hooks/useAuthForm'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Input } from '../../shared/ui/Input'
import { Layout } from '../../shared/ui/Layout'

export default function SigninPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const { fields, errors, setField, validate, toRequest } = useLoginForm()
  const [serverError, setServerError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate()) return
    setIsLoading(true)
    setServerError(null)
    try {
      await login(toRequest())
      void navigate('/reviews')
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setServerError(err.apiError.error ?? 'Login failed')
      } else {
        setServerError('An unexpected error occurred')
      }
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <Layout title="Sign In">
      <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 400 }}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="email"
          label="Email"
          type="email"
          value={fields.email}
          onChange={(e) => setField('email', e.target.value)}
          error={errors.email}
          autoComplete="email"
        />
        <Input
          id="password"
          label="Password"
          type="password"
          value={fields.password}
          onChange={(e) => setField('password', e.target.value)}
          error={errors.password}
          autoComplete="current-password"
        />
        <Button type="submit" isLoading={isLoading}>Sign in</Button>
        <p style={{ fontSize: '0.875rem', color: 'var(--color-text-muted)' }}>
          Don't have an account? <a href="/signup">Sign up</a>
        </p>
      </form>
    </Layout>
  )
}
