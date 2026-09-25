import { useTranslation } from "react-i18next";
import { Photo } from "../../../components/ui/Photo";
import { Link } from "react-router-dom";
import { formatDate, formatVisitedAt } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { Review } from "../../reviews/api/types";
import styles from "./profileReviewCard.module.css";

// 写真からレビューへ、名前から各詳細へ移動できるカード。リンクを入れ子にしない。
export function ProfileReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const dateLabel = formatDate(review.createdAt);
  const ariaLabel = review.burger
    ? t("users.detail.reviewAriaLabel", { burger: review.burger.name, date: dateLabel })
    : t("users.detail.reviewAriaLabelNoBurger", { date: dateLabel });
  return (
    <article className={styles.card}>
      <Link to={`/reviews/${review.id}`} className={styles.photoLink} aria-label={ariaLabel}>
        <Photo src={review.photoUrl} alt={t("reviews.photo.alt")} className={styles.photo} fallbackClassName={styles.noPhoto} fallback={t("shops.detail.noPhoto")} />
      </Link>
      <div className={styles.body}>
        <div className={styles.row}>
          {/* バーガーの名前は、利用者が自由に決めるので、HTML として解釈せず、文字として描画する */}
          {review.burger && <h3 className={styles.name}><Link to={`/burgers/${review.burger.id}`}>{review.burger.name}</Link></h3>}
        </div>
        {review.shop && <p className={styles.shop}><Link to={`/shops/${review.shop.id}`}>{review.shop.name}</Link></p>}
        {review.visitedAt && (
          <time className={styles.date} dateTime={review.visitedAt}>
            {t("reviews.detail.visitedOn", { date: formatVisitedAt(review.visitedAt) })}
          </time>
        )}
        {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} variant="stepped" />}
        {review.comment && <p className={styles.comment}>{review.comment}</p>}
        <time className={styles.postedDate} dateTime={review.createdAt}>
          {t("reviews.edit.postedOn", { date: dateLabel })}
        </time>
      </div>
    </article>
  );
}
