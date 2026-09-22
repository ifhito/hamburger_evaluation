import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAdminShops, useUpdateShop } from "../../hooks/useShopMutations";
import { useShopForm } from "../../hooks/useShopForm";
import { ApiError } from "../../../../api/client/buildApiClient";
import { useMeta } from "../../../../api/meta";
import { Alert } from "../../../../components/ui/Alert";
import { Button } from "../../../../components/ui/Button";
import { LinkButton } from "../../../../components/ui/LinkButton";
import { Loading } from "../../../../components/ui/states";
import { TextField } from "../../../../components/ui/TextField";
import { TextLink } from "../../../../components/ui/TextLink";
import { Layout } from "../../../../components/Layout";
import styles from "./adminShopEdit.module.css";

export default function AdminShopEditPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const shopId = id ?? "";

  const { data: shops, isLoading } = useAdminShops();
  const shop = shops?.find((s) => s.id === shopId);

  const { update } = useUpdateShop(shopId);
  const { register, handleSubmit, reset, watch } = useShopForm();
  const textLimits = useMeta().data?.text;

  useEffect(() => {
    if (shop) reset({ name: shop.name });
  }, [shop, reset]);

  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const onSubmit = handleSubmit(async (data) => {
    setServerError(null);
    setIsSubmitting(true);
    try {
      await update(data);
      void navigate("/admin/shops");
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("shops.admin.editError")]);
    } finally {
      setIsSubmitting(false);
    }
  });

  return (
    <Layout>
      <div className={styles.column}>
        <TextLink to="/admin/shops">{t("shops.admin.backToList")}</TextLink>
        <h1 className={styles.title}>{t("shops.admin.editTitle")}</h1>
        {isLoading ? (
          <Loading />
        ) : (
          <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
            {serverError && <Alert title={t("shops.admin.editErrorTitle")} message={serverError} />}
            <TextField
              id="name"
              label={t("shops.new.name")}
              type="text"
              counter={{ value: watch("name"), max: textLimits?.shopNameMaxChars }}
              {...register("name")}
            />
            <div className={styles.actions}>
              <Button type="submit" wide isLoading={isSubmitting}>
                {t("shops.admin.save")}
              </Button>
              <LinkButton to="/admin/shops">{t("shops.admin.cancel")}</LinkButton>
            </div>
          </form>
        )}
      </div>
    </Layout>
  );
}
