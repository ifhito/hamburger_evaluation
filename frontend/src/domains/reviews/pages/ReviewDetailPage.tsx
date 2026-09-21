import { useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useDeleteReview } from "../hooks/useReviewMutations";
import { useRatingRange } from "../hooks/useRatingRange";
import { formatDate } from "../../../lib/date";
import { formatRating } from "../../../lib/rating";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import styles from "./reviewDetail.module.css";

export default function ReviewDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  // 編集・削除できるか(canEdit)は backend が返す。閲覧者ごとに違うので、認証状態が確定してから取得する
  const { data: review, isLoading, error } = useReview(id, user?.id ?? null, {
    enabled: !authLoading,
  });
  const { destroy } = useDeleteReview();
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async () => {
    if (!confirm(t("reviews.detail.deleteConfirm"))) return;
    setIsDeleting(true);
    try {
      await destroy(id ?? "");
      void navigate("/reviews");
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <Layout title={t("reviews.detail.title")}>
      {(isLoading || authLoading) && <p className={styles.muted}>{t("reviews.detail.loading")}</p>}
      {error && <ErrorMessage message={t("reviews.detail.loadError")} />}

      {review && (
        <div className={styles.card}>
          <div className={styles.cardHeader}>
            <span className={styles.rating}>{formatRating(review.rating, ratingRange?.max)}</span>
            <span className={styles.date}>{formatDate(review.createdAt)}</span>
          </div>
          <p className={styles.comment}>{review.comment}</p>
          {review.photoUrl && (
            <img
              src={review.photoUrl}
              alt={t("reviews.photo.alt")}
              className={styles.photo}
            />
          )}
          {review.burger && (
            <p className={styles.burgerInfo}>
              {review.burger.name} · {t("reviews.detail.avgRating")}{" "}
              {review.burger.averageRating.toFixed(1)} ·{" "}
              {review.burger.reviewCount}
            </p>
          )}
          {review.user && (
            <p className={styles.userInfo}>
              {t("reviews.detail.by")}{" "}
              <Link to={`/users/${review.user.id}`}>{review.user.username}</Link>
            </p>
          )}
          {review.canEdit && (
            <div className={styles.actions}>
              <Link to={`/reviews/${review.id}/edit`} className={styles.editLink}>
                {t("reviews.detail.edit")}
              </Link>
              <Button
                variant="danger"
                isLoading={isDeleting}
                onClick={() => void handleDelete()}
              >
                {t("reviews.detail.delete")}
              </Button>
            </div>
          )}
        </div>
      )}
      <Link to="/reviews" className={styles.backLink}>
        {t("reviews.detail.backToReviews")}
      </Link>
    </Layout>
  );
}
