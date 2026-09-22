import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { ApiError } from "../../../api/client/buildApiClient";
import { Layout } from "../../../components/Layout";
import { Alert } from "../../../components/ui/Alert";
import { AuthLoading } from "./AuthLoading";
import styles from "./auth.module.css";

// 確認メールのリンク(/signup/confirm?token=…)の受け皿。トークンの有効・無効の判断は backend だけが持ち、
// ここでは API を呼んで、成功したらログイン状態でトップへ、失敗したらサーバーのメッセージと
// 「もう一度 signup する」への導線を表示する。
export default function SignupConfirmPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const { confirmSignup } = useAuth();
  const navigate = useNavigate();
  const [error, setError] = useState<string | string[] | null>(null);
  // トークンは単回使用なので、StrictMode の開発時の二重実行でも、確認は 1 回だけ呼ぶ。
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    confirmSignup(params.get("token") ?? "")
      .then(() => navigate("/reviews", { replace: true }))
      .catch((e: unknown) => {
        setError(e instanceof ApiError ? e.messages : [t("auth.confirm.error")]);
      });
  }, [confirmSignup, navigate, params, t]);

  return (
    <Layout>
      {error ? (
        <div className={styles.status}>
          <h1 className={styles.title}>{t("auth.confirm.failedTitle")}</h1>
          <div className={styles.alertBox}>
            <Alert title={t("auth.confirm.errorTitle")} message={error} />
          </div>
          <Link to="/signup" className={styles.textlink}>
            {t("auth.confirm.signUpAgain")}
          </Link>
        </div>
      ) : (
        <AuthLoading title={t("auth.confirm.loading")} />
      )}
    </Layout>
  );
}
