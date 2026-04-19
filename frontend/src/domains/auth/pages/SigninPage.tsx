import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { useLoginForm } from "../hooks/useAuthForm";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./auth.module.css";

export default function SigninPage() {
  const { t } = useTranslation();
  const { login } = useAuth();
  const navigate = useNavigate();
  const { register, handleSubmit, formState: { errors } } = useLoginForm();
  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const onSubmit = handleSubmit(async (data) => {
    setIsLoading(true);
    setServerError(null);
    try {
      await login(data);
      void navigate("/reviews");
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("auth.signin.error")]);
    } finally {
      setIsLoading(false);
    }
  });

  return (
    <Layout title={t("auth.signin.title")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="email"
          label={t("auth.signin.email")}
          type="email"
          autoComplete="email"
          error={errors.email?.message}
          {...register("email")}
        />
        <Input
          id="password"
          label={t("auth.signin.password")}
          type="password"
          autoComplete="current-password"
          error={errors.password?.message}
          {...register("password")}
        />
        <Button type="submit" isLoading={isLoading}>
          {t("auth.signin.submit")}
        </Button>
        <p className={styles.hint}>
          {t("auth.signin.noAccount")}{" "}
          <a href="/signup">{t("auth.signin.signUpLink")}</a>
        </p>
      </form>
    </Layout>
  );
}
