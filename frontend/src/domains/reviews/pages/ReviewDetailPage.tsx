import { useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useDeleteReview } from "../hooks/useReviewMutations";
import { formatDate } from "../../../lib/date";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import styles from "./reviewDetail.module.css";

export default function ReviewDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();
  const { data: review, isLoading, error } = useReview(Number(id));
  const { destroy } = useDeleteReview();
  const [isDeleting, setIsDeleting] = useState(false);

  const isOwner =
    user !== null && review?.user !== null && user.id === review?.user?.id;

  const handleDelete = async () => {
    if (!confirm(t("reviews.detail.deleteConfirm"))) return;
    setIsDeleting(true);
    try {
      await destroy(Number(id));
      void navigate("/reviews");
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <Layout title={t("reviews.detail.title")}>
      {isLoading && <p className={styles.muted}>{t("reviews.detail.loading")}</p>}
      {error && <ErrorMessage message={t("reviews.detail.loadError")} />}

      {review && (
        <div className={styles.card}>
          <div className={styles.cardHeader}>
            <span className={styles.rating}>
              {"★".repeat(review.rating)}
              {"☆".repeat(5 - review.rating)}
            </span>
            <span className={styles.date}>{formatDate(review.createdAt)}</span>
          </div>
          <p className={styles.comment}>{review.comment}</p>
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
          {isOwner && (
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
