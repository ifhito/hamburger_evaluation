import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReviews } from "../hooks/useReviews";
import { formatDate } from "../../../lib/date";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import styles from "./reviewList.module.css";

export default function ReviewListPage() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const [keyword, setKeyword] = useState("");
  const [ratingFilter, setRatingFilter] = useState<number | undefined>(undefined);

  const { data: reviews, isLoading, error } = useReviews(
    ratingFilter !== undefined || keyword
      ? { rating: ratingFilter, keyword: keyword || undefined }
      : undefined
  );

  return (
    <Layout title={t("reviews.list.title")}>
      <div className={styles.filters}>
        <input
          className={styles.searchInput}
          placeholder={t("reviews.list.searchPlaceholder")}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <select
          className={styles.ratingSelect}
          value={ratingFilter ?? ""}
          onChange={(e) =>
            setRatingFilter(e.target.value ? Number(e.target.value) : undefined)
          }
        >
          <option value="">{t("reviews.list.allRatings")}</option>
          {[5, 4, 3, 2, 1].map((r) => (
            <option key={r} value={r}>
              {"★".repeat(r)}
            </option>
          ))}
        </select>
        {user && (
          <Link to="/reviews/new" className={styles.newLink}>
            {t("reviews.list.newReview")}
          </Link>
        )}
      </div>

      {error && <ErrorMessage message={t("reviews.list.loadError")} />}
      {isLoading && <p className={styles.muted}>{t("reviews.list.loading")}</p>}
      {reviews && reviews.length === 0 && (
        <p className={styles.muted}>{t("reviews.list.noReviews")}</p>
      )}

      <div className={styles.list}>
        {reviews?.map((review) => (
          <div key={review.id} className={styles.card}>
            <div className={styles.cardHeader}>
              <span className={styles.rating}>
                {"★".repeat(review.rating)}
                {"☆".repeat(5 - review.rating)}
              </span>
              <span className={styles.date}>{formatDate(review.createdAt)}</span>
            </div>
            <p className={styles.comment}>{review.comment}</p>
            <div className={styles.meta}>
              {review.user && (
                <Link to={`/users/${review.user.id}`}>{review.user.username}</Link>
              )}
              {review.burger && (
                <span>
                  {review.burger.name} · {t("reviews.list.avg")}{" "}
                  {review.burger.averageRating.toFixed(1)} ·{" "}
                  {review.burger.reviewCount} {t("reviews.list.reviews")}
                </span>
              )}
            </div>
            <div className={styles.viewLink}>
              <Link to={`/reviews/${review.id}`}>{t("reviews.list.viewLink")}</Link>
            </div>
          </div>
        ))}
      </div>
    </Layout>
  );
}
