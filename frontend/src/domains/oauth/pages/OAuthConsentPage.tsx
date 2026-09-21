import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import { oauthApi } from "../api/oauthApiClient";
import type { AuthorizeRequestView } from "../api/types";
import { hostOf, isNavigable } from "../navigation";
import styles from "./oauthConsent.module.css";

type State =
  | { status: "loading" }
  | { status: "asking"; view: AuthorizeRequestView }
  | { status: "redirecting" }
  | { status: "failed"; messages: string[] };

// AI アプリが、利用者のログインと許可だけでこのアプリにつなぐための、許可を尋ねる画面。
// アプリの認可の URL(backend の GET /oauth/authorize)から、同じ値(クエリ)のまま渡されてくる。
// 未ログインなら、ProtectedRoute がログイン画面へ送り、ログイン後にここへ戻す。
// 要求の検証、範囲の説明、尋ねる必要があるか(すでに許可済みの範囲に収まるか)の判断は、すべて backend が行い、
// frontend は、その結果の表示と、利用者の選択の送信だけを行う。
export default function OAuthConsentPage() {
  const { t } = useTranslation();
  const { search } = useLocation();
  const { user } = useAuth();
  const [state, setState] = useState<State>({ status: "loading" });
  const [isDeciding, setIsDeciding] = useState(false);
  // 戻り先のホスト名は、利用者が「どのアプリ・どこへ」を見比べられるよう、表示のためだけに、要求の値から読む
  // (要求の検証は backend が行う。ここでは判断に使わない)。
  const returnHost = hostOf(new URLSearchParams(search).get("redirect_uri"));

  // アプリへ戻る(ブラウザを、backend が返した戻り先へ移す)。
  const goBackToApp = (target: string) => {
    if (!isNavigable(target)) {
      setState({ status: "failed", messages: [t("oauth.consent.cannotOpen")] });
      return;
    }
    setState({ status: "redirecting" });
    window.location.assign(target);
  };

  const decide = async (approve: boolean) => {
    setIsDeciding(true);
    try {
      goBackToApp((await oauthApi.decide(search, approve)).redirectTo);
    } catch (e) {
      setState({ status: "failed", messages: e instanceof ApiError ? e.messages : [t("oauth.consent.decideError")] });
    } finally {
      setIsDeciding(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    oauthApi
      .describe(search)
      .then(async (view) => {
        if (cancelled) return;
        if (view.consentRequired) {
          setState({ status: "asking", view });
          return;
        }
        // すでに許可済みの範囲に収まる要求は、尋ねずに、そのまま許可を送る。
        const { redirectTo } = await oauthApi.decide(search, true);
        if (!cancelled) goBackToApp(redirectTo);
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setState({ status: "failed", messages: e instanceof ApiError ? e.messages : [t("oauth.consent.loadError")] });
      });
    return () => {
      cancelled = true;
    };
    // goBackToApp は、search と t だけに依存する。search が変わったときだけ、取り直す。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  return (
    <Layout title={t("oauth.consent.title")}>
      {state.status === "loading" && <p className={styles.muted}>{t("oauth.consent.loading")}</p>}
      {state.status === "redirecting" && <p className={styles.muted}>{t("oauth.consent.connecting")}</p>}
      {state.status === "failed" && (
        <>
          <ErrorMessage message={state.messages} />
          <p className={styles.muted}>{t("oauth.consent.loadError")}</p>
        </>
      )}
      {state.status === "asking" && (
        <div className={styles.card}>
          {/* アプリの名前は、アプリが自由に決めるので、HTML として解釈せず、文字として描画する */}
          <p className={styles.intro}>{t("oauth.consent.intro", { name: state.view.client.name })}</p>
          <p className={styles.meta}>{t("oauth.consent.appId", { id: state.view.client.id })}</p>
          <div>
            <p className={styles.heading}>{t("oauth.consent.permissionsHeading")}</p>
            <ul className={styles.scopes}>
              {state.view.scopes.map((scope) => (
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
