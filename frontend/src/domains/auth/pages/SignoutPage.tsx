import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { Layout } from "../../../components/Layout";
import styles from "./signout.module.css";

export default function SignoutPage() {
  const { t } = useTranslation();
  const { logout } = useAuth();
  const navigate = useNavigate();
  // StrictMode の開発時の二重実行でも、サインアウトの要求は 1 回だけ送る。
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void logout().finally(() => navigate("/reviews", { replace: true }));
  }, [logout, navigate]);

  return (
    <Layout title={t("auth.signout.title")}>
      <p className={styles.muted}>{t("auth.signout.message")}</p>
    </Layout>
  );
}
