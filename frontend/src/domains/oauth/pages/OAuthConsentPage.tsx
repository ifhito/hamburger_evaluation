import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import { oauthApi } from "../api/oauthApiClient";
import { phaseFor, redirectPhase, searchToDecide, startConsent, NotNavigableError, type ConsentState } from "../consentFlow";
import { hostOf } from "../navigation";
import styles from "./oauthConsent.module.css";

// アプリへ戻る(ブラウザを、backend が返した戻り先へ移す)。開けない戻り先(http(s) 以外)は開かず、失敗にする。
function moveToApp(setState: Dispatch<SetStateAction<ConsentState | null>>, search: string, target: string) {
  const next = redirectPhase(target);
  setState({ search, phase: next });
  if (next.status === "redirecting") window.location.assign(target);
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

  const decide = async (approve: boolean) => {
    // 送るのは、いま画面に内容を見せている要求だけ(確認中や、前の要求の画面では何も送らない)。
    const target = searchToDecide(state, search);
    if (target === null) return;
    setDecidingSearch(target);
    try {
      moveToApp(setState, target, (await oauthApi.decide(target, approve)).redirectTo);
    } catch (e) {
      setState({ search: target, phase: { status: "failed", error: e } });
    } finally {
      setDecidingSearch(null);
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
    <Layout title={t("oauth.consent.title")}>
      {phase.status === "loading" && <p className={styles.muted}>{t("oauth.consent.loading")}</p>}
      {phase.status === "redirecting" && <p className={styles.muted}>{t("oauth.consent.connecting")}</p>}
      {phase.status === "failed" && (
        <>
          <ErrorMessage message={failureMessages(phase.error)} />
          <p className={styles.muted}>{t("oauth.consent.loadError")}</p>
        </>
      )}
      {phase.status === "asking" && (
        <div className={styles.card}>
          {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
          <p className={styles.intro}>{t("oauth.consent.intro", { name: phase.view.client.name })}</p>
          <p className={styles.meta}>{t("oauth.consent.appId", { id: phase.view.client.id })}</p>
          <div>
            <p className={styles.heading}>{t("oauth.consent.permissionsHeading")}</p>
            <ul className={styles.scopes}>
              {phase.view.scopes.map((scope) => (
                <li key={scope.name}>{scope.description}</li>
              ))}
            </ul>
          </div>
          {returnHost && <p className={styles.meta}>{t("oauth.consent.returnsTo", { host: returnHost })}</p>}
          {user && <p className={styles.signedIn}>{t("oauth.consent.signedInAs", { name: user.username })}</p>}
          <div className={styles.actions}>
            <Button type="button" isLoading={isDeciding} onClick={() => void decide(true)}>
              {t("oauth.consent.allow")}
            </Button>
            <Button type="button" variant="secondary" disabled={isDeciding} onClick={() => void decide(false)}>
              {t("oauth.consent.deny")}
            </Button>
          </div>
        </div>
      )}
    </Layout>
  );
}
