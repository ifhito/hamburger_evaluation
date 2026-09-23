import { useTranslation } from "react-i18next";
import { useBurgers } from "../hooks/useBurgers";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { BurgerRankingCard } from "../components/BurgerRankingCard";
import styles from "./burgerList.module.css";

// バーガー一覧。weighted_score の高い順は backend が返す既定の順で、frontend は並び替え・絞り込みを行わない
// (#196 の非ゴール)。ランキングの順位バッジ(rank)は、このページでは渡さず出さない(トップページだけの見せ方)。
export default function BurgerListPage() {
  const { t } = useTranslation();
  const ratingRange = useRatingRange();
  const { data: burgers, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } = useBurgers();

  return (
    <Layout>
      <div className={styles.head}>
        <h1 className={styles.heading}>{t("burgers.list.title")}</h1>
      </div>

      {isLoading && <Loading />}
      {error && <Alert message={t("burgers.list.loadError")} />}
      {burgers && burgers.length === 0 && <EmptyState />}

      <div className={styles.grid}>
        {burgers?.map((burger) => (
          <BurgerRankingCard key={burger.id} burger={burger} ratingMax={ratingRange?.max} />
        ))}
      </div>

      {hasNextPage && (
        <div className={styles.loadMore}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
            {t("burgers.list.loadMore")}
          </Button>
        </div>
      )}
    </Layout>
  );
}
