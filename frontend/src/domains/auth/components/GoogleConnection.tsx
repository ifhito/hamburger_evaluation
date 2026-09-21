import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { authApi } from "../api/authApiClient";
import { GOOGLE_PROVIDER, googleEnabled, isNavigableUrl } from "../googleFlow";
import { useIdentities } from "../hooks/useIdentities";
import type { Identity } from "../types";
import styles from "./googleConnection.module.css";

interface ViewProps {
  // 結び付いていれば、その内容。なければ null。
  identity: Identity | null;
  loadFailed: boolean;
  actionError: string[] | null;
  busy: "connect" | "disconnect" | null;
  onConnect: () => void;
  onDisconnect: () => void;
}

// プロフィールの Google の連携の見た目。何が押せるかは、backend が返す canUnlink に従う(解除してよいかの判断は持たない)。
export function GoogleConnectionView({ identity, loadFailed, actionError, busy, onConnect, onDisconnect }: ViewProps) {
  const { t } = useTranslation();
  return (
    <section className={styles.section}>
      <h2 className={styles.heading}>{t("auth.google.profile.heading")}</h2>
      {loadFailed && <ErrorMessage message={t("auth.google.profile.loadError")} />}
      {actionError && <ErrorMessage message={actionError} />}
      {identity ? (
        <div className={styles.row}>
          <div>
            {/* メールは Google が返した文字列なので、HTML として解釈せず、文字として描画する */}
            <p className={styles.email}>{t("auth.google.profile.connectedAs", { email: identity.email })}</p>
            {!identity.canUnlink && <p className={styles.muted}>{t("auth.google.profile.cannotUnlink")}</p>}
          </div>
          {identity.canUnlink && (
            <Button type="button" variant="secondary" isLoading={busy === "disconnect"} onClick={onDisconnect}>
              {t("auth.google.profile.disconnect")}
            </Button>
          )}
        </div>
      ) : (
        <div className={styles.row}>
          <p className={styles.muted}>{t("auth.google.profile.notConnected")}</p>
          <Button type="button" variant="secondary" isLoading={busy === "connect"} onClick={onConnect}>
            {t("auth.google.profile.connect")}
          </Button>
        </div>
      )}
    </section>
  );
}

// 本人のプロフィールに出す、Google アカウントの結び付けと解除。GET /meta が、Google を使えると返したときだけ出す
// (使えない・取得できていない間は何も出さない)。結び付けは、認証つきの POST で、このブラウザに手続きの cookie を
// 設定して始め、返された Google の URL へ、ブラウザが移動する(戻ってきたら、このプロフィールへ戻る)。
export function GoogleConnection({ viewerId }: { viewerId: string }) {
  const { t } = useTranslation();
  const enabled = googleEnabled(useMeta().data);
  const { identities, error, isLoading, refresh } = useIdentities(viewerId, enabled);
  const [busy, setBusy] = useState<"connect" | "disconnect" | null>(null);
  const [actionError, setActionError] = useState<string[] | null>(null);

  if (!enabled || isLoading) return null;
  const identity = identities?.find((i) => i.provider === GOOGLE_PROVIDER) ?? null;

  const onConnect = async () => {
    setBusy("connect");
    setActionError(null);
    try {
      // 手続きの cookie は、この POST の応答で、このブラウザに設定される。返された Google の URL へ、同じブラウザで移動する。
      const { redirectUrl } = await authApi.startGoogleLink(`/users/${viewerId}`);
      if (!isNavigableUrl(redirectUrl)) throw new Error("unexpected redirect URL");
      window.location.assign(redirectUrl);
    } catch (e) {
      setActionError(e instanceof ApiError ? e.messages : [t("auth.google.profile.connectError")]);
      setBusy(null);
    }
  };

  const onDisconnect = async () => {
    if (!window.confirm(t("auth.google.profile.disconnectConfirm"))) return;
    setBusy("disconnect");
    setActionError(null);
    try {
      await authApi.unlinkGoogle();
      await refresh();
    } catch (e) {
      setActionError(e instanceof ApiError ? e.messages : [t("auth.google.profile.disconnectError")]);
    } finally {
      setBusy(null);
    }
  };

  return (
    <GoogleConnectionView
      identity={identity}
      loadFailed={error !== undefined}
      actionError={actionError}
      busy={busy}
      onConnect={() => void onConnect()}
      onDisconnect={() => void onDisconnect()}
    />
  );
}
