import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useShops } from "../hooks/useShops";
import { useAuth } from "../../auth/AuthProvider";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { ShopPosterCard } from "../components/ShopPosterCard";
import styles from "./shopList.module.css";

// ショップ一覧(design/redesign/shops.html)。「ショップを追加」はサインインしている人に出す(API の項目ではなく、
// サインインの状態で決める)。「ショップを管理」は backend が返す can_moderate の人だけに出す。並ぶショップの絞り込み
// (公開中だけ・自分が作ったものも含む・すべて)は API が行い、frontend は返された一覧をそのまま出す。
export default function ShopListPage() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const ratingRange = useRatingRange();
  const [keyword, setKeyword] = useState("");
  const {
    data: shops,
    isLoading,
    error,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useShops(keyword ? { keyword } : undefined);

  return (
    <Layout>
      <div className={styles.head}>
        <h1 className={styles.heading}>{t("shops.list.title")}</h1>
      </div>
      <div className={styles.toolbar}>
        <input
          aria-label={t("shops.list.searchLabel")}
          className={styles.search}
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t("shops.list.searchPlaceholder")}
        />
        {user?.canModerate && <LinkButton to="/admin/shops">{t("shops.list.moderate")}</LinkButton>}
        {user && (
          <LinkButton variant="primary" to="/shops/new">
            {t("shops.list.addShop")}
          </LinkButton>
        )}
      </div>

      {isLoading && <Loading />}
      {error && <Alert message={t("shops.list.loadError")} />}
      {shops && shops.length === 0 && <EmptyState />}

      <div className={styles.grid}>
        {shops?.map((shop) => (
          <ShopPosterCard key={shop.id} shop={shop} ratingMax={ratingRange?.max} />
        ))}
      </div>

      {hasNextPage && (
        <div className={styles.loadMore}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
            {t("shops.list.loadMore")}
          </Button>
        </div>
      )}
    </Layout>
  );
}
