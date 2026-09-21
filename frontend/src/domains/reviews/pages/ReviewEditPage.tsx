import { useEffect, useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useUpdateReview } from "../hooks/useReviewMutations";
import { useUpdateReviewForm } from "../hooks/useReviewForm";
import { useRatingRange } from "../hooks/useRatingRange";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import { RatingSelect } from "../../../components/RatingSelect";
import { Textarea } from "../../../components/Textarea";
import styles from "./reviewForm.module.css";

export default function ReviewEditPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user, isLoading: authLoading } = useAuth();
  const { data: review, isLoading: reviewLoading } = useReview(id, user?.id ?? null, {
    enabled: !authLoading,
  });
  const isLoading = reviewLoading || authLoading;
  const { update } = useUpdateReview(id ?? "");
  const ratingRange = useRatingRange();
  const textLimits = useMeta().data?.text;

  const { register, handleSubmit, setValue, watch, reset } = useUpdateReviewForm();

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [photo, setPhoto] = useState<File | null>(null);

  useEffect(() => {
    if (review) {
      reset({ rating: review.rating, comment: review.comment ?? "" });
    }
  }, [review, reset]);

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      await update(data, photo);
      void navigate(`/reviews/${id}`);
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("reviews.edit.error")]);
    } finally {
      setIsSubmitting(false);
    }
  });

  return (
    <Layout title={t("reviews.edit.title")}>
      {isLoading && <p className={styles.muted}>{t("reviews.edit.loading")}</p>}
      {/* 編集してよいか(canEdit)は backend が返す。他人のレビューでは、フォームを出さない */}
      {!isLoading && review && !review.canEdit && (
        <>
          <ErrorMessage message={t("reviews.edit.forbidden")} />
          <Link to={`/reviews/${id}`}>{t("reviews.edit.cancel")}</Link>
        </>
      )}
      {!isLoading && !(review && !review.canEdit) && (
        <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
          {serverError && <ErrorMessage message={serverError} />}
          {ratingRange && (
            <RatingSelect
              value={watch("rating")}
              onChange={(v) => setValue("rating", v)}
              min={ratingRange.min}
              max={ratingRange.max}
            />
          )}
          <Textarea
            id="comment"
            label={t("reviews.edit.comment")}
            counter={{ value: watch("comment"), max: textLimits?.reviewCommentMaxChars }}
            {...register("comment")}
          />
          <div className={styles.field}>
            <label htmlFor="photo" className={styles.fieldLabel}>
              {review?.photoUrl ? t("reviews.photo.replaceLabel") : t("reviews.photo.label")}
            </label>
            {review?.photoUrl && (
              <img
                src={review.photoUrl}
                alt={t("reviews.photo.currentAlt")}
                className={styles.currentPhoto}
              />
            )}
            <input
              id="photo"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={(e) => setPhoto(e.target.files?.[0] ?? null)}
            />
          </div>
          <div className={styles.actions}>
            <Button type="submit" isLoading={isSubmitting}>
              {t("reviews.edit.submit")}
            </Button>
            <Link to={`/reviews/${id}`}>
              <Button type="button" variant="secondary">
                {t("reviews.edit.cancel")}
              </Button>
            </Link>
          </div>
        </form>
      )}
    </Layout>
  );
}
