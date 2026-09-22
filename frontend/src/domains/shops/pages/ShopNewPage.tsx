import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useCreateShop } from "../hooks/useShopMutations";
import { useShopForm } from "../hooks/useShopForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextField } from "../../../components/ui/TextField";
import { TextLink } from "../../../components/ui/TextLink";
import { Layout } from "../../../components/Layout";
import styles from "./shopForm.module.css";

// ショップの追加(design/redesign/shop-new.html)。名前の規則(空・長さ)の判定と文言は backend だけが持つ。
export default function ShopNewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { create } = useCreateShop();
  const { register, handleSubmit, watch } = useShopForm();
  const meta = useMeta().data;

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      const shop = await create(data);
      void navigate(`/shops/${shop.id}`);
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("shops.new.error")]);
    } finally {
      setIsSubmitting(false);
    }
  });

  return (
    <Layout>
      <TextLink to="/shops">{t("shops.new.backToShops")}</TextLink>
      <div className={styles.narrow}>
        <h1 className={styles.title}>{t("shops.new.title")}</h1>
        <p className={styles.lead}>{t("shops.new.lead")}</p>
        <form onSubmit={(e) => void onSubmit(e)} className={styles.form} noValidate>
          {serverError && <Alert title={t("shops.new.errorTitle")} message={serverError} />}
          <div className={styles.notice}>{t("shops.new.pendingNotice")}</div>
          <TextField
            id="name"
            label={t("shops.new.name")}
            placeholder={t("shops.new.namePlaceholder")}
            counter={{ value: watch("name"), max: meta?.text.shopNameMaxChars }}
            {...register("name")}
          />
          <div className={styles.actionsRow}>
            <Button type="submit" wide isLoading={isSubmitting}>
              {t("shops.new.submit")}
            </Button>
            <LinkButton to="/shops">{t("shops.new.cancel")}</LinkButton>
          </div>
        </form>
      </div>
    </Layout>
  );
}
