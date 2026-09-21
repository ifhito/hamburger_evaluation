import { API_BASE_URL } from "../../api/client/buildApiClient";
import type { Meta } from "../../api/meta";
import { appPathOrNull } from "../../app/router/returnTo";
import type { GoogleExchangeResponse, GoogleSignedInResponse } from "./types";

// サインイン方法の名前(GET /meta の loginProviders に含まれる値)。表示の出し分けにだけ使う。
export const GOOGLE_PROVIDER = "google";

// GET /meta が、Google でのサインインを使えると返しているか(取得できていない間は false。値を推測しない)。
// GET /meta は 1 時間キャッシュされうるので、loginProviders を足す前の応答が残っていて、この項目がないときも、
// 落ちずに false にする。
export function googleEnabled(meta: Pick<Meta, "loginProviders"> | undefined): boolean {
  return meta?.loginProviders?.includes(GOOGLE_PROVIDER) ?? false;
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
