import { Link, useParams } from 'react-router-dom'
import { useAuth } from '../../app/providers/AuthProvider'
import { useShopDetail } from '../../shared/lib/hooks/useShops'
import { Button } from '../../shared/ui/Button'
import { Layout } from '../../shared/ui/Layout'

export default function ShopDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { user } = useAuth()
  const { data: shop, isLoading, isError } = useShopDetail(Number(id))

  if (isLoading) return <Layout><p>Loading…</p></Layout>
  if (isError || !shop) return <Layout><p>Shop not found.</p></Layout>

  return (
    <Layout title={shop.name}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
        {user && (
          <div>
            <Link to={`/reviews/new?shop_id=${shop.id}`}>
              <Button type="button">Write a Review</Button>
            </Link>
          </div>
        )}
        <section>
          <h2 style={{ fontSize: '1.1rem', marginBottom: 12 }}>Reviews</h2>
          {shop.reviews.length === 0 && <p style={{ color: '#666' }}>No reviews yet.</p>}
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {shop.reviews.map((review) => (
              <li
                key={review.id}
                style={{ border: '1px solid #ddd', borderRadius: 6, padding: '12px 16px' }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                  <strong>{review.burger?.name ?? '—'}</strong>
                  <span>{'★'.repeat(review.rating)}{'☆'.repeat(5 - review.rating)}</span>
                </div>
                <p style={{ margin: '4px 0', color: '#444' }}>{review.comment}</p>
                <small style={{ color: '#888' }}>
                  by {review.user?.username ?? 'unknown'}
                </small>
              </li>
            ))}
          </ul>
        </section>
        <Link to="/shops" style={{ color: '#666', fontSize: '0.875rem' }}>← Back to shops</Link>
      </div>
    </Layout>
  )
}
