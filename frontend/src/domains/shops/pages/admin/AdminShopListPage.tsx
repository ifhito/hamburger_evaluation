import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAdminShops, useShopModeration } from "../../hooks/useShopMutations";
import type { ShopStatus } from "../../api/types";
import { ApiError } from "../../../../api/client/buildApiClient";
import { useMeta } from "../../../../api/meta";
import { Alert } from "../../../../components/ui/Alert";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
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
  // 行ごとの処理中を、行の id の集合で持つ(単一の id だと、別の行の処理が、その行の処理中を消してしまう)
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());
  // 却下の失敗(例: note が長すぎる 422)は、サーバーのメッセージをそのまま表示する
  const [rejectError, setRejectError] = useState<string | string[] | null>(null);
  const [approveError, setApproveError] = useState<string | string[] | null>(null);

  // 絞り込みを変えると、開いていた却下の理由の欄(とその失敗の表示)は、対象のショップが一覧から消えることがあるので閉じる。
  const changeFilter = (s: ShopStatus | "all") => {
    setFilter(s);
    setRejectingId(null);
    setRejectError(null);
    setApproveError(null);
  };

  const addBusy = (id: string) => setBusyIds((prev) => new Set(prev).add(id));
  const removeBusy = (id: string) =>
    setBusyIds((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });

  const handleApprove = async (id: string) => {
    addBusy(id);
    setApproveError(null);
    try {
      await approve(id);
    } catch (e) {
      setApproveError(e instanceof ApiError ? e.messages : t("shops.admin.approveError"));
    } finally {
      removeBusy(id);
    }
  };

  const handleReject = async (id: string) => {
    addBusy(id);
    setRejectError(null);
    try {
      await reject(id, note);
      setRejectingId(null);
      setNote("");
    } catch (e) {
      setRejectError(e instanceof ApiError ? e.messages : t("shops.admin.rejectError"));
    } finally {
      removeBusy(id);
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
              onClick={() => changeFilter(s)}
            >
              {t(`shops.admin.filter.${s}`)}
            </Button>
          ))}
        </div>

        {rejectError && <Alert title={t("shops.admin.rejectErrorTitle")} message={rejectError} />}
        {approveError && <Alert title={t("shops.admin.approveErrorTitle")} message={approveError} />}
        {isLoading && <Loading />}
        {shops && shops.length === 0 &&
          (filter === "pending" ? (
            <EmptyState title={t("shops.admin.emptyPendingTitle")} description={t("shops.admin.emptyPendingDescription")} />
          ) : (
            <p className={styles.muted}>{t("shops.admin.empty")}</p>
          ))}

        <div className={styles.list}>
          {shops?.map((shop) => (
            <article key={shop.id} className={styles.row}>
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
                    <Button type="button" variant="dark" onClick={() => void handleReject(shop.id)} isLoading={busyIds.has(shop.id)}>
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
                    <Button type="button" onClick={() => void handleApprove(shop.id)} isLoading={busyIds.has(shop.id)}>
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
            </article>
          ))}
        </div>
      </div>
    </Layout>
  );
}
