import { useTranslation } from "react-i18next";
import { Layout } from "../../components/Layout";
import { LinkButton } from "../../components/ui/LinkButton";
import { NotFound } from "../../components/ui/states";

// 未定義のパスを開いたときに、枠組み(ヘッダー・ナビゲーション)つきの 404 画面を出す。
export default function NotFoundPage() {
  const { t } = useTranslation();
  return (
    <Layout>
      <NotFound action={<LinkButton to="/">{t("navigation.home")}</LinkButton>} />
    </Layout>
  );
}
