import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { formatDate } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { shopReviewAriaLabel } from "../../../components/reviewCardAriaLabel";
import type { Review } from "../../reviews/api/types";
import styles from "./shopReviewCard.module.css";

// ショップ詳細に並ぶ、レビュー 1 件のカード(design/redesign/shop-detail.html の .rcard)。枠・角丸・背景は
// Card の CSS を composes で借りる(値の複製をしない)。バーガー名・評価・コメント・投稿者で構成する。
// カード全体をレビュー詳細への <Link> にする(読み上げでは 1 つのリンクとして aria-label が読まれる)。
export function ShopReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const username = review.user?.username ?? t("shops.detail.unknown");
  const ariaLabel = shopReviewAriaLabel(review, t);
  return (
    <Link to={`/reviews/${review.id}`} className={styles.card} aria-label={ariaLabel}>
      {review.photoUrl ? (
        <img src={review.photoUrl} alt={t("reviews.photo.alt")} loading="lazy" className={styles.photo} />
      ) : (
        <div className={styles.noPhoto}>{t("shops.detail.noPhoto")}</div>
      )}
      <div className={styles.body}>
        <div className={styles.row}>
          {review.burger && <span className={styles.name}>{review.burger.name}</span>}
          <time className={styles.date} dateTime={review.createdAt}>
            {formatDate(review.createdAt)}
          </time>
        </div>
        {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} variant="stepped" />}
        {review.comment && <p className={styles.comment}>{review.comment}</p>}
        <div className={styles.foot}>
          <span>{username}</span>
        </div>
      </div>
    </Link>
  );
}
