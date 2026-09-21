import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { GoogleExchangeError, authApi } from "../api/authApiClient";
import { ApiError } from "../../../api/client/buildApiClient";
import { appPathOrNull } from "../../../app/router/returnTo";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import { isGoogleSignIn } from "../googleFlow";
import styles from "./auth.module.css";

interface Failure {
  messages: string[];
  // backend が失敗の応答に含めた、手続きを始めた画面(なければ空)。
  returnTo: string;
  // 同じコードでやり直せる失敗か(サーバーの障害・通信の失敗。backend は、そのとき、コードを消費しない)。
  retryable: boolean;
}

// Google でのサインインの手続きの結果の受け皿(/auth/google/complete?code=…)。backend が、成功も失敗も、
// 1 回限りのコードに入れて、この画面へ戻す。ここでは、そのコードを API と交換して、結果を受け取るだけで、
// 成功か失敗か・その理由の判断は持たない(サーバーの文言をそのまま出す)。コードは、URL からはすぐに消し(履歴に
// 残さない)、この画面の state にだけ持つ。ログインの証(JWT)は URL に載らない。
export default function GoogleCompletePage() {
  const { t } = useTranslation();
  const [params, setSearchParams] = useSearchParams();
  const { user, isLoading: restoringAuth, signInWithResponse } = useAuth();
  const navigate = useNavigate();
  const [code] = useState(() => params.get("code") ?? "");
  const [failure, setFailure] = useState<Failure | null>(null);
  // 何回目の交換か(0 が最初)。「Try again」で増やす。
  const [attempt, setAttempt] = useState(0);
  // 同じ回の交換を、StrictMode の開発時の二重実行でも、1 回だけ呼ぶ(コードは単回使用)。
  const startedFor = useRef(-1);
  // 画面を離れたあとに、結果が返っても、勝手に移動させない。
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (startedFor.current === attempt) return;
    startedFor.current = attempt;
    if (attempt === 0) setSearchParams({}, { replace: true });
    // コードがない(URL から消したあとに、戻る操作でこの画面へ戻ったときなど)ときは、要求を送らない(失敗の表示は、下で決める)。
    if (!code) return;
    authApi
      .exchangeGoogleCode(code)
      .then((res) => {
        // 交換に成功したら、画面を離れていても、サインインは反映する(コードは使い切られている)。
        if (isGoogleSignIn(res)) signInWithResponse(res);
        // サインインも結び付けも、手続きを始めた画面(なければ既定の画面)へ戻す。
        if (mounted.current) void navigate(appPathOrNull(res.returnTo) ?? "/reviews", { replace: true });
      })
      .catch((e: unknown) =>
        setFailure(
          e instanceof ApiError
            ? { messages: e.messages, returnTo: e instanceof GoogleExchangeError ? e.returnTo : "", retryable: e.status >= 500 }
            : { messages: [t("auth.google.error")], returnTo: "", retryable: true },
        ),
      );
  }, [attempt, code, navigate, setSearchParams, signInWithResponse, t]);

  const retry = () => {
    setFailure(null);
    setAttempt((n) => n + 1);
  };
  const shown: Failure | null = code ? failure : { messages: [t("auth.google.error")], returnTo: "", retryable: false };
  // 失敗の画面の導線は、ログインの状態の復元(GET /me)が済んでから決める(復元の前は、ログイン中でも、user がまだない)。

  return (
    <Layout title={t("auth.google.complete.title")}>
      {shown ? (
        <div className={styles.form}>
          <ErrorMessage message={shown.messages} />
          {shown.retryable && (
            <Button type="button" variant="secondary" onClick={retry}>
              {t("auth.google.complete.retry")}
            </Button>
          )}
          {!restoringAuth && (
            <p className={styles.hint}>
              {user ? (
                <Link to={`/users/${user.id}`}>{t("auth.google.complete.backToProfile")}</Link>
              ) : (
                <Link to="/signin" state={shown.returnTo ? { from: shown.returnTo } : undefined}>
                  {t("auth.google.complete.backToSignin")}
                </Link>
              )}
            </p>
          )}
        </div>
      ) : (
        <p className={styles.muted}>{t("auth.google.complete.loading")}</p>
      )}
    </Layout>
  );
}
