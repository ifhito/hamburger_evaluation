import { useEffect, useState } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import { ApiRequestError } from '../../shared/lib/api'
import { useReview } from '../../shared/lib/hooks/useReview'
import { useReviewForm } from '../../shared/lib/hooks/useReviewForm'
import { useUpdateReview } from '../../shared/lib/hooks/useReviewMutations'
import { Button } from '../../shared/ui/Button'
import { ErrorMessage } from '../../shared/ui/ErrorMessage'
import { Layout } from '../../shared/ui/Layout'
import { RatingSelect } from '../../shared/ui/RatingSelect'
import { Textarea } from '../../shared/ui/Textarea'

export default function ReviewEditPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: review, isLoading } = useReview(Number(id))
  const updateReview = useUpdateReview(Number(id))
  const { fields, errors, setField, validate } = useReviewForm()
  const [initialized, setInitialized] = useState(false)
  const [serverError, setServerError] = useState<string | string[] | null>(null)

  useEffect(() => {
    if (review && !initialized) {
      setField('rating', review.rating)
      setField('comment', review.comment)
      setInitialized(true)
    }
  }, [review, initialized, setField])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate(['comment'])) return
    setServerError(null)
    try {
      await updateReview.mutateAsync({ rating: fields.rating, comment: fields.comment })
      void navigate(`/reviews/${id}`)
    } catch (err) {
      if (err instanceof ApiRequestError) {
        setServerError(err.apiError.errors ?? err.apiError.error ?? 'Failed to update review')
      } else {
        setServerError('An unexpected error occurred')
      }
    }
  }

  return (
    <Layout title="Edit Review">
      {isLoading && <p style={{ color: 'var(--color-text-muted)' }}>Loading…</p>}
      {!isLoading && (
        <form onSubmit={(e) => void handleSubmit(e)} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 500 }}>
          {serverError && <ErrorMessage message={serverError} />}
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
          />
          <div style={{ display: 'flex', gap: 12 }}>
            <Button type="submit" isLoading={updateReview.isPending}>Save Changes</Button>
            <Link to={`/reviews/${id}`}>
              <Button type="button" variant="secondary">Cancel</Button>
            </Link>
          </div>
        </form>
      )}
    </Layout>
  )
}
