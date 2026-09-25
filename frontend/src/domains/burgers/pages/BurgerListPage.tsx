import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useBurgers } from "../hooks/useBurgers";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { BurgerRankingCard } from "../components/BurgerRankingCard";
import styles from "./burgerList.module.css";

// バーガー一覧。条件はAPIへ送り、返された順序で表示する。
export default function BurgerListPage() {
  const { t } = useTranslation();
  const ratingRange = useRatingRange();
  const [sort, setSort] = useState("ranking");
  const [keyword, setKeyword] = useState("");
  const { data: burgers, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } = useBurgers({ keyword, sort });

  return (
    <Layout>
      <div className={styles.head}>
        <h1 className={styles.heading}>{t("burgers.list.title")}</h1>
      </div>

      <div className={styles.toolbar}>
        <select className={styles.sort} aria-label={t("lists.sortLabel")} value={sort} onChange={(e) => setSort(e.target.value)}>
          <option value="ranking">{t("lists.ranking")}</option>
          <option value="newest">{t("lists.newest")}</option>
          <option value="name">{t("lists.name")}</option>
        </select>
        <input className={styles.search} aria-label={t("lists.burgerSearch")} placeholder={t("lists.burgerSearch")} value={keyword} onChange={(e) => setKeyword(e.target.value)} />
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
