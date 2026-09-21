import { useTranslation } from "react-i18next";
import { useMeta } from "../../../api/meta";
import { googleEnabled, googleStartUrl } from "../googleFlow";
import googleLogo from "./google-g.svg";
import styles from "./googleSignIn.module.css";

// 「Google でサインイン」のリンク(見た目はボタン)。ブラウザが API の開始の URL へ移動するので、ボタンではなく
// リンク(a)にする。
export function GoogleSignInLink({ href, label }: { href: string; label: string }) {
  return (
    <a className={styles.button} href={href}>
      {/* Google の「G」のロゴ(Google のブランドの決まりに沿った、色つきのマーク)。装飾なので、読み上げない */}
      <img className={styles.logo} src={googleLogo} alt="" width={18} height={18} />
      <span>{label}</span>
    </a>
  );
}

// サインイン・新規登録の画面に置く、Google のボタン。GET /meta が、Google を使えると返したときだけ出す
// (取得できていない間・使えないときは、何も出さない。値を推測しない)。returnTo は、手続きのあとに戻る画面。
export function GoogleSignIn({ mode, returnTo }: { mode: "signin" | "signup"; returnTo?: string | null }) {
  const { t } = useTranslation();
  const meta = useMeta().data;
  if (!googleEnabled(meta)) return null;
  return (
    <div className={styles.group}>
      <p className={styles.or} aria-hidden="true">
        {t("auth.google.or")}
      </p>
      <GoogleSignInLink href={googleStartUrl({ returnTo })} label={t(mode === "signin" ? "auth.google.signIn" : "auth.google.signUp")} />
    </div>
  );
}
