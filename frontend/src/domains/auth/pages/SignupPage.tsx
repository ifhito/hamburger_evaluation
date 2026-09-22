import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { useSignupForm } from "../hooks/useAuthForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Layout } from "../../../components/Layout";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { TextField } from "../../../components/ui/TextField";
import { GoogleSignIn } from "../components/GoogleSignIn";
import styles from "./auth.module.css";

export default function SignupPage() {
  const { t } = useTranslation();
  const { signup } = useAuth();
  const { register, handleSubmit, watch } = useSignupForm();
  const meta = useMeta().data;
  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  // 確認メールの送信を申し込んだあとは、送り先を表示する(登録済みでも同じ画面。応答が同じなので区別できない)。
  const [sentTo, setSentTo] = useState<string | null>(null);

  const onSubmit = handleSubmit(async (data) => {
    setIsLoading(true);
    setServerError(null);
    try {
      await signup(data);
      setSentTo(data.email);
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("auth.signup.error")]);
    } finally {
      setIsLoading(false);
    }
  });

  if (sentTo !== null) {
    return (
      <Layout>
        <div className={styles.auth}>
          <h1 className={styles.title}>{t("auth.signup.sent.title")}</h1>
          <div className={styles.form}>
            <p>
              {/* メールアドレスは values(再び文言として解釈される)ではなく、components の中身として渡す(利用者の入力を、文言の組み立てに混ぜない) */}
              <Trans i18nKey="auth.signup.sent.message" components={{ email: <b>{sentTo}</b> }} />
            </p>
            <p className={styles.hint}>{t("auth.signup.sent.hint")}</p>
            <Button type="button" variant="secondary" onClick={() => setSentTo(null)}>
              {t("auth.signup.sent.back")}
            </Button>
          </div>
        </div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className={styles.auth}>
        <h1 className={styles.title}>{t("auth.signup.title")}</h1>
        <p className={styles.lead}>{t("auth.signup.lead")}</p>
        {/* デザイン(design/redesign/signup.html)は、Google のボタンを <form> の先頭、「サインイン」の導線を
            末尾に置く(どちらも <form> の中。<form> の外に出さない)。 */}
        <form onSubmit={(e) => void onSubmit(e)} className={styles.form} noValidate>
          <GoogleSignIn mode="signup" />
          {serverError && <Alert title={t("auth.signup.errorTitle")} message={serverError} />}
          <TextField
            id="username"
            label={t("auth.signup.username")}
            autoComplete="username"
            counter={{ value: watch("username"), max: meta?.text.usernameMaxChars }}
            {...register("username")}
          />
          <TextField
            id="email"
            label={t("auth.signup.email")}
            type="email"
            autoComplete="email"
            placeholder={t("auth.emailPlaceholder")}
            {...register("email")}
          />
          <TextField
            id="password"
            label={t("auth.signup.password")}
            type="password"
            autoComplete="new-password"
            hint={meta && t("auth.passwordHint", { min: meta.password.minBytes, max: meta.password.maxBytes })}
            {...register("password")}
          />
          <TextField
            id="passwordConfirmation"
            label={t("auth.signup.confirmPassword")}
            type="password"
            autoComplete="new-password"
            {...register("passwordConfirmation")}
          />
          <Button type="submit" block isLoading={isLoading}>
            {t("auth.signup.submit")}
          </Button>
          <p className={styles.linkline}>
            {t("auth.signup.hasAccount")}{" "}
            <a href="/signin">{t("auth.signup.signInLink")}</a>
          </p>
        </form>
      </div>
    </Layout>
  );
}
