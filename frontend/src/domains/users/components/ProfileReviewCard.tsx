import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { formatDate } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { Review } from "../../reviews/api/types";
import styles from "./profileReviewCard.module.css";

// プロフィールに並べる、レビュー 1 件のカード(写真・バーガー名・日付・評価・コメント)。評価の最大値は GET /meta の値(未取得の間は数字だけ)。
export function ProfileReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  return (
    <article className={styles.card}>
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
            {formatDate(review.createdAt)}
          </time>
        </div>
        {ratingMax === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingMax} />}
        <p className={styles.comment}>{review.comment}</p>
        <div className={styles.foot}>
          <Link to={`/reviews/${review.id}`} className={styles.more}>
            {t("users.detail.viewLink")}
          </Link>
        </div>
      </div>
    </article>
  );
}
