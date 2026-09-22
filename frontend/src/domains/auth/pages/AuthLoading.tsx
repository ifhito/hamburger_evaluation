import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { RatingBurgerIcon } from "../../../components/ui/RatingBurger";
import { LOADING_LEVEL } from "../../../components/ui/states";
import styles from "./auth.module.css";

// メールの確認・Google の結果の交換の待ち画面。絵は飾りなので読み上げず、見出しと「少しお待ちください」を読み上げる。
// 知らせの領域(role="status")は、空のまま先に描き、中身は次の描画で入れる(空でない状態のまま挿入すると、
// スクリーンリーダーが読み上げないことがある。GoogleConnection の知らせの領域と同じ理由)。
export function AuthLoading({ title }: { title: string }) {
  const { t } = useTranslation();
  const description = t("common.states.loading.description");
  const [announced, setAnnounced] = useState("");
  useEffect(() => {
    // 次の描画で入れる(この effect の中で直接ではなく)。空のまま挿入したのと同じ描画で埋めると、
    // スクリーンリーダーが、中身の変化として気づかないことがあるため。
    const timer = setTimeout(() => setAnnounced(`${title} ${description}`));
    return () => clearTimeout(timer);
  }, [title, description]);
  return (
    <div className={styles.status}>
      <RatingBurgerIcon ratio={LOADING_LEVEL} size="lg" />
      <h1 className={styles.title}>{title}</h1>
      <p className={styles.muted}>{description}</p>
      <p role="status" className={styles.srOnly}>
        {announced}
      </p>
    </div>
  );
}
