import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useBurgerRanking } from "../../burgers/hooks/useBurgerRanking";
import { BurgerRankingCard } from "../../burgers/components/BurgerRankingCard";
import { useReviews } from "../../reviews/hooks/useReviews";
import { ReviewListCard } from "../../reviews/components/ReviewListCard";
import { useShops } from "../../shops/hooks/useShops";
import { ShopPosterCard } from "../../shops/components/ShopPosterCard";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import styles from "./home.module.css";

const RANKING_LIMIT = 6;
const REVIEWS_LIMIT = 5;
const SHOPS_LIMIT = 6;

// トップページ(design/redesign/home.html)。検索のヒーロー + バーガーランキング + 最新レビュー + 新着ショップの
// 4 セクション。並び順・絞り込みは、すべて backend の応答の順そのままで、frontend では並べ替えない。各セクションは、
// 取得中・0 件・失敗を、それぞれの状態で表示する(1 セクションの失敗がほかのセクションを止めない)。
export default function HomePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  const [keyword, setKeyword] = useState("");

  const { data: ranking, isLoading: rankingLoading, error: rankingError } = useBurgerRanking();
  const { data: reviews, isLoading: reviewsLoading, error: reviewsError } = useReviews(undefined, user?.id ?? null, {
    enabled: !authLoading,
  });
  const { data: shops, isLoading: shopsLoading, error: shopsError } = useShops({ sort: "newest" }, user?.id ?? null, {
    enabled: !authLoading,
  });

  const topRanking = ranking?.slice(0, RANKING_LIMIT);
  const latestReviews = reviews?.slice(0, REVIEWS_LIMIT);
  const newShops = shops?.slice(0, SHOPS_LIMIT);

  const handleSearch = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = keyword.trim();
    void navigate(trimmed ? `/shops?${new URLSearchParams({ keyword: trimmed })}` : "/shops");
  };

  return (
    <Layout>
      <section className={styles.hero}>
        <div className={styles.heroPhoto}>
          <div className={styles.heroScrim} />
          <div className={styles.heroContent}>
            <h1>{t("home.hero.title")}</h1>
            <p className={styles.heroLead}>{t("home.hero.lead")}</p>
            <form className={styles.heroSearch} onSubmit={handleSearch}>
              <input
                aria-label={t("shops.list.searchLabel")}
                className={styles.heroInput}
                type="text"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder={t("shops.list.searchPlaceholder")}
              />
              <Button type="submit">{t("home.hero.searchButton")}</Button>
            </form>
          </div>
        </div>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHead}>
          <h2 className={styles.heading}>{t("home.ranking.heading")}</h2>
          {topRanking && topRanking.length > 0 && <span className={styles.count}>{t("home.ranking.subheading")}</span>}
        </div>
        {rankingLoading && <Loading />}
        {rankingError && <Alert message={t("home.ranking.loadError")} />}
        {topRanking && topRanking.length === 0 && (
          <EmptyState title={t("home.ranking.emptyTitle")} description={t("home.ranking.emptyDescription")} />
        )}
        {topRanking && topRanking.length > 0 && (
          <div className={styles.itemGrid} data-testid="ranking-grid">
            {topRanking.map((burger, i) => (
              <BurgerRankingCard key={burger.id} burger={burger} rank={i + 1} ratingMax={ratingRange?.max} />
            ))}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHead}>
          <h2 className={styles.heading}>{t("home.reviews.heading")}</h2>
          {latestReviews && latestReviews.length > 0 && <span className={styles.count}>{t("reviews.detail.newestFirst")}</span>}
        </div>
        {reviewsLoading && <Loading />}
        {reviewsError && <Alert message={t("reviews.list.loadError")} />}
        {latestReviews && latestReviews.length === 0 && <EmptyState />}
        {latestReviews && latestReviews.length > 0 && (
          <div className={styles.reviewGrid} data-testid="reviews-grid">
            {latestReviews.map((review) => (
              <ReviewListCard key={review.id} review={review} ratingMax={ratingRange?.max} />
            ))}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHead}>
          <h2 className={styles.heading}>{t("home.shops.heading")}</h2>
          {newShops && newShops.length > 0 && <span className={styles.count}>{t("reviews.detail.newestFirst")}</span>}
        </div>
        {shopsLoading && <Loading />}
        {shopsError && <Alert message={t("shops.list.loadError")} />}
        {newShops && newShops.length === 0 && (
          <EmptyState title={t("home.shops.emptyTitle")} description={t("home.shops.emptyDescription")} />
        )}
        {newShops && newShops.length > 0 && (
          <div className={styles.itemGrid} data-testid="shops-grid">
            {newShops.map((shop) => (
              <ShopPosterCard key={shop.id} shop={shop} ratingMax={ratingRange?.max} headingLevel="h3" />
            ))}
          </div>
        )}
      </section>
    </Layout>
  );
}
