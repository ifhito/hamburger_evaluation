import { useTranslation } from "react-i18next";
import { Layout } from "../../../components/Layout";
import { LinkButton } from "../../../components/ui/LinkButton";
import styles from "./about.module.css";

export default function AboutPage() {
  const { t } = useTranslation();
  return (
    <Layout title={t("about.title")}>
      <div className={styles.content}>
        <p className={styles.lead}>{t("about.lead")}</p>
        <section>
          <h2>{t("about.discoverTitle")}</h2>
          <p>{t("about.discoverBody")}</p>
        </section>
        <section>
          <h2>{t("about.recordTitle")}</h2>
          <p>{t("about.recordBody")}</p>
        </section>
        <section>
          <h2>{t("about.profileTitle")}</h2>
          <p>{t("about.profileBody")}</p>
        </section>
        <div className={styles.actions}>
          <LinkButton to="/shops" variant="primary">{t("about.start")}</LinkButton>
          <LinkButton to="/reviews">{t("about.browseReviews")}</LinkButton>
        </div>
      </div>
    </Layout>
  );
}
