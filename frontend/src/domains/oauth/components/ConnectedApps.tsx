import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../../../api/client/buildApiClient";
import { formatDate } from "../../../lib/date";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { Loading } from "../../../components/ui/states";
import { useConnectedApps } from "../hooks/useConnectedApps";
import { ScopeList } from "./ScopeList";
import section from "../../../components/profileSection.module.css";
import styles from "./connectedApps.module.css";

// 利用者が許可した AI アプリの一覧と、取り消し。本人のプロフィールにだけ出す(呼び出し側が canEdit で出し分ける)。
// 取り消すと、そのアプリのトークンは、backend がすぐ使えなくする。OAuth の認可サーバーが無効なとき(API が 404)は、何も出さない。
export function ConnectedApps({ viewerId }: { viewerId: string }) {
  const { t } = useTranslation();
  const { data, error, isLoading, isDisabled, hasNextPage, fetchNextPage, isFetchingNextPage, revoke } = useConnectedApps(viewerId);
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [revokeError, setRevokeError] = useState<string[] | null>(null);

  if (isDisabled) return null;

  const onRevoke = async (id: string, name: string) => {
    if (!window.confirm(t("oauth.apps.revokeConfirm", { name }))) return;
    setRevokingId(id);
    setRevokeError(null);
    try {
      await revoke(id);
    } catch (e) {
      setRevokeError(e instanceof ApiError ? e.messages : [t("oauth.apps.revokeError")]);
    } finally {
      setRevokingId(null);
    }
  };

  return (
    <section className={section.section}>
      <div className={section.sectionHead}>
        <h2 className={section.heading}>{t("oauth.apps.heading")}</h2>
      </div>
      {revokeError && <Alert title={t("oauth.apps.revokeErrorTitle")} message={revokeError} />}
      {isLoading && <Loading />}
      {error && <Alert title={t("oauth.apps.loadErrorTitle")} message={t("oauth.apps.loadError")} />}
      {data && data.length === 0 && <p className={styles.muted}>{t("oauth.apps.empty")}</p>}
      {data && data.length > 0 && (
        <div className={styles.list}>
          {data.map((app) => (
            <article key={app.id} className={styles.appitem}>
              <div className={styles.info}>
                {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
                <b className={styles.name}>{app.clientName}</b>
                <ScopeList scopes={app.scopes} className={styles.scopes} />
                <p className={styles.meta}>{t("oauth.apps.connectedOn", { date: formatDate(app.createdAt) })}</p>
              </div>
              <Button type="button" variant="danger" isLoading={revokingId === app.id} onClick={() => void onRevoke(app.id, app.clientName)}>
                {t("oauth.apps.revoke")}
              </Button>
            </article>
          ))}
        </div>
      )}
      {hasNextPage && (
        <div className={styles.loadMore}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
            {t("oauth.apps.loadMore")}
          </Button>
        </div>
      )}
    </section>
  );
}
