import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { useLoginForm } from "../hooks/useAuthForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { returnPathFrom } from "../../../app/router/returnTo";
import { Layout } from "../../../components/Layout";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { TextField } from "../../../components/ui/TextField";
import { GoogleSignIn } from "../components/GoogleSignIn";
import styles from "./auth.module.css";

export default function SigninPage() {
  const { t } = useTranslation();
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { register, handleSubmit } = useLoginForm();
  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const onSubmit = handleSubmit(async (data) => {
    setIsLoading(true);
    setServerError(null);
    try {
      await login(data);
      // ログインが必要な画面から送られてきたときは、その画面へ戻す。
      void navigate(returnPathFrom(location.state) ?? "/reviews", { replace: true });
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("auth.signin.error")]);
    } finally {
      setIsLoading(false);
    }
  });

  return (
    <Layout>
      <div className={styles.auth}>
        <h1 className={styles.title}>{t("auth.signin.title")}</h1>
        <p className={styles.lead}>{t("auth.signin.lead")}</p>
        {/* デザイン(design/redesign/signin.html)は、Google のボタンと「新規登録」の導線を、<form> の中(送信ボタンの
            あと)に置く。<form> の外に出さない。 */}
        <form onSubmit={(e) => void onSubmit(e)} className={styles.form} noValidate>
          {serverError && <Alert title={t("auth.signin.errorTitle")} message={serverError} />}
          <TextField
            id="email"
            label={t("auth.signin.email")}
            type="email"
            autoComplete="email"
            placeholder={t("auth.emailPlaceholder")}
            {...register("email")}
          />
          <TextField
            id="password"
            label={t("auth.signin.password")}
            type="password"
            autoComplete="current-password"
            {...register("password")}
          />
          <Button type="submit" block isLoading={isLoading}>
            {t("auth.signin.submit")}
          </Button>
          {/* ログインが必要な画面から送られてきたときは、Google でのサインインのあとも、その画面へ戻す */}
          <GoogleSignIn mode="signin" returnTo={returnPathFrom(location.state)} />
          <p className={styles.linkline}>
            {t("auth.signin.noAccount")}{" "}
            <a href="/signup">{t("auth.signin.signUpLink")}</a>
          </p>
        </form>
      </div>
    </Layout>
  );
}
