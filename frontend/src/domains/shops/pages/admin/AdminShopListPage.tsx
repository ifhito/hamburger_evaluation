import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAdminShops, useShopModeration } from "../../hooks/useShopMutations";
import type { ShopStatus } from "../../api/types";
import { ApiError } from "../../../../api/client/buildApiClient";
import { useMeta } from "../../../../api/meta";
import { Button } from "../../../../components/Button";
import { ErrorMessage } from "../../../../components/ErrorMessage";
import { Input } from "../../../../components/Input";
import { Layout } from "../../../../components/Layout";
import styles from "./adminShops.module.css";

const STATUS_FILTERS: (ShopStatus | "all")[] = ["all", "pending", "active", "rejected"];

function badgeClass(status: ShopStatus): string {
  if (status === "pending") return styles.badgePending;
  if (status === "active") return styles.badgeActive;
  return styles.badgeRejected;
}

export default function AdminShopListPage() {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<ShopStatus | "all">("all");
  const { data: shops, isLoading } = useAdminShops(filter === "all" ? undefined : filter);
  const { approve, reject } = useShopModeration();
  const textLimits = useMeta().data?.text;

  const [rejectingId, setRejectingId] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);
  // 却下の失敗(例: note が長すぎる 422)は、サーバーのメッセージをそのまま表示する
  const [rejectError, setRejectError] = useState<string | string[] | null>(null);

  const handleApprove = async (id: string) => {
    setBusyId(id);
    try {
      await approve(id);
    } finally {
      setBusyId(null);
    }
  };

  const handleReject = async (id: string) => {
    setBusyId(id);
    setRejectError(null);
    try {
      await reject(id, note);
      setRejectingId(null);
      setNote("");
    } catch (e) {
      setRejectError(e instanceof ApiError ? e.messages : t("shops.admin.rejectError"));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <Layout title={t("shops.admin.title")}>
      <div className={styles.container}>
        <div className={styles.filters}>
          {STATUS_FILTERS.map((s) => (
            <Button
              key={s}
              type="button"
              variant={filter === s ? "primary" : "secondary"}
              onClick={() => setFilter(s)}
            >
              {t(`shops.admin.filter.${s}`)}
            </Button>
          ))}
        </div>

        {isLoading && <p className={styles.muted}>{t("shops.admin.loading")}</p>}
        {shops && shops.length === 0 && (
          <p className={styles.muted}>{t("shops.admin.empty")}</p>
        )}

        <ul className={styles.list}>
          {shops?.map((shop) => (
            <li key={shop.id} className={styles.row}>
              <div className={styles.rowHeader}>
                <strong>{shop.name}</strong>
                <span className={`${styles.badge} ${badgeClass(shop.status)}`}>
                  {t(`shops.status.${shop.status}`)}
                </span>
              </div>
              <span className={styles.note}>
                {t("shops.admin.creator")}: {shop.creator?.username ?? "—"}
              </span>
              {shop.moderationNote && (
                <span className={styles.note}>
                  {t("shops.admin.note")}: {shop.moderationNote}
                </span>
              )}
              <div className={styles.actions}>
                {shop.canApprove && (
                  <Button
                    type="button"
                    onClick={() => void handleApprove(shop.id)}
                    isLoading={busyId === shop.id}
                  >
                    {t("shops.admin.approve")}
                  </Button>
                )}
                {shop.canReject && rejectingId !== shop.id && (
                  <Button
                    type="button"
                    variant="danger"
                    onClick={() => {
                      setRejectingId(shop.id);
                      setNote("");
                      setRejectError(null);
                    }}
                  >
                    {t("shops.admin.reject")}
                  </Button>
                )}
                <Link to={`/admin/shops/${shop.id}/edit`}>
                  <Button type="button" variant="secondary">
                    {t("shops.admin.edit")}
                  </Button>
                </Link>
              </div>
              {rejectingId === shop.id && (
                <div className={styles.rejectBox}>
                  {rejectError && <ErrorMessage message={rejectError} />}
                  <Input
                    id={`note-${shop.id}`}
                    label={t("shops.admin.noteLabel")}
                    type="text"
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    counter={{ value: note, max: textLimits?.moderationNoteMaxChars }}
                    placeholder={t("shops.admin.notePlaceholder")}
                  />
                  <Button
                    type="button"
                    variant="danger"
                    onClick={() => void handleReject(shop.id)}
                    isLoading={busyId === shop.id}
                  >
                    {t("shops.admin.confirmReject")}
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => {
                      setRejectingId(null);
                      setRejectError(null);
                    }}
                  >
                    {t("shops.admin.cancel")}
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      </div>
    </Layout>
  );
}
