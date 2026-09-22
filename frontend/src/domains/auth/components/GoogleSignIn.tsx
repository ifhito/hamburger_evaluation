import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMeta } from "../../../api/meta";
import { googleEnabled, googleStartUrl } from "../googleFlow";
import googleLogo from "./google-g.svg";
import styles from "./googleSignIn.module.css";

// 移動が始まらなかったとき(ブロックされた・キャンセルされたなど)、移動中のまま固まらないための保険の秒数。
// ponytail: 「戻る操作で復元されたら戻す」だけでは、そもそも移動しなかった場合を拾えない。時間で戻すだけの、簡易な保険。
const STUCK_TIMEOUT_MS = 10_000;

// 「Google で続ける」のリンク(見た目はボタン)。ブラウザが API の開始の URL へ移動するので、ボタンではなく
// リンク(a)にする。押したあと、Google の画面へ移るまでの間は、押せなくして(二重に開始しない)、移動中と示す。
export function GoogleSignInLink({ href, label }: { href: string; label: string }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const stuckTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const reset = () => {
    clearTimeout(stuckTimer.current);
    setBusy(false);
  };
  // 戻る操作で、移動する前の状態のまま復元されたとき(bfcache)は、押せる状態に戻す。
  useEffect(() => {
    const onPageShow = (e: PageTransitionEvent) => {
      if (e.persisted) reset();
    };
    window.addEventListener("pageshow", onPageShow);
    return () => {
      window.removeEventListener("pageshow", onPageShow);
      clearTimeout(stuckTimer.current);
    };
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
        if (e.button === 0 && !(e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)) {
          setBusy(true);
          stuckTimer.current = setTimeout(reset, STUCK_TIMEOUT_MS);
        }
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
    <p key="divider" className={styles.or} aria-hidden="true">
      {t("auth.google.or")}
    </p>
  );
  const button = <GoogleSignInLink key="button" href={googleStartUrl({ returnTo })} label={t("auth.google.continue")} />;
  const parts = mode === "signin" ? [divider, button] : [button, divider];
  return <div className={styles.group}>{parts}</div>;
}
