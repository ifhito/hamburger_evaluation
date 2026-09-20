import { useParams, Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { useUser } from '../../shared/lib/hooks/useUser'
import { useReviews } from '../../shared/lib/hooks/useReviews'
import { formatDate } from '../../shared/lib/date'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Layout } from '../../shared/ui/Layout'

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { user: authUser, isLoading: authLoading } = useAuth()

  const userId = Number(id)
  // /users/abc（NaN）など不正な id では user も reviews も取得しない
  const isValidId = Number.isInteger(userId)
  // 認証状態の復元前は authUser が null でも token は localStorage にあり得る。閲覧者が確定してから取得する
  const { data: user, isLoading: userLoading, error: userError } = useUser(userId, authUser?.id ?? null, {
    enabled: !authLoading,
  })
  // user_id を省略すると全件フィードになり他人のレビューをこのユーザーのものとして表示してしまうため、不正 id では取得自体を止める
  const {
    data: reviews,
    isLoading: reviewsLoading,
    error: reviewsError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useReviews({ user_id: userId }, { enabled: isValidId })
  const isOwner = authUser?.id === userId

  return (
    <Layout title={user ? `${user.username}'s Profile` : 'User Profile'}>
      {(userLoading || reviewsLoading) && <p style={{ color: 'var(--color-text-muted)' }}>Loading…</p>}
      {(!isValidId || userError) && <ErrorMessage message="Failed to load user." />}

      {user && (
        <div style={{ marginBottom: 32 }}>
          <div style={{ border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', padding: 24, background: '#fff', marginBottom: 16 }}>
            <h2 style={{ marginBottom: 8 }}>{user.username}</h2>
            {/* email は API が本人の閲覧時だけ返す。isOwner ではなく API の返却有無で出し分ける */}
            {user.email && <p style={{ color: 'var(--color-text-muted)', fontSize: '0.875rem' }}>{user.email}</p>}
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
      {reviewsError && <ErrorMessage message="Failed to load reviews." />}
      {reviews && reviews.length === 0 && (
        <p style={{ color: 'var(--color-text-muted)' }}>No reviews yet.</p>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {reviews?.map((review) => (
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
      {hasNextPage && (
        <div style={{ marginTop: 16, textAlign: 'center' }}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={() => void fetchNextPage()}>
            Load more
          </Button>
        </div>
      )}
    </Layout>
  )
}
