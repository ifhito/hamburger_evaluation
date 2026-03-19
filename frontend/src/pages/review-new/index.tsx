import { useState } from 'react'
import { Navigate, useNavigate, useSearchParams, Link } from 'react-router-dom'
import { ApiRequestError } from '../../shared/lib/api'
import { useReviewForm } from '../../shared/lib/hooks/useReviewForm'
import { useCreateReview } from '../../shared/lib/hooks/useReviewMutations'
import { useShops } from '../../shared/lib/hooks/useShops'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Input } from '../../shared/ui/Input'
import { Layout } from '../../shared/ui/Layout'
import { RatingSelect } from '../../shared/ui/RatingSelect'
import { Textarea } from '../../shared/ui/Textarea'

export default function ReviewNewPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const shopId = Number(searchParams.get('shop_id'))

  const { data: shops } = useShops()
  const shopName = shops?.find((s) => s.id === shopId)?.name

  const { fields, errors, setField, validate, toCreateInput } = useReviewForm({ shop_id: shopId })
  const createReview = useCreateReview()
  const [serverError, setServerError] = useState<string | string[] | null>(null)

  if (!shopId) return <Navigate to="/shops" replace />

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate(['comment', 'burger_name'])) return
    setServerError(null)
    try {
      const review = await createReview.mutateAsync(toCreateInput())
      void navigate(`/reviews/${review.id}`)
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setServerError(err.apiError.errors ?? err.apiError.error ?? 'Failed to create review')
      } else {
        setServerError('An unexpected error occurred')
      }
    }
  }

  return (
    <Layout title="New Review">
      <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 500 }}>
        {serverError && <ErrorMessage message={serverError} />}
        <div style={{ padding: '8px 12px', background: '#f5f5f5', borderRadius: 4, fontSize: '0.95rem' }}>
          <strong>Shop:</strong> {shopName ?? `#${shopId}`}
        </div>
        <RatingSelect
          value={fields.rating}
          onChange={(v) => setField('rating', v)}
          error={errors.rating}
        />
        <Textarea
          id="comment"
          label="Comment"
          value={fields.comment}
          onChange={(e) => setField('comment', e.target.value)}
          error={errors.comment}
          placeholder="Write your burger review…"
        />
        <Input
          id="burger_name"
          label="Burger Name"
          type="text"
          value={fields.burger_name}
          onChange={(e) => setField('burger_name', e.target.value)}
          error={errors.burger_name}
          placeholder="Enter burger name"
        />
        <div style={{ display: 'flex', gap: 12 }}>
          <Button type="submit" isLoading={createReview.isPending}>Submit Review</Button>
          <Link to={`/shops/${shopId}`}>
            <Button type="button" variant="secondary">Cancel</Button>
          </Link>
        </div>
      </form>
    </Layout>
  )
}
