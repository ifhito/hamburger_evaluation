import { useState } from "react";
import { useTranslation } from "react-i18next";
import { copyToClipboard } from "../../../lib/clipboard";
import { Button } from "../../../components/Button";
import styles from "./shareLinkButton.module.css";

// プロフィールの共有用リンク(このサイトのオリジンに /users/<ユーザーの ID> を付けたもの)をコピーするボタン。
// プロフィールはログインしていない人にも公開されているので、共有の入口(このボタン)も、ログイン状態や本人かどうかに関係なく出す。
// コピーできない環境(ブラウザの機能がない、権限を拒否された)では、リンクを選択できる読み取り専用の入力欄を出して、
// 手動でコピーしてもらう。
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
