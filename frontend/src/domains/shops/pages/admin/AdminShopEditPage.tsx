import { useEffect, useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAdminShops, useUpdateShop } from "../../hooks/useShopMutations";
import { useShopForm } from "../../hooks/useShopForm";
import { ApiError } from "../../../../api/client/buildApiClient";
import { useMeta } from "../../../../api/meta";
import { Button } from "../../../../components/Button";
import { ErrorMessage } from "../../../../components/ErrorMessage";
import { Input } from "../../../../components/Input";
import { Layout } from "../../../../components/Layout";
import styles from "../shopForm.module.css";

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

  if (isLoading) {
    return (
      <Layout title={t("shops.admin.editTitle")}>
        <p>{t("shops.admin.loading")}</p>
      </Layout>
    );
  }

  return (
    <Layout title={t("shops.admin.editTitle")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="name"
          label={t("shops.new.name")}
          type="text"
          counter={{ value: watch("name"), max: textLimits?.shopNameMaxChars }}
          {...register("name")}
        />
        <div className={styles.actions}>
          <Button type="submit" isLoading={isSubmitting}>
            {t("shops.admin.save")}
          </Button>
          <Link to="/admin/shops">
            <Button type="button" variant="secondary">
              {t("shops.admin.cancel")}
            </Button>
          </Link>
        </div>
      </form>
    </Layout>
  );
}
