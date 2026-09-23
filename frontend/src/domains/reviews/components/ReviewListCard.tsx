import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { formatDate } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { Review } from "../api/types";
import styles from "./reviewListCard.module.css";

// レビュー一覧のカード(design/redesign/reviews.html の .rcard)。評価・日付を上に、コメント、対象のバーガー、
// 投稿者を下に置く。枠・角丸・背景は Card の CSS を composes で借りる。カード全体をレビュー詳細への <Link> にする。
// GET /reviews の一覧にはショップ名を含まない(shop はレビュー詳細だけが返す)ため、対象はバーガー名だけを出す。
export function ReviewListCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const username = review.user?.username ?? t("shops.detail.unknown");
  const ariaLabel = review.burger
    ? t("shops.detail.reviewAriaLabel", { burger: review.burger.name, user: username })
    : t("shops.detail.reviewAriaLabelNoBurger", { user: username });
  return (
    <Link to={`/reviews/${review.id}`} className={styles.card} aria-label={ariaLabel}>
      {review.photoUrl ? (
        <img src={review.photoUrl} alt={t("reviews.photo.alt")} loading="lazy" className={styles.photo} />
      ) : (
        <div className={styles.noPhoto}>{t("shops.detail.noPhoto")}</div>
      )}
      <div className={styles.body}>
        <div className={styles.row}>
          {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} variant="stepped" />}
          <time className={styles.date} dateTime={review.createdAt}>
            {formatDate(review.createdAt)}
          </time>
        </div>
        <p className={styles.comment}>{review.comment}</p>
        {review.burger && <p className={styles.target}>{review.burger.name}</p>}
        <div className={styles.foot}>
          <span>{username}</span>
        </div>
      </div>
    </Link>
  );
}
