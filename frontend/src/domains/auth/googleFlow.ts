import { API_BASE_URL } from "../../api/client/buildApiClient";
import type { Meta } from "../../api/meta";
import { returnPathFrom } from "../../app/router/returnTo";
import type { GoogleExchangeResponse, GoogleSignedInResponse } from "./types";

// サインイン方法の名前(GET /meta の loginProviders に含まれる値)。表示の出し分けにだけ使う。
export const GOOGLE_PROVIDER = "google";

// GET /meta が、Google でのサインインを使えると返しているか(取得できていない間は false。値を推測しない)。
export function googleEnabled(meta: Pick<Meta, "loginProviders"> | undefined): boolean {
  return meta?.loginProviders.includes(GOOGLE_PROVIDER) ?? false;
}

// Google でのサインインの手続きを始める URL。ブラウザが、この URL へそのまま移動する(API が、Google の
// 画面へ送る)。returnTo は、手続きのあとに戻る画面(アプリの中のパスだけ。それ以外は付けない。判断の本体は
// backend の規則で、ここは付けるかどうかを決めるだけ)。結び付け(ログイン済みの利用者)は、この URL では始めない
// (認証つきの POST で、そのブラウザに cookie が設定される。startGoogleLink)。
export function googleStartUrl(
  options: { returnTo?: string | null } = {},
  base: string = API_BASE_URL,
): string {
  const params = new URLSearchParams();
  const returnTo = options.returnTo ? returnPathFrom({ from: options.returnTo }) : null;
  if (returnTo) params.set("return_to", returnTo);
  const query = params.toString();
  return `${base.replace(/\/+$/, "")}/auth/google/start${query ? `?${query}` : ""}`;
}

// 交換の結果が、サインインの成功(token つき)かを判定する(型の絞り込みだけで、規則の判断ではない)。
export function isGoogleSignIn(res: GoogleExchangeResponse): res is GoogleSignedInResponse {
  return "token" in res;
}

// 結果の画面から戻る先。backend が確かめた戻り先(空は既定)を、もう一度アプリの中のパスかだけ確かめて使う。
export function landingPath(returnTo: string, fallback: string): string {
  return returnPathFrom({ from: returnTo }) ?? fallback;
}

// 失敗の応答が返した戻り先(手続きを始めた画面。backend が確かめたもの)を、サインインの画面へ渡す遷移の state にする。
// アプリの中のパスだけを渡す(それ以外・なければ、state なし)。パスワードでサインインしたあと、その画面へ戻れる。
export function signinStateFor(returnTo: string): { from: string } | undefined {
  const from = returnPathFrom({ from: returnTo });
  return from ? { from } : undefined;
}

// backend が返した Google の認可の URL へ、ブラウザを移動してよいか(http・https だけ。javascript: などの
// スキームだと、ページの中で任意のコードが動いてしまうため)。規則の判断ではなく、ページを開くときの安全のための確認。
export function isNavigableUrl(target: string): boolean {
  try {
    const { protocol } = new URL(target);
    return protocol === "https:" || protocol === "http:";
  } catch {
    return false;
  }
}
