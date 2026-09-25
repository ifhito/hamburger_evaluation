import { useTranslation } from "react-i18next";
import { Photo } from "../../../components/ui/Photo";
import { Link } from "react-router-dom";
import { formatDate, formatVisitedAt } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { shopReviewAriaLabel } from "../../../components/reviewCardAriaLabel";
import type { Review } from "../api/types";
import styles from "./reviewListCard.module.css";

// 写真からレビューへ、名前から各詳細へ移動できるカード。リンクを入れ子にしない。
export function ReviewListCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const username = review.user?.username ?? t("shops.detail.unknown");
  const ariaLabel = shopReviewAriaLabel(review, t);
  return (
    <article className={styles.card}>
      <Link to={`/reviews/${review.id}`} className={styles.photoLink} aria-label={ariaLabel}>
        <Photo src={review.photoUrl} alt={t("reviews.photo.alt")} className={styles.photo} fallbackClassName={styles.noPhoto} fallback={t("shops.detail.noPhoto")} />
      </Link>
      <div className={styles.body}>
        <div className={styles.row}>
          {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} variant="stepped" />}
        </div>
        {review.shop && <p className={styles.shop}><Link to={`/shops/${review.shop.id}`}>{review.shop.name}</Link></p>}
        {review.visitedAt && (
          <time className={styles.date} dateTime={review.visitedAt}>
            {t("reviews.detail.visitedOn", { date: formatVisitedAt(review.visitedAt) })}
          </time>
        )}
        {review.comment && <p className={styles.comment}>{review.comment}</p>}
        {review.burger && <p className={styles.target}><Link to={`/burgers/${review.burger.id}`}>{review.burger.name}</Link></p>}
        <div className={styles.foot}>
          {review.user ? <Link to={`/users/${review.user.id}`}>{username}</Link> : <span>{username}</span>}
        </div>
        <time className={styles.postedDate} dateTime={review.createdAt}>
          {t("reviews.edit.postedOn", { date: formatDate(review.createdAt) })}
        </time>
      </div>
    </article>
  );
}
