import { useState } from "react";
import { useTranslation } from "react-i18next";
import { copyToClipboard } from "../../../lib/clipboard";
import { Button } from "../../../components/Button";
import styles from "./shareLinkButton.module.css";

// プロフィールの共有用リンク(<サイトの origin>/users/<id>)をコピーするボタン。ログインなしの閲覧者にも出す。
// コピーできない環境では、リンクを選択できる読み取り専用の入力欄を出して、手動でコピーしてもらう。
export function ShareLinkButton({ userId }: { userId: string }) {
  const { t } = useTranslation();
  const [state, setState] = useState<"idle" | "copied" | "manual">("idle");
  const url = `${window.location.origin}/users/${userId}`;

  const handleClick = async () => {
    setState((await copyToClipboard(url)) ? "copied" : "manual");
  };

  return (
    <div className={styles.share}>
      <Button type="button" variant="secondary" onClick={() => void handleClick()}>
        {t("users.detail.copyLink")}
      </Button>
      {state === "copied" && (
        <span role="status" className={styles.copied}>
          {t("users.detail.linkCopied")}
        </span>
      )}
      {state === "manual" && (
        <div className={styles.manual}>
          <label htmlFor="share-link" className={styles.manualLabel}>
            {t("users.detail.copyLinkManually")}
          </label>
          <input
            id="share-link"
            className={styles.linkInput}
            readOnly
            value={url}
            onFocus={(e) => e.currentTarget.select()}
          />
        </div>
      )}
    </div>
  );
}
