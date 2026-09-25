import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Layout } from "../../../components/Layout";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { copyToClipboard } from "../../../lib/clipboard";
import styles from "./mcpGuide.module.css";

const connectionUrl = "https://burger-stack.com/api/mcp";
const config = `[mcp_servers.burger_stack]
url = "${connectionUrl}"`;
const readLogin = "codex mcp login burger_stack --oauth-client-registration cimd --scopes hamburger:read";
const writeLogin = "codex mcp login burger_stack --oauth-client-registration cimd --scopes hamburger:read,hamburger:write";

export default function McpGuidePage() {
  const { t } = useTranslation();
  const [copyState, setCopyState] = useState<"idle" | "copied" | "manual">("idle");
  const copy = async () => setCopyState(await copyToClipboard(connectionUrl) ? "copied" : "manual");

  return (
    <Layout>
      <article className={styles.content}>
        <header className={styles.hero}>
          <h1>{t("mcp.title")}</h1>
          <p>{t("mcp.lead")}</p>
        </header>
        <section className={styles.connection} aria-labelledby="mcp-url-label">
          <label id="mcp-url-label" htmlFor="mcp-url">{t("mcp.urlLabel")}</label>
          <div className={styles.copyRow}>
            <input id="mcp-url" readOnly value={connectionUrl} onFocus={(event) => event.currentTarget.select()} />
            <Button onClick={copy}>{t("mcp.copy")}</Button>
          </div>
          <p role="status">{copyState === "copied" ? t("mcp.copied") : copyState === "manual" ? t("mcp.manual") : ""}</p>
          <p>{t("mcp.account")}</p>
          <LinkButton to="/signup">{t("mcp.signup")}</LinkButton>
        </section>
        <section>
          <h2>{t("mcp.examplesTitle")}</h2>
          <ul className={styles.examples}>
            {["searchExample", "readExample", "writeExample"].map((key) => <li key={key}>{t(`mcp.${key}`)}</li>)}
          </ul>
          <p>{t("mcp.writeHint")}</p>
        </section>
        <section>
          <h2>{t("mcp.setupTitle")}</h2>
          <p className={styles.notice}>{t("mcp.setupStatus")}</p>
          <ol className={styles.steps}>
            <li><p>{t("mcp.setupConfig")}</p><pre><code>{config}</code></pre></li>
            <li><p>{t("mcp.setupLogin")}</p><pre><code>{readLogin}</code></pre></li>
            <li><p>{t("mcp.setupConsent")}</p></li>
          </ol>
          <p>{t("mcp.setupWrite")}</p>
          <pre><code>{writeLogin}</code></pre>
          <p>{t("mcp.setupVersion")}</p>
          <a href="https://developers.openai.com/codex/mcp/">{t("mcp.official")}</a>
        </section>
        <section>
          <h2>{t("mcp.statusTitle")}</h2>
          <p>{t("mcp.checkedOn")}</p>
          <table className={styles.compatibility}>
            <thead><tr><th scope="col">{t("mcp.app")}</th><th scope="col">{t("mcp.status")}</th></tr></thead>
            <tbody>
              <tr><th scope="row">Codex</th><td>{t("mcp.codexStatus")}</td></tr>
              <tr><th scope="row">ChatGPT</th><td>{t("mcp.unverified")}</td></tr>
              <tr><th scope="row">Claude</th><td>{t("mcp.unverified")}</td></tr>
              <tr><th scope="row">{t("mcp.unsupportedName")}</th><td>{t("mcp.unsupported")}</td></tr>
            </tbody>
          </table>
        </section>
        <section>
          <h2>{t("mcp.disconnectTitle")}</h2>
          <p>{t("mcp.disconnect")}</p>
          <LinkButton to="/signin">{t("mcp.signin")}</LinkButton>
        </section>
        <section>
          <h2>{t("mcp.troubleTitle")}</h2>
          {["auth", "scope", "client"].map((key) => (
            <details key={key}><summary>{t(`mcp.${key}Problem`)}</summary><p>{t(`mcp.${key}Answer`)}</p></details>
          ))}
          <details>
            <summary>{t("mcp.detailsTitle")}</summary>
            <p>{t("mcp.transport")}</p>
            <ul><li>{t("mcp.readScope")}</li><li>{t("mcp.writeScope")}</li></ul>
            <p>{t("mcp.registration")}</p>
          </details>
        </section>
      </article>
    </Layout>
  );
}
