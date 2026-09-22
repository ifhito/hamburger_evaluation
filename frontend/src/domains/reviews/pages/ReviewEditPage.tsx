import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useUpdateReview } from "../hooks/useReviewMutations";
import { useUpdateReviewForm } from "../hooks/useReviewForm";
import { useRatingRange } from "../hooks/useRatingRange";
import { formatDate } from "../../../lib/date";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { TextArea } from "../../../components/ui/TextField";
import { RatingInput } from "../../../components/ui/RatingInput";
import { Loading, NotFound } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { PhotoField } from "../components/PhotoField";
import styles from "./reviewForm.module.css";

// レビューの編集(design/redesign/review-edit.html)。編集してよいか(can_edit)は backend が返す。他人のレビューでは
// フォームを出さない。バーガー名はあとから変えられない(表示だけ)。評価は、既存の値を初期値にする(R6 は投稿だけ)。
export default function ReviewEditPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user, isLoading: authLoading } = useAuth();
  const { data: review, isLoading: reviewLoading, error } = useReview(id, user?.id ?? null, { enabled: !authLoading });
  const isLoading = reviewLoading || authLoading;
  const { update } = useUpdateReview(id ?? "");
  const ratingRange = useRatingRange();
  const meta = useMeta().data;

  const { register, handleSubmit, setValue, watch, reset } = useUpdateReviewForm();

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [photo, setPhoto] = useState<File | null>(null);
  const [photoShrinking, setPhotoShrinking] = useState(false);

  useEffect(() => {
    if (review) reset({ rating: review.rating, comment: review.comment ?? "" });
  }, [review, reset]);

  if (isLoading) {
    return (
      <Layout>
        <Loading />
      </Layout>
    );
  }
  if (error || !review) {
    return (
      <Layout>
        <NotFound action={<LinkButton to="/reviews">{t("reviews.detail.backToReviewsAction")}</LinkButton>} />
      </Layout>
    );
  }

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
    <Layout>
      <div className={styles.container}>
        <TextLink to={`/reviews/${id}`}>{t("reviews.edit.backToReview")}</TextLink>
        <h1 className={styles.title}>{t("reviews.edit.title")}</h1>
        {review.burger && (
          <p className={styles.shopline}>
            <b>{review.burger.name}</b>
            {review.shop && <span>{review.shop.name}</span>}
          </p>
        )}

        {!review.canEdit ? (
          <div className={styles.narrow}>
            <Alert title={t("reviews.edit.forbiddenTitle")} message={t("reviews.edit.forbiddenBody")} />
            <p className={styles.forbidden}>
              <TextLink to={`/reviews/${id}`}>{t("reviews.edit.backToReview")}</TextLink>
            </p>
          </div>
        ) : (
          <form onSubmit={(e) => void onSubmit(e)} noValidate>
            {serverError && <Alert title={t("reviews.edit.errorTitle")} message={serverError} />}
            <div className={styles.formgrid}>
              <div className={styles.column}>
                <RatingInput
                  label={t("common.rating")}
                  value={watch("rating")}
                  onChange={(v) => setValue("rating", v)}
                  min={ratingRange?.min}
                  max={ratingRange?.max}
                />
                <TextArea
                  id="comment"
                  label={t("reviews.edit.comment")}
                  counter={{ value: watch("comment"), max: meta?.text.reviewCommentMaxChars }}
                  rows={6}
                  {...register("comment")}
                />
              </div>
              <div className={styles.column}>
                <PhotoField
                  photo={photo}
                  onChange={setPhoto}
                  limits={meta?.photo}
                  onShrinkingChange={setPhotoShrinking}
                  existingPhotoUrl={review.photoUrl}
                  existingPhotoCaption={t("reviews.edit.postedOn", { date: formatDate(review.createdAt) })}
                />
              </div>
            </div>
            <div className={styles.actionsRow}>
              <Button type="submit" wide disabled={photoShrinking} isLoading={isSubmitting} loadingLabel={t("reviews.new.posting")}>
                {photoShrinking ? t("reviews.new.submitWaitingForShrink") : t("reviews.edit.submit")}
              </Button>
              <LinkButton to={`/reviews/${id}`}>{t("reviews.edit.cancel")}</LinkButton>
            </div>
          </form>
        )}
      </div>
    </Layout>
  );
}
