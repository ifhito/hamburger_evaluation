import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { copyToClipboard } from "../../../lib/clipboard";
import { Button } from "../../../components/ui/Button";
import styles from "./shareLinkButton.module.css";

const COPY_FEEDBACK_RESET_MS = 1800;

// プロフィールの共有用リンク(このサイトのオリジンに /users/<ユーザーの ID> を付けたもの)をコピーするボタン。
// プロフィールはログインしていない人にも公開されているので、共有の入口(このボタン)も、ログイン状態や本人かどうかに関係なく出す。
// コピーできない環境(ブラウザの機能がない、権限を拒否された)では、リンクを選択できる読み取り専用の入力欄を出して、
// 手動でコピーしてもらう。
export function ShareLinkButton({ userId }: { userId: string }) {
  const { t } = useTranslation();
  // 別のプロフィール(userId)へ遷移したときは、前のユーザーのコピー状態を持ち越さない。
  const [copyState, setCopyState] = useState<{ forUserId: string; status: "idle" | "copied" | "manual" }>({
    forUserId: userId,
    status: "idle",
  });
  const state = copyState.forUserId === userId ? copyState.status : "idle";
  const url = `${window.location.origin}/users/${userId}`;

  useEffect(() => {
    if (state !== "copied") return;
    const timer = window.setTimeout(() => setCopyState({ forUserId: userId, status: "idle" }), COPY_FEEDBACK_RESET_MS);
    return () => window.clearTimeout(timer);
  }, [state, userId]);

  const handleClick = async () => {
    setCopyState({ forUserId: userId, status: (await copyToClipboard(url)) ? "copied" : "manual" });
  };

  // 親(プロフィールの操作の並び。折り返す横並び)の中に、そのまま並べる。コピーできなかったときの入力欄は、次の行いっぱいに広げる。
  return (
    <>
      {/* コピーできたことは、ボタンの文言の入れ替えで伝える(あとから知らせの要素を足すと、読み上げられないことがあるため) */}
      <Button type="button" variant="secondary" aria-live="polite" onClick={() => void handleClick()}>
        {state === "copied" ? t("users.detail.linkCopied") : t("users.detail.copyLink")}
      </Button>
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
    </>
  );
}
