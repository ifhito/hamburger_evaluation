import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useShops } from "../hooks/useShops";
import { useAuth } from "../../auth/AuthProvider";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { prefectureName, useMeta } from "../../../api/meta";
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
  return <Layout><ShopListContent /></Layout>;
}

export function ShopListContent({ hideHeading = false, record = false }: { hideHeading?: boolean; record?: boolean }) {
  const { t, i18n } = useTranslation();
  const { user, isLoading: authLoading } = useAuth();
  const prefectures = useMeta().data?.prefectures;
  const ratingRange = useRatingRange();
  const [sort, setSort] = useState("name");
  const [searchParams] = useSearchParams();
  const [keyword, setKeyword] = useState(() => searchParams.get("keyword") ?? "");
  // 都道府県の絞り込み。"" はすべての都道府県(prefecture_code を送らない)。
  const [prefectureCode, setPrefectureCode] = useState("");
  const {
    data: shops,
    isLoading,
    error,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useShops({ keyword, sort, prefectureCode }, user?.id ?? null, { enabled: !authLoading });

  return (
    <>
      {!hideHeading && <div className={styles.head}>
        <h1 className={styles.heading}>{t("shops.list.title")}</h1>
      </div>}
      <div className={styles.toolbar}>
        <select className={styles.sort} aria-label={t("lists.sortLabel")} value={sort} onChange={(e) => setSort(e.target.value)}>
          <option value="name">{t("lists.name")}</option>
          <option value="newest">{t("lists.newest")}</option>
          <option value="rating">{t("lists.rating")}</option>
        </select>
        <input
          aria-label={t("shops.list.searchLabel")}
          className={styles.search}
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t("shops.list.searchPlaceholder")}
        />
        <select
          className={styles.sort}
          aria-label={t("shops.list.prefectureFilterLabel")}
          value={prefectureCode}
          onChange={(e) => setPrefectureCode(e.target.value)}
        >
          <option value="">{t("shops.list.allPrefectures")}</option>
          {prefectures?.map((p) => (
            <option key={p.code} value={p.code}>
              {prefectureName(p, i18n.language)}
            </option>
          ))}
        </select>
        {user?.canModerate && <LinkButton to="/admin/shops">{t("shops.list.moderate")}</LinkButton>}
        {user && (
          <LinkButton variant="primary" to="/shops/new">
            {t("shops.list.addShop")}
          </LinkButton>
        )}
      </div>

      {(isLoading || authLoading) && <Loading />}
      {error && <Alert message={t("shops.list.loadError")} />}
      {shops && shops.length === 0 && <EmptyState />}

      <div className={styles.grid}>
        {shops?.map((shop) => (
          <ShopPosterCard key={shop.id} shop={shop} ratingMax={ratingRange?.max} to={record ? `/shops/${shop.id}?from=record` : undefined} />
        ))}
      </div>

      {hasNextPage && (
        <div className={styles.loadMore}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
            {t("shops.list.loadMore")}
          </Button>
        </div>
      )}
    </>
  );
}
