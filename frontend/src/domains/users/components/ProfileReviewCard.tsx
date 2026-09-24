import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { formatDate } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { Review } from "../../reviews/api/types";
import styles from "./profileReviewCard.module.css";

// プロフィールに並べる、レビュー 1 件のカード(写真・バーガー名・日付・評価・コメント)。評価の最大値は GET /meta の値(未取得の間は数字だけ)。
// デザイン(design/redesign/profile.html)は article.rcard(繰り返す項目なので article)。枠・角丸・背景は、
// Card の CSS を composes で借りる(値の複製をしない)。写真を端まで見せるので、内側の余白は body 側だけに付ける。
// カード全体をレビュー詳細への <Link> にする。
export function ProfileReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const dateLabel = formatDate(review.createdAt);
  const ariaLabel = review.burger
    ? t("users.detail.reviewAriaLabel", { burger: review.burger.name, date: dateLabel })
    : t("users.detail.reviewAriaLabelNoBurger", { date: dateLabel });
  return (
    <Link to={`/reviews/${review.id}`} className={styles.card} aria-label={ariaLabel}>
      {review.photoUrl ? (
        <img src={review.photoUrl} alt={t("reviews.photo.alt")} loading="lazy" className={styles.photo} />
      ) : (
        <div className={styles.noPhoto}>{t("users.detail.noPhoto")}</div>
      )}
      <div className={styles.body}>
        <div className={styles.row}>
          {/* バーガーの名前は、利用者が自由に決めるので、HTML として解釈せず、文字として描画する */}
          {review.burger && <h3 className={styles.name}>{review.burger.name}</h3>}
          <time className={styles.date} dateTime={review.createdAt}>
            {dateLabel}
          </time>
        </div>
        {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} variant="stepped" />}
        {review.comment && <p className={styles.comment}>{review.comment}</p>}
      </div>
    </Link>
  );
}
