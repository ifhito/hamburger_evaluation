import { API_BASE_URL } from "../../api/client/buildApiClient";
import type { Meta } from "../../api/meta";
import { appPathOrNull } from "../../app/router/returnTo";
import type { GoogleExchangeResponse, GoogleSignedInResponse } from "./types";

// サインイン方法の名前(GET /meta の loginProviders に含まれる値)。表示の出し分けにだけ使う。
export const GOOGLE_PROVIDER = "google";

// 画面と同じオリジンの API か(API の根が、相対の path、または、画面と同じオリジンの絶対 URL)。交換と結び付けの cookie は、
// 同じオリジンの /api の道筋でだけ往復するので、別のオリジンの API では、Google の手続きは、必ず失敗する。
// 画面のオリジンが分からない環境(サーバー側の描画など)では、判断しない(true)。
function isSameOriginApi(apiBaseUrl: string, pageOrigin: string | undefined): boolean {
  if (pageOrigin === undefined) return true;
  try {
    return new URL(apiBaseUrl, pageOrigin).origin === pageOrigin;
  } catch {
    return false;
  }
}

// GET /meta が、Google でのサインインを使えると返していて、API が画面と同じオリジンにあるか(取得できていない間は false。
// 値を推測しない)。GET /meta は 1 時間キャッシュされうるので、loginProviders を足す前の応答が残っていて、この項目が
// ないときも、落ちずに false にする。API が別のオリジンにあるときは、必ず失敗するので、ボタンを出さない。
export function googleEnabled(
  meta: Pick<Meta, "loginProviders"> | undefined,
  apiBaseUrl: string = API_BASE_URL,
  pageOrigin: string | undefined = globalThis.location?.origin,
): boolean {
  return (meta?.loginProviders?.includes(GOOGLE_PROVIDER) ?? false) && isSameOriginApi(apiBaseUrl, pageOrigin);
}

// Google でのサインインの手続きを始める URL。ブラウザが、この URL へそのまま移動する(API が、Google の
// 画面へ送る)。returnTo は、手続きのあとに戻る画面(アプリの中のパスだけ。それ以外は付けない。判断の本体は
// backend の規則で、ここは付けるかどうかを決めるだけ)。結び付け(ログイン済みの利用者)は、この URL では始めない
// (認証つきの POST で、そのブラウザに cookie が設定される。startGoogleLink)。
// ブラウザの移動は axios を通らないので、query のキーは、ワイヤ上の名前(snake_case)で書く(useReviews と同じ)。
export function googleStartUrl(
  options: { returnTo?: string | null } = {},
  base: string = API_BASE_URL,
): string {
  const params = new URLSearchParams();
  const returnTo = options.returnTo ? appPathOrNull(options.returnTo) : null;
  if (returnTo) params.set("return_to", returnTo);
  const query = params.toString();
  return `${base.replace(/\/+$/, "")}/auth/google/start${query ? `?${query}` : ""}`;
}

// 交換の結果が、サインインの成功(token つき)かを判定する(型の絞り込みだけで、規則の判断ではない)。
export function isGoogleSignIn(res: GoogleExchangeResponse): res is GoogleSignedInResponse {
  return "token" in res;
}
