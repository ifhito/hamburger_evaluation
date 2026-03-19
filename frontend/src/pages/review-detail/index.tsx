import { useNavigate, useParams, Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { useReview } from '../../shared/lib/hooks/useReview'
import { useDeleteReview } from '../../shared/lib/hooks/useReviewMutations'
import { formatDate } from '../../shared/lib/date'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Layout } from '../../shared/ui/Layout'

export default function ReviewDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user } = useAuth()
  const { data: review, isLoading, error } = useReview(Number(id))
  const deleteReview = useDeleteReview()

  const isOwner = user !== null && review?.user !== null && user.id === review?.user?.id

  const handleDelete = async () => {
    if (!confirm('Delete this review?')) return
    await deleteReview.mutateAsync(Number(id))
    void navigate('/reviews')
  }

  return (
    <Layout title="Review Detail">
      {isLoading && <p style={{ color: 'var(--color-text-muted)' }}>Loading…</p>}
      {error && <ErrorMessage message="Failed to load review." />}

      {review && (
        <div style={{ border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', padding: 24, background: '#fff' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
            <span style={{ fontSize: '1.5rem' }}>{'★'.repeat(review.rating)}{'☆'.repeat(5 - review.rating)}</span>
            <span style={{ color: 'var(--color-text-muted)', fontSize: '0.875rem' }}>{formatDate(review.created_at)}</span>
          </div>
          <p style={{ marginBottom: 16, lineHeight: 1.7 }}>{review.comment}</p>
          {review.burger && (
            <p style={{ fontSize: '0.875rem', color: 'var(--color-text-muted)', marginBottom: 8 }}>
              Burger #{review.burger.id} · Avg rating: {review.burger.average_rating.toFixed(1)} · {review.burger.review_count} reviews
            </p>
          )}
          {review.user && (
            <p style={{ fontSize: '0.875rem', color: 'var(--color-text-muted)' }}>
              By <Link to={`/users/${review.user.id}`}>{review.user.username}</Link>
            </p>
          )}
          {isOwner && (
            <div style={{ marginTop: 24, display: 'flex', gap: 12 }}>
              <Link
                to={`/reviews/${review.id}/edit`}
                style={{ padding: '8px 16px', background: 'var(--color-secondary-bg)', borderRadius: 'var(--radius)', fontSize: '0.875rem', fontWeight: 500 }}
              >
                Edit
              </Link>
              <Button variant="danger" isLoading={deleteReview.isPending} onClick={() => void handleDelete()}>
                Delete
              </Button>
            </div>
          )}
        </div>
      )}
      <div style={{ marginTop: 16 }}>
        <Link to="/reviews">← Back to reviews</Link>
      </div>
    </Layout>
  )
}
