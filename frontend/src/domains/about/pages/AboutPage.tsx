import { useTranslation } from "react-i18next";
import { Layout } from "../../../components/Layout";
import { Logo } from "../../../components/Logo";
import { LinkButton } from "../../../components/ui/LinkButton";
import styles from "./about.module.css";

export default function AboutPage() {
  const { t } = useTranslation();
  return (
    <Layout>
      <article className={styles.content}>
        <header className={styles.hero}>
          <p className={styles.eyebrow}>{t("about.title")}</p>
          <div className={styles.brand}><Logo name="BurgerStack" size={64} /></div>
          <h1>{t("about.tagline")}</h1>
          <p className={styles.lead}>{t("about.lead")}</p>
        </header>

        <section className={styles.message}>
          <h2>{t("about.messageTitle")}</h2>
          <p>{t("about.messageFirst")}</p>
          <p>{t("about.messageSecond")}</p>
          <p className={styles.emphasis}>{t("about.messageLast")}</p>
        </section>

        <section className={styles.values}>
          <h2 className={styles.valuesTitle}>{t("about.valuesTitle")}</h2>
          <div className={styles.value}>
            <span className={styles.number} aria-hidden="true">01</span>
            <div><h3>{t("about.recordTitle")}</h3><p>{t("about.recordBody")}</p></div>
          </div>
          <div className={styles.value}>
            <span className={styles.number} aria-hidden="true">02</span>
            <div><h3>{t("about.discoverTitle")}</h3><p>{t("about.discoverBody")}</p></div>
          </div>
          <div className={styles.value}>
            <span className={styles.number} aria-hidden="true">03</span>
            <div><h3>{t("about.shareTitle")}</h3><p>{t("about.shareBody")}</p></div>
          </div>
        </section>

        <section className={styles.invitation}>
          <h2>{t("about.inviteTitle")}</h2>
          <p>{t("about.inviteBody")}</p>
          <div className={styles.actions}>
            <LinkButton to="/shops" variant="primary">{t("about.start")}</LinkButton>
            <LinkButton to="/reviews">{t("about.browseReviews")}</LinkButton>
          </div>
        </section>
      </article>
    </Layout>
  );
}
