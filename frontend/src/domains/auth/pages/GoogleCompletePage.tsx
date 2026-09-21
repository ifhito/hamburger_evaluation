import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { authApi } from "../api/authApiClient";
import { ApiError } from "../../../api/client/buildApiClient";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import { isGoogleSignIn, landingPath, signinStateFor } from "../googleFlow";
import styles from "./auth.module.css";

// Google でのサインインの手続きの結果の受け皿(/auth/google/complete?code=…)。backend が、成功も失敗も、
// 1 回限りのコードに入れて、この画面へ戻す。ここでは、そのコードを API と交換して、結果を受け取るだけで、
// 成功か失敗か・その理由の判断は持たない(サーバーの文言をそのまま出す)。コードは 1 回しか使えないので、
// 最初に 1 回だけ読み、URL からはすぐに消す(履歴に残さない)。ログインの証(JWT)は URL に載らない。
export default function GoogleCompletePage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const { user, signInWithResponse } = useAuth();
  const navigate = useNavigate();
  const [code] = useState(() => params.get("code") ?? "");
  // 失敗の内容。returnTo は、backend が失敗の応答に含めた、手続きを始めた画面(なければ空)。
  const [error, setError] = useState<{ messages: string[]; returnTo: string } | null>(null);
  // コードは単回使用なので、StrictMode の開発時の二重実行でも、交換は 1 回だけ呼ぶ。
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    window.history.replaceState(window.history.state, "", window.location.pathname);
    authApi
      .exchangeGoogleCode(code)
      .then((res) => {
        if (isGoogleSignIn(res)) signInWithResponse(res);
        // サインインも結び付けも、手続きを始めた画面(なければ既定の画面)へ戻す。
        void navigate(landingPath(res.returnTo, "/reviews"), { replace: true });
      })
      .catch((e: unknown) =>
        setError(e instanceof ApiError ? { messages: e.messages, returnTo: e.returnTo ?? "" } : { messages: [t("auth.google.error")], returnTo: "" }),
      );
  }, [code, navigate, signInWithResponse, t]);

  return (
    <Layout title={t("auth.google.complete.title")}>
      {error ? (
        <div className={styles.form}>
          <ErrorMessage message={error.messages} />
          <p className={styles.hint}>
            {user ? (
              <Link to={`/users/${user.id}`}>{t("auth.google.complete.backToProfile")}</Link>
            ) : (
              <Link to="/signin" state={signinStateFor(error.returnTo)}>
                {t("auth.google.complete.backToSignin")}
              </Link>
            )}
          </p>
        </div>
      ) : (
        <p className={styles.muted}>{t("auth.google.complete.loading")}</p>
      )}
    </Layout>
  );
}
