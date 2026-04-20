import { useState } from "react";
import { useNavigate } from "react-router-dom";
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
  const navigate = useNavigate();
  const { register, handleSubmit, formState: { errors } } = useSignupForm();
  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const onSubmit = handleSubmit(async (data) => {
    setIsLoading(true);
    setServerError(null);
    try {
      await signup(data);
      void navigate("/reviews");
    } catch (e) {
      setServerError(e instanceof ApiError ? e.messages : [t("auth.signup.error")]);
    } finally {
      setIsLoading(false);
    }
  });

  return (
    <Layout title={t("auth.signup.title")}>
      <form onSubmit={(e) => void onSubmit(e)} className={styles.form}>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="username"
          label={t("auth.signup.username")}
          autoComplete="username"
          error={errors.username?.message}
          {...register("username")}
        />
        <Input
          id="email"
          label={t("auth.signup.email")}
          type="email"
          autoComplete="email"
          error={errors.email?.message}
          {...register("email")}
        />
        <Input
          id="password"
          label={t("auth.signup.password")}
          type="password"
          autoComplete="new-password"
          error={errors.password?.message}
          {...register("password")}
        />
        <Input
          id="passwordConfirmation"
          label={t("auth.signup.confirmPassword")}
          type="password"
          autoComplete="new-password"
          error={errors.passwordConfirmation?.message}
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
