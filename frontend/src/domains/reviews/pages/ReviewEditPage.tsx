import { useEffect, useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useReview } from "../hooks/useReview";
import { useUpdateReview } from "../hooks/useReviewMutations";
import { useUpdateReviewForm } from "../hooks/useReviewForm";
import { ApiError } from "../../../api/client/buildApiClient";
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
  const { data: review, isLoading } = useReview(Number(id));
  const { update } = useUpdateReview(Number(id));

  const {
    register,
    handleSubmit,
    setValue,
    watch,
    reset,
    formState: { errors },
  } = useUpdateReviewForm();

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (review) {
      reset({ rating: review.rating, comment: review.comment });
    }
  }, [review, reset]);

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      await update(data);
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
      {!isLoading && (
        <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
          {serverError && <ErrorMessage message={serverError} />}
          <RatingSelect
            value={watch("rating")}
            onChange={(v) => setValue("rating", v)}
            error={errors.rating?.message}
          />
          <Textarea
            id="comment"
            label={t("reviews.edit.comment")}
            error={errors.comment?.message}
            {...register("comment")}
          />
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
