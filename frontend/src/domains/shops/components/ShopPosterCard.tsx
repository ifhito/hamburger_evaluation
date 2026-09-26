import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Badge } from "../../../components/ui/Badge";
import { RatingBurger, RatingBurgerIcon } from "../../../components/ui/RatingBurger";
import type { Shop } from "../api/types";
import styles from "./shopPosterCard.module.css";

// ショップ一覧の、ポスター風のカード(design/redesign/shops.html の .poster)。写真を大きく見せ、店名と評価は
// 写真の下に置く。card.module.css の枠・背景は使わない(デザインに、そのふちがないため)。
// カード全体を押せるようにする(stretched link)。実際のリンクは店名の 1 つだけにして(読み上げの二重を避ける)、
// CSS(nameLink::after)で、そのリンクの当たり判定を、カード全体(.poster、position: relative)まで広げる。
export function ShopPosterCard({
  shop,
  ratingMax,
  headingLevel = "h2",
  to = `/shops/${shop.id}`,
}: {
  shop: Shop;
  ratingMax: number | undefined;
  headingLevel?: "h2" | "h3";
  to?: string;
}) {
  const { t } = useTranslation();
  const Heading = headingLevel;
  return (
    <article className={styles.poster}>
      <div className={styles.photoWrap}>
        {(shop.status !== "active" || shop.closedAt) && (
          <div className={styles.chip}>
            {shop.status !== "active" && <Badge>{t(`shops.statusBadge.${shop.status}`)}</Badge>}
            {shop.closedAt && <Badge>{t("shops.closedBadge")}</Badge>}
          </div>
        )}
        {shop.photoUrl ? (
          <img src={shop.photoUrl} alt="" loading="lazy" className={styles.photo} />
        ) : (
          <div className={styles.noPhoto}>{t("shops.list.noPhoto")}</div>
        )}
      </div>
      <div className={styles.info}>
        <Heading className={styles.name}>
          <Link to={to} className={styles.nameLink}>
            {shop.name}
          </Link>
        </Heading>
        <p className={styles.meta}>
          {shop.reviewCount > 0 && shop.averageRating !== null ? (
            ratingMax === undefined ? (
              <b>{shop.averageRating.toFixed(1)}</b>
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
