import { useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { ApiError } from "../../../api/client/buildApiClient";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { RatingBurgerIcon } from "../../../components/ui/RatingBurger";
import { Layout } from "../../../components/Layout";
import { oauthApi } from "../api/oauthApiClient";
import { phaseFor, redirectPhase, searchToDecide, startConsent, NotNavigableError, type ConsentState } from "../consentFlow";
import { hostOf } from "../navigation";
import { ScopeList } from "../components/ScopeList";
import styles from "./oauthConsent.module.css";

// 水位の絵は装飾(見た目だけの進み具合。評価ではない)。確認中より、接続中(移動の直前)を多めに塗る。
const CHECKING_LEVEL = 0.6;
const CONNECTING_LEVEL = 0.9;

// アプリへ戻る(ブラウザを、backend が返した戻り先へ移す)。開けない戻り先(http(s) 以外)は開かず、失敗にする。
function moveToApp(setState: Dispatch<SetStateAction<ConsentState | null>>, search: string, target: string) {
  const next = redirectPhase(target);
  setState({ search, phase: next });
  if (next.status === "redirecting") window.location.assign(target);
}

// 「確認しています…」「接続しています…」の状態画面。水位の絵は装飾(読み上げない)。見出しだけ読み上げに伝える。
function StatusView({ ratio, title, description }: { ratio: number; title: string; description: string }) {
  return (
    <div className={styles.status}>
      <RatingBurgerIcon ratio={ratio} size="lg" />
      <h1 aria-live="polite" className={styles.statusTitle}>
        {title}
      </h1>
      <p className={styles.muted}>{description}</p>
    </div>
  );
}

// AI アプリが、利用者のログインと許可だけでこのアプリにつなぐための、許可を尋ねる画面。
// アプリの認可の URL(backend の GET /oauth/authorize)から、同じ値(クエリ)のまま渡されてくる。
// 未ログインなら、ProtectedRoute がログイン画面へ送り、ログイン後にここへ戻す。
// 要求の検証、範囲の説明、尋ねる必要があるか(すでに許可済みの範囲に収まるか)の判断は、すべて backend が行い、
// frontend は、その結果の表示と、利用者の選択の送信だけを行う。
//
// 状態は、それを作った要求(URL の query。search)に結び付ける(consentFlow.ts)。同じ画面のまま query だけが
// 変わったとき(履歴を戻る・進むなど)に、前のアプリの内容が残ったまま、新しいアプリへの許可を送らない。
export default function OAuthConsentPage() {
  const { t } = useTranslation();
  const { search } = useLocation();
  const { user } = useAuth();
  const [state, setState] = useState<ConsentState | null>(null);
  // 許可・拒否を送っている最中の要求。この要求の画面でだけ、ボタンを押せなくする。
  const [decidingSearch, setDecidingSearch] = useState<string | null>(null);
  const phase = phaseFor(state, search);
  // 戻り先のホスト名は、利用者が「どのアプリ・どこへ」を見比べられるよう、表示のためだけに、要求の値から読む
  // (要求の検証は backend が行う。ここでは判断に使わない)。
  const returnHost = hostOf(new URLSearchParams(search).get("redirect_uri"));

  // 画面を離れたあとに、許可・拒否の応答が返っても、勝手に移動させない・表示を更新しない。
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const decide = async (approve: boolean) => {
    // 送るのは、いま画面に内容を見せている要求だけ(確認中や、前の要求の画面では何も送らない)。
    const target = searchToDecide(state, search);
    if (target === null) return;
    setDecidingSearch(target);
    try {
      const { redirectTo } = await oauthApi.decide(target, approve);
      if (mounted.current) moveToApp(setState, target, redirectTo);
    } catch (e) {
      if (mounted.current) setState({ search: target, phase: { status: "failed", error: e } });
    } finally {
      if (mounted.current) setDecidingSearch(null);
    }
  };

  // URL(search)が変わったら、前の要求の取得を取り消し、新しい要求の取得を始める。
  useEffect(
    () =>
      startConsent(search, oauthApi, {
        asking: (view) => setState({ search, phase: { status: "asking", view } }),
        redirect: (target) => moveToApp(setState, search, target),
        failed: (error) => setState({ search, phase: { status: "failed", error } }),
      }),
    [search],
  );

  const failureMessages = (error: unknown): string[] =>
    error instanceof NotNavigableError ? [t("oauth.consent.cannotOpen")] : error instanceof ApiError ? error.messages : [t("oauth.consent.decideError")];
  const isDeciding = decidingSearch === search;

  return (
    <Layout>
      <div className={styles.consent}>
        {phase.status === "loading" && (
          <StatusView ratio={CHECKING_LEVEL} title={t("oauth.consent.loadingTitle")} description={t("oauth.consent.loading")} />
        )}
        {phase.status === "redirecting" && (
          <StatusView ratio={CONNECTING_LEVEL} title={t("oauth.consent.connectingTitle")} description={t("oauth.consent.connecting")} />
        )}
        {phase.status === "failed" && (
          <>
            <p className={styles.eyebrow}>{t("oauth.consent.eyebrow")}</p>
            <h1 className={styles.heading}>{t("oauth.consent.failedHeading")}</h1>
            <Alert title={t("oauth.consent.cannotCompleteTitle")} message={failureMessages(phase.error)} />
            <p className={styles.below}>{t("oauth.consent.tryAgain")}</p>
          </>
        )}
        {phase.status === "asking" && (
          <>
            <p className={styles.eyebrow}>{t("oauth.consent.eyebrow")}</p>
            <h1 className={styles.heading}>{t("oauth.consent.heading")}</h1>

            <div className={styles.appbox}>
              <span className={styles.avatar} aria-hidden="true">
                {[...phase.view.client.name][0]}
              </span>
              <div className={styles.appboxInfo}>
                {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
                <p className={styles.appName}>{phase.view.client.name}</p>
                <p className={styles.appId}>{t("oauth.consent.appId", { id: phase.view.client.id })}</p>
              </div>
            </div>

            <section className={styles.perm}>
              <h2 className={styles.permHeading}>{t("oauth.consent.permissionsHeading")}</h2>
              <ScopeList scopes={phase.view.scopes} className={styles.scopes} />
            </section>

            <div className={styles.facts}>
              {returnHost && (
                <span>
                  <b>{t("oauth.consent.returnsToLabel")}</b> {returnHost}
                </span>
              )}
              {user && (
                <span>
                  {t("oauth.consent.signedInAsLabel")} <b>{user.username}</b>
                </span>
              )}
            </div>

            <div className={styles.actions}>
              <Button type="button" wide isLoading={isDeciding} onClick={() => void decide(true)}>
                {t("oauth.consent.allow")}
              </Button>
              <Button type="button" variant="secondary" wide disabled={isDeciding} onClick={() => void decide(false)}>
                {t("oauth.consent.deny")}
              </Button>
            </div>
            <p className={styles.hint}>{t("oauth.consent.disconnectHint")}</p>
          </>
        )}
      </div>
    </Layout>
  );
}
