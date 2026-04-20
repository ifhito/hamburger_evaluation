import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { Layout } from "../../../components/Layout";
import styles from "./auth.module.css";

export default function SignoutPage() {
  const { t } = useTranslation();
  const { logout } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    void logout().then(() => navigate("/reviews"));
  }, [logout, navigate]);

  return (
    <Layout title={t("auth.signout.title")}>
      <p className={styles.muted}>{t("auth.signout.message")}</p>
    </Layout>
  );
}
