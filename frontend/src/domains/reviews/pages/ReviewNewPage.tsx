import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useCreateReview } from "../hooks/useReviewMutations";
import { useCreateReviewForm } from "../hooks/useReviewForm";
import { useShopDetail } from "../../shops/hooks/useShops";
import { todayDateOnly } from "../../../lib/date";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { TextField, TextArea } from "../../../components/ui/TextField";
import { RatingInput } from "../../../components/ui/RatingInput";
import { Layout } from "../../../components/Layout";
import { PhotoField } from "../components/PhotoField";
import { useRatingRange } from "../hooks/useRatingRange";
import styles from "./reviewForm.module.css";

// レビューの投稿(design/redesign/review-new.html)。評価は、開いた直後は未選択(R6)。選ばずに送信したときは
// API を呼ばず、評価の入力の近くにエラーを出し、フォーカスを評価の入力へ移す(送信のボタンは無効にしない)。
export default function ReviewNewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const shopId = searchParams.get("shop_id") ?? "";

  // ショップ名は、その shop 1 件を id で取得して表示する(一覧のページ送りに依存しない)
  const { user, isLoading: authLoading } = useAuth();
  const { data: shop } = useShopDetail(shopId, user?.id ?? null, { enabled: !authLoading });

  const { create } = useCreateReview();
  const ratingRange = useRatingRange();
  const meta = useMeta().data;
  const { register, handleSubmit, setValue, watch } = useCreateReviewForm({ shopId });

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [ratingMissing, setRatingMissing] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [photo, setPhoto] = useState<File | null>(null);
  const [photoShrinking, setPhotoShrinking] = useState(false);

  if (!shopId) return <Navigate to="/shops" replace />;

  const onSubmit = handleSubmit(async (data) => {
    if (data.rating === null) {
      setRatingMissing(true);
      // ref ではなく id で探す(ref を送信ハンドラの中で読むのを避けるため)。DOM に 1 つしかない前提の id。
      document.getElementById("rating-field")?.querySelector<HTMLElement>('[role="radio"]')?.focus();
      return;
    }
    setRatingMissing(false);
    setServerError(null);
    setIsSubmitting(true);
    try {
      const review = await create({ ...data, rating: data.rating }, photo);
      void navigate(`/reviews/${review.id}`);
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("reviews.new.error")]);
    } finally {
      setIsSubmitting(false);
    }
  });

  return (
    <Layout>
      <div className={styles.container}>
        <TextLink to={`/shops/${shopId}`}>{t("reviews.new.backToShop")}</TextLink>
        <h1 className={styles.title}>{t("reviews.new.title")}</h1>
        <p className={styles.shopline}>
          <span>{t("reviews.new.shopLabel")}</span>
          <b>{shop?.name ?? shopId}</b>
        </p>

        <form onSubmit={(e) => void onSubmit(e)} noValidate>
          {serverError && <Alert title={t("reviews.new.errorTitle")} message={serverError} />}

          <div className={styles.formgrid}>
            <div className={styles.column}>
              <TextField
                id="burgerName"
                label={t("reviews.new.burgerName")}
                placeholder={t("reviews.new.burgerNamePlaceholder")}
                counter={{ value: watch("burgerName"), max: meta?.text.burgerNameMaxChars }}
                {...register("burgerName")}
              />
              <div id="rating-field">
                <RatingInput
                  label={t("common.rating")}
                  value={watch("rating")}
                  onChange={(v) => {
                    setValue("rating", v);
                    setRatingMissing(false);
                  }}
                  min={ratingRange?.min}
                  max={ratingRange?.max}
                />
              </div>
              {ratingMissing && (
                <div className={styles.ratingError}>
                  <Alert message={t("reviews.new.ratingRequired")} />
                </div>
              )}
              <TextArea
                id="comment"
                label={t("reviews.new.comment")}
                placeholder={t("reviews.new.commentPlaceholder")}
                counter={{ value: watch("comment"), max: meta?.text.reviewCommentMaxChars }}
                rows={6}
                {...register("comment")}
              />
              <TextField
                id="visitedAt"
                type="date"
                label={t("reviews.new.visitedAt")}
                optional={t("reviews.new.visitedAtOptional")}
                max={todayDateOnly()}
                {...register("visitedAt")}
              />
            </div>
            <div className={styles.column}>
              <PhotoField photo={photo} onChange={setPhoto} limits={meta?.photo} onShrinkingChange={setPhotoShrinking} allowRemove />
            </div>
          </div>

          <div className={styles.actionsRow}>
            <Button type="submit" wide disabled={photoShrinking} isLoading={isSubmitting} loadingLabel={t("reviews.new.posting")}>
              {photoShrinking ? t("reviews.new.submitWaitingForShrink") : t("reviews.new.submit")}
            </Button>
            <LinkButton to={`/shops/${shopId}`}>{t("reviews.new.cancel")}</LinkButton>
          </div>
        </form>
      </div>
    </Layout>
  );
}
