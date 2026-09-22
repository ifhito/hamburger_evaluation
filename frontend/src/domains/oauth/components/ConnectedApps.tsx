import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../../../api/client/buildApiClient";
import { formatDate } from "../../../lib/date";
import { Alert } from "../../../components/ui/Alert";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Loading } from "../../../components/ui/states";
import { useConnectedApps } from "../hooks/useConnectedApps";
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
      <h2 className={section.heading}>{t("oauth.apps.heading")}</h2>
      {revokeError && <Alert title={t("oauth.apps.revokeErrorTitle")} message={revokeError} />}
      {isLoading && <Loading />}
      {error && <Alert title={t("oauth.apps.loadErrorTitle")} message={t("oauth.apps.loadError")} />}
      {data && data.length === 0 && <p className={styles.muted}>{t("oauth.apps.empty")}</p>}
      {data && data.length > 0 && (
        <ul className={styles.list}>
          {data.map((app) => (
            <li key={app.id}>
              <Card>
                <div className={styles.row}>
                  <div className={styles.info}>
                    {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
                    <b className={styles.name}>{app.clientName}</b>
                    <ul className={styles.scopes}>
                      {app.scopes.map((scope) => (
                        <li key={scope.name} className={styles.scopeItem}>
                          <span>{scope.description}</span>
                          {/* 書き込みの範囲かは、API の印(writes)だけで決める(範囲の名前を比べない) */}
                          {scope.writes && <Badge tone="accent">{t("oauth.writeAccess")}</Badge>}
                        </li>
                      ))}
                    </ul>
                    <p className={styles.meta}>{t("oauth.apps.connectedOn", { date: formatDate(app.createdAt) })}</p>
                  </div>
                  <Button type="button" variant="danger" isLoading={revokingId === app.id} onClick={() => void onRevoke(app.id, app.clientName)}>
                    {t("oauth.apps.revoke")}
                  </Button>
                </div>
              </Card>
            </li>
          ))}
        </ul>
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
