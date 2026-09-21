import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../../../api/client/buildApiClient";
import { formatDate } from "../../../lib/date";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
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
      {isLoading && <p className={styles.muted}>{t("oauth.apps.loading")}</p>}
      {error && <ErrorMessage message={t("oauth.apps.loadError")} />}
      {revokeError && <ErrorMessage message={revokeError} />}
      {data && data.length === 0 && <p className={styles.muted}>{t("oauth.apps.empty")}</p>}
      {data && data.length > 0 && (
        <ul className={styles.list}>
          {data.map((app) => (
            <li key={app.id} className={styles.item}>
              <div>
                {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
                <p className={styles.name}>{app.clientName}</p>
                <ul className={styles.scopes}>
                  {app.scopes.map((scope) => (
                    <li key={scope.name}>{scope.description}</li>
                  ))}
                </ul>
                <p className={styles.meta}>{t("oauth.apps.connectedOn", { date: formatDate(app.createdAt) })}</p>
              </div>
              <Button
                type="button"
                variant="danger"
                isLoading={revokingId === app.id}
                onClick={() => void onRevoke(app.id, app.clientName)}
              >
                {t("oauth.apps.revoke")}
              </Button>
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
