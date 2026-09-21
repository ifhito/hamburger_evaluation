import { useState } from "react";
import { Navigate, useNavigate, useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useCreateReview } from "../hooks/useReviewMutations";
import { useCreateReviewForm } from "../hooks/useReviewForm";
import { useShopDetail } from "../../shops/hooks/useShops";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import { RatingSelect } from "../../../components/RatingSelect";
import { Textarea } from "../../../components/Textarea";
import styles from "./reviewForm.module.css";

export default function ReviewNewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const shopId = searchParams.get("shop_id") ?? "";

  // ショップ名は、その shop 1 件を id で取得して表示する(一覧のページ送りに依存しない)
  const { user, isLoading: authLoading } = useAuth();
  const { data: shop } = useShopDetail(shopId, user?.id ?? null, { enabled: !authLoading });
  const shopName = shop?.name;

  const { create } = useCreateReview();
  const { register, handleSubmit, setValue, watch } = useCreateReviewForm({ shopId });

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [photo, setPhoto] = useState<File | null>(null);

  if (!shopId) return <Navigate to="/shops" replace />;

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      const review = await create(data, photo);
      void navigate(`/reviews/${review.id}`);
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("reviews.new.error")]);
    } finally {
      setIsSubmitting(false);
    }
  });

  return (
    <Layout title={t("reviews.new.title")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <div className={styles.shopInfo}>
          <strong>{t("reviews.new.shop")}</strong> {shopName ?? `#${shopId}`}
        </div>
        <RatingSelect
          value={watch("rating")}
          onChange={(v) => setValue("rating", v)}
        />
        <Textarea
          id="comment"
          label={t("reviews.new.comment")}
          placeholder={t("reviews.new.commentPlaceholder")}
          {...register("comment")}
        />
        <Input
          id="burgerName"
          label={t("reviews.new.burgerName")}
          type="text"
          placeholder={t("reviews.new.burgerNamePlaceholder")}
          {...register("burgerName")}
        />
        <div className={styles.field}>
          <label htmlFor="photo" className={styles.fieldLabel}>
            {t("reviews.photo.label")}
          </label>
          <input
            id="photo"
            type="file"
            accept="image/jpeg,image/png,image/webp"
            onChange={(e) => setPhoto(e.target.files?.[0] ?? null)}
          />
        </div>
        <div className={styles.actions}>
          <Button type="submit" isLoading={isSubmitting}>
            {t("reviews.new.submit")}
          </Button>
          <Link to={`/shops/${shopId}`}>
            <Button type="button" variant="secondary">
              {t("reviews.new.cancel")}
            </Button>
          </Link>
        </div>
      </form>
    </Layout>
  );
}
