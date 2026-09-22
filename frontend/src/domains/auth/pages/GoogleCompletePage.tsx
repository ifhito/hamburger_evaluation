import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../AuthProvider";
import { GoogleExchangeError, authApi } from "../api/authApiClient";
import { ApiError } from "../../../api/client/buildApiClient";
import { appPathOrNull } from "../../../app/router/returnTo";
import { Layout } from "../../../components/Layout";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import buttonStyles from "../../../components/ui/button.module.css";
import { GoogleSignInLink } from "../components/GoogleSignIn";
import { googleStartUrl, isGoogleSignIn } from "../googleFlow";
import { getToken } from "../storage";
import { AuthLoading } from "./AuthLoading";
import styles from "./auth.module.css";

interface Failure {
  messages: string[];
  // backend が失敗の応答に含めた、手続きを始めた画面(なければ空)。
  returnTo: string;
  // デザイン(design/redesign/google-complete.html)が対応する理由か(exists/taken/alreadyLinked/signInFailed/
  // linkInvalid)。backend の応答の reason(言語によらない識別子)から決める。対応する理由がない(サーバーの
  // 障害・通信の失敗など、デザインに状態がない)ときは undefined。
  reason: GoogleFailureReason | undefined;
  // 同じコードでやり直せる失敗か(サーバーの障害・通信の失敗。backend は、そのとき、コードを消費しない)。
  retryable: boolean;
}

// 5xx(サーバーの障害)か。先頭の桁で判定する(noDuplicatedLimits の検査は、ソースの中の、backend の上限と同じ数字を探すため、
// HTTP ステータスの数字は、直書きしない)。
const isServerError = (status: number) => String(status).startsWith("5");

// デザイン(design/redesign/google-complete.html)は、失敗の理由ごとに、小見出し・導線を出し分ける。backend の
// 応答の reason(messages.go の key* 定数と同じ、言語によらない識別子。例: "google.account_exists")を、この
// 画面の理由の名前に変換する(この変換だけが、この画面の言葉と backend の識別子を結び付ける)。API の文言
// (messages)は Accept-Language で日本語にもなるので、文言では判定しない。
type GoogleFailureReason = "exists" | "taken" | "alreadyLinked" | "signInFailed" | "linkInvalid";
const GOOGLE_FAILURE_REASON: Record<string, GoogleFailureReason> = {
  "google.account_exists": "exists",
  "google.identity_taken": "taken",
  "google.already_linked": "alreadyLinked",
  "google.sign_in_failed": "signInFailed",
  "google.code_invalid": "linkInvalid",
};

// Google でのサインインの手続きの結果の受け皿(/auth/google/complete?code=…)。backend が、成功も失敗も、
// 1 回限りのコードに入れて、この画面へ戻す。ここでは、そのコードを API と交換して、結果を受け取るだけで、
// 成功か失敗か・その理由の判断は持たない(サーバーの文言をそのまま出す)。コードは、URL からはすぐに消し(履歴に
// 残さない)、この画面の state にだけ持つ。ログインの証(JWT)は URL に載らない。
export default function GoogleCompletePage() {
  const { t } = useTranslation();
  const [params, setSearchParams] = useSearchParams();
  const { user, isLoading: restoringAuth, signInWithResponse } = useAuth();
  const navigate = useNavigate();
  const [code] = useState(() => params.get("code") ?? "");
  const [failure, setFailure] = useState<Failure | null>(null);
  // 何回目の交換か(0 が最初)。「Try again」で増やす。
  const [attempt, setAttempt] = useState(0);
  // 同じ回の交換を、StrictMode の開発時の二重実行でも、1 回だけ呼ぶ(コードは単回使用)。
  const startedFor = useRef(-1);
  // 画面を離れたあとに、結果が返っても、勝手に移動させない。
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (startedFor.current === attempt) return;
    startedFor.current = attempt;
    if (attempt === 0) setSearchParams({}, { replace: true });
    // コードがない(URL から消したあとに、戻る操作でこの画面へ戻ったときなど)ときは、要求を送らない(失敗の表示は、下で決める)。
    if (!code) return;
    const run = async () => {
      let res;
      try {
        res = await authApi.exchangeGoogleCode(code);
      } catch (e) {
        // 交換そのものの失敗。サーバーの障害・通信の失敗は、コードが消費されていないので、やり直せる。
        // サーバーの障害(5xx)・通信の失敗は、サーバーの生の文言(internal server error など)ではなく、こちらの文言で案内する。
        // 決まった失敗(409・400)は、API の文言をそのまま出す。
        const retryable = !(e instanceof ApiError) || isServerError(e.status);
        setFailure(
          e instanceof ApiError
            ? {
                messages: retryable ? [t("auth.google.complete.temporary")] : e.messages,
                returnTo: e instanceof GoogleExchangeError ? e.returnTo : "",
                // retryable(サーバーの障害・通信の失敗)は、デザインに対応する状態がない。
                reason: retryable ? undefined : GOOGLE_FAILURE_REASON[e instanceof GoogleExchangeError ? e.reason : ""],
                retryable,
              }
            : { messages: [t("auth.google.complete.temporary")], returnTo: "", reason: undefined, retryable },
        );
        return;
      }
      try {
        // 交換に成功したら、画面を離れていても、サインインは反映する(コードは使い切られている)。
        if (isGoogleSignIn(res)) signInWithResponse(res);
        // サインインも結び付けも、手続きを始めた画面(なければ既定の画面)へ戻す。
        if (mounted.current) void navigate(appPathOrNull(res.returnTo) ?? "/reviews", { replace: true });
      } catch {
        // 成功したあとの処理の失敗(ログインの状態を保存できない・想定外の本文など)。コードは使い切られているので、
        // やり直せない。同じ状況(Google でのサインインに失敗した)として案内し、Google のボタンをもう一度出す。
        setFailure({ messages: [t("auth.google.error")], returnTo: "", reason: "signInFailed", retryable: false });
      }
    };
    void run();
  }, [attempt, code, navigate, setSearchParams, signInWithResponse, t]);

  const retry = () => {
    setFailure(null);
    setAttempt((n) => n + 1);
  };
  const shown: Failure | null = code
    ? failure
    : { messages: [t("auth.google.complete.expired")], returnTo: "", reason: "linkInvalid", retryable: false };
  // 見出し(サインインの失敗 / 連携の失敗)は、保存済みのトークンの有無だけで決める(GET /me の応答を待たない)。
  // user は GET /me が終わるまで null のままなので、待つと、ログイン中の利用者にも一瞬「サインインできませんでした」が出てしまう。
  const isLinkAttempt = getToken() !== null;
  // 失敗の画面の導線(戻り先のリンク)は、ログインの状態の復元(GET /me)が済んでから決める(復元の前は、ログイン中でも、user がまだない)。

  const reason = shown?.reason;
  // デザインは、サインインの試み(連携ではない)で、理由が「Google のサインインに失敗した」「リンクが無効」の
  // ときだけ、Google のボタンをもう一度出す(連携の失敗は、常にプロフィールへ戻るだけ。google-complete.html の
  // actions-col の出し分け)。isLinkAttempt(トークンの有無)で判定し、GET /me の復元を待たない(h1 の見出しと同じ理由)。
  const showGoogleRestart = !isLinkAttempt && (reason === "signInFailed" || reason === "linkInvalid");
  // 導線がこれ 1 つだけのときは、デザインどおり目立つボタン(.btn.primary.linkbtn)にする。Google のボタンや、
  // 同じコードでの再試行(デザインに対応する状態がない、既存の挙動)と並ぶときは、控えめなテキストリンクのままにする。
  const soleAction = shown !== null && !shown.retryable && !showGoogleRestart;
  const linkClassName = (soleAction ? [buttonStyles.btn, buttonStyles.primary, buttonStyles.block] : [styles.textlink])
    .filter(Boolean)
    .join(" ");
  // exists-oauth: 許可の画面から始めた Google のサインインが「同じメールのアカウントがある」で失敗したとき、
  // パスワードでサインインすれば、元の画面に戻れることを案内する(google-complete.html の .notice)。
  const showContinueNotice = !isLinkAttempt && reason === "exists" && shown !== null && shown.returnTo !== "";

  return (
    <Layout>
      {shown ? (
        <div className={styles.status}>
          <h1 className={styles.title}>{t(isLinkAttempt ? "auth.google.complete.linkFailedTitle" : "auth.google.complete.failedTitle")}</h1>
          {showContinueNotice && (
            // Alert(role="alert")ほど割り込まない、案内としての扱い(role="status")。挿入されたときに、
            // スクリーンリーダーが読み上げる。
            <div className={styles.notice} role="status">
              <b>{t("auth.google.complete.continueNoticeTitle")}</b>
              <span>{t("auth.google.complete.continueNoticeBody")}</span>
            </div>
          )}
          <div className={styles.alertBox}>
            <Alert title={reason && t(`auth.google.complete.reasons.${reason}.title`)} message={shown.messages} />
          </div>
          <div className={styles.actions}>
            {showGoogleRestart && <GoogleSignInLink href={googleStartUrl({ returnTo: shown.returnTo })} label={t("auth.google.continue")} />}
            {shown.retryable && (
              <Button type="button" variant="secondary" block onClick={retry}>
                {t("auth.google.complete.retry")}
              </Button>
            )}
            {!restoringAuth &&
              (user ? (
                <Link to={`/users/${user.id}`} className={linkClassName}>
                  {t("auth.google.complete.backToProfile")}
                </Link>
              ) : (
                <Link to="/signin" state={shown.returnTo ? { from: shown.returnTo } : undefined} className={linkClassName}>
                  {t("auth.google.complete.backToSignin")}
                </Link>
              ))}
          </div>
        </div>
      ) : (
        <AuthLoading title={t("auth.google.complete.loading")} />
      )}
    </Layout>
  );
}
