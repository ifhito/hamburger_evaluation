import { useTranslation } from "react-i18next";
import { formatDate } from "../../../lib/date";
import { Card } from "../../../components/ui/Card";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { TextLink } from "../../../components/ui/TextLink";
import type { Review } from "../../reviews/api/types";
import styles from "./profileReviewCard.module.css";

// プロフィールに並べる、レビュー 1 件のカード(写真・バーガー名・日付・評価・コメント)。評価の最大値は GET /meta の値(未取得の間は数字だけ)。
// 写真を端まで見せるので、Card は padded={false}(中身の body 側で、自分の余白を持つ)。
export function ProfileReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  return (
    <Card padded={false}>
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
          <TextLink to={`/reviews/${review.id}`}>{t("users.detail.viewLink")}</TextLink>
        </div>
      </div>
    </Card>
  );
}
