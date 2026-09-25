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

const claudeConfig = JSON.stringify({ mcpServers: { burger_stack: {
  type: "http", url: connectionUrl, oauth: { scopes: "hamburger:read" },
} } }, null, 2);

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
          <h2>{t("mcp.claudeTitle")}</h2>
          <ol className={styles.steps}>
            <li><p>{t("mcp.claudeOpen")}</p></li>
            <li><p>{t("mcp.claudeUrl")}</p><pre><code>{connectionUrl}</code></pre></li>
            <li><p>{t("mcp.claudeConnect")}</p></li>
          </ol>
          <p>{t("mcp.claudePlan")}</p>
          <a href="https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp">{t("mcp.claudeOfficial")}</a>
        </section>
        <section>
          <h2>{t("mcp.claudeCodeTitle")}</h2>
          <ol className={styles.steps}>
            <li><p>{t("mcp.claudeCodeConfig")}</p><pre><code>{claudeConfig}</code></pre></li>
            <li><p>{t("mcp.claudeCodeLogin")}</p></li>
          </ol>
          <p>{t("mcp.claudeCodeWrite")}</p>
          <a href="https://code.claude.com/docs/en/mcp">{t("mcp.claudeCodeOfficial")}</a>
        </section>
        <section>
          <h2>{t("mcp.setupTitle")}</h2>
          <ol className={styles.steps}>
            <li><p>{t("mcp.setupConfig")}</p><pre><code>{config}</code></pre></li>
            <li><p>{t("mcp.setupLogin")}</p><pre><code>{readLogin}</code></pre></li>
            <li><p>{t("mcp.setupConsent")}</p></li>
          </ol>
          <p>{t("mcp.setupWrite")}</p>
          <pre><code>{writeLogin}</code></pre>
          <a href="https://developers.openai.com/codex/mcp/">{t("mcp.official")}</a>
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
