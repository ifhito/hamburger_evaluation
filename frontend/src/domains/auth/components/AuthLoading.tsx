import { useTranslation } from "react-i18next";
import { RatingBurgerIcon } from "../../../components/ui/RatingBurger";
import { LOADING_LEVEL } from "../../../components/ui/states";
import styles from "../pages/auth.module.css";

// メールの確認・Google の結果の交換の待ち画面。絵は飾りなので読み上げず、見出しと「少しお待ちください」を読み上げる。
export function AuthLoading({ title }: { title: string }) {
  const { t } = useTranslation();
  return (
    <div role="status" className={styles.status}>
      <RatingBurgerIcon ratio={LOADING_LEVEL} size="lg" />
      <h1 className={styles.title}>{title}</h1>
      <p className={styles.muted}>{t("common.states.loading.description")}</p>
    </div>
  );
}
