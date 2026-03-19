import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Input } from '../../shared/ui/Input'
import { Layout } from '../../shared/ui/Layout'
import { useSignupForm } from '../../shared/lib/hooks/useAuthForm'
import { ApiRequestError } from '../../shared/lib/api'

export default function SignupPage() {
  const { signup } = useAuth()
  const navigate = useNavigate()
  const { fields, errors, setField, validate, toRequest } = useSignupForm()
  const [serverError, setServerError] = useState<string | string[] | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate()) return
    setIsLoading(true)
    setServerError(null)
    try {
      await signup(toRequest())
      void navigate('/reviews')
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setServerError(err.apiError.errors ?? err.apiError.error ?? 'Signup failed')
      } else {
        setServerError('An unexpected error occurred')
      }
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <Layout title="Sign Up">
      <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 400 }}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="username"
          label="Username"
          value={fields.username}
          onChange={(e) => setField('username', e.target.value)}
          error={errors.username}
          autoComplete="username"
        />
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
          autoComplete="new-password"
        />
        <Input
          id="password_confirmation"
          label="Confirm Password"
          type="password"
          value={fields.password_confirmation}
          onChange={(e) => setField('password_confirmation', e.target.value)}
          error={errors.password_confirmation}
          autoComplete="new-password"
        />
        <Button type="submit" isLoading={isLoading}>Create account</Button>
        <p style={{ fontSize: '0.875rem', color: 'var(--color-text-muted)' }}>
          Already have an account? <a href="/signin">Sign in</a>
        </p>
      </form>
    </Layout>
  )
}
