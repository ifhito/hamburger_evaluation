import { useState } from "react";
import { Navigate, useNavigate, useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useCreateReview } from "../hooks/useReviewMutations";
import { useCreateReviewForm } from "../hooks/useReviewForm";
import { useShops } from "../../shops/hooks/useShops";
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
  const shopId = Number(searchParams.get("shop_id"));

  const { data: shops } = useShops();
  const shopName = shops?.find((s) => s.id === shopId)?.name;

  const { create } = useCreateReview();
  const {
    register,
    handleSubmit,
    setValue,
    watch,
    formState: { errors },
  } = useCreateReviewForm({ shopId });

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!shopId) return <Navigate to="/shops" replace />;

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      const review = await create(data);
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
          error={errors.rating?.message}
        />
        <Textarea
          id="comment"
          label={t("reviews.new.comment")}
          placeholder={t("reviews.new.commentPlaceholder")}
          error={errors.comment?.message}
          {...register("comment")}
        />
        <Input
          id="burgerName"
          label={t("reviews.new.burgerName")}
          type="text"
          placeholder={t("reviews.new.burgerNamePlaceholder")}
          error={errors.burgerName?.message}
          {...register("burgerName")}
        />
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
