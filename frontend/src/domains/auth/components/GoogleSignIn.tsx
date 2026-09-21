import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMeta } from "../../../api/meta";
import { googleEnabled, googleStartUrl } from "../googleFlow";
import googleLogo from "./google-g.svg";
import styles from "./googleSignIn.module.css";

// 「Google で続ける」のリンク(見た目はボタン)。ブラウザが API の開始の URL へ移動するので、ボタンではなく
// リンク(a)にする。押したあと、Google の画面へ移るまでの間は、押せなくして(二重に開始しない)、移動中と示す。
export function GoogleSignInLink({ href, label }: { href: string; label: string }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  // 戻る操作で、移動する前の状態のまま復元されたとき(bfcache)は、押せる状態に戻す。
  useEffect(() => {
    const reset = (e: PageTransitionEvent) => {
      if (e.persisted) setBusy(false);
    };
    window.addEventListener("pageshow", reset);
    return () => window.removeEventListener("pageshow", reset);
  }, []);
  return (
    <a
      className={styles.button}
      href={href}
      aria-busy={busy || undefined}
      aria-disabled={busy || undefined}
      tabIndex={busy ? -1 : undefined}
      onClick={(e) => {
        if (busy) return e.preventDefault();
        // 新しいタブで開くなど、この画面から移動しない押し方のときは、移動中にしない。
        if (e.button === 0 && !(e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) setBusy(true);
      }}
    >
      {/* Google の「G」のロゴ(Google のブランドの決まりに沿った、色つきのマーク)。装飾なので、読み上げない */}
      <img className={styles.logo} src={googleLogo} alt="" width={18} height={18} />
      <span>{busy ? t("auth.google.busy") : label}</span>
    </a>
  );
}

// サインイン・新規登録の画面に置く、Google のボタン。GET /meta が、Google を使えると返したときだけ出す
// (取得できていない間・使えないときは、何も出さない。値を推測しない)。returnTo は、手続きのあとに戻る画面。
// 置き場所: サインインはフォームの下(「または」→ ボタン)、新規登録はフォームの上(ボタン →「または」)。
export function GoogleSignIn({ mode, returnTo }: { mode: "signin" | "signup"; returnTo?: string | null }) {
  const { t } = useTranslation();
  const meta = useMeta().data;
  if (!googleEnabled(meta)) return null;
  const divider = (
    <p className={styles.or} aria-hidden="true">
      {t("auth.google.or")}
    </p>
  );
  const button = <GoogleSignInLink href={googleStartUrl({ returnTo })} label={t("auth.google.continue")} />;
  return (
    <div className={styles.group}>
      {mode === "signin" ? divider : button}
      {mode === "signin" ? button : divider}
    </div>
  );
}
