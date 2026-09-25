import { Photo } from "../../../components/ui/Photo";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { BurgerRanking } from "../api/types";
import styles from "./burgerRankingCard.module.css";

// バーガーの写真・評価・店舗を示すカード。順位はホームで指定されたときだけ表示する。
export function BurgerRankingCard({
  burger,
  rank,
  ratingMax,
}: {
  burger: BurgerRanking;
  rank?: number;
  ratingMax: number | undefined;
}) {
  const { t } = useTranslation();
  return (
    <article className={styles.card}>
      <Link to={`/burgers/${burger.id}`} aria-label={burger.name} className={styles.photoLink}>
        <Photo src={burger.photoUrl} alt="" className={styles.photo} fallbackClassName={styles.noPhoto} fallback={t("shops.list.noPhoto")} />
      </Link>
      <div className={styles.body}>
        <div className={styles.row}>
          {rank !== undefined && (
            <span className={styles.rankNo} aria-label={t("home.ranking.rankAria", { rank })}>
              {rank}
            </span>
          )}
          {ratingMax === undefined ? (
            <b>{burger.averageRating.toFixed(1)}</b>
          ) : (
            <RatingBurger value={burger.averageRating} max={ratingMax} fractionDigits={1} />
          )}
        </div>
        <p className={styles.name}>
          <Link to={`/burgers/${burger.id}`} className={styles.nameLink}>
            {burger.name}
          </Link>
        </p>
        <p className={styles.shop}><Link to={`/shops/${burger.shop.id}`}>{burger.shop.name}</Link></p>
        <p className={styles.count}>{t("common.reviewCount", { count: burger.reviewCount })}</p>
      </div>
    </article>
  );
}
