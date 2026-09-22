import { useTranslation } from "react-i18next";
import { formatDate } from "../../../lib/date";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { TextLink } from "../../../components/ui/TextLink";
import type { Review } from "../../reviews/api/types";
import styles from "./shopReviewCard.module.css";

// ショップ詳細に並ぶ、レビュー 1 件のカード(design/redesign/shop-detail.html の .rcard)。枠・角丸・背景は
// Card の CSS を composes で借りる(値の複製をしない)。バーガー名・評価・コメント・投稿者・「詳しく見る」で構成する。
export function ShopReviewCard({ review, ratingMax }: { review: Review; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  return (
    <article className={styles.card}>
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
        <p className={styles.comment}>{review.comment}</p>
        <div className={styles.foot}>
          <span>{review.user?.username ?? t("shops.detail.unknown")}</span>
          <TextLink to={`/reviews/${review.id}`}>{t("shops.detail.readMore")}</TextLink>
        </div>
      </div>
    </article>
  );
}
