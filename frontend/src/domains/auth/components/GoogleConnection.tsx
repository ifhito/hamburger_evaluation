import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { authApi } from "../api/authApiClient";
import { isNavigable } from "../../oauth/navigation";
import { GOOGLE_PROVIDER, googleEnabled } from "../googleFlow";
import { useIdentities } from "../hooks/useIdentities";
import type { Identity } from "../types";
import section from "../../../components/profileSection.module.css";
import styles from "./googleConnection.module.css";

// 連携の状態。一覧を取得できたとき(loaded)だけ、連携の有無と操作を出す(取得中・取得の失敗を、「未連携」と混ぜない)。
// loaded の identity は、結び付いていればその内容、なければ null。
type ConnectionState = { kind: "loading" } | { kind: "failed" } | { kind: "loaded"; identity: Identity | null };

// 操作の失敗。見出しは、どの操作かに応じた画面の言葉、messages は、API が返した文言(なければ画面側の文言)。
type ActionError = { title: string; messages: string[] };

interface ViewProps {
  state: ConnectionState;
  actionError: ActionError | null;
  // 解除に成功したことの知らせ。
  disconnected: boolean;
  // 取得を取り直している間(失敗の表示の Retry を、処理中にする)。
  retrying: boolean;
  busy: "connect" | "disconnect" | null;
  onConnect: () => void;
  onDisconnect: () => void;
  onRetry: () => void;
}

// プロフィールの Google の連携の見た目。何が押せるかは、backend が返す canUnlink に従う(解除してよいかの判断は持たない)。
function GoogleConnectionView({ state, actionError, disconnected, retrying, busy, onConnect, onDisconnect, onRetry }: ViewProps) {
  const { t } = useTranslation();
  const identity = state.kind === "loaded" ? state.identity : null;
  return (
    <section className={section.section}>
      <h2 className={section.heading}>{t("auth.google.profile.heading")}</h2>
      {actionError && (
        <div className={styles.alert}>
          <Alert title={actionError.title} message={actionError.messages} />
        </div>
      )}
      {/* 知らせの領域は、知らせが出る前から画面に置く(あとから中身ごと挿入すると、スクリーンリーダーが読み上げないことがある) */}
      <p role="status" className={styles.notice}>
        {disconnected ? t("auth.google.profile.disconnected") : ""}
      </p>
      {state.kind === "loading" && <p className={styles.muted}>{t("common.loading")}</p>}
      {state.kind === "failed" && (
        <>
          <Alert title={t("auth.google.profile.loadErrorTitle")} message={t("auth.google.profile.loadError")} />
          <div className={styles.retry}>
            <Button type="button" variant="secondary" isLoading={retrying} onClick={onRetry}>
              {t("auth.google.profile.retry")}
            </Button>
          </div>
        </>
      )}
      {state.kind === "loaded" && (
        <Card>
          <div className={styles.row}>
            <div className={styles.info}>
              <b className={styles.name}>{t("auth.google.profile.accountName")}</b>
              {identity ? (
                <>
                  {/* メールは Google が返した文字列なので、HTML として解釈せず、文字として描画する */}
                  <p className={styles.meta}>{t("auth.google.profile.connectedAs", { email: identity.email })}</p>
                  {!identity.canUnlink && <p className={styles.reason}>{t("auth.google.profile.cannotUnlink")}</p>}
                </>
              ) : (
                <p className={styles.meta}>{t("auth.google.profile.notConnected")}</p>
              )}
            </div>
            {identity?.canUnlink && (
              <Button type="button" variant="danger" isLoading={busy === "disconnect"} onClick={onDisconnect}>
                {t("auth.google.profile.disconnect")}
              </Button>
            )}
            {!identity && (
              <Button type="button" variant="secondary" isLoading={busy === "connect"} onClick={onConnect}>
                {t("auth.google.profile.connect")}
              </Button>
            )}
          </div>
        </Card>
      )}
    </section>
  );
}

// 本人のプロフィールに出す、Google アカウントの結び付けと解除。GET /meta が、Google を使えると返したときだけ出す
// (使えない・取得できていない間は何も出さない)。結び付けは、認証つきの POST で、このブラウザに手続きの cookie を
// 設定して始め、返された Google の URL へ、ブラウザが移動する(戻ってきたら、このプロフィールへ戻る)。
// navigateTo は、返された Google の URL へブラウザを移動する処理(テストで差し替えるため、引数にしている)。
export function GoogleConnection({
  viewerId,
  navigateTo = (url) => window.location.assign(url),
}: {
  viewerId: string;
  navigateTo?: (url: string) => void;
}) {
  const { t } = useTranslation();
  const enabled = googleEnabled(useMeta().data);
  const { identities, error, isValidating, refresh, removeProvider } = useIdentities(viewerId, enabled);
  const [busy, setBusy] = useState<"connect" | "disconnect" | null>(null);
  const [actionError, setActionError] = useState<ActionError | null>(null);
  const [disconnected, setDisconnected] = useState(false);

  // Google の画面から「戻る」で戻ると、ブラウザが、画面の状態ごとページを復元する(bfcache)ことがある。移動する直前の
  // 「処理中」のままだと、再読み込みするまで結び付けをやり直せないので、復元されたときは、処理中を戻す。
  useEffect(() => {
    const onPageShow = (e: Event) => {
      if ((e as PageTransitionEvent).persisted) setBusy(null);
    };
    window.addEventListener("pageshow", onPageShow);
    return () => window.removeEventListener("pageshow", onPageShow);
  }, []);

  if (!enabled) return null;
  // 一覧を取得できたときだけ、連携の有無を決める(取得できていないのに、「未連携」にしない)。
  const state: ConnectionState = identities
    ? { kind: "loaded", identity: identities.find((i) => i.provider === GOOGLE_PROVIDER) ?? null }
    : error
      ? { kind: "failed" } // 取り直している間も、失敗の表示のまま(「読み込み中」に切り替えて、Retry を消さない)
      : { kind: "loading" };

  const onConnect = async () => {
    setBusy("connect");
    setActionError(null);
    setDisconnected(false);
    try {
      // 手続きの cookie は、この POST の応答で、このブラウザに設定される。返された Google の URL へ、同じブラウザで移動する。
      const { redirectUrl } = await authApi.startGoogleLink(`/users/${viewerId}`);
      if (!isNavigable(redirectUrl)) throw new Error("unexpected redirect URL");
      navigateTo(redirectUrl);
    } catch (e) {
      setActionError({
        title: t("auth.google.profile.connectErrorTitle"),
        messages: e instanceof ApiError ? e.messages : [t("auth.google.profile.connectError")],
      });
      setBusy(null);
    }
  };

  const onDisconnect = async () => {
    if (!window.confirm(t("auth.google.profile.disconnectConfirm"))) return;
    setBusy("disconnect");
    setActionError(null);
    setDisconnected(false);
    try {
      await authApi.unlinkGoogle();
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) {
        setActionError({
          title: t("auth.google.profile.disconnectErrorTitle"),
          messages: e instanceof ApiError ? e.messages : [t("auth.google.profile.disconnectError")],
        });
        setBusy(null);
        return;
      }
      // 404: 別のタブなどで、すでに解除済みかもしれない。ただし、Google の機能が止まっているときの 404 も、同じ形になる。
      // 取り直した一覧で確かめ、Google の連携が(取り直せなかったときも)残っていれば、解除できたとは言わない。
      const fresh = await refresh();
      if (!fresh || fresh.identities.some((i) => i.provider === GOOGLE_PROVIDER)) {
        setActionError({ title: t("auth.google.profile.disconnectErrorTitle"), messages: [t("auth.google.profile.disconnectError")] });
      } else {
        setDisconnected(true);
      }
      setBusy(null);
      return;
    }
    // 解除は済んだ。このあとの再取得の成否は、解除の成否とは別(失敗しても、解除できたことは変わらない)。
    await removeProvider(GOOGLE_PROVIDER);
    setDisconnected(true);
    setBusy(null);
    void refresh();
  };

  return (
    <GoogleConnectionView
      state={state}
      actionError={actionError}
      disconnected={disconnected}
      retrying={isValidating}
      busy={busy}
      onConnect={() => void onConnect()}
      onDisconnect={() => void onDisconnect()}
      onRetry={() => void refresh()}
    />
  );
}
