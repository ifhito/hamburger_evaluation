import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import type { BurgerRanking } from "../api/types";
import styles from "./burgerRankingCard.module.css";

// トップページのバーガーランキングのカード(design/redesign/home.html の .rcard.rankcard)。黒い丸の順位バッジは、
// このトップページのランキングだけの見せ方(design/redesign/home.html の実装メモにあるとおり、ほかで rcard を
// 使い回すときは出さない)。バーガー名だけを /burgers/:id への Link にする(#188 の comment のとおり。カード全体は
// リンクにしない。ReviewDetailPage の burger 名の見せ方と同じ考え方)。
export function BurgerRankingCard({
  burger,
  rank,
  ratingMax,
}: {
  burger: BurgerRanking;
  rank: number;
  ratingMax: number | undefined;
}) {
  const { t } = useTranslation();
  return (
    <article className={styles.card}>
      <div className={styles.row}>
        <span className={styles.rankNo}>{rank}</span>
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
      <p className={styles.shop}>{burger.shop.name}</p>
      <p className={styles.count}>{t("common.reviewCount", { count: burger.reviewCount })}</p>
    </article>
  );
}
