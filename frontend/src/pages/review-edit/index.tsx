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
  const [photo, setPhoto] = useState<File | null>(null)

  useEffect(() => {
    if (review && !initialized) {
      setField('rating', review.rating)
      setField('comment', review.comment ?? '')
      setInitialized(true)
    }
  }, [review, initialized, setField])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!validate(['comment'])) return
    setServerError(null)
    try {
      let input: FormData | { rating: number; comment: string }
      if (photo) {
        const formData = new FormData()
        formData.append('rating', String(fields.rating))
        formData.append('comment', fields.comment)
        formData.append('photo', photo)
        input = formData
      } else {
        input = { rating: fields.rating, comment: fields.comment }
      }
      await updateReview.mutateAsync(input)
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
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <label htmlFor="photo" style={{ fontSize: '0.875rem', fontWeight: 500 }}>
              {review?.photo_url ? 'Replace Photo (optional)' : 'Photo (optional)'}
            </label>
            {review?.photo_url && (
              <img
                src={review.photo_url}
                alt="Current review photo"
                style={{ maxWidth: '100%', maxHeight: 200, objectFit: 'contain', alignSelf: 'flex-start', borderRadius: 'var(--radius)' }}
              />
            )}
            <input
              id="photo"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={(e) => setPhoto(e.target.files?.[0] ?? null)}
            />
          </div>
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
