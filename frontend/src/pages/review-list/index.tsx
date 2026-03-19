import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { useReviews } from '../../shared/lib/hooks/useReviews'
import { formatDate } from '../../shared/lib/date'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Layout } from '../../shared/ui/Layout'

export default function ReviewListPage() {
  const { user } = useAuth()
  const [keyword, setKeyword] = useState('')
  const [ratingFilter, setRatingFilter] = useState<number | undefined>(undefined)

  const { data: reviews, isLoading, error } = useReviews(
    ratingFilter !== undefined || keyword ? { rating: ratingFilter, keyword: keyword || undefined } : undefined,
  )

  return (
    <Layout title="Reviews">
      <div style={{ display: 'flex', gap: 12, marginBottom: 24, flexWrap: 'wrap', alignItems: 'center' }}>
        <input
          placeholder="Search by keyword…"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          style={{ padding: '8px 12px', border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', fontSize: '0.875rem', flex: 1, minWidth: 180 }}
        />
        <select
          value={ratingFilter ?? ''}
          onChange={(e) => setRatingFilter(e.target.value ? Number(e.target.value) : undefined)}
          style={{ padding: '8px 12px', border: '1px solid var(--color-border)', borderRadius: 'var(--radius)', fontSize: '0.875rem' }}
        >
          <option value="">All ratings</option>
          {[5, 4, 3, 2, 1].map((r) => (
            <option key={r} value={r}>{'★'.repeat(r)}</option>
          ))}
        </select>
        {user && (
          <Link
            to="/reviews/new"
            style={{
              background: 'var(--color-primary)',
              color: '#fff',
              padding: '8px 16px',
              borderRadius: 'var(--radius)',
              fontWeight: 500,
              fontSize: '0.875rem',
            }}
          >
            + New Review
          </Link>
        )}
      </div>

      {error && <ErrorMessage message="Failed to load reviews." />}

      {isLoading && <p style={{ color: 'var(--color-text-muted)' }}>Loading…</p>}

      {reviews && reviews.length === 0 && (
        <p style={{ color: 'var(--color-text-muted)' }}>No reviews yet.</p>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {reviews?.map((review) => (
          <div
            key={review.id}
            style={{
              border: '1px solid var(--color-border)',
              borderRadius: 'var(--radius)',
              padding: 16,
              background: '#fff',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
              <span style={{ fontSize: '1.1rem' }}>{'★'.repeat(review.rating)}{'☆'.repeat(5 - review.rating)}</span>
              <span style={{ fontSize: '0.75rem', color: 'var(--color-text-muted)' }}>{formatDate(review.created_at)}</span>
            </div>
            <p style={{ marginBottom: 8 }}>{review.comment}</p>
            <div style={{ fontSize: '0.75rem', color: 'var(--color-text-muted)', display: 'flex', gap: 12 }}>
              {review.user && (
                <Link to={`/users/${review.user.id}`}>{review.user.username}</Link>
              )}
              {review.burger && (
                <span>Burger #{review.burger.id} · avg {review.burger.average_rating.toFixed(1)} · {review.burger.review_count} reviews</span>
              )}
            </div>
            <div style={{ marginTop: 8 }}>
              <Link to={`/reviews/${review.id}`} style={{ fontSize: '0.875rem' }}>View →</Link>
            </div>
          </div>
        ))}
      </div>
    </Layout>
  )
}
