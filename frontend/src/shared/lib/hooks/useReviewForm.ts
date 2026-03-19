import { useCallback, useState } from 'react'
import type { ReviewCreateInput } from '../types/review'

interface ReviewFormState {
  rating: number
  comment: string
  shop_id: number
  burger_name: string
}

interface ReviewFormErrors {
  rating?: string
  comment?: string
  shop_id?: string
  burger_name?: string
}

export function useReviewForm(initial?: Partial<ReviewFormState>) {
  const [fields, setFields] = useState<ReviewFormState>({
    rating: initial?.rating ?? 3,
    comment: initial?.comment ?? '',
    shop_id: initial?.shop_id ?? 0,
    burger_name: initial?.burger_name ?? '',
  })
  const [errors, setErrors] = useState<ReviewFormErrors>({})

  const setField = useCallback(<K extends keyof ReviewFormState>(key: K, value: ReviewFormState[K]) => {
    setFields((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => ({ ...prev, [key]: undefined }))
  }, [])

  const validate = (required: (keyof ReviewFormState)[] = []): boolean => {
    const newErrors: ReviewFormErrors = {}
    if (!fields.comment.trim()) newErrors.comment = 'Comment is required'
    if (fields.rating < 1 || fields.rating > 5) newErrors.rating = 'Rating must be between 1 and 5'
    if (required.includes('shop_id') && !fields.shop_id) {
      newErrors.shop_id = 'Shop is required'
    }
    if (required.includes('burger_name') && !fields.burger_name.trim()) {
      newErrors.burger_name = 'Burger name is required'
    }
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const toCreateInput = (): ReviewCreateInput => ({
    rating: fields.rating,
    comment: fields.comment,
    shop_id: fields.shop_id,
    burger_name: fields.burger_name,
  })

  return { fields, errors, setField, validate, toCreateInput }
}
