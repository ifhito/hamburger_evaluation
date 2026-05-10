import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useShops } from "../hooks/useShops";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./shopList.module.css";

export default function ShopListPage() {
  const { t } = useTranslation();
  const [keyword, setKeyword] = useState("");
  const { data: shops, isLoading } = useShops(keyword ? { keyword } : undefined);

  return (
    <Layout title={t("shops.list.title")}>
      <div className={styles.container}>
        <Input
          id="keyword"
          label={t("shops.list.searchLabel")}
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t("shops.list.searchPlaceholder")}
        />
        {isLoading && <p className={styles.muted}>{t("shops.list.loading")}</p>}
        {shops && shops.length === 0 && (
          <p className={styles.muted}>{t("shops.list.noShops")}</p>
        )}
        <ul className={styles.shopList}>
          {shops?.map((shop) => (
            <li key={shop.id}>
              <Link to={`/shops/${shop.id}`} className={styles.shopLink}>
                {shop.name}
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </Layout>
  );
}
