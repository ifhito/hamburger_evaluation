import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { useSignupForm } from "../hooks/useAuthForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./auth.module.css";

export default function SignupPage() {
  const { t } = useTranslation();
  const { signup } = useAuth();
  const { register, handleSubmit } = useSignupForm();
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
      <Layout title={t("auth.signup.sent.title")}>
        <div className={styles.form}>
          <p>{t("auth.signup.sent.message", { email: sentTo })}</p>
          <p className={styles.hint}>{t("auth.signup.sent.hint")}</p>
          <Button type="button" variant="secondary" onClick={() => setSentTo(null)}>
            {t("auth.signup.sent.back")}
          </Button>
        </div>
      </Layout>
    );
  }

  return (
    <Layout title={t("auth.signup.title")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="username"
          label={t("auth.signup.username")}
          autoComplete="username"
          {...register("username")}
        />
        <Input
          id="email"
          label={t("auth.signup.email")}
          type="email"
          autoComplete="email"
          {...register("email")}
        />
        <Input
          id="password"
          label={t("auth.signup.password")}
          type="password"
          autoComplete="new-password"
          hint={t("auth.passwordHint")}
          {...register("password")}
        />
        <Input
          id="passwordConfirmation"
          label={t("auth.signup.confirmPassword")}
          type="password"
          autoComplete="new-password"
          {...register("passwordConfirmation")}
        />
        <Button type="submit" isLoading={isLoading}>
          {t("auth.signup.submit")}
        </Button>
        <p className={styles.hint}>
          {t("auth.signup.hasAccount")}{" "}
          <a href="/signin">{t("auth.signup.signInLink")}</a>
        </p>
      </form>
    </Layout>
  );
}
