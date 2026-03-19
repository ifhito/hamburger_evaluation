import { useParams, Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { useUsers } from '../../shared/lib/hooks/useUsers'
import { useReviews } from '../../shared/lib/hooks/useReviews'
import { formatDate } from '../../shared/lib/date'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Layout } from '../../shared/ui/Layout'

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { user: authUser } = useAuth()
  const { data: users, isLoading: usersLoading, error: usersError } = useUsers()
  const { data: allReviews, isLoading: reviewsLoading } = useReviews()

  const userId = Number(id)
  const user = users?.find((u) => u.id === userId)
  // Client-side filter since no /users/:id/reviews endpoint exists yet
  const userReviews = allReviews?.filter((r) => r.user?.id === userId)
  const isOwner = authUser?.id === userId

  return (
    <Layout title={user ? `${user.username}'s Profile` : 'User Profile'}>
      {(usersLoading || reviewsLoading) && <p style={{ color: 'var(--color-text-muted)' }}>Loading…</p>}
      {usersError && <ErrorMessage message="Failed to load user." />}

      {user && (
        <div style={{ marginBottom: 32 }}>
          <div style={{ border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', padding: 24, background: '#fff', marginBottom: 16 }}>
            <h2 style={{ marginBottom: 8 }}>{user.username}</h2>
            <p style={{ color: 'var(--color-text-muted)', fontSize: '0.875rem' }}>{user.email}</p>
          </div>
          {isOwner && (
            <Link
              to={`/users/${userId}/edit`}
              style={{ fontSize: '0.875rem', padding: '6px 12px', background: 'var(--color-secondary-bg)', borderRadius: 'var(--radius)', fontWeight: 500 }}
            >
              Edit Profile
            </Link>
          )}
        </div>
      )}

      <h2 style={{ marginBottom: 16, fontSize: '1.1rem' }}>Reviews</h2>
      {userReviews && userReviews.length === 0 && (
        <p style={{ color: 'var(--color-text-muted)' }}>No reviews yet.</p>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {userReviews?.map((review) => (
          <div
            key={review.id}
            style={{ border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', padding: 16, background: '#fff' }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
              <span>{'★'.repeat(review.rating)}{'☆'.repeat(5 - review.rating)}</span>
              <span style={{ fontSize: '0.75rem', color: 'var(--color-text-muted)' }}>{formatDate(review.created_at)}</span>
            </div>
            <p style={{ marginBottom: 8, fontSize: '0.875rem' }}>{review.comment}</p>
            <Link to={`/reviews/${review.id}`} style={{ fontSize: '0.875rem' }}>View →</Link>
          </div>
        ))}
      </div>
    </Layout>
  )
}
