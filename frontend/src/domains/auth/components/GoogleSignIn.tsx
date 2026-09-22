import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMeta } from "../../../api/meta";
import { googleEnabled, googleStartUrl } from "../googleFlow";
import styles from "./googleSignIn.module.css";

// 移動が始まらなかったとき(ブロックされた・キャンセルされたなど)、移動中のまま固まらないための保険の秒数。
// ponytail: 「戻る操作で復元されたら戻す」だけでは、そもそも移動しなかった場合を拾えない。時間で戻すだけの、簡易な保険。
const STUCK_TIMEOUT_MS = 10_000;

// Google の「G」のロゴ(4 色。Google のブランドの決まりに沿ったマーク)。デザイン(design/redesign/google-login.js の
// G)と同じ、インラインの SVG にする(画像ファイルの参照にしない)。装飾なので aria-hidden で読み上げない。
function GoogleLogo() {
  return (
    <svg className={styles.logo} viewBox="0 0 18 18" width={18} height={18} aria-hidden="true">
      <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
      <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" />
      <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z" />
    </svg>
  );
}

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
      <GoogleLogo />
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
  // デザイン(design/redesign/google-login.js の .gor)は、この区切りを <div> が <span> を包む形にしている(<p> ではない)。
  const divider = (
    <div key="divider" className={styles.or} aria-hidden="true">
      <span>{t("auth.google.or")}</span>
    </div>
  );
  const button = <GoogleSignInLink key="button" href={googleStartUrl({ returnTo })} label={t("auth.google.continue")} />;
  const parts = mode === "signin" ? [divider, button] : [button, divider];
  return <div className={styles.group}>{parts}</div>;
}
