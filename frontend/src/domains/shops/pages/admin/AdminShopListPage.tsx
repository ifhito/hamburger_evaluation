import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAdminShops, useShopModeration } from "../../hooks/useShopMutations";
import type { ShopStatus } from "../../api/types";
import { ApiError } from "../../../../api/client/buildApiClient";
import { useMeta } from "../../../../api/meta";
import { Alert } from "../../../../components/ui/Alert";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { LinkButton } from "../../../../components/ui/LinkButton";
import { EmptyState, Loading } from "../../../../components/ui/states";
import { TextArea } from "../../../../components/ui/TextField";
import { Layout } from "../../../../components/Layout";
import styles from "./adminShops.module.css";

const STATUS_FILTERS: (ShopStatus | "all")[] = ["all", "pending", "active", "rejected"];

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
    <Layout>
      <div className={styles.container}>
        <div className={styles.head}>
          <h1 className={styles.title}>{t("shops.admin.title")}</h1>
          {shops && <span className={styles.count}>{t("shops.admin.count", { count: shops.length })}</span>}
        </div>
        <div className={styles.filters} role="group" aria-label={t("shops.admin.filterLabel")}>
          {STATUS_FILTERS.map((s) => (
            <Button
              key={s}
              type="button"
              variant={filter === s ? "dark" : "secondary"}
              aria-pressed={filter === s}
              onClick={() => setFilter(s)}
            >
              {t(`shops.admin.filter.${s}`)}
            </Button>
          ))}
        </div>

        {rejectError && <Alert title={t("shops.admin.rejectErrorTitle")} message={rejectError} />}
        {isLoading && <Loading />}
        {shops && shops.length === 0 &&
          (filter === "pending" ? (
            <EmptyState title={t("shops.admin.emptyPendingTitle")} description={t("shops.admin.emptyPendingDescription")} />
          ) : (
            <p className={styles.muted}>{t("shops.admin.empty")}</p>
          ))}

        <ul className={styles.list}>
          {shops?.map((shop) => (
            <li key={shop.id}>
              <Card>
                <div className={styles.row}>
                  <div className={styles.rowHead}>
                    <h2 className={styles.name}>{shop.name}</h2>
                    <Badge tone={shop.status === "active" ? "accent" : "outline"}>{t(`shops.status.${shop.status}`)}</Badge>
                  </div>
                  <p className={styles.meta}>
                    {t("shops.admin.creator")}: {shop.creator?.username ?? "—"}
                  </p>
                  {shop.moderationNote && (
                    <p className={styles.note}>
                      <b>{t("shops.admin.note")}:</b> {shop.moderationNote}
                    </p>
                  )}
                  {rejectingId === shop.id ? (
                    <div className={styles.rejectForm}>
                      <TextArea
                        short
                        id={`note-${shop.id}`}
                        label={t("shops.admin.noteLabel")}
                        optional={t("shops.admin.optional")}
                        value={note}
                        onChange={(e) => setNote(e.target.value)}
                        counter={{ value: note, max: textLimits?.moderationNoteMaxChars }}
                        placeholder={t("shops.admin.notePlaceholder")}
                      />
                      <div className={styles.actions}>
                        <Button type="button" variant="dark" onClick={() => void handleReject(shop.id)} isLoading={busyId === shop.id}>
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
                    </div>
                  ) : (
                    <div className={styles.actions}>
                      <LinkButton to={`/admin/shops/${shop.id}/edit`}>{t("shops.admin.edit")}</LinkButton>
                      {/* 承認・却下を出すかは、ショップごとに backend が返す(canApprove・canReject)。状態からは決めない */}
                      {shop.canApprove && (
                        <Button type="button" onClick={() => void handleApprove(shop.id)} isLoading={busyId === shop.id}>
                          {t("shops.admin.approve")}
                        </Button>
                      )}
                      {shop.canReject && (
                        <Button
                          type="button"
                          variant="secondary"
                          onClick={() => {
                            setRejectingId(shop.id);
                            setNote("");
                            setRejectError(null);
                          }}
                        >
                          {t("shops.admin.reject")}
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              </Card>
            </li>
          ))}
        </ul>
      </div>
    </Layout>
  );
}
