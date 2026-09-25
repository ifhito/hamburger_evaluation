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
          <p>{t("about.introduction")}</p>
          <p>{t("about.question")}</p>
          <p>{t("about.discovery")}</p>
          <p>{t("about.charm")}</p>
        </section>

        <section className={styles.message}>
          <h2>{t("about.wishTitle")}</h2>
          <p>{t("about.missing")}</p>
          <p>{t("about.ramen")}</p>
          <p className={styles.emphasis}>{t("about.wish")}</p>
          <p>{t("about.motivation")}</p>
        </section>

        <section className={styles.message}>
          <h2>{t("about.everydayTitle")}</h2>
          <p>{t("about.everyday")}</p>
          <p>{t("about.community")}</p>
        </section>

        <section className={styles.message}>
          <h2>{t("about.nameTitle")}</h2>
          <p>{t("about.nameOrigin")}</p>
          <p>{t("about.moon")}</p>
        </section>

        <section className={styles.invitation}>
          <h2>{t("about.inviteTitle")}</h2>
          <p>{t("about.inviteBody")}</p>
          <div className={styles.actions}>
            <LinkButton to="/shops" variant="primary">{t("about.start")}</LinkButton>
            <LinkButton to="/reviews">{t("about.browseReviews")}</LinkButton>
            <LinkButton to="/mcp">{t("mcp.title")}</LinkButton>
          </div>
        </section>
      </article>
    </Layout>
  );
}
