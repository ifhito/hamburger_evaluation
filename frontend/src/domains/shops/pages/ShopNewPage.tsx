import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useCreateShop } from "../hooks/useShopMutations";
import { useShopForm } from "../hooks/useShopForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./shopForm.module.css";

export default function ShopNewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { create } = useCreateShop();
  const { register, handleSubmit, watch } = useShopForm();
  const textLimits = useMeta().data?.text;

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
    <Layout title={t("shops.new.title")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <p className={styles.notice}>{t("shops.new.pendingNotice")}</p>
        <Input
          id="name"
          label={t("shops.new.name")}
          type="text"
          placeholder={t("shops.new.namePlaceholder")}
          counter={{ value: watch("name"), max: textLimits?.shopNameMaxChars }}
          {...register("name")}
        />
        <div className={styles.actions}>
          <Button type="submit" isLoading={isSubmitting}>
            {t("shops.new.submit")}
          </Button>
          <Link to="/shops">
            <Button type="button" variant="secondary">
              {t("shops.new.cancel")}
            </Button>
          </Link>
        </div>
      </form>
    </Layout>
  );
}
