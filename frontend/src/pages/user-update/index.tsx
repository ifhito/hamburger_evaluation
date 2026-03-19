import { useState } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { ApiRequestError } from '../../shared/lib/api'
import { useUpdateUser, useDeleteUser } from '../../shared/lib/hooks/useUserMutations'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Input } from '../../shared/ui/Input'
import { Layout } from '../../shared/ui/Layout'

export default function UserUpdatePage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user: authUser, logout, refreshUser } = useAuth()
  const updateUser = useUpdateUser(Number(id))
  const deleteUser = useDeleteUser()

  const [username, setUsername] = useState(authUser?.username ?? '')
  const [email, setEmail] = useState(authUser?.email ?? '')
  const [password, setPassword] = useState('')
  const [passwordConfirmation, setPasswordConfirmation] = useState('')
  const [serverError, setServerError] = useState<string | string[] | null>(null)

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault()
    setServerError(null)
    const data: Record<string, string> = {}
    if (username !== authUser?.username) data.username = username
    if (email !== authUser?.email) data.email = email
    if (password) {
      data.password = password
      data.password_confirmation = passwordConfirmation
    }
    if (Object.keys(data).length === 0) {
      void navigate(`/users/${id}`)
      return
    }
    try {
      const updated = await updateUser.mutateAsync(data)
      refreshUser({ id: updated.id, username: updated.username, email: updated.email })
      void navigate(`/users/${id}`)
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setServerError(err.apiError.errors ?? err.apiError.error ?? 'Update failed')
      } else {
        setServerError('An unexpected error occurred')
      }
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete your account? This cannot be undone.')) return
    try {
      await deleteUser.mutateAsync(Number(id))
      await logout()
      void navigate('/reviews')
    } catch {
      setServerError('Failed to delete account')
    }
  }

  return (
    <Layout title="Edit Profile">
      <form onSubmit={(e) => void handleUpdate(e)} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 400 }}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="username"
          label="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
        />
        <Input
          id="email"
          label="Email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
        />
        <Input
          id="password"
          label="New Password (leave blank to keep current)"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
        />
        <Input
          id="password_confirmation"
          label="Confirm New Password"
          type="password"
          value={passwordConfirmation}
          onChange={(e) => setPasswordConfirmation(e.target.value)}
          autoComplete="new-password"
        />
        <div style={{ display: 'flex', gap: 12 }}>
          <Button type="submit" isLoading={updateUser.isPending}>Save Changes</Button>
          <Link to={`/users/${id}`}>
            <Button type="button" variant="secondary">Cancel</Button>
          </Link>
        </div>
      </form>

      <hr style={{ margin: '32px 0', borderColor: 'var(--color-border)' }} />
      <div>
        <h3 style={{ marginBottom: 12, color: 'var(--color-danger)', fontSize: '1rem' }}>Danger Zone</h3>
        <Button variant="danger" isLoading={deleteUser.isPending} onClick={() => void handleDelete()}>
          Delete Account
        </Button>
      </div>
    </Layout>
  )
}
