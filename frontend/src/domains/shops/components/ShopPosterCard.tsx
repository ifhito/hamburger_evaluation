import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Badge } from "../../../components/ui/Badge";
import { RatingBurger, RatingBurgerIcon } from "../../../components/ui/RatingBurger";
import type { Shop } from "../api/types";
import styles from "./shopPosterCard.module.css";

// ショップ一覧の、ポスター風のカード(design/redesign/shops.html の .poster)。写真を大きく見せ、店名と評価は
// 写真の下に置く。card.module.css の枠・背景は使わない(デザインに、そのふちがないため)。
// 写真は、リンクを 2 つ(写真・店名)にして押せる場所を広げるが、同じ行き先を読み上げで二重に伝えないよう、
// 写真側は aria-hidden + tabIndex=-1 にする(店名側だけを、実際に読み上げられるリンクにする)。
export function ShopPosterCard({ shop, ratingMax }: { shop: Shop; ratingMax: number | undefined }) {
  const { t } = useTranslation();
  const to = `/shops/${shop.id}`;
  return (
    <article className={styles.poster}>
      <div className={styles.photoWrap}>
        {shop.status !== "active" && (
          <div className={styles.chip}>
            <Badge>{t(`shops.statusBadge.${shop.status}`)}</Badge>
          </div>
        )}
        <Link to={to} aria-hidden="true" tabIndex={-1} className={styles.photoLink}>
          {shop.photoUrl ? (
            <img src={shop.photoUrl} alt="" loading="lazy" className={styles.photo} />
          ) : (
            <div className={styles.noPhoto}>{t("shops.list.noPhoto")}</div>
          )}
        </Link>
      </div>
      <div className={styles.info}>
        <h2 className={styles.name}>
          <Link to={to} className={styles.nameLink}>
            {shop.name}
          </Link>
        </h2>
        <p className={styles.meta}>
          {shop.reviewCount > 0 && shop.averageRating !== null ? (
            ratingMax === undefined ? (
              <b>{shop.averageRating}</b>
            ) : (
              <RatingBurger value={shop.averageRating} max={ratingMax} fractionDigits={1} />
            )
          ) : (
            <RatingBurgerIcon ratio={0} />
          )}
          {shop.reviewCount > 0 ? (
            <span>{t("common.reviewCount", { count: shop.reviewCount })}</span>
          ) : (
            <span>{t("shops.list.noReviewsYet")}</span>
          )}
        </p>
      </div>
    </article>
  );
}
